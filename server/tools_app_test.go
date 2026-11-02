package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestAppList_CompactSortedSummary(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "app.query" {
				t.Errorf("method = %q, want app.query", method)
			}
			if len(params) != 0 {
				t.Errorf("expected no params, got %d: %v", len(params), params)
			}
			return json.RawMessage(`[
				{
					"name":"syncthing",
					"id":"syncthing",
					"state":"RUNNING",
					"version":"1.27.0",
					"human_version":"1.27.0_1.0.0",
					"latest_version":"1.27.1",
					"latest_app_version":"1.0.1",
					"upgrade_available":true,
					"image_updates_available":false,
					"custom_app":false,
					"migrated":false,
					"action_required":null,
					"portals":{"web_portal":{"host":"0.0.0.0","port":8384}},
					"notes":"## Syncthing notes...",
					"metadata":{"train":"community","app_version":"1.0.0"},
					"active_workloads":{
						"containers":1,
						"container_details":[{"service_name":"syncthing","image":"syncthing/syncthing:1.27.0","state":"running"}],
						"networks":[{"name":"ix-syncthing_default","id":"abc123","subnet":"172.16.0.0/16","gateway":"172.16.0.1"}]
					}
				},
				{
					"name":"home-assistant",
					"id":"home-assistant",
					"state":"RUNNING",
					"version":"2025.1.0",
					"upgrade_available":false,
					"image_updates_available":true,
					"custom_app":false,
					"portals":{},
					"notes":"## Home Assistant notes...",
					"metadata":{"train":"stable"},
					"active_workloads":{"containers":1,"container_details":[{"service_name":"home-assistant"}],"networks":[{"name":"ix-home-assistant_default"}]}
				},
				{
					"name":"adguard-home",
					"id":"adguard-home",
					"state":"STOPPED",
					"version":"0.107.0",
					"upgrade_available":false,
					"image_updates_available":false,
					"custom_app":true,
					"portals":{"web_portal":{"port":3000}},
					"notes":"## AdGuard notes...",
					"metadata":{"train":"community"},
					"active_workloads":{"containers":0,"container_details":[],"networks":[]}
				}
			]`), nil
		},
	}

	result, err := callTool(t, mock, false, "truenas_app_list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := resultText(t, result)
	for _, dropped := range []string{"notes", "metadata", "active_workloads", "networks"} {
		if strings.Contains(text, `"`+dropped+`"`) {
			t.Errorf("result still contains %q: %s", dropped, text)
		}
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	summary, ok := payload["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary is %T, want object", payload["summary"])
	}
	if summary["apps_total"] != float64(3) {
		t.Errorf("apps_total = %v, want 3", summary["apps_total"])
	}

	apps, ok := payload["apps"].([]any)
	if !ok {
		t.Fatalf("apps is %T, want array", payload["apps"])
	}
	if len(apps) != 3 {
		t.Fatalf("apps len = %d, want 3", len(apps))
	}

	wantOrder := []string{"adguard-home", "home-assistant", "syncthing"}
	for i, want := range wantOrder {
		if got := apps[i].(map[string]any)["name"]; got != want {
			t.Errorf("apps[%d].name = %v, want %s", i, got, want)
		}
	}

	syncthing := apps[2].(map[string]any)
	wantKeys := map[string]any{
		"name":                    "syncthing",
		"id":                      "syncthing",
		"state":                   "RUNNING",
		"version":                 "1.27.0",
		"human_version":           "1.27.0_1.0.0",
		"latest_version":          "1.27.1",
		"latest_app_version":      "1.0.1",
		"upgrade_available":       true,
		"image_updates_available": false,
		"custom_app":              false,
		"migrated":                false,
		"action_required":         nil,
		"train":                   "community",
	}
	for key, want := range wantKeys {
		got, present := syncthing[key]
		if !present {
			t.Errorf("syncthing is missing key %q", key)
			continue
		}
		if got != want {
			t.Errorf("syncthing[%q] = %v, want %v", key, got, want)
		}
	}
	if len(syncthing) != len(wantKeys)+2 { // +portals +containers
		t.Errorf("syncthing has %d keys, want %d: %v", len(syncthing), len(wantKeys)+2, syncthing)
	}

	portals, ok := syncthing["portals"].(map[string]any)
	if !ok {
		t.Fatalf("portals is %T, want object", syncthing["portals"])
	}
	if web, ok := portals["web_portal"].(map[string]any); !ok || web["port"] != float64(8384) {
		t.Errorf("portals = %v, want web_portal port 8384", portals)
	}

	containers, ok := syncthing["containers"].(float64)
	if !ok {
		t.Fatalf("containers is %T, want the container count as a number", syncthing["containers"])
	}
	if containers != 1 {
		t.Errorf("containers = %v, want 1", containers)
	}
	if _, present := syncthing["container_details"]; present {
		t.Errorf("entry carries container_details, want it dropped: %v", syncthing)
	}

	adguard := apps[0].(map[string]any)
	if adguard["custom_app"] != true || adguard["state"] != "STOPPED" {
		t.Errorf("adguard-home = %v, want custom STOPPED fields copied", adguard)
	}
}

