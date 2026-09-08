package server

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

const configureCurrent = `{
  "TZ": "America/New_York",
  "ix_context": {"app_metadata": {"name": "it-tools"}},
  "ix_volumes": {},
  "network": {"web_port": {"bind_mode": "published", "port_number": 30990, "host_ips": []}, "host_network": false, "networks": []},
  "resources": {"limits": {"cpus": 2, "memory": 4096}},
  "it_tools": {"additional_envs": [{"name": "A", "value": "1"}]},
  "labels": []
}`

// configureMock answers app.query and app.config for a catalog app and records
// the app.update payload.
func configureMock(t *testing.T, custom bool) (*mockCaller, *[]any) {
	t.Helper()
	var updates []any
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			switch method {
			case "app.query":
				return json.RawMessage(fmt.Sprintf(`[{"name":"it-tools","custom_app":%t}]`, custom)), nil
			case "app.config":
				if len(params) != 1 || params[0] != "it-tools" {
					t.Errorf("app.config params = %v", params)
				}
				return json.RawMessage(configureCurrent), nil
			case "app.update":
				if len(params) != 2 || params[0] != "it-tools" {
					t.Errorf("app.update params = %v", params)
				}
				updates = append(updates, params[1])
				return json.RawMessage(`4242`), nil
			}
			t.Errorf("unexpected method %q", method)
			return nil, fmt.Errorf("unexpected method %q", method)
		},
	}
	return mock, &updates
}

func TestAppConfigure_DeepMergesAndStripsReserved(t *testing.T) {
	mock, updates := configureMock(t, false)
	result, err := callTool(t, mock, false, "truenas_app_configure", map[string]any{
		"name": "it-tools",
		"values": map[string]any{
			"network":   map[string]any{"web_port": map[string]any{"port_number": 30991}},
			"resources": map[string]any{"limits": map[string]any{"cpus": 1}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", resultText(t, result))
	}
	if len(*updates) != 1 {
		t.Fatalf("app.update called %d times, want 1", len(*updates))
	}
	payload := (*updates)[0].(map[string]any)
	values := payload["values"].(map[string]any)

	// Siblings of the changed leaves must survive: this is the whole point.
	web := values["network"].(map[string]any)["web_port"].(map[string]any)
	if web["port_number"] != float64(30991) || web["bind_mode"] != "published" {
		t.Errorf("web_port merged wrong: %v", web)
	}
	if values["network"].(map[string]any)["host_network"] != false {
		t.Errorf("network.host_network lost: %v", values["network"])
	}
	limits := values["resources"].(map[string]any)["limits"].(map[string]any)
	if limits["cpus"] != float64(1) || limits["memory"] != float64(4096) {
		t.Errorf("resources.limits merged wrong: %v", limits)
	}
	if values["TZ"] != "America/New_York" {
		t.Errorf("untouched top-level key lost: %v", values["TZ"])
	}
	for _, k := range []string{"ix_context", "ix_volumes"} {
		if _, present := values[k]; present {
			t.Errorf("reserved key %q must be stripped from payload", k)
		}
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(resultText(t, result)), &got); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if got["job_id"] != float64(4242) {
		t.Errorf("job_id = %v, want 4242", got["job_id"])
	}
	wantChanged := []any{"network.web_port.port_number", "resources.limits.cpus"}
	if !reflect.DeepEqual(got["changed_paths"], wantChanged) {
		t.Errorf("changed_paths = %v, want %v", got["changed_paths"], wantChanged)
	}
}

func TestAppConfigure_ListsReplaceWhole(t *testing.T) {
	mock, updates := configureMock(t, false)
	_, err := callTool(t, mock, false, "truenas_app_configure", map[string]any{
		"name":   "it-tools",
		"values": map[string]any{"it_tools": map[string]any{"additional_envs": []any{}}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	values := (*updates)[0].(map[string]any)["values"].(map[string]any)
	envs := values["it_tools"].(map[string]any)["additional_envs"].([]any)
	if len(envs) != 0 {
		t.Errorf("list should be replaced, got %v", envs)
	}
}

func TestAppConfigure_DryRunDoesNotUpdate(t *testing.T) {
	mock, updates := configureMock(t, false)
	result, err := callTool(t, mock, false, "truenas_app_configure", map[string]any{
		"name":    "it-tools",
		"dry_run": true,
		"values":  map[string]any{"resources": map[string]any{"limits": map[string]any{"memory": 512}}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(*updates) != 0 {
		t.Fatalf("dry_run must not call app.update, got %d calls", len(*updates))
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(resultText(t, result)), &got); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if got["dry_run"] != true || got["job_id"] != nil {
		t.Errorf("dry_run result shape wrong: %v", got)
	}
	if !reflect.DeepEqual(got["changed_paths"], []any{"resources.limits.memory"}) {
		t.Errorf("changed_paths = %v", got["changed_paths"])
	}
}

func TestAppConfigure_Refusals(t *testing.T) {
	cases := []struct {
		name   string
		custom bool
		args   map[string]any
		want   string
	}{
		{"missing values", false, map[string]any{"name": "it-tools"}, `"values"`},
		{"empty values", false, map[string]any{"name": "it-tools", "values": map[string]any{}}, "at least one key"},
		{"unknown section", false, map[string]any{"name": "it-tools", "values": map[string]any{"netwrk": map[string]any{}}}, `unknown top-level configuration section "netwrk"`},
		{"reserved section", false, map[string]any{"name": "it-tools", "values": map[string]any{"ix_volumes": map[string]any{}}}, "managed by TrueNAS"},
		{"no-op", false, map[string]any{"name": "it-tools", "values": map[string]any{"TZ": "America/New_York"}}, "identical"},
		{"custom app", true, map[string]any{"name": "it-tools", "values": map[string]any{"TZ": "UTC"}}, "custom compose app"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock, updates := configureMock(t, tc.custom)
			result, err := callTool(t, mock, false, "truenas_app_configure", tc.args)
			if err == nil && (result == nil || !result.IsError) {
				t.Fatalf("expected refusal")
			}
			if len(*updates) != 0 {
				t.Errorf("refusal must not call app.update")
			}
			msg := ""
			if err != nil {
				msg = err.Error()
			} else {
				msg = resultText(t, result)
			}
			if !strings.Contains(msg, tc.want) {
				t.Errorf("error %q does not contain %q", msg, tc.want)
			}
		})
	}
}

func TestAppConfigure_NotFound(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "app.query" {
				t.Errorf("unexpected method %q", method)
			}
			return json.RawMessage(`[]`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_app_configure", map[string]any{
		"name": "nope", "values": map[string]any{"TZ": "UTC"},
	})
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected not-found error")
	}
}

func TestAppConfigure_NotRegisteredReadOnly(t *testing.T) {
	mock, updates := configureMock(t, false)
	result, err := callTool(t, mock, true, "truenas_app_configure", map[string]any{
		"name": "it-tools", "values": map[string]any{"TZ": "UTC"},
	})
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("configure must be unavailable in read-only mode")
	}
	if len(*updates) != 0 {
		t.Error("read-only mode must never reach app.update")
	}
}
