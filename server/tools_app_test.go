package server

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestAppList_Success(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "app.query" {
				t.Errorf("method = %q, want app.query", method)
			}
			if len(params) != 0 {
				t.Errorf("expected no params, got %d", len(params))
			}
			return json.RawMessage(`[{"name":"plex","status":"running"}]`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_app_list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if text == "" {
		t.Error("result text is empty")
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

func TestAppRestart_Success(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "app.restart" {
				t.Errorf("method = %q, want app.restart", method)
			}
			if len(params) == 0 || params[0] != "plex" {
				t.Errorf("params = %v, want [plex]", params)
			}
			return json.RawMessage(`null`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_app_restart", map[string]any{"name": "plex"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if text == "" {
		t.Error("result text is empty")
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