func TestAppList_StaysUnderClientCutoff(t *testing.T) {
	const appCount = 13
	var raw strings.Builder
	raw.WriteString("[")
	for i := 0; i < appCount; i++ {
		if i > 0 {
			raw.WriteString(",")
		}
		name := fmt.Sprintf("app-%02d", i)
		notes := strings.Repeat("long markdown notes about this app. ", 8000/37+1)
		fmt.Fprintf(&raw, `{"name":%q,"id":%q,"state":"RUNNING","version":"1.0.0","notes":%q,`+
			`"metadata":{"train":"stable"},`+
			`"active_workloads":{"containers":1,`+
			`"container_details":[{"service_name":%q,"image":"example/app:1.0.0","state":"running"}],`+
			`"networks":[{"name":"ix-%s_default","id":"netid","subnet":"172.16.0.0/16","gateway":"172.16.0.1"}]},`+
			`"portals":{"web_portal":{"host":"0.0.0.0","port":8080}}}`, name, name, notes, name, name)
	}
	raw.WriteString("]")
	payload := json.RawMessage(raw.String())
	if len(payload) < 100000 {
		t.Fatalf("regression input is only %d characters, want at least 100000 like the live NAS", len(payload))
	}

	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "app.query" {
				t.Errorf("method = %q, want app.query", method)
			}
			if len(params) != 0 {
				t.Errorf("expected no params, got %d", len(params))
			}
			return payload, nil
		},
	}

	result, err := callTool(t, mock, false, "truenas_app_list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if len(text) >= 40000 {
		t.Errorf("result is %d characters, want under the 40000 character client cutoff", len(text))
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	summary := got["summary"].(map[string]any)
	if summary["apps_total"] != float64(appCount) {
		t.Errorf("apps_total = %v, want %d", summary["apps_total"], appCount)
	}
	apps := got["apps"].([]any)
	if len(apps) != appCount {
		t.Fatalf("apps len = %d, want %d", len(apps), appCount)
	}
	for i := 0; i < appCount; i++ {
		want := fmt.Sprintf("app-%02d", i)
		if apps[i].(map[string]any)["name"] != want {
			t.Errorf("apps[%d].name = %v, want %s", i, apps[i].(map[string]any)["name"], want)
		}
	}
}

func TestAppList_MissingFieldsAreNil(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			return json.RawMessage(`[{"name":"bare"}]`), nil
		},
	}

	result, err := callTool(t, mock, false, "truenas_app_list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(resultText(t, result)), &payload); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	apps := payload["apps"].([]any)
	if len(apps) != 1 {
		t.Fatalf("apps len = %d, want 1", len(apps))
	}
	app := apps[0].(map[string]any)
	if app["name"] != "bare" {
		t.Errorf("name = %v, want bare", app["name"])
	}
	for _, key := range []string{
		"id", "state", "version", "human_version", "latest_version", "latest_app_version",
		"upgrade_available", "image_updates_available", "custom_app", "migrated",
		"action_required", "portals", "train", "containers",
	} {
		got, present := app[key]
		if !present {
			t.Errorf("missing key %q", key)
			continue
		}
		if got != nil {
			t.Errorf("app[%q] = %v, want nil", key, got)
		}
	}
}

func TestAppList_EmptyListReturnsEmptyArray(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			return json.RawMessage(`[]`), nil
		},
	}

	result, err := callTool(t, mock, false, "truenas_app_list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, `"apps": []`) {
		t.Errorf("result should carry an empty apps array: %s", text)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	summary := payload["summary"].(map[string]any)
	if summary["apps_total"] != float64(0) {
		t.Errorf("apps_total = %v, want 0", summary["apps_total"])
	}
}

