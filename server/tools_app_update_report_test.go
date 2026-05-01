package server

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAppsUpdateReport_Success(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "app.query" {
				t.Fatalf("method = %q, want app.query", method)
			}
			return json.RawMessage(`[
				{"name":"plex","state":"RUNNING","version":"1.0.0","human_version":"1.0.0","latest_version":"1.1.0","upgrade_available":true,"image_updates_available":false},
				{"name":"home-assistant","state":"RUNNING","version":"2.0.0","upgrade_available":false,"image_updates_available":true},
				{"name":"syncthing","state":"RUNNING","version":"3.0.0","upgrade_available":false,"image_updates_available":false}
			]`), nil
		},
	}

	result, err := callTool(t, mock, true, "truenas_apps_update_report", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report map[string]any
	if err := json.Unmarshal([]byte(resultText(t, result)), &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	summary := report["summary"].(map[string]any)
	if summary["apps_total"] != float64(3) {
		t.Errorf("apps_total = %v, want 3", summary["apps_total"])
	}
	if summary["updates_available"] != float64(2) {
		t.Errorf("updates_available = %v, want 2", summary["updates_available"])
	}
	apps := report["apps"].([]any)
	if len(apps) != 2 {
		t.Fatalf("apps len = %d, want 2", len(apps))
	}
}

func TestAppsUpdateReport_NoMutatingCalls(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if strings.Contains(method, "upgrade") || strings.Contains(method, "update") || strings.Contains(method, "refresh") {
				t.Fatalf("unexpected mutating or noisy method %q", method)
			}
			return json.RawMessage(`[]`), nil
		},
	}

	_, err := callTool(t, mock, true, "truenas_apps_update_report", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
