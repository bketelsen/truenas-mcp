package server

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// snapshotTasksFixture is out of id order on purpose. Task 7 failed, task 3
// has a plain string state, and task 5 has no state at all.
const snapshotTasksFixture = `[
  {"id": 7, "dataset": "tank/data", "recursive": true, "lifetime_value": 2, "lifetime_unit": "WEEK",
   "enabled": true, "exclude": ["tank/data/tmp"], "naming_schema": "auto-%Y-%m-%d_%H-%M", "allow_empty": false,
   "schedule": {"minute": "0", "hour": "*/4", "dom": "*", "month": "*", "dow": "*", "begin": "00:00", "end": "23:59"},
   "vmware_sync": false,
   "state": {"state": "ERROR", "datetime": {"$date": 1761220800000}, "last_snapshot": "tank/data@auto-2025-10-23_12-00", "error": "dataset is busy"}},
  {"id": 3, "dataset": "tank/media", "recursive": false, "lifetime_value": 1, "lifetime_unit": "MONTH",
   "enabled": false, "exclude": [], "naming_schema": "daily-%Y%m%d", "allow_empty": true,
   "schedule": {"minute": "30", "hour": "1", "dom": "*", "month": "*", "dow": "*", "begin": "00:00", "end": "23:59"},
   "vmware_sync": false, "state": "PENDING"},
  {"id": 5, "dataset": "tank/apps", "recursive": false, "lifetime_value": 3, "lifetime_unit": "DAY",
   "enabled": true, "exclude": [], "naming_schema": "x-%Y", "allow_empty": true, "schedule": {}, "vmware_sync": false},
  {"id": 9, "dataset": "tank/ok", "recursive": false, "lifetime_value": 1, "lifetime_unit": "YEAR",
   "enabled": true, "exclude": [], "naming_schema": "y-%Y", "allow_empty": true, "schedule": {}, "vmware_sync": false,
   "state": {"state": "FINISHED", "datetime": "2025-10-01T00:00:00Z", "last_snapshot": "tank/ok@y-2025"}}
]`

