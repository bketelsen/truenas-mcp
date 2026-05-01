package server

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestHealthReport_SuccessWarning(t *testing.T) {
	calls := []string{}
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			calls = append(calls, method)
			switch method {
			case "system.info":
				return json.RawMessage(`{"hostname":"nas","version":"25.10"}`), nil
			case "system.state":
				return json.RawMessage(`"READY"`), nil
			case "pool.query":
				return json.RawMessage(`[{"name":"tank","healthy":true,"status":"ONLINE"}]`), nil
			case "disk.query":
				return json.RawMessage(`[{"name":"sda","model":"disk"}]`), nil
			case "alert.list":
				return json.RawMessage(`[{"id":"a1","level":"WARNING","text":"check something"}]`), nil
			default:
				t.Fatalf("unexpected method %q", method)
				return nil, nil
			}
		},
	}

	result, err := callTool(t, mock, true, "truenas_health_report", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report map[string]any
	if err := json.Unmarshal([]byte(resultText(t, result)), &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	summary := report["summary"].(map[string]any)
	if summary["status"] != "warning" {
		t.Errorf("status = %v, want warning", summary["status"])
	}
	if summary["alerts_warning"] != float64(1) {
		t.Errorf("alerts_warning = %v, want 1", summary["alerts_warning"])
	}

	wantCalls := []string{"system.info", "system.state", "pool.query", "disk.query", "alert.list"}
	if fmt.Sprint(calls) != fmt.Sprint(wantCalls) {
		t.Errorf("calls = %v, want %v", calls, wantCalls)
	}
}

func TestHealthReport_PartialFailure(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method == "disk.query" {
				return nil, fmt.Errorf("disk unavailable")
			}
			switch method {
			case "system.info":
				return json.RawMessage(`{"hostname":"nas"}`), nil
			case "system.state":
				return json.RawMessage(`"READY"`), nil
			case "pool.query":
				return json.RawMessage(`[{"name":"tank","healthy":false}]`), nil
			case "alert.list":
				return json.RawMessage(`[{"level":"CRITICAL"}]`), nil
			default:
				return nil, fmt.Errorf("unexpected method %s", method)
			}
		},
	}

	result, err := callTool(t, mock, true, "truenas_health_report", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var report map[string]any
	if err := json.Unmarshal([]byte(resultText(t, result)), &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	summary := report["summary"].(map[string]any)
	if summary["status"] != "critical" {
		t.Errorf("status = %v, want critical", summary["status"])
	}
	if summary["failed_sections"] != float64(1) {
		t.Errorf("failed_sections = %v, want 1", summary["failed_sections"])
	}
}

func TestJobsList_WithFilters(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "core.get_jobs" {
				t.Fatalf("method = %q, want core.get_jobs", method)
			}
			if len(params) != 2 {
				t.Fatalf("params len = %d, want 2", len(params))
			}
			filters, ok := params[0].([][]any)
			if !ok {
				t.Fatalf("filters = %T, want [][]any", params[0])
			}
			if fmt.Sprint(filters) != "[[state = FAILED] [method = app.upgrade]]" {
				t.Errorf("filters = %v", filters)
			}
			options, ok := params[1].(map[string]any)
			if !ok {
				t.Fatalf("options = %T, want map[string]any", params[1])
			}
			if options["limit"] != 10 {
				t.Errorf("limit = %v, want 10", options["limit"])
			}
			return json.RawMessage(`[{"id":1,"state":"FAILED","method":"app.upgrade"}]`), nil
		},
	}

	result, err := callTool(t, mock, true, "truenas_jobs_list", map[string]any{
		"state":  "failed",
		"method": "app.upgrade",
		"limit":  10.0,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resultText(t, result) == "" {
		t.Error("empty result")
	}
}

func TestJobsList_LimitClamped(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "core.get_jobs" {
				t.Fatalf("method = %q, want core.get_jobs", method)
			}
			options := params[1].(map[string]any)
			if options["limit"] != 200 {
				t.Errorf("limit = %v, want 200", options["limit"])
			}
			return json.RawMessage(`[]`), nil
		},
	}

	_, err := callTool(t, mock, true, "truenas_jobs_list", map[string]any{"limit": 1000.0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