func TestAppList_APIError(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			return nil, fmt.Errorf("[ECONNREFUSED] transport is gone")
		},
	}

	result, err := callTool(t, mock, false, "truenas_app_list", nil)
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error from app.query")
	}
	if result != nil && result.IsError && !strings.Contains(resultText(t, result), "app.query") {
		t.Errorf("error should name the API call: %s", resultText(t, result))
	}
}

func TestAppList_InvalidJSON(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			return json.RawMessage(`{"not":"an array"}`), nil
		},
	}

	result, err := callTool(t, mock, false, "truenas_app_list", nil)
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error for a non-array app.query payload")
	}
	if result != nil && result.IsError && !strings.Contains(resultText(t, result), "parsing app.query") {
		t.Errorf("error should name the parse step: %s", resultText(t, result))
	}
}

func TestAppGet_Success(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "app.query" {
				t.Errorf("method = %q, want app.query", method)
			}
			if len(params) == 0 {
				t.Fatal("expected filter params")
			}
			filter, ok := params[0].([][]any)
			if !ok {
				t.Fatalf("params[0] is %T, want [][]any", params[0])
			}
			if filter[0][0] != "name" || filter[0][2] != "plex" {
				t.Errorf("filter = %v, want [[name = plex]]", filter)
			}
			return json.RawMessage(`[{"name":"plex","status":"running"}]`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_app_get", map[string]any{"name": "plex"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if text == "" {
		t.Error("result text is empty")
	}
}

func TestAppGet_MissingName(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			t.Fatal("Call should not be invoked")
			return nil, nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_app_get", nil)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for missing name")
	}
}

func TestAppConfig_Success(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "app.config" {
				t.Errorf("method = %q, want app.config", method)
			}
			if len(params) != 1 || params[0] != "plex" {
				t.Errorf("params = %v, want [plex]", params)
			}
			return json.RawMessage(`{"network":{"web_port":32400},"plex":{"claim_token":"claim-secret"}}`), nil
		},
	}
	result, err := callTool(t, mock, true, "truenas_app_config", map[string]any{"name": "plex"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(resultText(t, result)), &got); err != nil {
		t.Fatalf("result is not JSON: %v", err)
	}
	if got["name"] != "plex" {
		t.Errorf("name = %v, want plex", got["name"])
	}
	config, ok := got["config"].(map[string]any)
	if !ok {
		t.Fatalf("config is %T, want object", got["config"])
	}
	if config["plex"].(map[string]any)["claim_token"] != "claim-secret" {
		t.Errorf("config not passed through verbatim: %v", config)
	}
}

func TestAppConfig_MissingName(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			t.Fatal("Call should not be invoked")
			return nil, nil
		},
	}
	result, err := callTool(t, mock, true, "truenas_app_config", nil)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for missing name")
	}
}

func TestAppConfig_APIError(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			return nil, fmt.Errorf("[ENOENT] app not found")
		},
	}
	result, err := callTool(t, mock, true, "truenas_app_config", map[string]any{"name": "nope"})
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error from app.config")
	}
	if result != nil && result.IsError && !strings.Contains(resultText(t, result), "app.config") {
		t.Errorf("error should name the API call: %s", resultText(t, result))
	}
}

func TestAppStart_Success(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "app.start" {
				t.Errorf("method = %q, want app.start", method)
			}
			if len(params) == 0 || params[0] != "plex" {
				t.Errorf("params = %v, want [plex]", params)
			}
			return json.RawMessage(`null`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_app_start", map[string]any{"name": "plex"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if text == "" {
		t.Error("result text is empty")
	}
}

func TestAppStart_MissingName(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			t.Fatal("Call should not be invoked")
			return nil, nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_app_start", nil)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for missing name")
	}
}

func TestAppStop_Success(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "app.stop" {
				t.Errorf("method = %q, want app.stop", method)
			}
			if len(params) == 0 || params[0] != "plex" {
				t.Errorf("params = %v, want [plex]", params)
			}
			return json.RawMessage(`null`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_app_stop", map[string]any{"name": "plex"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if text == "" {
		t.Error("result text is empty")
	}
}

func TestAppStop_MissingName(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			t.Fatal("Call should not be invoked")
			return nil, nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_app_stop", nil)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for missing name")
	}
}

func TestAppRestart_UsesRedeployAndReturnsJobID(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			// app.restart does not exist in TrueNAS SCALE; the restart primitive is app.redeploy.
			if method != "app.redeploy" {
				t.Errorf("method = %q, want app.redeploy", method)
			}
			if len(params) != 1 || params[0] != "plex" {
				t.Errorf("params = %v, want [plex]", params)
			}
			return json.RawMessage(`15122`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_app_restart", map[string]any{"name": "plex"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(resultText(t, result)), &got); err != nil {
		t.Fatalf("result is not JSON: %v", err)
	}
	if got["name"] != "plex" || got["job_id"] != float64(15122) {
		t.Errorf("result = %v, want name=plex job_id=15122", got)
	}
}

