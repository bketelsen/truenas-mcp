package server

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"truenas-mcp/truenas"
)

func registerPoolTools(s *mcp.Server, client truenas.Caller) {
	s.AddTool(&mcp.Tool{
		Name:        "truenas_pool_list",
		Description: "List all ZFS pools with name, status, size, and health.",
		InputSchema: noArgs(),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := client.Call("pool.query")
		if err != nil {
			return nil, fmt.Errorf("pool.query: %w", err)
		}
		return jsonResult(result)
	})

	s.AddTool(&mcp.Tool{
		Name:        "truenas_pool_get",
		Description: "Get detailed information for a specific pool by name, including topology (vdevs, disks).",
		InputSchema: schema(map[string]any{
			"name": stringProp("name of the pool to inspect"),
		}, "name"),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := args(req)
		name, ok := a["name"].(string)
		if !ok || name == "" {
			return nil, fmt.Errorf("required parameter 'name' missing")
		}
		result, err := client.Call("pool.query", [][]any{{"name", "=", name}})
		if err != nil {
			return nil, fmt.Errorf("pool.query: %w", err)
		}
		return jsonResult(result)
	})
}
