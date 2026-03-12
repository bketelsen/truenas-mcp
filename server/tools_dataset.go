package server

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"truenas-mcp/truenas"
)

func registerDatasetReadTools(s *mcp.Server, client *truenas.Client) {
	s.AddTool(&mcp.Tool{
		Name:        "truenas_dataset_list",
		Description: "List datasets with name, used space, available space, mountpoint, and compression. Optionally filter by pool.",
		InputSchema: schema(map[string]any{
			"pool": stringProp("optional pool name to filter datasets"),
		}),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := args(req)
		params := []any{}
		if pool, ok := a["pool"].(string); ok && pool != "" {
			params = append(params, [][]any{{"pool", "=", pool}})
		}
		result, err := client.Call("pool.dataset.query", params...)
		if err != nil {
			return nil, fmt.Errorf("pool.dataset.query: %w", err)
		}
		return jsonResult(result)
	})

	s.AddTool(&mcp.Tool{
		Name:        "truenas_dataset_get",
		Description: "Get full properties for a specific dataset by path.",
		InputSchema: schema(map[string]any{
			"path": stringProp("full dataset path (e.g. tank/data)"),
		}, "path"),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := args(req)
		path, ok := a["path"].(string)
		if !ok || path == "" {
			return nil, fmt.Errorf("required parameter 'path' missing")
		}
		result, err := client.Call("pool.dataset.query", [][]any{{"id", "=", path}})
		if err != nil {
			return nil, fmt.Errorf("pool.dataset.query: %w", err)
		}
		return jsonResult(result)
	})

}

func registerDatasetWriteTools(s *mcp.Server, client *truenas.Client) {
	s.AddTool(&mcp.Tool{
		Name:        "truenas_dataset_create",
		Description: "Create a new ZFS dataset. Requires the full path (e.g. tank/newdata).",
		InputSchema: schema(map[string]any{
			"name":        stringProp("full dataset path to create (e.g. tank/newdata)"),
			"comments":    stringProp("optional description"),
			"compression": stringProp("compression algorithm (e.g. lz4, zstd, off)"),
		}, "name"),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := args(req)
		name, ok := a["name"].(string)
		if !ok || name == "" {
			return nil, fmt.Errorf("required parameter 'name' missing")
		}
		params := map[string]any{"name": name}
		if comments, ok := a["comments"].(string); ok && comments != "" {
			params["comments"] = comments
		}
		if compression, ok := a["compression"].(string); ok && compression != "" {
			params["compression"] = compression
		}
		result, err := client.Call("pool.dataset.create", params)
		if err != nil {
			return nil, fmt.Errorf("pool.dataset.create: %w", err)
		}
		return jsonResult(result)
	})

	s.AddTool(&mcp.Tool{
		Name:        "truenas_dataset_delete",
		Description: "Delete a ZFS dataset by path. WARNING: This is destructive and cannot be undone.",
		InputSchema: schema(map[string]any{
			"path": stringProp("full dataset path to delete (e.g. tank/olddata)"),
		}, "path"),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := args(req)
		path, ok := a["path"].(string)
		if !ok || path == "" {
			return nil, fmt.Errorf("required parameter 'path' missing")
		}
		result, err := client.Call("pool.dataset.delete", path)
		if err != nil {
			return nil, fmt.Errorf("pool.dataset.delete: %w", err)
		}
		return jsonResult(result)
	})
}