func TestAppRestart_APIError(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			return nil, fmt.Errorf("[ENOENT] App nope does not exist")
		},
	}
	result, err := callTool(t, mock, false, "truenas_app_restart", map[string]any{"name": "nope"})
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error from app.redeploy")
	}
	if result != nil && result.IsError && !strings.Contains(resultText(t, result), "app.redeploy") {
		t.Errorf("error should name the API call: %s", resultText(t, result))
	}
}

func TestAppRestart_MissingName(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			t.Fatal("Call should not be invoked")
			return nil, nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_app_restart", nil)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for missing name")
	}
}

func TestAppUpdate_SuccessReturnsJobID(t *testing.T) {
	calls := []string{}
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			calls = append(calls, method)
			switch method {
			case "app.query":
				if len(params) != 1 {
					t.Fatalf("app.query params = %v, want name filter", params)
				}
				filter, ok := params[0].([][]any)
				if !ok {
					t.Fatalf("app.query params[0] is %T, want [][]any", params[0])
				}
				if len(filter) != 1 || filter[0][0] != "name" || filter[0][1] != "=" || filter[0][2] != "plex" {
					t.Fatalf("app.query filter = %v, want [[name = plex]]", filter)
				}
				return json.RawMessage(`[{"name":"plex","upgrade_available":true}]`), nil
			case "app.upgrade":
				if len(params) != 2 || params[0] != "plex" {
					t.Fatalf("app.upgrade params = %v, want [plex options]", params)
				}
				options, ok := params[1].(map[string]any)
				if !ok {
					t.Fatalf("app.upgrade options is %T, want map[string]any", params[1])
				}
				if options["app_version"] != "latest" {
					t.Fatalf("app_version = %v, want latest", options["app_version"])
				}
				if options["snapshot_hostpaths"] != true {
					// Errorf, not Fatalf: this runs on the server's handler goroutine.
					t.Errorf("snapshot_hostpaths = %v, want true so host-path data is snapshotted", options["snapshot_hostpaths"])
					return nil, fmt.Errorf("snapshot_hostpaths not true")
				}
				return json.RawMessage(`123`), nil
			default:
				t.Fatalf("unexpected method %q", method)
			}
			return nil, nil
		},
	}

	result, err := callTool(t, mock, false, "truenas_app_update", map[string]any{"name": "plex"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(resultText(t, result)), &payload); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if payload["name"] != "plex" {
		t.Errorf("name = %v, want plex", payload["name"])
	}
	if payload["job_id"] != float64(123) {
		t.Errorf("job_id = %v, want 123", payload["job_id"])
	}
	if fmt.Sprint(calls) != "[app.query app.upgrade]" {
		t.Errorf("calls = %v, want [app.query app.upgrade]", calls)
	}
}

func TestAppUpdate_NoUpdateAvailableReturnsError(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "app.query" {
				t.Fatalf("method = %q, want app.query", method)
			}
			return json.RawMessage(`[{"name":"plex","upgrade_available":false}]`), nil
		},
	}

	result, err := callTool(t, mock, false, "truenas_app_update", map[string]any{"name": "plex"})
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error for app with no update available")
	}
}

func TestAppUpdate_MissingName(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			t.Fatal("Call should not be invoked")
			return nil, nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_app_update", nil)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for missing name")
	}
}

