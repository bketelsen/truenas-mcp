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
