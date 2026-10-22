package server

import (
	"encoding/json"
	"errors"
	"strings"
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
	calls := 0
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			calls++
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
			if p["purpose"] != "LEGACY_SHARE" {
				t.Errorf("purpose = %v, want LEGACY_SHARE", p["purpose"])
			}
			opts, _ := p["options"].(map[string]any)
			if opts["guestok"] != true {
				t.Errorf("options.guestok = %v, want true", opts["guestok"])
			}
			if _, ok := p["guestok"]; ok {
				t.Error("unexpected top-level guestok key")
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
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestSMBCreate_GuestFallsBackToLegacyGuestOK(t *testing.T) {
	// TrueNAS before 25.10 has no purpose or options and rejects them in
	// validation, before the create runs.
	const errText = `TrueNAS API error from sharing.smb.create: {"code": -32602, "message": "Invalid params", "data": {"errname": "EINVAL", "reason": "[EINVAL] sharing_smb_create.options: Extra inputs are not permitted"}}`
	var calls []map[string]any
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			p := params[0].(map[string]any)
			calls = append(calls, p)
			if len(calls) == 1 {
				return nil, errors.New(errText)
			}
			return json.RawMessage(`{"id":1}`), nil
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
	if text := resultText(t, result); text == "" {
		t.Error("result text is empty")
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(calls))
	}
	if calls[0]["purpose"] != "LEGACY_SHARE" {
		t.Errorf("first call purpose = %v, want LEGACY_SHARE", calls[0]["purpose"])
	}
	if _, ok := calls[1]["purpose"]; ok {
		t.Error("second call has a purpose key")
	}
	if _, ok := calls[1]["options"]; ok {
		t.Error("second call has an options key")
	}
	if calls[1]["guestok"] != true {
		t.Errorf("second call guestok = %v, want true", calls[1]["guestok"])
	}
	if calls[1]["comment"] != "test share" {
		t.Errorf("second call comment = %v, want 'test share'", calls[1]["comment"])
	}
}

func TestSMBCreate_GuestNoFallbackOnOtherErrors(t *testing.T) {
	calls := 0
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			calls++
			return nil, errors.New("connection reset")
		},
	}
	if _, err := callTool(t, mock, false, "truenas_smb_create", map[string]any{
		"name":     "myshare",
		"path":     "/mnt/tank/data",
		"guest_ok": true,
	}); err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestSMBCreate_GuestNoFallbackOnUnrelatedValidationError(t *testing.T) {
	// A traceback in the envelope can name purpose and options in source lines;
	// only the validation reason decides whether to retry.
	const errText = `TrueNAS API error from sharing.smb.create: {"code": -32602, "message": "Invalid params", "data": {"errname": "EINVAL", "reason": "[EINVAL] sharing_smb_create.path: Path does not exist", "trace": "File sharing.py, in validate: if data['purpose'] is None: options = data['options']"}}`
	calls := 0
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			calls++
			return nil, errors.New(errText)
		},
	}
	_, err := callTool(t, mock, false, "truenas_smb_create", map[string]any{
		"name":     "myshare",
		"path":     "/mnt/tank/missing",
		"guest_ok": true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Path does not exist") {
		t.Errorf("error = %v, want the path validation error", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestSMBCreate_GuestFallbackErrorKeepsBothMessages(t *testing.T) {
	var calls []map[string]any
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			calls = append(calls, params[0].(map[string]any))
			if len(calls) == 1 {
				return nil, errors.New("[EINVAL] sharing_smb_create.options: Extra inputs are not permitted")
			}
			return nil, errors.New("permission denied")
		},
	}
	_, err := callTool(t, mock, false, "truenas_smb_create", map[string]any{
		"name":     "myshare",
		"path":     "/mnt/tank/data",
		"guest_ok": true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(calls))
	}
	for _, want := range []string{"permission denied", "sharing_smb_create.options"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to contain %q", err, want)
		}
	}
}

func TestSMBCreate_RequiredOnly(t *testing.T) {
	calls := 0
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			calls++
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
			if _, ok := p["purpose"]; ok {
				t.Error("unexpected purpose key")
			}
			if _, ok := p["options"]; ok {
				t.Error("unexpected options key")
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
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
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
