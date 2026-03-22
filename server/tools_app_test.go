package server

import (
	"encoding/json"
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
