package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

type deleteCalls struct {
	queries int
	deletes [][]any
}

// deleteMock answers app.query for an app named plex (or no app) and records
// the app.delete calls.
func deleteMock(t *testing.T, found bool) (*mockCaller, *deleteCalls) {
	t.Helper()
	calls := &deleteCalls{}
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			switch method {
			case "app.query":
				calls.queries++
				filter, ok := params[0].([][]any)
				if !ok || filter[0][0] != "name" || filter[0][1] != "=" || filter[0][2] != "plex" {
					t.Errorf("app.query params = %v, want [[name = plex]]", params)
				}
				if !found {
					return json.RawMessage(`[]`), nil
				}
				return json.RawMessage(`[{"name":"plex","state":"RUNNING","version":"1.2.3","custom_app":false}]`), nil
			case "app.delete":
				calls.deletes = append(calls.deletes, params)
				return json.RawMessage(`777`), nil
			}
			t.Errorf("unexpected method %q", method)
			return nil, fmt.Errorf("unexpected method %q", method)
		},
	}
	return mock, calls
}

func neverCalled(t *testing.T) *mockCaller {
	return &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			t.Fatalf("Call should not be invoked, got %q", method)
			return nil, nil
		},
	}
}

func TestAppDelete_ConfirmMismatch(t *testing.T) {
	for _, confirm := range []string{"other", "Plex", "plex ", ""} {
		t.Run(fmt.Sprintf("%q", confirm), func(t *testing.T) {
			result, err := callTool(t, neverCalled(t), false, "truenas_app_delete", map[string]any{
				"name": "plex", "confirm": confirm,
			})
			if err == nil && (result == nil || !result.IsError) {
				t.Fatal("expected error for confirm mismatch")
			}
			if err == nil && confirm != "" && !strings.Contains(resultText(t, result), "must equal") {
				t.Errorf("error text = %q, want it to say confirm must equal the name", resultText(t, result))
			}
		})
	}
}

func TestAppDelete_MissingConfirm(t *testing.T) {
	result, err := callTool(t, neverCalled(t), false, "truenas_app_delete", map[string]any{"name": "plex"})
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error for missing confirm")
	}
}

func TestAppDelete_MissingName(t *testing.T) {
	result, err := callTool(t, neverCalled(t), false, "truenas_app_delete", map[string]any{"confirm": "plex"})
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error for missing name")
	}
}

func decodeObject(t *testing.T, text string) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("result is not JSON: %v\n%s", err, text)
	}
	return got
}

func TestAppDelete_DryRunRejectsMalformedValues(t *testing.T) {
	for _, value := range []any{"", "true", "false", 0, 1, []any{}, []any{true}, map[string]any{}} {
		t.Run(fmt.Sprintf("%T/%v", value, value), func(t *testing.T) {
			calls := 0
			mock := &mockCaller{CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
				calls++
				return nil, fmt.Errorf("unexpected middleware call %q", method)
			}}
			result, err := callTool(t, mock, false, "truenas_app_delete", map[string]any{
				"name": "plex", "confirm": "plex", "dry_run": value,
			})
			if calls != 0 {
				t.Errorf("middleware calls = %d, want 0", calls)
			}
			if err == nil && (result == nil || !result.IsError) {
				t.Fatal("expected malformed dry_run error")
			}
			msg := ""
			if err != nil {
				msg = err.Error()
			} else {
				msg = resultText(t, result)
			}
			if !strings.Contains(msg, `"dry_run" must be a boolean`) {
				t.Errorf("error = %q, want dry_run boolean validation", msg)
			}
		})
	}
}

