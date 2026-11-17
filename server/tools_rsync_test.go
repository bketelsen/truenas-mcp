package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

// rsyncFakeKey is key material that must never reach a tool result. The value is
// fake and the tests only check that it stays inside the Server.
const rsyncFakeKey = "-----BEGIN OPENSSH PRIVATE KEY-----\nFAKE-KEY-MATERIAL-DO-NOT-LEAK\n-----END OPENSSH PRIVATE KEY-----"

var rsyncJobStart = time.Date(2026, 11, 20, 2, 0, 0, 0, time.UTC)

// rsyncCredentialFixture is a keychain credential as the middleware sends it: a
// whole KeychainCredentialEntry, with type and attributes.
func rsyncCredentialFixture() map[string]any {
	return map[string]any{
		"id":   4,
		"name": "backup-key",
		"type": "SSH_CREDENTIALS",
		"attributes": map[string]any{
			"private_key":     rsyncFakeKey,
			"remote_host_key": "ssh-ed25519 AAAAC3NzaFAKE-HOST-KEY",
			"host":            "backup.example.invalid",
			"username":        "backup",
		},
	}
}

// rsyncTaskFixture is a full RsyncTaskEntry with more keys than the tool reports,
// so a field that leaks shows up in a result.
func rsyncTaskFixture(id int) map[string]any {
	return map[string]any{
		"id":              id,
		"path":            "/mnt/tank/example",
		"dataset":         "tank/example",
		"relative_path":   false,
		"user":            "backup",
		"mode":            "MODULE",
		"remotehost":      "backup.example.invalid",
		"remoteport":      873,
		"remotemodule":    "archive",
		"remotepath":      "/srv/example",
		"direction":       "PULL",
		"desc":            "nightly",
		"schedule":        map[string]any{"minute": "0", "hour": "2", "dom": "*", "month": "*", "dow": "*", "begin": "00:00", "end": "23:59"},
		"recursive":       false,
		"times":           true,
		"compress":        true,
		"archive":         false,
		"delete":          false,
		"quiet":           true,
		"preserveperm":    false,
		"preserveattr":    false,
		"delayupdates":    false,
		"extra":           []string{"--password-file=/marker"},
		"enabled":         true,
		"locked":          false,
		"job":             nil,
		"ssh_credentials": nil,
		"secret":          "omit",
	}
}

// rsyncJobFixture is a job dict as core.get_jobs sends it, with $date times and
// more keys than the tool reports.
func rsyncJobFixture(id any, state string) map[string]any {
	return map[string]any{
		"id":               id,
		"method":           "rsynctask.run",
		"arguments":        []any{"arguments-marker"},
		"state":            state,
		"error":            nil,
		"exception":        "exception-marker",
		"exc_info":         "exc-info-marker",
		"logs_excerpt":     "logs-marker",
		"logs_path":        "/tmp/logs-marker",
		"result":           "result-marker",
		"progress":         map[string]any{"percent": 50},
		"time_started":     map[string]any{"$date": rsyncJobStart.UnixMilli()},
		"time_finished":    map[string]any{"$date": rsyncJobStart.Add(4 * time.Minute).UnixMilli()},
		"credentials_user": "backup",
	}
}

// rsyncMock answers rsynctask.query with rows and records the params of each call.
func rsyncMock(t *testing.T, rows any) (*mockCaller, *[][]interface{}) {
	t.Helper()
	recorded := &[][]interface{}{}
	mock := &mockCaller{CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
		if method != "rsynctask.query" {
			t.Errorf("method = %q, want rsynctask.query", method)
			return nil, errors.New("unexpected method")
		}
		*recorded = append(*recorded, params)
		raw, err := json.Marshal(rows)
		if err != nil {
			t.Errorf("marshal rows: %v", err)
			return nil, err
		}
		return raw, nil
	}}
	return mock, recorded
}

// callRsyncReport runs the tool and returns the decoded report, its raw text and
// the params of every middleware call.
func callRsyncReport(t *testing.T, rows any, args map[string]any) (map[string]any, string, [][]interface{}) {
	t.Helper()
	mock, recorded := rsyncMock(t, rows)
	result, err := callTool(t, mock, true, "truenas_rsync_task_list", args)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatal(resultText(t, result))
	}
	text := resultText(t, result)
	var got map[string]any
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatal(err)
	}
	return got, text, *recorded
}

