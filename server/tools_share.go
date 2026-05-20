package server

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"truenas-mcp/truenas"
)

func registerShareReadTools(s *mcp.Server, client truenas.Caller) {
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

func registerShareWriteTools(s *mcp.Server, client truenas.Caller) {
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
		name, err := requireString(req, "name")
		if err != nil {
			return nil, err
		}
		path, err := requireString(req, "path")
		if err != nil {
			return nil, err
		}
		params := map[string]any{"name": name, "path": path}
		if comment := optionalString(req, "comment"); comment != "" {
			params["comment"] = comment
		}
		if optionalBool(req, "guest_ok") {
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
		id, err := requireFloat64(req, "id")
		if err != nil {
			return nil, err
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
		path, err := requireString(req, "path")
		if err != nil {
			return nil, err
		}
		params := map[string]any{"path": path}
		if networks := optionalSlice(req, "networks"); len(networks) > 0 {
			params["networks"] = networks
		}
		if hosts := optionalSlice(req, "hosts"); len(hosts) > 0 {
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
		id, err := requireFloat64(req, "id")
		if err != nil {
			return nil, err
		}
		result, err := client.Call("sharing.nfs.delete", int(id))
		if err != nil {
			return nil, fmt.Errorf("sharing.nfs.delete: %w", err)
		}
		return jsonResult(result)
	})
}
