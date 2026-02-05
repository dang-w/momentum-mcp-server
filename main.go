package main

import (
	"context"
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
	store, err := storage.NewGitHubStorage(cfg.GitHubToken, cfg.GitHubRepo)
	if err != nil {
		log.Fatalf("Failed to create storage: %v", err)
	}

	// Create MCP server with storage
	mcpServer := server.New(store)

	// Create the streamable HTTP handler for MCP
	mcpHandler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return mcpServer
	}, nil)

	// Set up HTTP routes
	mux := http.NewServeMux()

	// Health check endpoint (no auth required)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// MCP endpoint (auth required)
	// The MCP SDK handler handles both GET and POST for the streamable HTTP transport
	authMiddleware := auth.Middleware(cfg.AuthToken)
	mux.Handle("/mcp", authMiddleware(mcpHandler))

	// Create HTTP server
	httpServer := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mux,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Momentum MCP server starting on port %s", cfg.Port)
		log.Printf("Health check: http://localhost:%s/health", cfg.Port)
		log.Printf("MCP endpoint: http://localhost:%s/mcp", cfg.Port)

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