// rsyncErrorText runs the tool and returns its error text, from the client error
// or from the result.
func rsyncErrorText(t *testing.T, mock *mockCaller, args map[string]any) string {
	t.Helper()
	result, err := callTool(t, mock, true, "truenas_rsync_task_list", args)
	if err != nil {
		return err.Error()
	}
	if result == nil || !result.IsError {
		t.Fatalf("expected an error, got %v", result)
	}
	return resultText(t, result)
}

func TestRsyncTaskList_Normal(t *testing.T) {
	pull := rsyncTaskFixture(3)
	pull["direction"] = "PULL"
	pull["mode"] = "MODULE"
	pull["desc"] = "nightly pull"
	pull["archive"] = true
	pull["job"] = rsyncJobFixture(41, "SUCCESS")

	push := rsyncTaskFixture(9)
	push["path"] = "/mnt/tank/example/site"
	push["dataset"] = "tank/example/site"
	push["direction"] = "PUSH"
	push["mode"] = "SSH"
	push["remoteport"] = 22
	push["remotemodule"] = nil
	push["desc"] = "web push"
	push["schedule"] = map[string]any{"minute": "30", "hour": "3", "dom": "*", "month": "*", "dow": "*", "begin": "00:00", "end": "23:59"}
	push["recursive"] = true
	push["delete"] = true
	push["ssh_credentials"] = rsyncCredentialFixture()

	disabled := rsyncTaskFixture(6)
	disabled["enabled"] = false
	disabled["locked"] = true
	disabled["mode"] = "SSH"
	disabled["remotehost"] = "backup.example.invalid"
	disabled["remoteport"] = 22
	disabled["remotemodule"] = nil
	disabled["ssh_credentials"] = rsyncCredentialFixture()
	disabled["job"] = rsyncJobFixture(nil, "FAILED")
	disabled["job"].(map[string]any)["error"] = "rsync exited with code 23"

	got, _, calls := callRsyncReport(t, []map[string]any{push, disabled, pull}, nil)
	if len(calls) != 1 {
		t.Fatalf("calls = %v", calls)
	}
	if len(calls[0]) != 0 {
		t.Fatalf("params = %v, want none", calls[0])
	}

	expected := `{
 "summary":{"tasks_total":3,"tasks_enabled":2,"disabled_task_ids":[6],"failing_task_ids":[6],"running_task_ids":[],"locked_task_ids":[6],"never_run_task_ids":[9]},
 "tasks":[
 {"id":3,"path":"/mnt/tank/example","dataset":"tank/example","direction":"PULL","mode":"MODULE","remotehost":"backup.example.invalid","remoteport":873,"remotemodule":"archive","remotepath":"/srv/example","user":"backup","desc":"nightly pull","schedule":{"minute":"0","hour":"2","dom":"*","month":"*","dow":"*"},"enabled":true,"locked":false,"recursive":false,"delete":false,"archive":true,"ssh_credentials":null,"job":{"id":41,"state":"SUCCESS","time_started":"2026-11-20T02:00:00Z","time_finished":"2026-11-20T02:04:00Z","error":null}},
 {"id":6,"path":"/mnt/tank/example","dataset":"tank/example","direction":"PULL","mode":"SSH","remotehost":"backup.example.invalid","remoteport":22,"remotemodule":null,"remotepath":"/srv/example","user":"backup","desc":"nightly","schedule":{"minute":"0","hour":"2","dom":"*","month":"*","dow":"*"},"enabled":false,"locked":true,"recursive":false,"delete":false,"archive":false,"ssh_credentials":{"id":4,"name":"backup-key"},"job":{"id":null,"state":"FAILED","time_started":"2026-11-20T02:00:00Z","time_finished":"2026-11-20T02:04:00Z","error":"rsync exited with code 23"}},
 {"id":9,"path":"/mnt/tank/example/site","dataset":"tank/example/site","direction":"PUSH","mode":"SSH","remotehost":"backup.example.invalid","remoteport":22,"remotemodule":null,"remotepath":"/srv/example","user":"backup","desc":"web push","schedule":{"minute":"30","hour":"3","dom":"*","month":"*","dow":"*"},"enabled":true,"locked":false,"recursive":true,"delete":true,"archive":false,"ssh_credentials":{"id":4,"name":"backup-key"},"job":null}],
 "notes":["Schedules run in the Appliance's local time.","job is the running or queued job of the task, else the last finished job TrueNAS stored when the job ended; null means no run is recorded.","locked true means the task path is in a locked encrypted dataset, so TrueNAS skips the run.","An SSH credential shows only its id and name, and extra options are not returned."]}`
	var want map[string]any
	if err := json.Unmarshal([]byte(expected), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("report = %#v, want %#v", got, want)
	}
}