func decodeReport(t *testing.T, text string) (summary map[string]any, tasks []map[string]any) {
	t.Helper()
	var got struct {
		Summary map[string]any   `json:"summary"`
		Tasks   []map[string]any `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("result is not JSON: %v\n%s", err, text)
	}
	return got.Summary, got.Tasks
}

func TestSnapshotTaskList_Success(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "pool.snapshottask.query" {
				t.Errorf("method = %q, want pool.snapshottask.query", method)
			}
			if len(params) != 0 {
				t.Errorf("expected no params, got %v", params)
			}
			return json.RawMessage(snapshotTasksFixture), nil
		},
	}
	result, err := callTool(t, mock, true, "truenas_snapshot_task_list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", resultText(t, result))
	}
	summary, tasks := decodeReport(t, resultText(t, result))

	if summary["total"] != float64(4) || summary["enabled"] != float64(3) || summary["failing"] != float64(1) {
		t.Errorf("summary = %v, want total 4, enabled 3, failing 1", summary)
	}
	var ids []float64
	for _, task := range tasks {
		ids = append(ids, task["id"].(float64))
	}
	if len(ids) != 4 || ids[0] != 3 || ids[1] != 5 || ids[2] != 7 || ids[3] != 9 {
		t.Fatalf("task ids = %v, want sorted [3 5 7 9]", ids)
	}

	byID := map[float64]map[string]any{}
	for _, task := range tasks {
		byID[task["id"].(float64)] = task
	}

	failed := byID[7]
	if failed["dataset"] != "tank/data" || failed["recursive"] != true || failed["enabled"] != true ||
		failed["naming_schema"] != "auto-%Y-%m-%d_%H-%M" || failed["allow_empty"] != false {
		t.Errorf("task 7 fields wrong: %v", failed)
	}
	retention := failed["retention"].(map[string]any)
	if retention["value"] != float64(2) || retention["unit"] != "WEEK" {
		t.Errorf("retention = %v, want 2 WEEK", retention)
	}
	if ex := failed["exclude"].([]any); len(ex) != 1 || ex[0] != "tank/data/tmp" {
		t.Errorf("exclude = %v", ex)
	}
	if sched := failed["schedule"].(map[string]any); sched["hour"] != "*/4" || sched["begin"] != "00:00" {
		t.Errorf("schedule not passed through: %v", sched)
	}
	if _, leaked := failed["vmware_sync"]; leaked {
		t.Error("unexpected vmware_sync in output")
	}
	lr := failed["last_run"].(map[string]any)
	if lr["state"] != "ERROR" || lr["datetime"] != "2025-10-23T12:00:00Z" ||
		lr["last_snapshot"] != "tank/data@auto-2025-10-23_12-00" || lr["error"] != "dataset is busy" {
		t.Errorf("last_run = %v", lr)
	}

	// A plain string state is handled; absent keys are omitted.
	lr = byID[3]["last_run"].(map[string]any)
	if lr["state"] != "PENDING" || len(lr) != 1 {
		t.Errorf("string state last_run = %v, want only state PENDING", lr)
	}
	// No state at all gives an empty last_run, not a failure.
	if lr := byID[5]["last_run"].(map[string]any); len(lr) != 0 {
		t.Errorf("missing state last_run = %v, want empty", lr)
	}
	// A datetime in another form passes through unchanged.
	if lr := byID[9]["last_run"].(map[string]any); lr["datetime"] != "2025-10-01T00:00:00Z" || lr["state"] != "FINISHED" {
		t.Errorf("passthrough datetime last_run = %v", lr)
	}
}

func TestSnapshotTaskList_DatasetFilter(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "pool.snapshottask.query" {
				t.Errorf("method = %q, want pool.snapshottask.query", method)
			}
			if len(params) != 1 {
				t.Fatalf("expected 1 param, got %v", params)
			}
			filter, ok := params[0].([][]any)
			if !ok {
				t.Fatalf("params[0] is %T, want [][]any", params[0])
			}
			if len(filter) != 1 || filter[0][0] != "dataset" || filter[0][1] != "=" || filter[0][2] != "tank/data" {
				t.Errorf("filter = %v, want [[dataset = tank/data]]", filter)
			}
			return json.RawMessage(`[]`), nil
		},
	}
	result, err := callTool(t, mock, true, "truenas_snapshot_task_list", map[string]any{"dataset": "tank/data"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	summary, tasks := decodeReport(t, resultText(t, result))
	if summary["total"] != float64(0) || len(tasks) != 0 {
		t.Errorf("summary = %v, tasks = %v, want empty", summary, tasks)
	}
	if !strings.Contains(resultText(t, result), `"tasks": []`) {
		t.Errorf("tasks must be an empty array, got: %s", resultText(t, result))
	}
}

func TestSnapshotTaskList_APIError(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			return nil, errors.New("boom")
		},
	}
	result, err := callTool(t, mock, true, "truenas_snapshot_task_list", nil)
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error")
	}
	if err == nil && !strings.Contains(resultText(t, result), "pool.snapshottask.query") {
		t.Errorf("error text = %q, want the method name", resultText(t, result))
	}
}

// replicationFixture holds secrets on purpose: they must not reach the result.
const replicationFixture = `[
  {"id": 4, "name": "tank to backup", "direction": "PUSH", "transport": "SSH",
   "ssh_credentials": {"id": 2, "name": "backup-box", "type": "SSH_CREDENTIALS",
     "attributes": {"host": "backup.lan", "private_key": 12, "PRIVATE KEY": "-----BEGIN OPENSSH PRIVATE KEY-----\nSECRETKEYMATERIAL\n-----END OPENSSH PRIVATE KEY-----"}},
   "source_datasets": ["tank/data"], "target_dataset": "backup/data", "recursive": true, "exclude": ["tank/data/tmp"],
   "periodic_snapshot_tasks": [{"id": 7, "dataset": "tank/data"}, {"id": 3, "dataset": "tank/media"}],
   "naming_schema": ["auto-%Y-%m-%d_%H-%M"], "also_include_naming_schema": ["manual-%Y"],
   "auto": true, "schedule": {"minute": "0", "hour": "2", "dom": "*", "month": "*", "dow": "*"},
   "readonly": "SET", "retention_policy": "CUSTOM", "lifetime_value": 4, "lifetime_unit": "WEEK",
   "encryption": true, "encryption_key": "ENCRYPTIONKEYSECRET", "encryption_key_format": "HEX",
   "encryption_key_location": "/root/keys/tank.key", "enabled": true,
   "state": {"state": "ERROR", "datetime": {"$date": 1761220800000}, "last_snapshot": "tank/data@auto-1", "error": "connection refused", "job": {"id": 99, "state": "FAILED"}},
   "job": {"id": 101, "state": "FAILED", "arguments": ["secret-arg"], "logs_excerpt": "log text"}},
  {"id": 1, "name": "local copy", "direction": "PULL", "transport": "LOCAL", "ssh_credentials": null,
   "source_datasets": ["tank/a"], "target_dataset": "tank/b", "recursive": false, "exclude": [],
   "periodic_snapshot_tasks": [], "naming_schema": [], "also_include_naming_schema": [],
   "auto": false, "schedule": null, "readonly": "IGNORE", "retention_policy": "SOURCE",
   "lifetime_value": null, "lifetime_unit": null, "encryption": false, "enabled": false,
   "state": {"state": "RUNNING", "job": {"id": 55, "state": "RUNNING"}}, "job": null}
]`

func TestReplicationTaskList_Success(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			if method != "replication.query" {
				t.Errorf("method = %q, want replication.query", method)
			}
			if len(params) != 0 {
				t.Errorf("expected no params, got %v", params)
			}
			return json.RawMessage(replicationFixture), nil
		},
	}
	result, err := callTool(t, mock, true, "truenas_replication_task_list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", resultText(t, result))
	}
	text := resultText(t, result)
	summary, tasks := decodeReport(t, text)

	if summary["total"] != float64(2) || summary["enabled"] != float64(1) || summary["failing"] != float64(1) {
		t.Errorf("summary = %v, want total 2, enabled 1, failing 1", summary)
	}
	if len(tasks) != 2 || tasks[0]["id"] != float64(1) || tasks[1]["id"] != float64(4) {
		t.Fatalf("tasks not sorted by id: %v", tasks)
	}

	push := tasks[1]
	if push["name"] != "tank to backup" || push["direction"] != "PUSH" || push["transport"] != "SSH" ||
		push["target_dataset"] != "backup/data" || push["recursive"] != true || push["enabled"] != true ||
		push["auto"] != true || push["readonly"] != "SET" || push["encryption"] != true {
		t.Errorf("push task fields wrong: %v", push)
	}
	if src := push["source_datasets"].([]any); len(src) != 1 || src[0] != "tank/data" {
		t.Errorf("source_datasets = %v", src)
	}
	if ids := push["periodic_snapshot_task_ids"].([]any); len(ids) != 2 || ids[0] != float64(7) || ids[1] != float64(3) {
		t.Errorf("periodic_snapshot_task_ids = %v, want [7 3]", ids)
	}
	if ns := push["naming_schema"].([]any); len(ns) != 1 || ns[0] != "auto-%Y-%m-%d_%H-%M" {
		t.Errorf("naming_schema = %v", ns)
	}
	if ns := push["also_include_naming_schema"].([]any); len(ns) != 1 || ns[0] != "manual-%Y" {
		t.Errorf("also_include_naming_schema = %v", ns)
	}
	ret := push["retention"].(map[string]any)
	if ret["policy"] != "CUSTOM" || ret["value"] != float64(4) || ret["unit"] != "WEEK" {
		t.Errorf("retention = %v", ret)
	}
	cred := push["ssh_credentials"].(map[string]any)
	if len(cred) != 2 || cred["id"] != float64(2) || cred["name"] != "backup-box" {
		t.Errorf("ssh_credentials = %v, want only id and name", cred)
	}
	if sched := push["schedule"].(map[string]any); sched["hour"] != "2" {
		t.Errorf("schedule = %v", sched)
	}
	lr := push["last_run"].(map[string]any)
	if lr["state"] != "ERROR" || lr["datetime"] != "2025-10-23T12:00:00Z" || lr["last_snapshot"] != "tank/data@auto-1" ||
		lr["error"] != "connection refused" {
		t.Errorf("last_run = %v", lr)
	}
	// The top-level job wins; only id and state are reported.
	job := lr["job"].(map[string]any)
	if len(job) != 2 || job["id"] != float64(101) || job["state"] != "FAILED" {
		t.Errorf("last_run.job = %v, want id 101 and state FAILED only", job)
	}

	local := tasks[0]
	if local["ssh_credentials"] != nil || local["schedule"] != nil {
		t.Errorf("null ssh_credentials and schedule must stay null: %v", local)
	}
	if local["encryption"] != false {
		t.Errorf("encryption = %v, want false", local["encryption"])
	}
	if ids := local["periodic_snapshot_task_ids"].([]any); len(ids) != 0 {
		t.Errorf("periodic_snapshot_task_ids = %v, want empty", ids)
	}
	ret = local["retention"].(map[string]any)
	if ret["policy"] != "SOURCE" || ret["value"] != nil || ret["unit"] != nil {
		t.Errorf("retention = %v", ret)
	}
	// With a null top-level job, state.job is used.
	if job := local["last_run"].(map[string]any)["job"].(map[string]any); job["id"] != float64(55) || job["state"] != "RUNNING" {
		t.Errorf("state.job summary = %v", job)
	}
}

func TestReplicationTaskList_NoSecretLeak(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			return json.RawMessage(replicationFixture), nil
		},
	}
	result, err := callTool(t, mock, true, "truenas_replication_task_list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	for _, secret := range []string{
		"SECRETKEYMATERIAL", "BEGIN OPENSSH PRIVATE KEY", "PRIVATE KEY", "private_key",
		"ENCRYPTIONKEYSECRET", "encryption_key", "/root/keys/tank.key",
		"attributes", "backup.lan", "secret-arg", "log text",
	} {
		if strings.Contains(text, secret) {
			t.Errorf("result leaks %q:\n%s", secret, text)
		}
	}
}

func TestReplicationTaskList_APIError(t *testing.T) {
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			return nil, errors.New("boom")
		},
	}
	result, err := callTool(t, mock, true, "truenas_replication_task_list", nil)
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("expected error")
	}
	if err == nil && !strings.Contains(resultText(t, result), "replication.query") {
		t.Errorf("error text = %q, want the method name", resultText(t, result))
	}
}

func TestWireDatetime(t *testing.T) {
	if got := wireDatetime(map[string]any{"$date": float64(1761220800000)}); got != "2025-10-23T12:00:00Z" {
		t.Errorf("got %v", got)
	}
	for _, in := range []any{"2025-10-01", nil, float64(5), map[string]any{"$date": "bad"}, map[string]any{"x": 1}} {
		got := wireDatetime(in)
		a, _ := json.Marshal(got)
		b, _ := json.Marshal(in)
		if string(a) != string(b) {
			t.Errorf("wireDatetime(%v) = %v, want unchanged", in, got)
		}
	}
}
