package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dang-w/momentum-mcp-server/internal/auth"
	"github.com/dang-w/momentum-mcp-server/internal/config"
	"github.com/dang-w/momentum-mcp-server/server"
	"github.com/dang-w/momentum-mcp-server/storage"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create GitHub storage
	ghStorage, err := storage.NewGitHubStorage(cfg.GitHubToken, cfg.GitHubRepo)
	if err != nil {
		log.Fatalf("Failed to create storage: %v", err)
	}

	// Create OAuth token store
	tokenStore := auth.NewTokenStore(cfg.OAuthAccessTokenTTL, cfg.OAuthRefreshTokenTTL)

	// Create MCP server with storage and GitHub activity config
	mcpServer := server.New(server.Config{
		Storage:        ghStorage,
		GitHubToken:    cfg.GitHubToken,
		GitHubUsername: cfg.GitHubUsername(),
	})

	// Create the streamable HTTP handler for MCP
	mcpHandler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return mcpServer
	}, nil)

	// Determine base URL for OAuth metadata
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = fmt.Sprintf("http://localhost:%s", cfg.Port)
	}

	// Create OAuth server
	oauthServer := auth.NewOAuthServer(auth.OAuthConfig{
		TokenStore:   tokenStore,
		BaseURL:      baseURL,
		AuthorizePin: cfg.OAuthAuthorizePin,
	})

	// Create rate limiter for token endpoint (10 requests per minute per IP)
	tokenRateLimiter := auth.NewRateLimiter(10, time.Minute)

	// Set up HTTP routes
	mux := http.NewServeMux()

	// Health check endpoint (no auth required)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// OAuth metadata endpoints (no auth required - used for discovery)
	mux.HandleFunc("/.well-known/oauth-protected-resource", oauthServer.ProtectedResourceMetadata)
	mux.HandleFunc("/.well-known/oauth-authorization-server", oauthServer.AuthorizationServerMetadata)

	// OAuth flow endpoints (no auth required - these establish auth)
	mux.HandleFunc("/authorize", oauthServer.Authorize)
	// Token endpoint with rate limiting to prevent brute force
	mux.Handle("/token", auth.RateLimitMiddleware(tokenRateLimiter)(http.HandlerFunc(oauthServer.Token)))
	mux.HandleFunc("/register", oauthServer.Register)

	// Create unified auth middleware that accepts both static and OAuth tokens
	authMiddleware := auth.Middleware(auth.MiddlewareConfig{
		Validator: auth.NewMultiValidator(
			auth.NewStaticTokenValidator(cfg.AuthToken),
			auth.NewOAuthTokenValidator(tokenStore),
		),
		ResourceMetadataURL: baseURL + "/.well-known/oauth-protected-resource",
	})

	// MCP endpoint (auth required)
	// The MCP SDK handler handles both GET and POST for the streamable HTTP transport
	// Serve at both /mcp (explicit) and / (for Claude.ai custom connectors that use base URL)
	mux.Handle("/mcp", authMiddleware(mcpHandler))
	mux.Handle("/", authMiddleware(mcpHandler))

	// Create HTTP server
	httpServer := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mux,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Momentum MCP server starting on port %s", cfg.Port)
		log.Printf("Health check: %s/health", baseURL)
		log.Printf("MCP endpoint: %s/mcp", baseURL)
		log.Printf("OAuth metadata: %s/.well-known/oauth-authorization-server", baseURL)

		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Give outstanding requests 5 seconds to complete
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped")
}