// The middleware entry carries a private key, the job's arguments and logs, and
// an extra option with a password file; none may be in the result text.
func TestRsyncTaskList_NoSecrets(t *testing.T) {
	task := rsyncTaskFixture(6)
	task["mode"] = "SSH"
	task["direction"] = "PUSH"
	task["ssh_credentials"] = rsyncCredentialFixture()
	task["job"] = rsyncJobFixture(12, "SUCCESS")

	got, text, _ := callRsyncReport(t, []map[string]any{task}, nil)
	for _, marker := range []string{
		"PRIVATE KEY",
		"FAKE-KEY-MATERIAL-DO-NOT-LEAK",
		"attributes",
		"remote_host_key",
		"SSH_CREDENTIALS",
		"logs-marker",
		"arguments-marker",
		"exc-info-marker",
		"exception-marker",
		"result-marker",
		"logs_path",
		"progress",
		"password-file",
		"relative_path",
	} {
		if strings.Contains(text, marker) {
			t.Fatalf("result text contains %q:\n%s", marker, text)
		}
	}
	credential := got["tasks"].([]any)[0].(map[string]any)["ssh_credentials"]
	if !reflect.DeepEqual(credential, map[string]any{"id": float64(4), "name": "backup-key"}) {
		t.Fatalf("ssh_credentials = %v, want only id and name", credential)
	}
}

func TestRsyncTaskList_JobStates(t *testing.T) {
	running := rsyncTaskFixture(1)
	running["job"] = map[string]any{"id": 7, "state": "RUNNING", "time_started": map[string]any{"$date": rsyncJobStart.UnixMilli()}}
	waiting := rsyncTaskFixture(2)
	waiting["job"] = rsyncJobFixture(8, "WAITING")
	success := rsyncTaskFixture(3)
	success["job"] = rsyncJobFixture(nil, "SUCCESS")
	failed := rsyncTaskFixture(4)
	failed["job"] = rsyncJobFixture(9, "FAILED")
	aborted := rsyncTaskFixture(5)
	aborted["job"] = rsyncJobFixture(10, "ABORTED")
	never := rsyncTaskFixture(6)
	delete(never, "job")

	got, _, _ := callRsyncReport(t, []map[string]any{never, aborted, failed, success, waiting, running}, nil)
	summary := got["summary"].(map[string]any)
	for key, want := range map[string][]any{
		"failing_task_ids":   {float64(4), float64(5)},
		"running_task_ids":   {float64(1), float64(2)},
		"never_run_task_ids": {float64(6)},
		"locked_task_ids":    {},
		"disabled_task_ids":  {},
	} {
		if !reflect.DeepEqual(summary[key], want) {
			t.Fatalf("%s = %v, want %v", key, summary[key], want)
		}
	}
	tasks := got["tasks"].([]any)
	runningJob := tasks[0].(map[string]any)["job"].(map[string]any)
	if runningJob["id"] != float64(7) || runningJob["state"] != "RUNNING" {
		t.Fatalf("running job = %v", runningJob)
	}
	if runningJob["time_started"] != "2026-11-20T02:00:00Z" || runningJob["time_finished"] != nil || runningJob["error"] != nil {
		t.Fatalf("running job = %v, want a finished-less job with RFC 3339 UTC time", runningJob)
	}
	if !reflect.DeepEqual(tasks[2].(map[string]any)["job"], map[string]any{"id": nil, "state": "SUCCESS", "time_started": "2026-11-20T02:00:00Z", "time_finished": "2026-11-20T02:04:00Z", "error": nil}) {
		t.Fatalf("persisted job = %v, want id null and RFC 3339 UTC times", tasks[2].(map[string]any)["job"])
	}
	if tasks[5].(map[string]any)["job"] != nil {
		t.Fatalf("missing job = %v, want null", tasks[5].(map[string]any)["job"])
	}
}

