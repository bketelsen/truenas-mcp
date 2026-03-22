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
			return json.RawMessage(`[{"id":"abc","level":"INFO"}]`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_alert_list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if text == "" {
		t.Error("result text is empty")
	}
}

func TestAlertList_WithLevel(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if len(params) == 0 {
				t.Fatal("expected filter params")
			}
			filter, ok := params[0].([][]any)
			if !ok {
				t.Fatalf("params[0] is %T, want [][]any", params[0])
			}
			if filter[0][0] != "level" || filter[0][2] != "CRITICAL" {
				t.Errorf("filter = %v, want [[level = CRITICAL]]", filter)
			}
			return json.RawMessage(`[{"id":"abc","level":"CRITICAL"}]`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_alert_list", map[string]any{"level": "CRITICAL"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if text == "" {
		t.Error("result text is empty")
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
