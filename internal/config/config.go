// Package config handles loading and validating environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Default OAuth token lifetimes.
const (
	DefaultAccessTokenTTL  = time.Hour         // 1 hour
	DefaultRefreshTokenTTL = 7 * 24 * time.Hour // 7 days
)

// Config holds all configuration values for the server.
type Config struct {
	// GitHubToken is the personal access token for GitHub API access.
	GitHubToken string

	// GitHubRepo is the data repository in "owner/repo" format.
	GitHubRepo string

	// AuthToken is the shared secret for authenticating MCP clients (Claude Code).
	AuthToken string

	// Port is the HTTP port to listen on.
	Port string

	// OAuth Configuration

	// OAuthAuthorizePin is an optional PIN required on the authorize page.
	// If empty, authorization requests are auto-approved (single-user mode).
	OAuthAuthorizePin string

	// OAuthAccessTokenTTL is the lifetime of issued access tokens.
	OAuthAccessTokenTTL time.Duration

	// OAuthRefreshTokenTTL is the lifetime of issued refresh tokens.
	OAuthRefreshTokenTTL time.Duration

	// BaseURL is the public URL of this server (used for OAuth issuer).
	// If not set, it will be derived from request headers.
	BaseURL string

	// DataDir is the directory for persistent data (OAuth tokens, etc.).
	// If empty, data is stored in memory only (lost on restart).
	DataDir string
}

// Load reads configuration from environment variables and validates
// that all required values are present.
func Load() (*Config, error) {
	cfg := &Config{
		GitHubToken:       os.Getenv("GITHUB_TOKEN"),
		GitHubRepo:        os.Getenv("GITHUB_REPO"),
		AuthToken:         os.Getenv("AUTH_TOKEN"),
		Port:              os.Getenv("PORT"),
		OAuthAuthorizePin: os.Getenv("OAUTH_AUTHORIZE_PIN"),
		BaseURL:           os.Getenv("BASE_URL"),
		DataDir:           os.Getenv("DATA_DIR"),
	}

	// Default port if not specified
	if cfg.Port == "" {
		cfg.Port = "8080"
	}

	// Parse OAuth token TTLs with defaults
	cfg.OAuthAccessTokenTTL = parseDurationSeconds(
		os.Getenv("OAUTH_ACCESS_TOKEN_TTL"),
		DefaultAccessTokenTTL,
	)
	cfg.OAuthRefreshTokenTTL = parseDurationSeconds(
		os.Getenv("OAUTH_REFRESH_TOKEN_TTL"),
		DefaultRefreshTokenTTL,
	)

	// Validate required fields
	if cfg.GitHubToken == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN environment variable is required")
	}
	if cfg.GitHubRepo == "" {
		return nil, fmt.Errorf("GITHUB_REPO environment variable is required")
	}
	if cfg.AuthToken == "" {
		return nil, fmt.Errorf("AUTH_TOKEN environment variable is required")
	}

	return cfg, nil
}

// parseDurationSeconds parses a string as seconds and returns a Duration.
// If the string is empty or invalid, returns the default value.
func parseDurationSeconds(s string, defaultVal time.Duration) time.Duration {
	if s == "" {
		return defaultVal
	}
	seconds, err := strconv.Atoi(s)
	if err != nil || seconds <= 0 {
		return defaultVal
	}
	return time.Duration(seconds) * time.Second
}

// GitHubUsername extracts the owner/username from the GitHubRepo.
func (c *Config) GitHubUsername() string {
	parts := strings.SplitN(c.GitHubRepo, "/", 2)
	if len(parts) >= 1 {
		return parts[0]
	}
	return ""
}
