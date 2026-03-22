package server

import (
	"encoding/json"
	"regexp"
	"testing"
)

func TestSnapshotList_Success(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "zfs.snapshot.query" {
				t.Errorf("method = %q, want zfs.snapshot.query", method)
			}
			if len(params) == 0 {
				t.Fatal("expected filter params")
			}
			filter, ok := params[0].([][]any)
			if !ok {
				t.Fatalf("params[0] is %T, want [][]any", params[0])
			}
			if filter[0][0] != "dataset" || filter[0][2] != "tank/data" {
				t.Errorf("filter = %v, want [[dataset = tank/data]]", filter)
			}
			return json.RawMessage(`[{"id":"tank/data@snap1"}]`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_snapshot_list", map[string]any{"dataset": "tank/data"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if text == "" {
		t.Error("result text is empty")
	}
}

func TestSnapshotList_MissingDataset(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			t.Fatal("Call should not be invoked")
			return nil, nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_snapshot_list", nil)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for missing dataset")
	}
}

func TestSnapshotGet_Success(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if len(params) == 0 {
				t.Fatal("expected filter params")
			}
			filter := params[0].([][]any)
			if filter[0][0] != "id" || filter[0][2] != "tank/data@snap1" {
				t.Errorf("filter = %v, want [[id = tank/data@snap1]]", filter)
			}
			return json.RawMessage(`{"id":"tank/data@snap1"}`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_snapshot_get", map[string]any{"name": "tank/data@snap1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if text == "" {
		t.Error("result text is empty")
	}
}

func TestSnapshotGet_MissingName(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			t.Fatal("Call should not be invoked")
			return nil, nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_snapshot_get", nil)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for missing name")
	}
}

func TestSnapshotCreate_WithName(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "zfs.snapshot.create" {
				t.Errorf("method = %q, want zfs.snapshot.create", method)
			}
			p := params[0].(map[string]any)
			if p["dataset"] != "tank/data" {
				t.Errorf("dataset = %v, want tank/data", p["dataset"])
			}
			if p["name"] != "mysnap" {
				t.Errorf("name = %v, want mysnap", p["name"])
			}
			return json.RawMessage(`{"id":"tank/data@mysnap"}`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_snapshot_create", map[string]any{
		"dataset": "tank/data",
		"name":    "mysnap",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if text == "" {
		t.Error("result text is empty")
	}
}

func TestSnapshotCreate_AutoName(t *testing.T) {
	autoNameRe := regexp.MustCompile(`^auto-\d{8}-\d{6}$`)
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			p := params[0].(map[string]any)
			name, ok := p["name"].(string)
			if !ok {
				t.Fatal("name is not a string")
			}
			if !autoNameRe.MatchString(name) {
				t.Errorf("auto name %q does not match pattern auto-YYYYMMDD-HHMMSS", name)
			}
			return json.RawMessage(`{"id":"tank/data@` + name + `"}`), nil
		},
	}
	_, err := callTool(t, mock, false, "truenas_snapshot_create", map[string]any{"dataset": "tank/data"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSnapshotCreate_MissingDataset(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			t.Fatal("Call should not be invoked")
			return nil, nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_snapshot_create", nil)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for missing dataset")
	}
}

func TestSnapshotDelete_Success(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "zfs.snapshot.delete" {
				t.Errorf("method = %q, want zfs.snapshot.delete", method)
			}
			if len(params) == 0 || params[0] != "tank/data@snap1" {
				t.Errorf("params = %v, want [tank/data@snap1]", params)
			}
			return json.RawMessage(`true`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_snapshot_delete", map[string]any{"name": "tank/data@snap1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	if text == "" {
		t.Error("result text is empty")
	}
}

func TestSnapshotDelete_MissingName(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			t.Fatal("Call should not be invoked")
			return nil, nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_snapshot_delete", nil)
	if err == nil && (result == nil || !result.IsError) {
		t.Error("expected error for missing name")
	}
}
