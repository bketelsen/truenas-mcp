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

// queryRows checks the result of one middleware query call and decodes its
// rows. A call error becomes "<label>: <error>" and a decode error becomes
// "parsing <label>: <error>", both wrapped.
func queryRows(label string, raw json.RawMessage, err error) ([]map[string]any, error) {
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", label, err)
	}
	return rows, nil
}

// callSection calls one health report section and decodes its answer.
func callSection(client truenas.Caller, method string, params ...interface{}) (any, error) {
	result, err := client.Call(method, params...)
	if err != nil {
		return nil, err
	}
	var parsed any
	if err := json.Unmarshal(result, &parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

// countUnhealthyPools counts the pools that are not healthy, carry a warning,
// or are not ONLINE. A value that is not a list counts 0.
func countUnhealthyPools(value any) int {
	pools, ok := value.([]any)
	if !ok {
		return 0
	}
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
	return unhealthy
}

// countAlertLevels counts the CRITICAL and the WARNING (or WARN) alerts. A value
// that is not a list counts 0.
func countAlertLevels(value any) (critical, warning int) {
	alerts, ok := value.([]any)
	if !ok {
		return 0, 0
	}
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
	return critical, warning
}

func registerHealthReportTools(s *mcp.Server, client truenas.Caller) {
	s.AddTool(&mcp.Tool{
		Name:        "truenas_health_report",
		Description: "Return a read-only health report aggregated from system state, pools, disks, and alerts.",
		InputSchema: noArgs(),
		Annotations: &mcp.ToolAnnotations{
			Title:         "Health Report",
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		failedSections := 0
		details := map[string]any{}
		errors := map[string]string{}

		addSection := func(name, method string, params ...interface{}) {
			parsed, err := callSection(client, method, params...)
			if err != nil {
				errors[name] = err.Error()
				failedSections++
				return
			}
			details[name] = parsed
		}

		addSection("system", "system.info")
		addSection("system_state", "system.state")
		addSection("pools", "pool.query")
		addSection("disks", "disk.query")
		addSection("alerts", "alert.list")

		poolsUnhealthy := countUnhealthyPools(details["pools"])
		alertsCritical, alertsWarning := countAlertLevels(details["alerts"])

		status := "ok"
		if failedSections > 0 || alertsCritical > 0 || poolsUnhealthy > 0 {
			status = "critical"
		} else if alertsWarning > 0 {
			status = "warning"
		}

		report := map[string]any{
			"summary": map[string]any{
				"status":          status,
				"failed_sections": failedSections,
				"pools_unhealthy": poolsUnhealthy,
				"alerts_critical": alertsCritical,
				"alerts_warning":  alertsWarning,
			},
			"details": details,
			"errors":  errors,
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
		Annotations: &mcp.ToolAnnotations{
			Title:         "List Jobs",
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filters := [][]any{}
		if state := optionalString(req, "state"); state != "" {
			filters = append(filters, []any{"state", "=", strings.ToUpper(state)})
		}
		if method := optionalString(req, "method"); method != "" {
			filters = append(filters, []any{"method", "=", method})
		}
		limit := 50
		if rawLimit, ok := optionalFloat64(req, "limit"); ok {
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
