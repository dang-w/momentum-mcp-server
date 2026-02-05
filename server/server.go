// Package server provides the MCP server setup and configuration.
package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	ServerName    = "momentum"
	ServerVersion = "0.1.0"
)

// New creates and configures a new MCP server with all resources and tools registered.
func New() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    ServerName,
		Version: ServerVersion,
	}, nil)

	// Register placeholder ping tool for initial verification
	registerPingTool(server)

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