func TestAppDelete_DryRunBooleanSemantics(t *testing.T) {
	for _, tc := range []struct {
		name    string
		present bool
		value   any
		dryRun  bool
	}{
		{name: "absent"},
		{name: "null", present: true},
		{name: "false", present: true, value: false},
		{name: "true", present: true, value: true, dryRun: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock, calls := deleteMock(t, true)
			args := map[string]any{"name": "plex", "confirm": "plex"}
			if tc.present {
				args["dry_run"] = tc.value
			}
			result, err := callTool(t, mock, false, "truenas_app_delete", args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.IsError {
				t.Fatalf("tool error: %s", resultText(t, result))
			}
			wantDeletes := 1
			if tc.dryRun {
				wantDeletes = 0
			}
			if calls.queries != 1 || len(calls.deletes) != wantDeletes {
				t.Errorf("queries = %d, deletes = %d, want 1 and %d", calls.queries, len(calls.deletes), wantDeletes)
			}
			got := decodeObject(t, resultText(t, result))
			if tc.dryRun {
				if got["dry_run"] != true || got["job_id"] != nil {
					t.Errorf("dry run result = %v", got)
				}
			} else if got["job_id"] != float64(777) {
				t.Errorf("job_id = %v, want 777", got["job_id"])
			}
		})
	}
}

