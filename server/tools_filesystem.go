package server

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"unicode"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"truenas-mcp/truenas"
)

// validateMntPath validates and normalizes a filesystem path below /mnt.
func validateMntPath(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("path must not be empty")
	}
	for _, r := range p {
		if r == '\x00' || unicode.IsControl(r) {
			return "", fmt.Errorf("path must not contain NUL or control characters")
		}
	}
	if !strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("path must start with /")
	}
	for _, segment := range strings.Split(p, "/") {
		if segment == ".." {
			return "", fmt.Errorf("path segment .. is not allowed")
		}
	}
	cleaned := path.Clean(p)
	if !strings.HasPrefix(cleaned, "/mnt/") {
		return "", fmt.Errorf("path must be below /mnt")
	}
	return cleaned, nil
}

func registerFilesystemReadTools(s *mcp.Server, client truenas.Caller) {
	s.AddTool(&mcp.Tool{
		Name:        "truenas_filesystem_stat",
		Description: "Return owner (uid, gid, user, group), mode and permissions, file type and the ACL type for 1 path below /mnt. It refuses paths outside /mnt and any '..' segment. It calls filesystem.stat and filesystem.getacl.",
		InputSchema: schema(map[string]any{
			"path": stringProp("absolute path below /mnt (e.g. /mnt/tank/data)"),
		}, "path"),
		Annotations: &mcp.ToolAnnotations{
			Title:         "Filesystem Stat",
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		m := parseArgs(req)
		v, present := m["path"]
		if !present || v == nil {
			return nil, fmt.Errorf("required parameter \"path\" missing")
		}
		p, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("parameter \"path\" must be a string, got %T", v)
		}

		cleaned, err := validateMntPath(p)
		if err != nil {
			return nil, err
		}

		raw, err := client.Call("filesystem.stat", cleaned)
		if err != nil {
			return nil, fmt.Errorf("filesystem.stat: %w", err)
		}

		var stat struct {
			Realpath *string `json:"realpath"`
			Type     *string `json:"type"`
			UID      *int64  `json:"uid"`
			GID      *int64  `json:"gid"`
			User     *string `json:"user"`
			Group    *string `json:"group"`
			Mode     *int64  `json:"mode"`
		}
		if err := json.Unmarshal(raw, &stat); err != nil {
			return nil, fmt.Errorf("filesystem.stat: decoding response: %w", err)
		}
		if stat.Realpath == nil {
			return nil, fmt.Errorf("filesystem.stat returned missing realpath")
		}
		realpathCleaned := path.Clean(*stat.Realpath)
		if !strings.HasPrefix(realpathCleaned, "/mnt/") {
			return nil, fmt.Errorf("path resolves outside /mnt")
		}
		if stat.Mode == nil {
			return nil, fmt.Errorf("filesystem.stat returned missing mode")
		}
		if stat.UID == nil {
			return nil, fmt.Errorf("filesystem.stat returned missing uid")
		}
		if stat.GID == nil {
			return nil, fmt.Errorf("filesystem.stat returned missing gid")
		}

		rawACL, err := client.Call("filesystem.getacl", realpathCleaned, true)
		if err != nil {
			return nil, fmt.Errorf("filesystem.getacl: %w", err)
		}
		var acl struct {
			ACLType *string `json:"acltype"`
			Trivial *bool   `json:"trivial"`
		}
		if err := json.Unmarshal(rawACL, &acl); err != nil {
			return nil, fmt.Errorf("filesystem.getacl: decoding response: %w", err)
		}
		if acl.ACLType == nil || acl.Trivial == nil {
			return nil, fmt.Errorf("filesystem.getacl returned missing acltype or trivial")
		}

		var user any
		if stat.User != nil {
			user = *stat.User
		}
		var group any
		if stat.Group != nil {
			group = *stat.Group
		}
		var fileType any
		if stat.Type != nil {
			fileType = *stat.Type
		}

		return jsonValueResult(map[string]any{
			"path":        cleaned,
			"realpath":    realpathCleaned,
			"type":        fileType,
			"uid":         *stat.UID,
			"gid":         *stat.GID,
			"user":        user,
			"group":       group,
			"mode":        *stat.Mode,
			"permissions": fmt.Sprintf("%04o", *stat.Mode&07777),
			"acl_type":    *acl.ACLType,
			"acl_trivial": *acl.Trivial,
		})
	})
}
