package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"truenas-mcp/truenas"
)

func jsonValueResult(v any) (*mcp.CallToolResult, error) {
	pretty, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("formatting result: %w", err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(pretty)},
		},
	}, nil
}

func registerHealthReportTools(s *mcp.Server, client truenas.Caller) {
	s.AddTool(&mcp.Tool{
		Name:        "truenas_health_report",
		Description: "Return a read-only health report aggregated from system state, pools, disks, and alerts.",
		InputSchema: noArgs(),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		report := map[string]any{
			"summary": map[string]any{
				"status":          "ok",
				"failed_sections": 0,
				"pools_unhealthy": 0,
				"alerts_critical": 0,
				"alerts_warning":  0,
			},
			"details": map[string]any{},
			"errors":  map[string]string{},
		}
		summary := report["summary"].(map[string]any)
		details := report["details"].(map[string]any)
		errors := report["errors"].(map[string]string)

		addSection := func(name, method string, params ...interface{}) {
			result, err := client.Call(method, params...)
			if err != nil {
				errors[name] = err.Error()
				summary["failed_sections"] = summary["failed_sections"].(int) + 1
				return
			}
			var parsed any
			if err := json.Unmarshal(result, &parsed); err != nil {
				errors[name] = err.Error()
				summary["failed_sections"] = summary["failed_sections"].(int) + 1
				return
			}
			details[name] = parsed
		}

		addSection("system", "system.info")
		addSection("system_state", "system.state")
		addSection("pools", "pool.query")
		addSection("disks", "disk.query")
		addSection("alerts", "alert.list")

		if pools, ok := details["pools"].([]any); ok {
			unhealthy := 0
			for _, item := range pools {
				pool, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if healthy, ok := pool["healthy"].(bool); ok && !healthy {
					unhealthy++
					continue
				}
				if warning, ok := pool["warning"].(bool); ok && warning {
					unhealthy++
					continue
				}
				if status, ok := pool["status"].(string); ok && !strings.EqualFold(status, "ONLINE") {
					unhealthy++
				}
			}
			summary["pools_unhealthy"] = unhealthy
		}

		if alerts, ok := details["alerts"].([]any); ok {
			critical := 0
			warning := 0
			for _, item := range alerts {
				alert, ok := item.(map[string]any)
				if !ok {
					continue
				}
				level, _ := alert["level"].(string)
				switch strings.ToUpper(level) {
				case "CRITICAL":
					critical++
				case "WARNING", "WARN":
					warning++
				}
			}
			summary["alerts_critical"] = critical
			summary["alerts_warning"] = warning
		}

		if summary["failed_sections"].(int) > 0 || summary["alerts_critical"].(int) > 0 || summary["pools_unhealthy"].(int) > 0 {
			summary["status"] = "critical"
		} else if summary["alerts_warning"].(int) > 0 {
			summary["status"] = "warning"
		}

		return jsonValueResult(report)
	})
}

func registerJobReadTools(s *mcp.Server, client truenas.Caller) {
	s.AddTool(&mcp.Tool{
		Name:        "truenas_jobs_list",
		Description: "List recent TrueNAS jobs, optionally filtered by state or method.",
		InputSchema: schema(map[string]any{
			"state":  stringProp("optional job state filter: WAITING, RUNNING, SUCCESS, FAILED, or ABORTED"),
			"method": stringProp("optional job method filter"),
			"limit":  numberProp("maximum jobs to return, default 50"),
		}),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		a := args(req)
		filters := [][]any{}
		if state, ok := a["state"].(string); ok && state != "" {
			filters = append(filters, []any{"state", "=", strings.ToUpper(state)})
		}
		if method, ok := a["method"].(string); ok && method != "" {
			filters = append(filters, []any{"method", "=", method})
		}
		limit := 50
		if rawLimit, ok := a["limit"].(float64); ok && rawLimit > 0 {
			limit = int(rawLimit)
		}
		if limit > 200 {
			limit = 200
		}
		options := map[string]any{
			"order_by": []string{"-time_started"},
			"limit":    limit,
		}
		result, err := client.Call("core.get_jobs", filters, options)
		if err != nil {
			return nil, fmt.Errorf("core.get_jobs: %w", err)
		}
		return jsonResult(result)
	})
}
