package server

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"truenas-mcp/truenas"
)

func registerSnapshotReadTools(s *mcp.Server, client *truenas.Client) {
	s.AddTool(&mcp.Tool{
		Name:        "truenas_snapshot_list",
		Description: "List snapshots for a specific dataset with name, creation time, and referenced size.",
		InputSchema: schema(map[string]any{
			"dataset": stringProp("dataset path to list snapshots for"),
		}, "dataset"),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := args(req)
		dataset, ok := a["dataset"].(string)
		if !ok || dataset == "" {
			return nil, fmt.Errorf("required parameter 'dataset' missing")
		}
		result, err := client.Call("zfs.snapshot.query", [][]any{{"dataset", "=", dataset}})
		if err != nil {
			return nil, fmt.Errorf("zfs.snapshot.query: %w", err)
		}
		return jsonResult(result)
	})

	s.AddTool(&mcp.Tool{
		Name:        "truenas_snapshot_get",
		Description: "Get full details for a specific snapshot by name.",
		InputSchema: schema(map[string]any{
			"name": stringProp("full snapshot name (e.g. tank/data@snap1)"),
		}, "name"),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := args(req)
		name, ok := a["name"].(string)
		if !ok || name == "" {
			return nil, fmt.Errorf("required parameter 'name' missing")
		}
		result, err := client.Call("zfs.snapshot.query", [][]any{{"id", "=", name}})
		if err != nil {
			return nil, fmt.Errorf("zfs.snapshot.query: %w", err)
		}
		return jsonResult(result)
	})

}

func registerSnapshotWriteTools(s *mcp.Server, client *truenas.Client) {
	s.AddTool(&mcp.Tool{
		Name:        "truenas_snapshot_create",
		Description: "Create a ZFS snapshot. Auto-generates a timestamp name if omitted.",
		InputSchema: schema(map[string]any{
			"dataset": stringProp("dataset path to snapshot (e.g. tank/data)"),
			"name":    stringProp("optional snapshot name (auto-generates if omitted)"),
		}, "dataset"),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := args(req)
		dataset, ok := a["dataset"].(string)
		if !ok || dataset == "" {
			return nil, fmt.Errorf("required parameter 'dataset' missing")
		}
		snapName, _ := a["name"].(string)
		if snapName == "" {
			snapName = "auto-" + time.Now().UTC().Format("20060102-150405")
		}
		params := map[string]any{
			"dataset": dataset,
			"name":    snapName,
		}
		result, err := client.Call("zfs.snapshot.create", params)
		if err != nil {
			return nil, fmt.Errorf("zfs.snapshot.create: %w", err)
		}
		return jsonResult(result)
	})

	s.AddTool(&mcp.Tool{
		Name:        "truenas_snapshot_delete",
		Description: "Delete a ZFS snapshot by full name (e.g. tank/data@snap1). This is destructive.",
		InputSchema: schema(map[string]any{
			"name": stringProp("full snapshot name to delete (e.g. tank/data@snap1)"),
		}, "name"),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := args(req)
		name, ok := a["name"].(string)
		if !ok || name == "" {
			return nil, fmt.Errorf("required parameter 'name' missing")
		}
		result, err := client.Call("zfs.snapshot.delete", name)
		if err != nil {
			return nil, fmt.Errorf("zfs.snapshot.delete: %w", err)
		}
		return jsonResult(result)
	})
}
