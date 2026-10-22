package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"truenas-mcp/truenas"
)

func registerShareReadTools(s *mcp.Server, client truenas.Caller) {
	s.AddTool(&mcp.Tool{
		Name:        "truenas_smb_list",
		Description: "List all SMB shares with name, path, and enabled status.",
		InputSchema: noArgs(),
		Annotations: &mcp.ToolAnnotations{
			Title:         "List SMB Shares",
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
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
		Annotations: &mcp.ToolAnnotations{
			Title:         "List NFS Exports",
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
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
			"guest_ok": boolProp("allow guest access (default false); on TrueNAS 25.10 and later this creates a LEGACY_SHARE share, the only purpose that allows guests"),
		}, "name", "path"),
		Annotations: &mcp.ToolAnnotations{
			Title:           "Create SMB Share",
			ReadOnlyHint:    false,
			DestructiveHint: new(false), // additive and undone by truenas_smb_delete; guest_ok widens access but loses no data
			IdempotentHint:  true,       // TrueNAS rejects a second share with the same name
			OpenWorldHint:   new(false),
		},
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
			return smbCreateWithGuest(client, params)
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
		Annotations: &mcp.ToolAnnotations{
			Title:           "Delete SMB Share",
			ReadOnlyHint:    false,
			DestructiveHint: new(true), // the share's settings are lost and clients are cut off, though the data stays
			IdempotentHint:  true,
			OpenWorldHint:   new(false),
		},
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
		Annotations: &mcp.ToolAnnotations{
			Title:           "Create NFS Export",
			ReadOnlyHint:    false,
			DestructiveHint: new(false),
			IdempotentHint:  false, // an export open to everybody (no hosts or networks) can be created twice
			OpenWorldHint:   new(false),
		},
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
		Annotations: &mcp.ToolAnnotations{
			Title:           "Delete NFS Export",
			ReadOnlyHint:    false,
			DestructiveHint: new(true), // the export's settings are lost and clients are cut off, though the data stays
			IdempotentHint:  true,
			OpenWorldHint:   new(false),
		},
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

// smbCreateWithGuest creates an SMB share that allows guest access. Since
// TrueNAS 25.10 the share schema has purpose plus a per-purpose options object,
// and guestok exists only inside options for purpose LEGACY_SHARE. Releases
// before 25.10 know neither field and reject them in validation, so try the
// 25.10+ payload first and fall back to the legacy top-level guestok only on a
// validation error. A validation error means the create never ran, so the retry
// cannot create a duplicate share. This follows the same rule as callSnapshot.
func smbCreateWithGuest(client truenas.Caller, base map[string]any) (*mcp.CallToolResult, error) {
	params := map[string]any{"purpose": "LEGACY_SHARE", "options": map[string]any{"guestok": true}}
	for k, v := range base {
		params[k] = v
	}
	result, err := client.Call("sharing.smb.create", params)
	if err == nil {
		return jsonResult(result)
	}
	if !isSMBShapeRejected(err) {
		return nil, fmt.Errorf("sharing.smb.create: %w", err)
	}
	firstErr := err
	legacy := map[string]any{"guestok": true}
	for k, v := range base {
		legacy[k] = v
	}
	result, err = client.Call("sharing.smb.create", legacy)
	if err != nil {
		return nil, fmt.Errorf("sharing.smb.create: %w (the LEGACY_SHARE payload was rejected first: %v)", err, firstErr)
	}
	return jsonResult(result)
}

// isSMBShapeRejected reports whether err is a validation error about the shape
// of the payload, as a release before 25.10 returns for the purpose or options
// fields it does not know. It matches only the field names in the validation
// reason (`sharing_smb_create.purpose`, `sharing_smb_create.options`), not any
// mention of them in the rest of the envelope, because a Python traceback in
// that envelope can name those words in source lines.
func isSMBShapeRejected(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "einval") &&
		(strings.Contains(msg, "sharing_smb_create.purpose") || strings.Contains(msg, "sharing_smb_create.options"))
}