func TestAppUpdateAll_SuccessReturnsJobIDs(t *testing.T) {
	calls := []string{}
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			calls = append(calls, method)
			switch method {
			case "app.query":
				if len(params) != 1 {
					t.Fatalf("app.query params = %v, want upgrade_available filter", params)
				}
				filter, ok := params[0].([][]any)
				if !ok {
					t.Fatalf("app.query params[0] is %T, want [][]any", params[0])
				}
				if len(filter) != 1 || filter[0][0] != "upgrade_available" || filter[0][1] != "=" || filter[0][2] != true {
					t.Fatalf("app.query filter = %v, want [[upgrade_available = true]]", filter)
				}
				return json.RawMessage(`[
					{"name":"plex","upgrade_available":true},
					{"name":"syncthing","upgrade_available":true}
				]`), nil
			case "app.upgrade":
				if len(params) != 2 {
					t.Fatalf("app.upgrade params = %v, want [name options]", params)
				}
				if options, ok := params[1].(map[string]any); !ok || options["snapshot_hostpaths"] != true {
					t.Errorf("app.upgrade options = %v, want snapshot_hostpaths true", params[1])
					return nil, fmt.Errorf("snapshot_hostpaths not true")
				}
				switch params[0] {
				case "plex":
					return json.RawMessage(`201`), nil
				case "syncthing":
					return json.RawMessage(`202`), nil
				default:
					t.Fatalf("unexpected app.upgrade app %v", params[0])
				}
			default:
				t.Fatalf("unexpected method %q", method)
			}
			return nil, nil
		},
	}

	result, err := callTool(t, mock, false, "truenas_app_update_all", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(resultText(t, result)), &payload); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	jobs := payload["jobs"].([]any)
	if len(jobs) != 2 {
		t.Fatalf("jobs len = %d, want 2", len(jobs))
	}
	if jobs[0].(map[string]any)["job_id"] != float64(201) {
		t.Errorf("first job_id = %v, want 201", jobs[0].(map[string]any)["job_id"])
	}
	if jobs[1].(map[string]any)["job_id"] != float64(202) {
		t.Errorf("second job_id = %v, want 202", jobs[1].(map[string]any)["job_id"])
	}
	if fmt.Sprint(calls) != "[app.query app.upgrade app.upgrade]" {
		t.Errorf("calls = %v, want [app.query app.upgrade app.upgrade]", calls)
	}
}

func TestAppUpdateAll_PartialSuccessReturnsStartedJobIDsAndFailures(t *testing.T) {
	calls := []string{}
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			calls = append(calls, method)
			switch method {
			case "app.query":
				return json.RawMessage(`[
					{"name":"plex","upgrade_available":true},
					{"name":"broken","upgrade_available":true},
					{"name":"syncthing","upgrade_available":true}
				]`), nil
			case "app.upgrade":
				if len(params) != 2 {
					t.Fatalf("app.upgrade params = %v, want [name options]", params)
				}
				if options, ok := params[1].(map[string]any); !ok || options["snapshot_hostpaths"] != true {
					t.Errorf("app.upgrade options = %v, want snapshot_hostpaths true", params[1])
					return nil, fmt.Errorf("snapshot_hostpaths not true")
				}
				switch params[0] {
				case "plex":
					return json.RawMessage(`201`), nil
				case "broken":
					return nil, fmt.Errorf("upgrade unavailable")
				case "syncthing":
					return json.RawMessage(`202`), nil
				default:
					t.Fatalf("unexpected app.upgrade app %v", params[0])
				}
			default:
				t.Fatalf("unexpected method %q", method)
			}
			return nil, nil
		},
	}

	result, err := callTool(t, mock, false, "truenas_app_update_all", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(resultText(t, result)), &payload); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	jobs := payload["jobs"].([]any)
	if len(jobs) != 2 {
		t.Fatalf("jobs len = %d, want 2", len(jobs))
	}
	if jobs[0].(map[string]any)["name"] != "plex" || jobs[0].(map[string]any)["job_id"] != float64(201) {
		t.Errorf("first job = %v, want plex job 201", jobs[0])
	}
	if jobs[1].(map[string]any)["name"] != "syncthing" || jobs[1].(map[string]any)["job_id"] != float64(202) {
		t.Errorf("second job = %v, want syncthing job 202", jobs[1])
	}

	summary := payload["summary"].(map[string]any)
	if summary["jobs_started"] != float64(2) {
		t.Errorf("jobs_started = %v, want 2", summary["jobs_started"])
	}
	if summary["failures"] != float64(1) {
		t.Errorf("failures = %v, want 1", summary["failures"])
	}

	failures := payload["failures"].([]any)
	if len(failures) != 1 {
		t.Fatalf("failures len = %d, want 1", len(failures))
	}
	failure := failures[0].(map[string]any)
	if failure["name"] != "broken" {
		t.Errorf("failure name = %v, want broken", failure["name"])
	}
	errorText, ok := failure["error"].(string)
	if !ok || !strings.Contains(errorText, "app.upgrade: upgrade unavailable") {
		t.Errorf("failure error = %v, want app.upgrade context", failure["error"])
	}
	if fmt.Sprint(calls) != "[app.query app.upgrade app.upgrade app.upgrade]" {
		t.Errorf("calls = %v, want [app.query app.upgrade app.upgrade app.upgrade]", calls)
	}
}
