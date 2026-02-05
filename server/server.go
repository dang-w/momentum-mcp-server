// Package server provides the MCP server setup and configuration.
package server

import (
	"context"

	"github.com/dang-w/momentum-mcp-server/resources"
	"github.com/dang-w/momentum-mcp-server/storage"
	"github.com/dang-w/momentum-mcp-server/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	ServerName    = "momentum"
	ServerVersion = "0.1.0"
)

// Config holds the configuration needed to create the MCP server.
type Config struct {
	// Storage is used for reading and writing productivity data files.
	Storage storage.Storage

	// GitHubToken is the personal access token for GitHub API access.
	// Used for fetching contribution activity data.
	GitHubToken string

	// GitHubUsername is the GitHub username to fetch activity for.
	GitHubUsername string
}

// New creates and configures a new MCP server with all resources and tools registered.
func New(cfg Config) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    ServerName,
		Version: ServerVersion,
	}, nil)

	// Register placeholder ping tool for verification
	registerPingTool(server)

	// Create GitHub activity resource (used by both github-activity and weekly-summary)
	var githubActivity *resources.GitHubActivityResource
	if cfg.GitHubToken != "" && cfg.GitHubUsername != "" {
		githubActivity = resources.NewGitHubActivityResource(cfg.GitHubToken, cfg.GitHubUsername)
	}

	// Register resources
	resources.NewTodosResource(cfg.Storage).Register(server)
	resources.NewStrategyResource(cfg.Storage).Register(server)
	resources.NewReadingResource(cfg.Storage).Register(server)
	resources.NewRemindersResource(cfg.Storage).Register(server)

	// Register GitHub activity resource if configured
	if githubActivity != nil {
		githubActivity.Register(server)
	}

	// Register weekly summary resource (aggregates all data)
	resources.NewSummaryResource(cfg.Storage, githubActivity).Register(server)

	// Register tools
	tools.NewTodoTools(cfg.Storage).Register(server)
	tools.NewStrategyTools(cfg.Storage).Register(server)
	tools.NewReadingTools(cfg.Storage).Register(server)
	tools.NewReminderTools(cfg.Storage).Register(server)

	return server
}

// PingInput is the input schema for the ping tool.
type PingInput struct{}

// PingOutput is the output schema for the ping tool.
type PingOutput struct {
	Message string `json:"message"`
}

// ping is a placeholder tool to verify the MCP protocol is working.
func ping(ctx context.Context, req *mcp.CallToolRequest, input PingInput) (*mcp.CallToolResult, PingOutput, error) {
	return nil, PingOutput{Message: "pong"}, nil
}

func registerPingTool(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "ping",
		Description: "Simple ping tool to verify the server is responding",
	}, ping)
}
