package server

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"truenas-mcp/truenas"
)

func registerAlertReadTools(s *mcp.Server, client *truenas.Client) {
	s.AddTool(&mcp.Tool{
		Name:        "truenas_alert_list",
		Description: "List active alerts with level (info/warning/critical), message, datetime, and dismissed status. Optionally filter by level.",
		InputSchema: schema(map[string]any{
			"level": stringProp("filter by alert level: INFO, WARNING, CRITICAL, or empty for all"),
		}),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := args(req)
		params := []any{}
		if level, ok := a["level"].(string); ok && level != "" {
			params = append(params, [][]any{{"level", "=", level}})
		}
		result, err := client.Call("alert.list", params...)
		if err != nil {
			return nil, fmt.Errorf("alert.list: %w", err)
		}
		return jsonResult(result)
	})

}

func registerAlertWriteTools(s *mcp.Server, client *truenas.Client) {
	s.AddTool(&mcp.Tool{
		Name:        "truenas_alert_dismiss",
		Description: "Dismiss an alert by ID.",
		InputSchema: schema(map[string]any{
			"id": stringProp("alert ID to dismiss"),
		}, "id"),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := args(req)
		id, ok := a["id"].(string)
		if !ok || id == "" {
			return nil, fmt.Errorf("required parameter 'id' missing")
		}
		result, err := client.Call("alert.dismiss", id)
		if err != nil {
			return nil, fmt.Errorf("alert.dismiss: %w", err)
		}
		return jsonResult(result)
	})
}
