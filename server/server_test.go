package server

import (
	"encoding/json"
	"testing"
)

func TestNew_ReadOnly(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			return json.RawMessage(`{}`), nil
		},
	}
	s := New(mock, true)
	if s == nil {
		t.Fatal("New returned nil")
	}
}

func TestNew_ReadWrite(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			return json.RawMessage(`{}`), nil
		},
	}
	s := New(mock, false)
	if s == nil {
		t.Fatal("New returned nil")
	}
}

func TestNew_ReadOnly_NoWriteTools(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			return json.RawMessage(`{}`), nil
		},
	}
	// Calling a write tool on a read-only server should fail
	_, err := callTool(t, mock, true, "truenas_dataset_create", map[string]any{"name": "tank/test"})
	if err == nil {
		t.Error("expected error calling write tool on read-only server, got nil")
	}
}

func TestNew_ReadOnly_ToolListContainsNoWriteTools(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			return json.RawMessage(`{}`), nil
		},
	}

	writeTools := map[string]bool{
		"truenas_dataset_create":  true,
		"truenas_dataset_delete":  true,
		"truenas_snapshot_create": true,
		"truenas_snapshot_delete": true,
		"truenas_smb_create":      true,
		"truenas_smb_delete":      true,
		"truenas_nfs_create":      true,
		"truenas_nfs_delete":      true,
		"truenas_alert_dismiss":   true,
		"truenas_app_start":       true,
		"truenas_app_stop":        true,
		"truenas_app_restart":     true,
	}

	for _, name := range listTools(t, mock, true) {
		if writeTools[name] {
			t.Fatalf("read-only server registered write tool %q", name)
		}
	}
}
