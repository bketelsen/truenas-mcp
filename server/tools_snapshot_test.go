package server

import (
	"encoding/json"
	"errors"
	"regexp"
	"testing"
)

func TestSnapshotList_Success(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "pool.snapshot.query" {
				t.Errorf("method = %q, want pool.snapshot.query", method)
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
			checkSnapshotOptions(t, params)
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
			checkSnapshotOptions(t, params)
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
			if method != "pool.snapshot.create" {
				t.Errorf("method = %q, want pool.snapshot.create", method)
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
			if method != "pool.snapshot.delete" {
				t.Errorf("method = %q, want pool.snapshot.delete", method)
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

func TestSnapshot_FallsBackToLegacyMethods(t *testing.T) {
	cases := []struct {
		tool string
		args map[string]any
		op   string
	}{
		{"truenas_snapshot_list", map[string]any{"dataset": "tank/data"}, "query"},
		{"truenas_snapshot_get", map[string]any{"name": "tank/data@s"}, "query"},
		{"truenas_snapshot_create", map[string]any{"dataset": "tank/data", "name": "s"}, "create"},
		{"truenas_snapshot_delete", map[string]any{"name": "tank/data@s"}, "delete"},
	}
	for _, c := range cases {
		t.Run(c.tool, func(t *testing.T) {
			var calls []string
			mock := &mockCaller{
				CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
					calls = append(calls, method)
					if method == "pool.snapshot."+c.op {
						return nil, errors.New("calling " + method + ": Method does not exist")
					}
					return json.RawMessage(`true`), nil
				},
			}
			if _, err := callTool(t, mock, false, c.tool, c.args); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want := []string{"pool.snapshot." + c.op, "zfs.snapshot." + c.op}
			if len(calls) != 2 || calls[0] != want[0] || calls[1] != want[1] {
				t.Errorf("calls = %v, want %v", calls, want)
			}
		})
	}
}

func TestSnapshot_NoFallbackOnOtherErrors(t *testing.T) {
	var calls []string
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			calls = append(calls, method)
			return nil, errors.New("permission denied")
		},
	}
	if _, err := callTool(t, mock, false, "truenas_snapshot_delete", map[string]any{"name": "tank/data@s"}); err == nil {
		t.Fatal("expected error")
	}
	if len(calls) != 1 || calls[0] != "pool.snapshot.delete" {
		t.Errorf("calls = %v, want only pool.snapshot.delete", calls)
	}
}

func checkSnapshotOptions(t *testing.T, params []interface{}) {
	t.Helper()
	if len(params) != 2 {
		t.Fatalf("params = %v, want filters and options", params)
	}
	opts, _ := params[1].(map[string]any)
	extra, _ := opts["extra"].(map[string]any)
	props, _ := extra["properties"].([]string)
	want := map[string]bool{"creation": true, "used": true, "referenced": true}
	for _, p := range props {
		delete(want, p)
	}
	if len(want) != 0 {
		t.Errorf("extra.properties = %v, missing %v", props, want)
	}
}

func TestSnapshotList_SortedNewestFirst(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			return json.RawMessage(`[
				{"id":"old","properties":{"creation":{"parsed":{"$date":1000000},"rawvalue":"1000"}}},
				{"id":"none","properties":{}},
				{"id":"new","properties":{"creation":{"parsed":{"$date":3000000},"rawvalue":"3000"}}},
				{"id":"raw","properties":{"creation":{"rawvalue":"2000"}}}
			]`), nil
		},
	}
	result, err := callTool(t, mock, false, "truenas_snapshot_list", map[string]any{"dataset": "tank/data"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var items []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(resultText(t, result)), &items); err != nil {
		t.Fatal(err)
	}
	got := ""
	for _, it := range items {
		got += it.ID + " "
	}
	if got != "new raw old none " {
		t.Errorf("order = %q, want \"new raw old none \"", got)
	}
}
