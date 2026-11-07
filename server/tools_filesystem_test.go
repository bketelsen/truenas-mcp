package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestValidateMntPath(t *testing.T) {
	got, err := validateMntPath("/mnt/./tank//data/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/mnt/tank/data" {
		t.Errorf("cleaned = %q, want /mnt/tank/data", got)
	}
}

func TestFilesystemStat_Happy(t *testing.T) {
	calls := 0
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			calls++
			switch calls {
			case 1:
				if method != "filesystem.stat" {
					t.Errorf("method = %q, want filesystem.stat", method)
				}
				if len(params) != 1 || params[0] != "/mnt/tank/data" {
					t.Errorf("filesystem.stat params = %v, want [/mnt/tank/data]", params)
				}
				return json.RawMessage(`{"realpath":"/mnt/tank/data","type":"DIRECTORY","uid":0,"gid":0,"user":"root","group":"root","mode":16877}`), nil
			case 2:
				if method != "filesystem.getacl" {
					t.Errorf("method = %q, want filesystem.getacl", method)
				}
				if len(params) != 2 || params[0] != "/mnt/tank/data" || params[1] != true {
					t.Errorf("filesystem.getacl params = %v, want [/mnt/tank/data true]", params)
				}
				return json.RawMessage(`{"acltype":"POSIX1E","trivial":false,"uid":0,"gid":0,"acl":[{"tag":"GROUP"}]}`), nil
			default:
				t.Errorf("unexpected call %d: %s", calls, method)
				return nil, fmt.Errorf("unexpected call")
			}
		},
	}

	result, err := callTool(t, mock, true, "truenas_filesystem_stat", map[string]any{"path": "/mnt/tank/data"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", resultText(t, result))
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(resultText(t, result)), &got); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	want := map[string]any{
		"path": "/mnt/tank/data", "realpath": "/mnt/tank/data", "type": "DIRECTORY",
		"uid": float64(0), "gid": float64(0), "user": "root", "group": "root",
		"mode": float64(16877), "permissions": "0755", "acl_type": "POSIX1E", "acl_trivial": false,
	}
	if len(got) != len(want) {
		t.Errorf("result has %d keys, want %d: %v", len(got), len(want), got)
	}
	for key, wantValue := range want {
		if gotValue, ok := got[key]; !ok {
			t.Errorf("missing key %q", key)
		} else if fmt.Sprint(gotValue) != fmt.Sprint(wantValue) {
			t.Errorf("%s = %v, want %v", key, gotValue, wantValue)
		}
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

func TestFilesystemStat_SetuidPermissions(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			switch method {
			case "filesystem.stat":
				return json.RawMessage(`{"realpath":"/mnt/tank/tool","type":"FILE","uid":0,"gid":0,"user":"root","group":"root","mode":35309}`), nil
			case "filesystem.getacl":
				return json.RawMessage(`{"acltype":"DISABLED","trivial":true}`), nil
			default:
				t.Errorf("unexpected method %q", method)
				return nil, fmt.Errorf("unexpected call")
			}
		},
	}
	result, err := callTool(t, mock, true, "truenas_filesystem_stat", map[string]any{"path": "/mnt/tank/tool"})
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: err=%v result=%v", err, result)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(resultText(t, result)), &got); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if got["permissions"] != "4755" {
		t.Errorf("permissions = %v, want 4755", got["permissions"])
	}
}

func TestFilesystemStatRefusedInputs(t *testing.T) {
	tests := []struct {
		name   string
		args   map[string]any
		reason string
	}{
		{"missing path", nil, "missing"},
		{"empty", map[string]any{"path": ""}, "empty"},
		{"number", map[string]any{"path": 123}, "string"},
		{"relative", map[string]any{"path": "relative/a"}, "start with"},
		{"etc", map[string]any{"path": "/etc"}, "below /mnt"},
		{"mnt", map[string]any{"path": "/mnt"}, "below /mnt"},
		{"mnt slash", map[string]any{"path": "/mnt/"}, "below /mnt"},
		{"mntx", map[string]any{"path": "/mntx/a"}, "below /mnt"},
		{"mnt2", map[string]any{"path": "/mnt2/a"}, "below /mnt"},
		{"dotdot direct", map[string]any{"path": "/mnt/../etc"}, ".."},
		{"dotdot nested", map[string]any{"path": "/mnt/tank/../../etc"}, ".."},
		{"dotdot trailing", map[string]any{"path": "/mnt/tank/.."}, ".."},
		{"nul", map[string]any{"path": "/mnt/tank/\x00x"}, "control"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockCaller{CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
				t.Errorf("Call invoked for refused path: %s", method)
				return nil, fmt.Errorf("unexpected call")
			}}
			result, err := callTool(t, mock, true, "truenas_filesystem_stat", tt.args)
			if err == nil && (result == nil || !result.IsError) {
				t.Fatal("expected error for refused path")
			}
			text := ""
			if err != nil {
				text = err.Error()
			} else {
				text = resultText(t, result)
			}
			if !strings.Contains(text, tt.reason) {
				t.Errorf("error %q does not contain %q", text, tt.reason)
			}
		})
	}
}

