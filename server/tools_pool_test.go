package server

import (
	"encoding/json"
	"testing"
)

func TestPoolList_Success(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "pool.query" {
				t.Errorf("method = %q, want pool.query", method)
			}
			if len(params) != 0 {
				t.Errorf("expected no params, got %d", len(params))
			}
			return json.RawMessage(`[{"name":"tank","status":"ONLINE"}]`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_pool_list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if text == "" {
		t.Error("result text is empty")
	}
}

func TestPoolGet_Success(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "pool.query" {
				t.Errorf("method = %q, want pool.query", method)
			}
			if len(params) == 0 {
				t.Fatal("expected filter params, got none")
			}
			filter, ok := params[0].([][]any)
			if !ok {
				t.Fatalf("params[0] is %T, want [][]any", params[0])
			}
			if len(filter) != 1 || filter[0][0] != "name" || filter[0][1] != "=" || filter[0][2] != "tank" {
				t.Errorf("filter = %v, want [[name = tank]]", filter)
			}
			return json.RawMessage(`[{"name":"tank","status":"ONLINE"}]`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_pool_get", map[string]any{"name": "tank"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if text == "" {
		t.Error("result text is empty")
	}
}

func TestPoolGet_MissingName(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			t.Fatal("Call should not be invoked for missing param")
			return nil, nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_pool_get", nil)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for missing name parameter")
	}
}