func TestRsyncTaskList_Locked(t *testing.T) {
	locked := rsyncTaskFixture(1)
	locked["locked"] = true
	unlocked := rsyncTaskFixture(2)
	unlocked["locked"] = false
	unknown := rsyncTaskFixture(3)
	unknown["locked"] = nil

	got, _, _ := callRsyncReport(t, []map[string]any{unknown, unlocked, locked}, nil)
	if !reflect.DeepEqual(got["summary"].(map[string]any)["locked_task_ids"], []any{float64(1)}) {
		t.Fatalf("locked_task_ids = %v, want [1]", got["summary"].(map[string]any)["locked_task_ids"])
	}
	tasks := got["tasks"].([]any)
	for i, want := range []any{true, false, nil} {
		if tasks[i].(map[string]any)["locked"] != want {
			t.Fatalf("task %d locked = %v, want %v", i, tasks[i].(map[string]any)["locked"], want)
		}
	}
}

func TestRsyncTaskList_IDFilter(t *testing.T) {
	found, _, calls := callRsyncReport(t, []map[string]any{rsyncTaskFixture(3)}, map[string]any{"id": 3})
	if len(found["tasks"].([]any)) != 1 {
		t.Fatalf("tasks = %v, want 1", found["tasks"])
	}
	encoded, err := json.Marshal(calls[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `[[["id","=",3]]]` {
		t.Fatalf("params = %s, want [[[\"id\",\"=\",3]]]", encoded)
	}

	// A null id means no filter, as an absent key does.
	if _, _, nullCalls := callRsyncReport(t, []map[string]any{rsyncTaskFixture(3)}, map[string]any{"id": nil}); len(nullCalls) != 1 || len(nullCalls[0]) != 0 {
		t.Fatalf("null id calls = %v, want 1 call with no params", nullCalls)
	}

	missingMock, missingCalls := rsyncMock(t, []map[string]any{})
	text := rsyncErrorText(t, missingMock, map[string]any{"id": 7})
	if !strings.Contains(text, "rsync task 7 not found") {
		t.Fatalf("error = %q, want %q", text, "rsync task 7 not found")
	}
	encoded, err = json.Marshal((*missingCalls)[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `[[["id","=",7]]]` {
		t.Fatalf("params = %s", encoded)
	}

	for _, bad := range []any{"3", 0, -1, 1.5, true, 2147483648} {
		t.Run(fmt.Sprintf("%v", bad), func(t *testing.T) {
			text := rsyncErrorText(t, neverCalled(t), map[string]any{"id": bad})
			if !strings.Contains(text, `parameter "id"`) {
				t.Fatalf("error = %q, want a complaint about the id parameter", text)
			}
		})
	}
}

// A number that json cannot decode into a float64 must not read as an absent id:
// parseArgs drops the decode error, so the filter would disappear and the query
// would return every task instead of an error.
func TestRsyncTaskList_IDDecodeError(t *testing.T) {
	for _, bad := range []json.RawMessage{json.RawMessage("1e309"), json.RawMessage("-1e309")} {
		t.Run(string(bad), func(t *testing.T) {
			args := map[string]any{"id": bad}
			encoded, err := json.Marshal(args)
			if err != nil {
				t.Fatal(err)
			}
			if want := `{"id":` + string(bad) + `}`; string(encoded) != want {
				t.Fatalf("request arguments = %s, want %s", encoded, want)
			}
			mock, calls := rsyncMock(t, []map[string]any{rsyncTaskFixture(3)})
			result, callErr := callTool(t, mock, true, "truenas_rsync_task_list", args)
			if len(*calls) != 0 {
				t.Fatalf("middleware calls = %v, want none", *calls)
			}
			text := ""
			if callErr != nil {
				text = callErr.Error()
			} else if result != nil && result.IsError {
				text = resultText(t, result)
			}
			if !strings.Contains(text, `parameter "id"`) {
				t.Fatalf("error = %q, want a complaint about the id parameter", text)
			}
		})
	}
}

func TestRsyncTaskList_Errors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		raw     string
		failure bool
		want    string
	}{
		{name: "call error", failure: true, want: "rsynctask.query: broken"},
		{name: "parse error", raw: "{", want: "parsing rsynctask.query:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockCaller{CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
				if tc.failure {
					return nil, errors.New("broken")
				}
				return json.RawMessage(tc.raw), nil
			}}
			text := rsyncErrorText(t, mock, nil)
			if !strings.Contains(text, tc.want) {
				t.Fatalf("error = %q, want %q", text, tc.want)
			}
		})
	}
}
