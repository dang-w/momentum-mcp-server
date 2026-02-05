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

// New creates and configures a new MCP server with all resources and tools registered.
// The storage parameter is used for reading and writing productivity data files.
func New(store storage.Storage) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    ServerName,
		Version: ServerVersion,
	}, nil)

	// Register placeholder ping tool for verification
	registerPingTool(server)

	// Register resources
	resources.NewTodosResource(store).Register(server)
	resources.NewStrategyResource(store).Register(server)
	resources.NewReadingResource(store).Register(server)
	resources.NewRemindersResource(store).Register(server)

	// Register tools
	tools.NewTodoTools(store).Register(server)
	tools.NewStrategyTools(store).Register(server)
	tools.NewReadingTools(store).Register(server)
	tools.NewReminderTools(store).Register(server)

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
