package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"truenas-mcp/truenas"
)

// New creates an MCP server with TrueNAS tools registered.
// When readOnly is true, only read-only tools (list, get, inspect) are registered.
func New(client *truenas.Client, readOnly bool) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "truenas-mcp",
		Version: "0.1.0",
	}, nil)

	// Read-only tools — always registered
	registerSystemTools(s, client)
	registerPoolTools(s, client)
	registerDatasetReadTools(s, client)
	registerSnapshotReadTools(s, client)
	registerShareReadTools(s, client)
	registerAlertReadTools(s, client)
	registerAppReadTools(s, client)

	// Mutating tools — only when not in read-only mode
	if !readOnly {
		registerDatasetWriteTools(s, client)
		registerSnapshotWriteTools(s, client)
		registerShareWriteTools(s, client)
		registerAlertWriteTools(s, client)
		registerAppWriteTools(s, client)
	}

	return s
}

// Run starts the MCP server over stdio, blocking until the client disconnects.
func Run(ctx context.Context, s *mcp.Server) error {
	return s.Run(ctx, &mcp.StdioTransport{})
}
