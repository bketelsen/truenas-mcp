package server

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestSystemInfo_Success(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "system.info" {
				t.Errorf("method = %q, want system.info", method)
			}
			return json.RawMessage(`{"hostname":"nas","version":"24.04"}`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_system_info", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if text == "" {
		t.Error("result text is empty")
	}
}

func TestSystemInfo_Error(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}
	result, err := callTool(t, mock, false, "truenas_system_info", nil)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error, got success")
	}
}

func TestDiskList_Success(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "disk.query" {
				t.Errorf("method = %q, want disk.query", method)
			}
			return json.RawMessage(`[{"name":"sda","size":1000000}]`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_disk_list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if text == "" {
		t.Error("result text is empty")
	}
}

func TestNetworkList_Success(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "interface.query" {
				t.Errorf("method = %q, want interface.query", method)
			}
			return json.RawMessage(`[{"name":"eth0","state":{"link_state":"LINK_STATE_UP"}}]`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_network_list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if text == "" {
		t.Error("result text is empty")
	}
}
