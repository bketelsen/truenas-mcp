package server

import (
	"encoding/json"
	"testing"
)

func TestAlertList_NoFilter(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "alert.list" {
				t.Errorf("method = %q, want alert.list", method)
			}
			if len(params) != 0 {
				t.Errorf("expected no params, got %d", len(params))
			}
			return json.RawMessage(`[{"id":"abc","level":"INFO"},{"id":"def","level":"CRITICAL"}]`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_alert_list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var items []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(resultText(t, result)), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != "abc" || items[1].ID != "def" {
		t.Errorf("items = %v, want both alerts unchanged", items)
	}
}

func TestAlertList_LevelFilter(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "alert.list" {
				t.Errorf("method = %q, want alert.list", method)
			}
			// alert.list takes no arguments; the tool filters the result.
			if len(params) != 0 {
				t.Errorf("expected no params, got %d", len(params))
			}
			return json.RawMessage(`[
				{"id":"a","level":"INFO"},
				{"id":"b","level":"CRITICAL"},
				{"id":"c","level":"critical"},
				{"id":"d","level":"WARNING"}
			]`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_alert_list", map[string]any{"level": "critical"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var items []struct {
		ID    string `json:"id"`
		Level string `json:"level"`
	}
	if err := json.Unmarshal([]byte(resultText(t, result)), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != "b" || items[1].ID != "c" {
		t.Errorf("items = %v, want only the two critical alerts", items)
	}
}

func TestAlertList_LevelFilterNoMatch(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			return json.RawMessage(`[{"id":"a","level":"INFO"}]`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_alert_list", map[string]any{"level": "CRITICAL"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text := resultText(t, result); text != "[]" {
		t.Errorf("result = %q, want []", text)
	}
}

func TestAlertDismiss_Success(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "alert.dismiss" {
				t.Errorf("method = %q, want alert.dismiss", method)
			}
			if len(params) == 0 || params[0] != "alert-123" {
				t.Errorf("params = %v, want [alert-123]", params)
			}
			return json.RawMessage(`true`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_alert_dismiss", map[string]any{"id": "alert-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if text == "" {
		t.Error("result text is empty")
	}
}

func TestAlertDismiss_MissingID(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			t.Fatal("Call should not be invoked")
			return nil, nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_alert_dismiss", nil)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for missing id")
	}
}
