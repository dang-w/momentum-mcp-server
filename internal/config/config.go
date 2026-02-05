// Package config handles loading and validating environment variables.
package config

import (
	"fmt"
	"os"
)

// Config holds all configuration values for the server.
type Config struct {
	// GitHubToken is the personal access token for GitHub API access.
	GitHubToken string

	// GitHubRepo is the data repository in "owner/repo" format.
	GitHubRepo string

	// AuthToken is the shared secret for authenticating MCP clients.
	AuthToken string

	// Port is the HTTP port to listen on.
	Port string
}

// Load reads configuration from environment variables and validates
// that all required values are present.
func Load() (*Config, error) {
	cfg := &Config{
		GitHubToken: os.Getenv("GITHUB_TOKEN"),
		GitHubRepo:  os.Getenv("GITHUB_REPO"),
		AuthToken:   os.Getenv("AUTH_TOKEN"),
		Port:        os.Getenv("PORT"),
	}

	// Default port if not specified
	if cfg.Port == "" {
		cfg.Port = "8080"
	}

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
