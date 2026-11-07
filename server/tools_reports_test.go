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

func TestHealthReport_Golden(t *testing.T) {
	tests := []struct {
		name      string
		responses map[string]struct {
			raw string
			err error
		}
		want string
	}{
		{
			name: "all sections healthy",
			responses: map[string]struct {
				raw string
				err error
			}{
				"system.info":  {raw: `{"hostname":"nas","version":"25.10"}`},
				"system.state": {raw: `"READY"`},
				"pool.query":   {raw: `[{"name":"tank","healthy":true,"status":"ONLINE"}]`},
				"disk.query":   {raw: `[{"name":"sda","model":"disk"}]`},
				"alert.list":   {raw: `[]`},
			},
			want: `{
  "details": {
    "alerts": [],
    "disks": [
      {
        "model": "disk",
        "name": "sda"
      }
    ],
    "pools": [
      {
        "healthy": true,
        "name": "tank",
        "status": "ONLINE"
      }
    ],
    "system": {
      "hostname": "nas",
      "version": "25.10"
    },
    "system_state": "READY"
  },
  "errors": {},
  "summary": {
    "alerts_critical": 0,
    "alerts_warning": 0,
    "failed_sections": 0,
    "pools_unhealthy": 0,
    "status": "ok"
  }
}`,
		},
		{
			name: "pools unhealthy counters",
			responses: map[string]struct {
				raw string
				err error
			}{
				"system.info":  {raw: `{}`},
				"system.state": {raw: `"READY"`},
				"pool.query": {raw: `[
  {"name":"false","healthy":false},
  {"name":"warning","warning":true},
  {"name":"degraded","status":"DEGRADED"},
  42,
  {"name":"online","healthy":true,"warning":false,"status":"ONLINE"}
]`},
				"disk.query": {raw: `[]`},
				"alert.list": {raw: `[]`},
			},
			want: `{
  "details": {
    "alerts": [],
    "disks": [],
    "pools": [
      {
        "healthy": false,
        "name": "false"
      },
      {
        "name": "warning",
        "warning": true
      },
      {
        "name": "degraded",
        "status": "DEGRADED"
      },
      42,
      {
        "healthy": true,
        "name": "online",
        "status": "ONLINE",
        "warning": false
      }
    ],
    "system": {},
    "system_state": "READY"
  },
  "errors": {},
  "summary": {
    "alerts_critical": 0,
    "alerts_warning": 0,
    "failed_sections": 0,
    "pools_unhealthy": 3,
    "status": "critical"
  }
}`,
		},
		{
			name: "alert level counters",
			responses: map[string]struct {
				raw string
				err error
			}{
				"system.info":  {raw: `{}`},
				"system.state": {raw: `"READY"`},
				"pool.query":   {raw: `[{"healthy":true,"status":"ONLINE"}]`},
				"disk.query":   {raw: `[]`},
				"alert.list": {raw: `[
  {"level":"CRITICAL"},
  {"level":"warning"},
  {"level":"WARN"},
  {"level":"INFO"},
  {},
  42
]`},
			},
			want: `{
  "details": {
    "alerts": [
      {
        "level": "CRITICAL"
      },
      {
        "level": "warning"
      },
      {
        "level": "WARN"
      },
      {
        "level": "INFO"
      },
      {},
      42
    ],
    "disks": [],
    "pools": [
      {
        "healthy": true,
        "status": "ONLINE"
      }
    ],
    "system": {},
    "system_state": "READY"
  },
  "errors": {},
  "summary": {
    "alerts_critical": 1,
    "alerts_warning": 2,
    "failed_sections": 0,
    "pools_unhealthy": 0,
    "status": "critical"
  }
}`,
		},
		{
			name: "transport and parse failures",
			responses: map[string]struct {
				raw string
				err error
			}{
				"system.info":  {raw: `{}`},
				"system.state": {raw: `"READY"`},
				"pool.query":   {raw: `{bad`},
				"disk.query":   {err: fmt.Errorf("disk unavailable")},
				"alert.list":   {raw: `[]`},
			},
			want: `{
  "details": {
    "alerts": [],
    "system": {},
    "system_state": "READY"
  },
  "errors": {
    "disks": "disk unavailable",
    "pools": "invalid character 'b' looking for beginning of object key string"
  },
  "summary": {
    "alerts_critical": 0,
    "alerts_warning": 0,
    "failed_sections": 2,
    "pools_unhealthy": 0,
    "status": "critical"
  }
}`,
		},
		{
			name: "warnings only",
			responses: map[string]struct {
				raw string
				err error
			}{
				"system.info":  {raw: `{}`},
				"system.state": {raw: `"READY"`},
				"pool.query":   {raw: `[{"healthy":true,"status":"ONLINE"}]`},
				"disk.query":   {raw: `[]`},
				"alert.list":   {raw: `[{"level":"WARNING"},{"level":"warn"}]`},
			},
			want: `{
  "details": {
    "alerts": [
      {
        "level": "WARNING"
      },
      {
        "level": "warn"
      }
    ],
    "disks": [],
    "pools": [
      {
        "healthy": true,
        "status": "ONLINE"
      }
    ],
    "system": {},
    "system_state": "READY"
  },
  "errors": {},
  "summary": {
    "alerts_critical": 0,
    "alerts_warning": 2,
    "failed_sections": 0,
    "pools_unhealthy": 0,
    "status": "warning"
  }
}`,
		},
		{
			name: "pools and alerts are not lists",
			responses: map[string]struct {
				raw string
				err error
			}{
				"system.info":  {raw: `{}`},
				"system.state": {raw: `"READY"`},
				"pool.query":   {raw: `{"healthy":false,"status":"DEGRADED"}`},
				"disk.query":   {raw: `[]`},
				"alert.list":   {raw: `{"level":"CRITICAL"}`},
			},
			want: `{
  "details": {
    "alerts": {
      "level": "CRITICAL"
    },
    "disks": [],
    "pools": {
      "healthy": false,
      "status": "DEGRADED"
    },
    "system": {},
    "system_state": "READY"
  },
  "errors": {},
  "summary": {
    "alerts_critical": 0,
    "alerts_warning": 0,
    "failed_sections": 0,
    "pools_unhealthy": 0,
    "status": "ok"
  }
}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockCaller{CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
				response, ok := tt.responses[method]
				if !ok {
					t.Fatalf("unexpected method %q", method)
				}
				if response.err != nil {
					return nil, response.err
				}
				return json.RawMessage(response.raw), nil
			}}

			result, err := callTool(t, mock, true, "truenas_health_report", nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := resultText(t, result); got != tt.want {
				t.Errorf("result text mismatch\ngot:\n%s\nwant:\n%s", got, tt.want)
			}
		})
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
