package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"truenas-mcp/truenas"
)

func registerAlertReadTools(s *mcp.Server, client truenas.Caller) {
	s.AddTool(&mcp.Tool{
		Name:        "truenas_alert_list",
		Description: "List active alerts with level (info/warning/critical), message, datetime, and dismissed status. Optionally filter by level.",
		InputSchema: schema(map[string]any{
			"level": stringProp("filter by alert level: INFO, WARNING, CRITICAL, or empty for all"),
		}),
		Annotations: &mcp.ToolAnnotations{
			Title:         "List Alerts",
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := client.Call("alert.list")
		if err != nil {
			return nil, fmt.Errorf("alert.list: %w", err)
		}
		if level := optionalString(req, "level"); level != "" {
			return jsonResult(filterAlertsByLevel(result, level))
		}
		return jsonResult(result)
	})

}

// filterAlertsByLevel keeps the alerts whose level equals level, ignoring case.
// alert.list accepts no arguments on TrueNAS 27.0 (a query filter is rejected),
// so the tool filters the result instead. Non-list input is returned unchanged.
func filterAlertsByLevel(raw json.RawMessage, level string) json.RawMessage {
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		return raw
	}
	kept := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if lv, ok := item["level"].(string); ok && strings.EqualFold(lv, level) {
			kept = append(kept, item)
		}
	}
	out, err := json.Marshal(kept)
	if err != nil {
		return raw
	}
	return out
}

func registerAlertWriteTools(s *mcp.Server, client truenas.Caller) {
	s.AddTool(&mcp.Tool{
		Name:        "truenas_alert_dismiss",
		Description: "Dismiss an alert by ID.",
		InputSchema: schema(map[string]any{
			"id": stringProp("alert ID to dismiss"),
		}, "id"),
		Annotations: &mcp.ToolAnnotations{
			Title:           "Dismiss Alert",
			ReadOnlyHint:    false,
			DestructiveHint: new(false), // hides or clears a notification; no appliance data or config changes
			IdempotentHint:  true,
			OpenWorldHint:   new(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := requireString(req, "id")
		if err != nil {
			return nil, err
		}
		result, err := client.Call("alert.dismiss", id)
		if err != nil {
			return nil, fmt.Errorf("alert.dismiss: %w", err)
		}
		return jsonResult(result)
	})
}