func TestAppDelete_DryRun(t *testing.T) {
	mock, calls := deleteMock(t, true)
	result, err := callTool(t, mock, false, "truenas_app_delete", map[string]any{
		"name": "plex", "confirm": "plex", "dry_run": true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", resultText(t, result))
	}
	if calls.queries != 1 {
		t.Errorf("app.query called %d times, want 1", calls.queries)
	}
	if len(calls.deletes) != 0 {
		t.Fatalf("app.delete called on a dry run: %v", calls.deletes)
	}
	got := decodeObject(t, resultText(t, result))
	if got["dry_run"] != true || got["name"] != "plex" || got["method"] != "app.delete" {
		t.Errorf("result = %v", got)
	}
	opts := got["options"].(map[string]any)
	if opts["remove_ix_volumes"] != false || opts["remove_images"] != true || opts["force_remove_custom_app"] != false || len(opts) != 3 {
		t.Errorf("options = %v", opts)
	}
	app := got["app"].(map[string]any)
	if app["state"] != "RUNNING" || app["version"] != "1.2.3" || app["custom_app"] != false {
		t.Errorf("app = %v", app)
	}
	effects := strings.ToLower(fmt.Sprint(got["effects"]))
	if !strings.Contains(effects, "kept") || strings.Contains(effects, "destroyed") {
		t.Errorf("effects should say ix-volumes are kept: %s", effects)
	}
	if !strings.Contains(effects, "host-path") {
		t.Errorf("effects should say host paths are untouched: %s", effects)
	}
}

func TestAppDelete_DryRunRemoveIxVolumes(t *testing.T) {
	mock, calls := deleteMock(t, true)
	result, err := callTool(t, mock, false, "truenas_app_delete", map[string]any{
		"name": "plex", "confirm": "plex", "dry_run": true, "remove_ix_volumes": true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calls.deletes) != 0 {
		t.Fatalf("app.delete called on a dry run: %v", calls.deletes)
	}
	got := decodeObject(t, resultText(t, result))
	if got["options"].(map[string]any)["remove_ix_volumes"] != true {
		t.Errorf("options = %v, want remove_ix_volumes true", got["options"])
	}
	effects := strings.ToLower(fmt.Sprint(got["effects"]))
	if !strings.Contains(effects, "destroyed") || strings.Contains(effects, "are kept") {
		t.Errorf("effects should say ix-volumes are destroyed: %s", effects)
	}
}

func TestAppDelete_DryRunNotFound(t *testing.T) {
	mock, calls := deleteMock(t, false)
	result, err := callTool(t, mock, false, "truenas_app_delete", map[string]any{
		"name": "plex", "confirm": "plex", "dry_run": true,
	})
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected not-found error on a dry run")
	}
	if len(calls.deletes) != 0 {
		t.Errorf("app.delete called: %v", calls.deletes)
	}
}

func TestAppDelete_Default(t *testing.T) {
	mock, calls := deleteMock(t, true)
	result, err := callTool(t, mock, false, "truenas_app_delete", map[string]any{
		"name": "plex", "confirm": "plex",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", resultText(t, result))
	}
	if len(calls.deletes) != 1 {
		t.Fatalf("app.delete called %d times, want 1", len(calls.deletes))
	}
	params := calls.deletes[0]
	if len(params) != 2 || params[0] != "plex" {
		t.Fatalf("app.delete params = %v, want [plex options]", params)
	}
	opts, ok := params[1].(map[string]any)
	if !ok {
		t.Fatalf("options is %T, want map", params[1])
	}
	if opts["remove_ix_volumes"] != false || opts["remove_images"] != true || opts["force_remove_custom_app"] != false || len(opts) != 3 {
		t.Errorf("options = %v", opts)
	}
	got := decodeObject(t, resultText(t, result))
	if got["name"] != "plex" || got["job_id"] != float64(777) || got["remove_ix_volumes"] != false {
		t.Errorf("result = %v", got)
	}
}

func TestAppDelete_RemoveIxVolumes(t *testing.T) {
	mock, calls := deleteMock(t, true)
	result, err := callTool(t, mock, false, "truenas_app_delete", map[string]any{
		"name": "plex", "confirm": "plex", "remove_ix_volumes": true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calls.deletes) != 1 {
		t.Fatalf("app.delete called %d times, want 1", len(calls.deletes))
	}
	opts := calls.deletes[0][1].(map[string]any)
	if opts["remove_ix_volumes"] != true || opts["remove_images"] != true || opts["force_remove_custom_app"] != false {
		t.Errorf("options = %v", opts)
	}
	got := decodeObject(t, resultText(t, result))
	if got["job_id"] != float64(777) || got["remove_ix_volumes"] != true {
		t.Errorf("result = %v", got)
	}
}

func TestAppDelete_NotFound(t *testing.T) {
	mock, calls := deleteMock(t, false)
	result, err := callTool(t, mock, false, "truenas_app_delete", map[string]any{
		"name": "plex", "confirm": "plex",
	})
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected not-found error")
	}
	if err == nil && !strings.Contains(resultText(t, result), `app "plex" not found`) {
		t.Errorf("error text = %q", resultText(t, result))
	}
	if len(calls.deletes) != 0 {
		t.Errorf("app.delete called for a missing app: %v", calls.deletes)
	}
}

func TestAppDelete_APIError(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method == "app.query" {
				return json.RawMessage(`[{"name":"plex"}]`), nil
			}
			return nil, fmt.Errorf("denied")
		},
	}
	result, err := callTool(t, mock, false, "truenas_app_delete", map[string]any{"name": "plex", "confirm": "plex"})
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error")
	}
	if err == nil && !strings.Contains(resultText(t, result), "app.delete") {
		t.Errorf("error text = %q, want the method name", resultText(t, result))
	}
}

func TestAppDelete_AbsentInReadOnlyMode(t *testing.T) {
	for _, name := range listTools(t, annotationsMock(), true) {
		if name == "truenas_app_delete" {
			t.Fatal("read-only server registered truenas_app_delete")
		}
	}
	_, err := callTool(t, neverCalled(t), true, "truenas_app_delete", map[string]any{"name": "plex", "confirm": "plex"})
	if err == nil {
		t.Error("expected error calling truenas_app_delete on a read-only server")
	}
}

func TestDataProtectionToolsRegisteredReadOnly(t *testing.T) {
	have := map[string]bool{}
	for _, name := range listTools(t, annotationsMock(), true) {
		have[name] = true
	}
	for _, name := range []string{"truenas_snapshot_task_list", "truenas_replication_task_list", "truenas_scrub_task_list", "truenas_rsync_task_list"} {
		if !have[name] {
			t.Errorf("%s not registered in read-only mode", name)
		}
	}
}
