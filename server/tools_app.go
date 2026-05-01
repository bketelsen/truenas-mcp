package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"truenas-mcp/truenas"
)

func registerAppReadTools(s *mcp.Server, client truenas.Caller) {
	s.AddTool(&mcp.Tool{
		Name:        "truenas_app_list",
		Description: "List all installed apps with name, version, status (running/stopped), and update availability.",
		InputSchema: noArgs(),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := client.Call("app.query")
		if err != nil {
			return nil, fmt.Errorf("app.query: %w", err)
		}
		return jsonResult(result)
	})

	s.AddTool(&mcp.Tool{
		Name:        "truenas_app_get",
		Description: "Get detailed information for a specific app by name.",
		InputSchema: schema(map[string]any{
			"name": stringProp("app name to inspect"),
		}, "name"),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := args(req)
		name, ok := a["name"].(string)
		if !ok || name == "" {
			return nil, fmt.Errorf("required parameter 'name' missing")
		}
		result, err := client.Call("app.query", [][]any{{"name", "=", name}})
		if err != nil {
			return nil, fmt.Errorf("app.query: %w", err)
		}
		return jsonResult(result)
	})

	s.AddTool(&mcp.Tool{
		Name:        "truenas_apps_update_report",
		Description: "Report installed apps with TrueNAS app or container image updates available.",
		InputSchema: noArgs(),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := client.Call("app.query")
		if err != nil {
			return nil, fmt.Errorf("app.query: %w", err)
		}

		var apps []map[string]any
		if err := json.Unmarshal(result, &apps); err != nil {
			return nil, fmt.Errorf("parsing app.query: %w", err)
		}

		candidates := []map[string]any{}
		for _, app := range apps {
			upgradeAvailable, _ := app["upgrade_available"].(bool)
			imageUpdatesAvailable, _ := app["image_updates_available"].(bool)
			if !upgradeAvailable && !imageUpdatesAvailable {
				continue
			}

			candidates = append(candidates, map[string]any{
				"name":                    app["name"],
				"id":                      app["id"],
				"state":                   app["state"],
				"version":                 app["version"],
				"human_version":           app["human_version"],
				"latest_version":          app["latest_version"],
				"upgrade_available":       upgradeAvailable,
				"image_updates_available": imageUpdatesAvailable,
			})
		}

		report := map[string]any{
			"summary": map[string]any{
				"apps_total":        len(apps),
				"updates_available": len(candidates),
			},
			"apps": candidates,
		}
		return jsonValueResult(report)
	})

}

func registerAppWriteTools(s *mcp.Server, client truenas.Caller) {
	s.AddTool(&mcp.Tool{
		Name:        "truenas_app_start",
		Description: "Start a stopped app by name.",
		InputSchema: schema(map[string]any{
			"name": stringProp("app name to start"),
		}, "name"),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := args(req)
		name, ok := a["name"].(string)
		if !ok || name == "" {
			return nil, fmt.Errorf("required parameter 'name' missing")
		}
		result, err := client.Call("app.start", name)
		if err != nil {
			return nil, fmt.Errorf("app.start: %w", err)
		}
		return jsonResult(result)
	})

	s.AddTool(&mcp.Tool{
		Name:        "truenas_app_stop",
		Description: "Stop a running app by name.",
		InputSchema: schema(map[string]any{
			"name": stringProp("app name to stop"),
		}, "name"),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := args(req)
		name, ok := a["name"].(string)
		if !ok || name == "" {
			return nil, fmt.Errorf("required parameter 'name' missing")
		}
		result, err := client.Call("app.stop", name)
		if err != nil {
			return nil, fmt.Errorf("app.stop: %w", err)
		}
		return jsonResult(result)
	})

	s.AddTool(&mcp.Tool{
		Name:        "truenas_app_restart",
		Description: "Restart an app by name.",
		InputSchema: schema(map[string]any{
			"name": stringProp("app name to restart"),
		}, "name"),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := args(req)
		name, ok := a["name"].(string)
		if !ok || name == "" {
			return nil, fmt.Errorf("required parameter 'name' missing")
		}
		result, err := client.Call("app.restart", name)
		if err != nil {
			return nil, fmt.Errorf("app.restart: %w", err)
		}
		return jsonResult(result)
	})
}
