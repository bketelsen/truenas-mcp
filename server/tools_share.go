package server

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"truenas-mcp/truenas"
)

func registerShareReadTools(s *mcp.Server, client *truenas.Client) {
	s.AddTool(&mcp.Tool{
		Name:        "truenas_smb_list",
		Description: "List all SMB shares with name, path, and enabled status.",
		InputSchema: noArgs(),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := client.Call("sharing.smb.query")
		if err != nil {
			return nil, fmt.Errorf("sharing.smb.query: %w", err)
		}
		return jsonResult(result)
	})

	s.AddTool(&mcp.Tool{
		Name:        "truenas_nfs_list",
		Description: "List all NFS exports with path, networks, and enabled status.",
		InputSchema: noArgs(),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := client.Call("sharing.nfs.query")
		if err != nil {
			return nil, fmt.Errorf("sharing.nfs.query: %w", err)
		}
		return jsonResult(result)
	})

}

func registerShareWriteTools(s *mcp.Server, client *truenas.Client) {
	s.AddTool(&mcp.Tool{
		Name:        "truenas_smb_create",
		Description: "Create an SMB share. The path must point to an existing dataset mountpoint.",
		InputSchema: schema(map[string]any{
			"name":     stringProp("share name"),
			"path":     stringProp("filesystem path to share (e.g. /mnt/tank/data)"),
			"comment":  stringProp("optional description"),
			"guest_ok": boolProp("allow guest access (default false)"),
		}, "name", "path"),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := args(req)
		name, ok := a["name"].(string)
		if !ok || name == "" {
			return nil, fmt.Errorf("required parameter 'name' missing")
		}
		path, ok := a["path"].(string)
		if !ok || path == "" {
			return nil, fmt.Errorf("required parameter 'path' missing")
		}
		params := map[string]any{"name": name, "path": path}
		if comment, ok := a["comment"].(string); ok && comment != "" {
			params["comment"] = comment
		}
		if guestOK, ok := a["guest_ok"].(bool); ok && guestOK {
			params["guestok"] = true
		}
		result, err := client.Call("sharing.smb.create", params)
		if err != nil {
			return nil, fmt.Errorf("sharing.smb.create: %w", err)
		}
		return jsonResult(result)
	})

	s.AddTool(&mcp.Tool{
		Name:        "truenas_smb_delete",
		Description: "Delete an SMB share by ID.",
		InputSchema: schema(map[string]any{
			"id": numberProp("share ID to delete"),
		}, "id"),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := args(req)
		id, ok := a["id"].(float64)
		if !ok {
			return nil, fmt.Errorf("required parameter 'id' missing")
		}
		result, err := client.Call("sharing.smb.delete", int(id))
		if err != nil {
			return nil, fmt.Errorf("sharing.smb.delete: %w", err)
		}
		return jsonResult(result)
	})

	s.AddTool(&mcp.Tool{
		Name:        "truenas_nfs_create",
		Description: "Create an NFS export. The path must point to an existing dataset mountpoint.",
		InputSchema: schema(map[string]any{
			"path":     stringProp("filesystem path to export (e.g. /mnt/tank/data)"),
			"networks": arrayProp("allowed networks (e.g. 192.168.1.0/24)"),
			"hosts":    arrayProp("allowed hosts"),
		}, "path"),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := args(req)
		path, ok := a["path"].(string)
		if !ok || path == "" {
			return nil, fmt.Errorf("required parameter 'path' missing")
		}
		params := map[string]any{"path": path}
		if networks, ok := a["networks"].([]any); ok && len(networks) > 0 {
			params["networks"] = networks
		}
		if hosts, ok := a["hosts"].([]any); ok && len(hosts) > 0 {
			params["hosts"] = hosts
		}
		result, err := client.Call("sharing.nfs.create", params)
		if err != nil {
			return nil, fmt.Errorf("sharing.nfs.create: %w", err)
		}
		return jsonResult(result)
	})

	s.AddTool(&mcp.Tool{
		Name:        "truenas_nfs_delete",
		Description: "Delete an NFS export by ID.",
		InputSchema: schema(map[string]any{
			"id": numberProp("export ID to delete"),
		}, "id"),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := args(req)
		id, ok := a["id"].(float64)
		if !ok {
			return nil, fmt.Errorf("required parameter 'id' missing")
		}
		result, err := client.Call("sharing.nfs.delete", int(id))
		if err != nil {
			return nil, fmt.Errorf("sharing.nfs.delete: %w", err)
		}
		return jsonResult(result)
	})
}