func TestValidateMntPathRefused(t *testing.T) {
	tests := []struct {
		path, reason string
	}{
		{"", "empty"},
		{"relative/a", "start with"},
		{"/etc", "below /mnt"},
		{"/mnt", "below /mnt"},
		{"/mnt/", "below /mnt"},
		{"/mntx/a", "below /mnt"},
		{"/mnt2/a", "below /mnt"},
		{"/mnt/../etc", ".."},
		{"/mnt/tank/../../etc", ".."},
		{"/mnt/tank/..", ".."},
		{"/mnt/tank/\x00x", "control"},
	}
	for _, tt := range tests {
		_, err := validateMntPath(tt.path)
		if err == nil {
			t.Errorf("validateMntPath(%q) returned no error", tt.path)
			continue
		}
		if !strings.Contains(err.Error(), tt.reason) {
			t.Errorf("validateMntPath(%q) error %q does not contain %q", tt.path, err, tt.reason)
		}
	}
}

func TestFilesystemStat_AppliesPathNormalization(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			switch method {
			case "filesystem.stat":
				if len(params) != 1 || params[0] != "/mnt/tank/data" {
					t.Errorf("filesystem.stat params = %v", params)
				}
				return json.RawMessage(`{"realpath":"/mnt/tank/data","type":"DIRECTORY","uid":0,"gid":0,"mode":16877}`), nil
			case "filesystem.getacl":
				return json.RawMessage(`{"acltype":"DISABLED","trivial":true}`), nil
			default:
				t.Errorf("unexpected method %q", method)
				return nil, fmt.Errorf("unexpected call")
			}
		},
	}
	result, err := callTool(t, mock, true, "truenas_filesystem_stat", map[string]any{"path": "/mnt/./tank//data/"})
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: err=%v result=%v", err, result)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(resultText(t, result)), &got); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if got["path"] != "/mnt/tank/data" {
		t.Errorf("path = %v, want /mnt/tank/data", got["path"])
	}
}

func TestFilesystemStat_RealpathOutside(t *testing.T) {
	calls := 0
	mock := &mockCaller{CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
		calls++
		if method != "filesystem.stat" {
			t.Errorf("getacl must not be called; got %s", method)
		}
		return json.RawMessage(`{"realpath":"/etc","type":"DIRECTORY","uid":0,"gid":0,"user":"root","group":"root","mode":16877}`), nil
	}}
	result, err := callTool(t, mock, true, "truenas_filesystem_stat", map[string]any{"path": "/mnt/tank/link"})
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected realpath refusal")
	}
	text := ""
	if err != nil {
		text = err.Error()
	} else {
		text = resultText(t, result)
	}
	if !strings.Contains(text, "outside /mnt") {
		t.Errorf("error %q does not mention outside /mnt", text)
	}
	if strings.Contains(text, "root") {
		t.Errorf("error text leaked stat values: %q", text)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestFilesystemStat_RealpathMissing(t *testing.T) {
	mock := &mockCaller{CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
		if method != "filesystem.stat" {
			t.Errorf("getacl must not be called; got %s", method)
		}
		return json.RawMessage(`{"type":"DIRECTORY","uid":0,"gid":0,"mode":16877}`), nil
	}}
	result, err := callTool(t, mock, true, "truenas_filesystem_stat", map[string]any{"path": "/mnt/tank/data"})
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected missing realpath error")
	}
	text := ""
	if err != nil {
		text = err.Error()
	} else {
		text = resultText(t, result)
	}
	if !strings.Contains(text, "realpath") {
		t.Errorf("error %q does not mention realpath", text)
	}
}

func TestFilesystemStat_StatError(t *testing.T) {
	mock := &mockCaller{CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
		if method != "filesystem.stat" {
			t.Errorf("method = %q, want filesystem.stat", method)
		}
		return nil, fmt.Errorf("connection refused")
	}}
	result, err := callTool(t, mock, true, "truenas_filesystem_stat", map[string]any{"path": "/mnt/tank/data"})
	text := ""
	if err != nil {
		text = err.Error()
	} else {
		text = resultText(t, result)
	}
	if !strings.Contains(text, "filesystem.stat") {
		t.Errorf("error %q does not name filesystem.stat", text)
	}
}

func TestFilesystemStat_GetACLError(t *testing.T) {
	calls := 0
	mock := &mockCaller{CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
		calls++
		switch method {
		case "filesystem.stat":
			return json.RawMessage(`{"realpath":"/mnt/tank/data","type":"FILE","uid":0,"gid":0,"mode":33188}`), nil
		case "filesystem.getacl":
			return nil, fmt.Errorf("permission denied")
		default:
			t.Errorf("unexpected method %q", method)
			return nil, fmt.Errorf("unexpected call")
		}
	}}
	result, err := callTool(t, mock, true, "truenas_filesystem_stat", map[string]any{"path": "/mnt/tank/data"})
	text := ""
	if err != nil {
		text = err.Error()
	} else {
		text = resultText(t, result)
	}
	if !strings.Contains(text, "filesystem.getacl") {
		t.Errorf("error %q does not name filesystem.getacl", text)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

func TestFilesystemStat_GetACLMissingFields(t *testing.T) {
	for _, body := range []string{`{"trivial":true}`, `{"acltype":"NFS4"}`} {
		mock := &mockCaller{CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method == "filesystem.stat" {
				return json.RawMessage(`{"realpath":"/mnt/tank/data","type":"FILE","uid":0,"gid":0,"mode":33188}`), nil
			}
			return json.RawMessage(body), nil
		}}
		result, err := callTool(t, mock, true, "truenas_filesystem_stat", map[string]any{"path": "/mnt/tank/data"})
		if err == nil && (result == nil || !result.IsError) {
			t.Errorf("getacl %s: expected error, got success", body)
		}
	}
}
