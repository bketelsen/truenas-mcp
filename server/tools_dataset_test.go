package server

import (
	"encoding/json"
	"testing"
)

func TestDatasetList_NoFilter(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "pool.dataset.query" {
				t.Errorf("method = %q, want pool.dataset.query", method)
			}
			if len(params) != 0 {
				t.Errorf("expected no params, got %d", len(params))
			}
			return json.RawMessage(`[{"id":"tank/data"}]`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_dataset_list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if text == "" {
		t.Error("result text is empty")
	}
}

func TestDatasetList_WithPool(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "pool.dataset.query" {
				t.Errorf("method = %q, want pool.dataset.query", method)
			}
			if len(params) == 0 {
				t.Fatal("expected filter params, got none")
			}
			filter, ok := params[0].([][]any)
			if !ok {
				t.Fatalf("params[0] is %T, want [][]any", params[0])
			}
			if len(filter) != 1 || filter[0][0] != "pool" || filter[0][1] != "=" || filter[0][2] != "tank" {
				t.Errorf("filter = %v, want [[pool = tank]]", filter)
			}
			return json.RawMessage(`[{"id":"tank/data"}]`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_dataset_list", map[string]any{"pool": "tank"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if text == "" {
		t.Error("result text is empty")
	}
}

func TestDatasetGet_Success(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if len(params) == 0 {
				t.Fatal("expected filter params")
			}
			filter, ok := params[0].([][]any)
			if !ok {
				t.Fatalf("params[0] is %T, want [][]any", params[0])
			}
			if filter[0][0] != "id" || filter[0][2] != "tank/data" {
				t.Errorf("filter = %v, want [[id = tank/data]]", filter)
			}
			return json.RawMessage(`{"id":"tank/data"}`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_dataset_get", map[string]any{"path": "tank/data"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if text == "" {
		t.Error("result text is empty")
	}
}

func TestDatasetGet_MissingPath(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			t.Fatal("Call should not be invoked")
			return nil, nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_dataset_get", nil)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for missing path")
	}
}

func TestDatasetCreate_AllParams(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "pool.dataset.create" {
				t.Errorf("method = %q, want pool.dataset.create", method)
			}
			if len(params) == 0 {
				t.Fatal("expected params")
			}
			p, ok := params[0].(map[string]any)
			if !ok {
				t.Fatalf("params[0] is %T, want map[string]any", params[0])
			}
			if p["name"] != "tank/newdata" {
				t.Errorf("name = %v, want tank/newdata", p["name"])
			}
			if p["comments"] != "test dataset" {
				t.Errorf("comments = %v, want 'test dataset'", p["comments"])
			}
			if p["compression"] != "lz4" {
				t.Errorf("compression = %v, want lz4", p["compression"])
			}
			return json.RawMessage(`{"id":"tank/newdata"}`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_dataset_create", map[string]any{
		"name":        "tank/newdata",
		"comments":    "test dataset",
		"compression": "lz4",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if text == "" {
		t.Error("result text is empty")
	}
}

func TestDatasetCreate_RequiredOnly(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			p := params[0].(map[string]any)
			if p["name"] != "tank/newdata" {
				t.Errorf("name = %v, want tank/newdata", p["name"])
			}
			if _, ok := p["comments"]; ok {
				t.Error("unexpected comments key")
			}
			if _, ok := p["compression"]; ok {
				t.Error("unexpected compression key")
			}
			return json.RawMessage(`{"id":"tank/newdata"}`), nil
		},
	}
	_, err := callTool(t, mock, false, "truenas_dataset_create", map[string]any{"name": "tank/newdata"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDatasetCreate_MissingName(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			t.Fatal("Call should not be invoked")
			return nil, nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_dataset_create", nil)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for missing name")
	}
}

func TestDatasetDelete_Success(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "pool.dataset.delete" {
				t.Errorf("method = %q, want pool.dataset.delete", method)
			}
			if len(params) == 0 || params[0] != "tank/olddata" {
				t.Errorf("params = %v, want [tank/olddata]", params)
			}
			return json.RawMessage(`true`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_dataset_delete", map[string]any{"path": "tank/olddata"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if text == "" {
		t.Error("result text is empty")
	}
}

func TestDatasetDelete_MissingPath(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			t.Fatal("Call should not be invoked")
			return nil, nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_dataset_delete", nil)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for missing path")
	}
}
