package server

import (
	"encoding/json"
	"testing"
)

func TestSMBList_Success(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "sharing.smb.query" {
				t.Errorf("method = %q, want sharing.smb.query", method)
			}
			return json.RawMessage(`[{"name":"share1","path":"/mnt/tank/data"}]`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_smb_list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if text == "" {
		t.Error("result text is empty")
	}
}

func TestNFSList_Success(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "sharing.nfs.query" {
				t.Errorf("method = %q, want sharing.nfs.query", method)
			}
			return json.RawMessage(`[{"path":"/mnt/tank/data"}]`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_nfs_list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if text == "" {
		t.Error("result text is empty")
	}
}

func TestSMBCreate_AllParams(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "sharing.smb.create" {
				t.Errorf("method = %q, want sharing.smb.create", method)
			}
			p := params[0].(map[string]any)
			if p["name"] != "myshare" {
				t.Errorf("name = %v, want myshare", p["name"])
			}
			if p["path"] != "/mnt/tank/data" {
				t.Errorf("path = %v, want /mnt/tank/data", p["path"])
			}
			if p["comment"] != "test share" {
				t.Errorf("comment = %v, want 'test share'", p["comment"])
			}
			if p["guestok"] != true {
				t.Errorf("guestok = %v, want true", p["guestok"])
			}
			return json.RawMessage(`{"id":1,"name":"myshare"}`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_smb_create", map[string]any{
		"name":     "myshare",
		"path":     "/mnt/tank/data",
		"comment":  "test share",
		"guest_ok": true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if text == "" {
		t.Error("result text is empty")
	}
}

func TestSMBCreate_RequiredOnly(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			p := params[0].(map[string]any)
			if p["name"] != "myshare" {
				t.Errorf("name = %v, want myshare", p["name"])
			}
			if p["path"] != "/mnt/tank/data" {
				t.Errorf("path = %v, want /mnt/tank/data", p["path"])
			}
			if _, ok := p["comment"]; ok {
				t.Error("unexpected comment key")
			}
			if _, ok := p["guestok"]; ok {
				t.Error("unexpected guestok key")
			}
			return json.RawMessage(`{"id":1}`), nil
		},
	}
	_, err := callTool(t, mock, false, "truenas_smb_create", map[string]any{
		"name": "myshare",
		"path": "/mnt/tank/data",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSMBCreate_MissingName(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			t.Fatal("Call should not be invoked")
			return nil, nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_smb_create", map[string]any{"path": "/mnt/tank/data"})
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for missing name")
	}
}

func TestSMBCreate_MissingPath(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			t.Fatal("Call should not be invoked")
			return nil, nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_smb_create", map[string]any{"name": "myshare"})
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for missing path")
	}
}

func TestSMBDelete_Success(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "sharing.smb.delete" {
				t.Errorf("method = %q, want sharing.smb.delete", method)
			}
			if len(params) == 0 {
				t.Fatal("expected params")
			}
			// Verify float64→int conversion
			id, ok := params[0].(int)
			if !ok {
				t.Fatalf("params[0] is %T, want int", params[0])
			}
			if id != 5 {
				t.Errorf("id = %d, want 5", id)
			}
			return json.RawMessage(`true`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_smb_delete", map[string]any{"id": 5.0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if text == "" {
		t.Error("result text is empty")
	}
}

func TestSMBDelete_MissingID(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			t.Fatal("Call should not be invoked")
			return nil, nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_smb_delete", nil)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for missing id")
	}
}

func TestNFSCreate_WithNetworksAndHosts(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "sharing.nfs.create" {
				t.Errorf("method = %q, want sharing.nfs.create", method)
			}
			p := params[0].(map[string]any)
			if p["path"] != "/mnt/tank/data" {
				t.Errorf("path = %v, want /mnt/tank/data", p["path"])
			}
			networks, ok := p["networks"].([]any)
			if !ok || len(networks) == 0 {
				t.Error("expected networks")
			}
			hosts, ok := p["hosts"].([]any)
			if !ok || len(hosts) == 0 {
				t.Error("expected hosts")
			}
			return json.RawMessage(`{"id":1}`), nil
		},
	}
	_, err := callTool(t, mock, false, "truenas_nfs_create", map[string]any{
		"path":     "/mnt/tank/data",
		"networks": []any{"192.168.1.0/24"},
		"hosts":    []any{"host1.local"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNFSCreate_RequiredOnly(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			p := params[0].(map[string]any)
			if p["path"] != "/mnt/tank/data" {
				t.Errorf("path = %v, want /mnt/tank/data", p["path"])
			}
			if _, ok := p["networks"]; ok {
				t.Error("unexpected networks key")
			}
			if _, ok := p["hosts"]; ok {
				t.Error("unexpected hosts key")
			}
			return json.RawMessage(`{"id":1}`), nil
		},
	}
	_, err := callTool(t, mock, false, "truenas_nfs_create", map[string]any{"path": "/mnt/tank/data"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNFSCreate_MissingPath(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			t.Fatal("Call should not be invoked")
			return nil, nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_nfs_create", nil)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for missing path")
	}
}

func TestNFSDelete_Success(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "sharing.nfs.delete" {
				t.Errorf("method = %q, want sharing.nfs.delete", method)
			}
			id, ok := params[0].(int)
			if !ok {
				t.Fatalf("params[0] is %T, want int", params[0])
			}
			if id != 3 {
				t.Errorf("id = %d, want 3", id)
			}
			return json.RawMessage(`true`), nil
		},
	}
	_, err := callTool(t, mock, false, "truenas_nfs_delete", map[string]any{"id": 3.0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNFSDelete_MissingID(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			t.Fatal("Call should not be invoked")
			return nil, nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_nfs_delete", nil)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for missing id")
	}
}
