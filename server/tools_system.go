package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"truenas-mcp/truenas"
)

// schema builds an MCP tool input schema. Pass nil for no-arg tools.
func schema(properties map[string]any, required ...string) map[string]any {
	s := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func noArgs() map[string]any {
	return schema(map[string]any{})
}

func stringProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func numberProp(desc string) map[string]any {
	return map[string]any{"type": "number", "description": desc}
}

func boolProp(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

func arrayProp(desc string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": desc}
}

func args(req *mcp.CallToolRequest) map[string]any {
	var m map[string]any
	if err := json.Unmarshal(req.Params.Arguments, &m); err != nil || m == nil {
		return map[string]any{}
	}
	return m
}

func jsonResult(raw json.RawMessage) (*mcp.CallToolResult, error) {
	pretty, err := json.MarshalIndent(json.RawMessage(raw), "", "  ")
	if err != nil {
		return nil, fmt.Errorf("formatting result: %w", err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(pretty)},
		},
	}, nil
}

func registerSystemTools(s *mcp.Server, client *truenas.Client) {
	s.AddTool(&mcp.Tool{
		Name:        "truenas_system_info",
		Description: "Get TrueNAS system information including hostname, version, uptime, and platform.",
		InputSchema: noArgs(),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := client.Call("system.info")
		if err != nil {
			return nil, fmt.Errorf("system.info: %w", err)
		}
		return jsonResult(result)
	})

	s.AddTool(&mcp.Tool{
		Name:        "truenas_disk_list",
		Description: "List all physical disks with name, size, model, serial, and health status.",
		InputSchema: noArgs(),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := client.Call("disk.query")
		if err != nil {
			return nil, fmt.Errorf("disk.query: %w", err)
		}
		return jsonResult(result)
	})

	s.AddTool(&mcp.Tool{
		Name:        "truenas_network_list",
		Description: "List network interfaces with IP addresses and link status.",
		InputSchema: noArgs(),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := client.Call("interface.query")
		if err != nil {
			return nil, fmt.Errorf("interface.query: %w", err)
		}
		return jsonResult(result)
	})
}
