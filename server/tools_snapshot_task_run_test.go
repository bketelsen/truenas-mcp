package server

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSnapshotRun is a fake middleware for truenas_snapshot_task_run. Before
// pool.snapshottask.run it reports the task with its initial state; after the
// run, each pool.snapshottask.query takes the next entry of states (the last one
// repeats). It records every call.
type fakeSnapshotRun struct {
	mu sync.Mutex

	task         map[string]any // nil: no such task
	replications string
	runResult    string
	runErr       error
	states       []map[string]any
	jobs         []string // core.get_jobs answers, in order; the last one repeats
	snapshots    map[string]string
	poolSnapGone bool // pool.snapshot.query answers "method does not exist"
	// preTasks, when set, replace the task for each query before the run, in
	// order (the last one repeats); a nil entry means the task is gone.
	preTasks []map[string]any
	// onCall, when set, runs before each answer, outside the lock; it can
	// cancel a context or block.
	onCall func(method string)

	methods   []string
	runParams [][]any
	ran       bool
	statePoll int
	preQuery  int
	jobPoll   int
	snapQuery []string // the method names of the snapshot queries
}

func (f *fakeSnapshotRun) runCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.runParams)
}

func (f *fakeSnapshotRun) caller() *mockCaller {
	return &mockCaller{CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
		if f.onCall != nil {
			f.onCall(method)
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.methods = append(f.methods, method)
		switch method {
		case "pool.snapshottask.query":
			base := f.task
			if !f.ran && len(f.preTasks) > 0 {
				base = f.preTasks[min(f.preQuery, len(f.preTasks)-1)]
				f.preQuery++
			}
			if base == nil {
				return json.RawMessage(`[]`), nil
			}
			task := map[string]any{}
			for k, v := range base {
				task[k] = v
			}
			if f.ran && len(f.states) > 0 {
				i := min(f.statePoll, len(f.states)-1)
				f.statePoll++
				task["state"] = f.states[i]
			}
			return json.Marshal([]any{task})
		case "replication.query":
			if f.replications == "" {
				return json.RawMessage(`[]`), nil
			}
			return json.RawMessage(f.replications), nil
		case "pool.snapshottask.run":
			f.runParams = append(f.runParams, params)
			f.ran = true
			if f.runErr != nil {
				return nil, f.runErr
			}
			return json.RawMessage(f.runResult), nil
		case "core.get_jobs":
			i := min(f.jobPoll, len(f.jobs)-1)
			f.jobPoll++
			return json.RawMessage(f.jobs[i]), nil
		case "pool.snapshot.query", "zfs.snapshot.query":
			f.snapQuery = append(f.snapQuery, method)
			if method == "pool.snapshot.query" && f.poolSnapGone {
				return nil, errors.New("[ENOMETHOD] Method does not exist")
			}
			filter, _ := params[0].([][]any)
			name, _ := filter[0][2].(string)
			if entry, ok := f.snapshots[name]; ok {
				return json.RawMessage("[" + entry + "]"), nil
			}
			return json.RawMessage(`[]`), nil
		}
		return nil, errors.New("unexpected method " + method)
	}}
}

// The state of task 7 before the run: it ran once on schedule.
var beforeState = map[string]any{
	"state":         "FINISHED",
	"datetime":      map[string]any{"$date": float64(1700000000000)},
	"last_snapshot": "tank/data@auto-2023-11-14_22-13",
}

func runTask() map[string]any {
	return map[string]any{
		"id": float64(7), "dataset": "tank/data", "recursive": true, "exclude": []any{},
		"lifetime_value": float64(2), "lifetime_unit": "WEEK", "enabled": true,
		"naming_schema": "auto-%Y-%m-%d_%H-%M", "allow_empty": true,
		"schedule": map[string]any{"minute": "0", "hour": "*", "dom": "*", "month": "*", "dow": "*", "begin": "00:00", "end": "23:59"},
		"state":    beforeState,
	}
}

// runReplications holds 1 replication the run starts (id 3) and 5 it does not.
// Entry 3 carries secrets that must never reach a result.
const runReplications = `[
  {"id": 4, "name": "own-schedule", "enabled": true, "auto": true, "schedule": {"minute": "0"},
   "periodic_snapshot_tasks": [{"id": 7}], "direction": "PUSH", "transport": "SSH", "target_dataset": "b/x"},
  {"id": 3, "name": "to-backup", "enabled": true, "auto": true, "schedule": null,
   "periodic_snapshot_tasks": [{"id": 9}, {"id": 7}], "direction": "PUSH", "transport": "SSH",
   "target_dataset": "backup/data", "encryption_key": "SECRET-ENC-KEY",
   "ssh_credentials": {"id": 1, "name": "nas2", "attributes": {"private_key": "-----BEGIN PRIVATE KEY-----"}}},
  {"id": 5, "name": "disabled", "enabled": false, "auto": true, "schedule": null,
   "periodic_snapshot_tasks": [{"id": 7}], "direction": "PUSH", "transport": "LOCAL", "target_dataset": "b/y"},
  {"id": 6, "name": "manual", "enabled": true, "auto": false, "schedule": null,
   "periodic_snapshot_tasks": [{"id": 7}], "direction": "PUSH", "transport": "LOCAL", "target_dataset": "b/z"},
  {"id": 8, "name": "other-task", "enabled": true, "auto": true, "schedule": null,
   "periodic_snapshot_tasks": [{"id": 70}], "direction": "PUSH", "transport": "LOCAL", "target_dataset": "b/w"}
]`

const newSnapshot = "tank/data@auto-2023-11-15_09-30"

func newSnapshotEntry(name string) string {
	return `{"id": "` + name + `", "name": "` + name + `", "dataset": "tank/data",
  "properties": {"creation": {"parsed": {"$date": 1700040600000}}}}`
}

func finished(datetime float64, snapshot string) map[string]any {
	return map[string]any{"state": "FINISHED", "datetime": map[string]any{"$date": datetime}, "last_snapshot": snapshot}
}

func running(datetime float64) map[string]any {
	return map[string]any{"state": "RUNNING", "datetime": map[string]any{"$date": datetime},
		"last_snapshot": beforeState["last_snapshot"]}
}

// fastSnapshotRun makes the state and job polls take milliseconds.
func fastSnapshotRun(t *testing.T, wait time.Duration) {
	t.Helper()
	oldPoll, oldWait := jobPollInterval, snapshotRunWait
	jobPollInterval = time.Millisecond
	snapshotRunWait = wait
	t.Cleanup(func() { jobPollInterval, snapshotRunWait = oldPoll, oldWait })
}

func newRunFake() *fakeSnapshotRun {
	return &fakeSnapshotRun{
		task:         runTask(),
		replications: runReplications,
		runResult:    `null`,
		states:       []map[string]any{running(1700040600000), finished(1700040601000, newSnapshot)},
		snapshots:    map[string]string{newSnapshot: newSnapshotEntry(newSnapshot)},
	}
}

// TestSnapshotTaskRun_DescriptionOffScheduleRetention keeps the warning that
// zettarepl retention never prunes a snapshot whose time is off the task's
// schedule, scoped to the task that made it: another task with the same
// dataset, naming schema and a matching schedule can prune it (issue #70).
func TestSnapshotTaskRun_DescriptionOffScheduleRetention(t *testing.T) {
	for _, tool := range listToolDefs(t, newRunFake().caller(), false) {
		if tool.Name != "truenas_snapshot_task_run" {
			continue
		}
		if !strings.Contains(tool.Description, "this task's retention never deletes its snapshot") ||
			!strings.Contains(tool.Description, "Another task on that dataset with the same naming schema") {
			t.Errorf("description lacks the off-schedule retention warning: %q", tool.Description)
		}
		return
	}
	t.Fatal("truenas_snapshot_task_run is not registered")
}

func TestSnapshotTaskRun_DryRunDefault(t *testing.T) {
	for _, args := range []map[string]any{
		{"id": 7},
		{"id": 7, "confirm": false},
	} {
		f := newRunFake()
		result, err := callTool(t, f.caller(), false, "truenas_snapshot_task_run", args)
		if err != nil || result.IsError {
			t.Fatalf("args %v: unexpected error: %v %v", args, err, result)
		}
		if f.runCalls() != 0 {
			t.Fatalf("args %v: pool.snapshottask.run called %d times on a dry run", args, f.runCalls())
		}
		out := resultJSON(t, resultText(t, result))
		if out["dry_run"] != true || out["method"] != "pool.snapshottask.run" {
			t.Errorf("dry run header = %v %v", out["dry_run"], out["method"])
		}
		assertSameJSON(t, out["params"], `[7]`)
		task, _ := out["task"].(map[string]any)
		if task["id"] != float64(7) || task["dataset"] != "tank/data" {
			t.Errorf("task summary = %v", task)
		}
		assertSameJSON(t, out["starts_replication"], `[{"id": 3, "name": "to-backup", "direction": "PUSH",
		  "transport": "SSH", "target_dataset": "backup/data"}]`)
		note, _ := out["note"].(string)
		if !strings.Contains(note, "confirm") || !strings.Contains(note, "retention") ||
			!strings.Contains(note, "this task's retention does not delete") ||
			!strings.Contains(note, "another task on that dataset with the same naming schema") {
			t.Errorf("note = %q", note)
		}
	}
}

func TestSnapshotTaskRun_NoSecretLeak(t *testing.T) {
	f := newRunFake()
	fastSnapshotRun(t, time.Second)
	for _, confirm := range []bool{false, true} {
		result, err := callTool(t, f.caller(), false, "truenas_snapshot_task_run", map[string]any{"id": 7, "confirm": confirm})
		if err != nil || result.IsError {
			t.Fatalf("unexpected error: %v %v", err, result)
		}
		text := resultText(t, result)
		for _, secret := range []string{"SECRET-ENC-KEY", "PRIVATE KEY", "private_key", "encryption_key", "ssh_credentials"} {
			if strings.Contains(text, secret) {
				t.Errorf("confirm=%v: result contains %q:\n%s", confirm, secret, text)
			}
		}
	}
}

func TestSnapshotTaskRun_ArgumentErrors(t *testing.T) {
	for _, args := range []map[string]any{
		{},
		{"id": 0},
		{"id": -3},
		{"id": 1.5},
		{"id": "7"},
		{"id": 7, "confirm": "true"},
		{"id": 7, "confirm": 1},
		{"id": 7, "confirm": true, "timeout_seconds": 5},
		{"id": 7, "confirm": true, "timeout_seconds": 601},
		{"id": 7, "confirm": true, "timeout_seconds": "60"},
	} {
		f := newRunFake()
		result, err := callTool(t, f.caller(), false, "truenas_snapshot_task_run", args)
		toolError(t, result, err)
		if f.runCalls() != 0 {
			t.Errorf("args %v: pool.snapshottask.run called", args)
		}
	}
}

func TestSnapshotTaskRun_TaskNotFound(t *testing.T) {
	for _, confirm := range []bool{false, true} {
		f := newRunFake()
		f.task = nil
		result, err := callTool(t, f.caller(), false, "truenas_snapshot_task_run", map[string]any{"id": 7, "confirm": confirm})
		msg := toolError(t, result, err)
		if !strings.Contains(msg, "no periodic snapshot task with id 7") || !strings.Contains(msg, "nothing was run") {
			t.Errorf("error = %q", msg)
		}
		if f.runCalls() != 0 {
			t.Error("pool.snapshottask.run called")
		}
	}
}

func TestSnapshotTaskRun_TaskDisabled(t *testing.T) {
	for _, confirm := range []bool{false, true} {
		f := newRunFake()
		f.task["enabled"] = false
		result, err := callTool(t, f.caller(), false, "truenas_snapshot_task_run", map[string]any{"id": 7, "confirm": confirm})
		msg := toolError(t, result, err)
		if !strings.Contains(msg, "is disabled") || !strings.Contains(msg, "tank/data") || !strings.Contains(msg, "nothing was run") {
			t.Errorf("confirm=%v: error = %q", confirm, msg)
		}
		if f.runCalls() != 0 {
			t.Error("pool.snapshottask.run called")
		}
	}
}

func TestSnapshotTaskRun_ReplicationQueryError(t *testing.T) {
	f := newRunFake()
	base := f.caller()
	mock := &mockCaller{CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
		if method == "replication.query" {
			return nil, errors.New("connection lost")
		}
		return base.Call(method, params...)
	}}
	result, err := callTool(t, mock, false, "truenas_snapshot_task_run", map[string]any{"id": 7, "confirm": true})
	msg := toolError(t, result, err)
	if !strings.Contains(msg, "replication.query") || !strings.Contains(msg, "nothing was run") {
		t.Errorf("error = %q", msg)
	}
	if f.runCalls() != 0 {
		t.Error("pool.snapshottask.run called")
	}
}

func TestSnapshotTaskRun_NullResultVerified(t *testing.T) {
	f := newRunFake()
	fastSnapshotRun(t, time.Second)
	result, err := callTool(t, f.caller(), false, "truenas_snapshot_task_run", map[string]any{"id": 7, "confirm": true})
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v %v", err, result)
	}
	if f.runCalls() != 1 {
		t.Fatalf("pool.snapshottask.run called %d times, want 1", f.runCalls())
	}
	if !reflect.DeepEqual(f.runParams[0], []any{int64(7)}) {
		t.Errorf("run params = %#v, want [7]", f.runParams[0])
	}
	out := resultJSON(t, resultText(t, result))
	if out["ran"] != true || out["snapshot"] != newSnapshot || out["verified"] != true || out["already_existed"] != false {
		t.Errorf("result = %v", out)
	}
	if out["task_id"] != float64(7) || out["dataset"] != "tank/data" || out["recursive"] != true {
		t.Errorf("task fields = %v", out)
	}
	if out["created"] != float64(1700040600) {
		t.Errorf("created = %v, want 1700040600", out["created"])
	}
	lr, _ := out["last_run"].(map[string]any)
	if lr["state"] != "FINISHED" || lr["last_snapshot"] != newSnapshot {
		t.Errorf("last_run = %v", lr)
	}
	if note, _ := out["note"].(string); !strings.Contains(note, "child datasets") {
		t.Errorf("note = %q, want the recursive note", note)
	}
	assertSameJSON(t, out["starts_replication"], `[{"id": 3, "name": "to-backup", "direction": "PUSH",
	  "transport": "SSH", "target_dataset": "backup/data"}]`)
}

func TestSnapshotTaskRun_NeverRanBefore(t *testing.T) {
	f := newRunFake()
	delete(f.task, "state")
	f.states = []map[string]any{finished(1700040601000, newSnapshot)}
	fastSnapshotRun(t, time.Second)
	result, err := callTool(t, f.caller(), false, "truenas_snapshot_task_run", map[string]any{"id": 7, "confirm": true})
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v %v", err, result)
	}
	if out := resultJSON(t, resultText(t, result)); out["snapshot"] != newSnapshot || out["already_existed"] != false {
		t.Errorf("result = %v", out)
	}
}

func TestSnapshotTaskRun_AlreadyExisted(t *testing.T) {
	f := newRunFake()
	old := beforeState["last_snapshot"].(string)
	f.states = []map[string]any{finished(1700040601000, old)}
	f.snapshots = map[string]string{old: newSnapshotEntry(old)}
	fastSnapshotRun(t, time.Second)
	result, err := callTool(t, f.caller(), false, "truenas_snapshot_task_run", map[string]any{"id": 7, "confirm": true})
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v %v", err, result)
	}
	out := resultJSON(t, resultText(t, result))
	if out["already_existed"] != true || out["snapshot"] != old || out["verified"] != true {
		t.Errorf("result = %v", out)
	}
	if note, _ := out["note"].(string); !strings.Contains(note, "no new snapshot") {
		t.Errorf("note = %q", note)
	}
	if f.runCalls() != 1 {
		t.Errorf("pool.snapshottask.run called %d times, want 1", f.runCalls())
	}
}

func TestSnapshotTaskRun_StateError(t *testing.T) {
	f := newRunFake()
	f.states = []map[string]any{running(1700040600000), {
		"state": "ERROR", "datetime": map[string]any{"$date": float64(1700040601000)},
		"last_snapshot": beforeState["last_snapshot"], "error": "cannot create snapshot: out of space",
	}}
	fastSnapshotRun(t, time.Second)
	result, err := callTool(t, f.caller(), false, "truenas_snapshot_task_run", map[string]any{"id": 7, "confirm": true})
	msg := toolError(t, result, err)
	if !strings.Contains(msg, "out of space") || !strings.Contains(msg, "no verified snapshot") {
		t.Errorf("error = %q", msg)
	}
	if f.runCalls() != 1 {
		t.Errorf("pool.snapshottask.run called %d times, want 1", f.runCalls())
	}
}

func TestSnapshotTaskRun_RunCallError(t *testing.T) {
	f := newRunFake()
	f.runErr = errors.New("websocket closed")
	fastSnapshotRun(t, time.Second)
	result, err := callTool(t, f.caller(), false, "truenas_snapshot_task_run", map[string]any{"id": 7, "confirm": true})
	msg := toolError(t, result, err)
	if !strings.Contains(msg, "websocket closed") || !strings.Contains(msg, "truenas_snapshot_task_list") {
		t.Errorf("error = %q", msg)
	}
	if f.runCalls() != 1 {
		t.Errorf("pool.snapshottask.run called %d times, want exactly 1", f.runCalls())
	}
}

func TestSnapshotTaskRun_Timeout(t *testing.T) {
	f := newRunFake()
	f.states = []map[string]any{beforeState} // the state never changes
	fastSnapshotRun(t, 50*time.Millisecond)
	result, err := callTool(t, f.caller(), false, "truenas_snapshot_task_run", map[string]any{"id": 7, "confirm": true})
	msg := toolError(t, result, err)
	if !strings.Contains(msg, "no finished run was seen") || !strings.Contains(msg, "do not run this tool again") {
		t.Errorf("error = %q", msg)
	}
	if f.runCalls() != 1 {
		t.Errorf("pool.snapshottask.run called %d times, want exactly 1", f.runCalls())
	}
}

func TestSnapshotTaskRun_RunningUntilTimeout(t *testing.T) {
	f := newRunFake()
	f.states = []map[string]any{running(1700040600000)}
	fastSnapshotRun(t, 50*time.Millisecond)
	result, err := callTool(t, f.caller(), false, "truenas_snapshot_task_run", map[string]any{"id": 7, "confirm": true})
	msg := toolError(t, result, err)
	if !strings.Contains(msg, "no finished run was seen") {
		t.Errorf("error = %q", msg)
	}
	if f.runCalls() != 1 {
		t.Errorf("pool.snapshottask.run called %d times, want exactly 1", f.runCalls())
	}
}

func TestSnapshotTaskRun_JobSuccess(t *testing.T) {
	f := newRunFake()
	f.runResult = `42`
	f.jobs = []string{`[{"id": 42, "state": "RUNNING"}]`, `[{"id": 42, "state": "SUCCESS"}]`}
	fastSnapshotRun(t, time.Second)
	result, err := callTool(t, f.caller(), false, "truenas_snapshot_task_run", map[string]any{"id": 7, "confirm": true})
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v %v", err, result)
	}
	out := resultJSON(t, resultText(t, result))
	if out["snapshot"] != newSnapshot || out["verified"] != true {
		t.Errorf("result = %v", out)
	}
	if f.jobPoll < 2 {
		t.Errorf("core.get_jobs polled %d times, want at least 2", f.jobPoll)
	}
	if f.runCalls() != 1 {
		t.Errorf("pool.snapshottask.run called %d times, want 1", f.runCalls())
	}
}

func TestSnapshotTaskRun_JobFailedAlreadyExisted(t *testing.T) {
	f := newRunFake()
	f.runResult = `42`
	f.jobs = []string{`[{"id": 42, "state": "FAILED",
	  "error": "[EEXIST] Snapshot tank/data@auto-2023-11-15_09-30 already existed. This probably happened because the task already ran on schedule."}]`}
	fastSnapshotRun(t, time.Second)
	result, err := callTool(t, f.caller(), false, "truenas_snapshot_task_run", map[string]any{"id": 7, "confirm": true})
	msg := toolError(t, result, err)
	if !strings.Contains(msg, "already existed") || !strings.Contains(msg, "nothing new was made") {
		t.Errorf("error = %q", msg)
	}
	if f.runCalls() != 1 {
		t.Errorf("pool.snapshottask.run called %d times, want 1", f.runCalls())
	}
}

func TestSnapshotTaskRun_JobFailedOther(t *testing.T) {
	f := newRunFake()
	f.runResult = `42`
	f.jobs = []string{`[{"id": 42, "state": "FAILED", "error": "dataset is busy"}]`}
	fastSnapshotRun(t, time.Second)
	result, err := callTool(t, f.caller(), false, "truenas_snapshot_task_run", map[string]any{"id": 7, "confirm": true})
	msg := toolError(t, result, err)
	if !strings.Contains(msg, "dataset is busy") || !strings.Contains(msg, "the run failed") || strings.Contains(msg, "nothing new") {
		t.Errorf("error = %q", msg)
	}
}

func TestSnapshotTaskRun_JobTimeout(t *testing.T) {
	f := newRunFake()
	f.runResult = `42`
	f.jobs = []string{`[{"id": 42, "state": "RUNNING"}]`}
	fastSnapshotRun(t, 50*time.Millisecond)
	result, err := callTool(t, f.caller(), false, "truenas_snapshot_task_run", map[string]any{"id": 7, "confirm": true})
	msg := toolError(t, result, err)
	if !strings.Contains(msg, "unknown") || !strings.Contains(msg, "truenas_snapshot_task_list") {
		t.Errorf("error = %q", msg)
	}
	if f.runCalls() != 1 {
		t.Errorf("pool.snapshottask.run called %d times, want 1", f.runCalls())
	}
}

func TestSnapshotTaskRun_SnapshotNotFound(t *testing.T) {
	f := newRunFake()
	f.snapshots = map[string]string{}
	fastSnapshotRun(t, time.Second)
	result, err := callTool(t, f.caller(), false, "truenas_snapshot_task_run", map[string]any{"id": 7, "confirm": true})
	msg := toolError(t, result, err)
	if !strings.Contains(msg, newSnapshot) || !strings.Contains(msg, "not verified") {
		t.Errorf("error = %q", msg)
	}
}

func TestSnapshotTaskRun_SnapshotQueryFallback(t *testing.T) {
	f := newRunFake()
	f.poolSnapGone = true
	fastSnapshotRun(t, time.Second)
	result, err := callTool(t, f.caller(), false, "truenas_snapshot_task_run", map[string]any{"id": 7, "confirm": true})
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v %v", err, result)
	}
	if !reflect.DeepEqual(f.snapQuery, []string{"pool.snapshot.query", "zfs.snapshot.query"}) {
		t.Errorf("snapshot queries = %v", f.snapQuery)
	}
	if out := resultJSON(t, resultText(t, result)); out["verified"] != true {
		t.Errorf("result = %v", out)
	}
}

func TestSnapshotTaskRun_RegistrationMode(t *testing.T) {
	f := newRunFake()
	for _, name := range listTools(t, f.caller(), true) {
		if name == "truenas_snapshot_task_run" {
			t.Fatal("truenas_snapshot_task_run registered on a read-only server")
		}
	}
	found := false
	for _, name := range listTools(t, f.caller(), false) {
		found = found || name == "truenas_snapshot_task_run"
	}
	if !found {
		t.Fatal("truenas_snapshot_task_run not registered with writes enabled")
	}
}

// withState returns a copy of the task of runTask with another state.
func withState(state map[string]any) map[string]any {
	task := runTask()
	task["state"] = state
	return task
}

func TestSnapshotTaskRun_CancelBeforeRun(t *testing.T) {
	f := newRunFake()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	f.onCall = func(method string) {
		if method == "replication.query" {
			cancel()
		}
	}
	fastSnapshotRun(t, time.Second)
	_, err := runSnapshotTask(ctx, f.caller(), makeReq(t, map[string]any{"id": 7, "confirm": true}))
	if err == nil || !strings.Contains(err.Error(), "cancelled before the run") || !strings.Contains(err.Error(), "nothing was run") {
		t.Fatalf("error = %v", err)
	}
	if f.runCalls() != 0 {
		t.Errorf("pool.snapshottask.run called %d times after a cancel, want 0", f.runCalls())
	}
}

func TestSnapshotTaskRun_FreshBaselineIgnoresEarlierRun(t *testing.T) {
	// A scheduled run finished between the preflight query and the call. The
	// poll keeps showing that run; it is not the result of this call.
	f := newRunFake()
	scheduled := finished(1700040700000, "tank/data@auto-2023-11-15_09-31")
	f.preTasks = []map[string]any{runTask(), withState(scheduled)}
	f.states = []map[string]any{scheduled}
	f.snapshots = map[string]string{
		"tank/data@auto-2023-11-15_09-31": newSnapshotEntry("tank/data@auto-2023-11-15_09-31"),
	}
	fastSnapshotRun(t, 50*time.Millisecond)
	result, err := callTool(t, f.caller(), false, "truenas_snapshot_task_run", map[string]any{"id": 7, "confirm": true})
	msg := toolError(t, result, err)
	if !strings.Contains(msg, "no finished run was seen") {
		t.Errorf("error = %q", msg)
	}
	if f.runCalls() != 1 {
		t.Errorf("pool.snapshottask.run called %d times, want 1", f.runCalls())
	}
}

func TestSnapshotTaskRun_FreshBaselineNewerRun(t *testing.T) {
	// With the fresh baseline, a run after it is still reported.
	f := newRunFake()
	scheduled := finished(1700040700000, "tank/data@auto-2023-11-15_09-31")
	f.preTasks = []map[string]any{runTask(), withState(scheduled)}
	f.states = []map[string]any{scheduled, finished(1700040800000, newSnapshot)}
	fastSnapshotRun(t, time.Second)
	result, err := callTool(t, f.caller(), false, "truenas_snapshot_task_run", map[string]any{"id": 7, "confirm": true})
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v %v", err, result)
	}
	if out := resultJSON(t, resultText(t, result)); out["snapshot"] != newSnapshot || out["already_existed"] != false {
		t.Errorf("result = %v", out)
	}
}

func TestSnapshotTaskRun_RunningAtCall(t *testing.T) {
	f := newRunFake()
	f.preTasks = []map[string]any{runTask(), withState(running(1700040700000))}
	fastSnapshotRun(t, time.Second)
	result, err := callTool(t, f.caller(), false, "truenas_snapshot_task_run", map[string]any{"id": 7, "confirm": true})
	msg := toolError(t, result, err)
	if !strings.Contains(msg, "is running now") || !strings.Contains(msg, "nothing was run") {
		t.Errorf("error = %q", msg)
	}
	if f.runCalls() != 0 {
		t.Errorf("pool.snapshottask.run called %d times, want 0", f.runCalls())
	}
}

func TestSnapshotTaskRun_GoneOrDisabledAtCall(t *testing.T) {
	disabled := runTask()
	disabled["enabled"] = false
	for name, second := range map[string]map[string]any{"gone": nil, "disabled": disabled} {
		f := newRunFake()
		f.preTasks = []map[string]any{runTask(), second}
		fastSnapshotRun(t, time.Second)
		result, err := callTool(t, f.caller(), false, "truenas_snapshot_task_run", map[string]any{"id": 7, "confirm": true})
		if msg := toolError(t, result, err); !strings.Contains(msg, "nothing was run") {
			t.Errorf("%s: error = %q", name, msg)
		}
		if f.runCalls() != 0 {
			t.Errorf("%s: pool.snapshottask.run called %d times, want 0", name, f.runCalls())
		}
	}
}

// blockSnapshotQuery makes every snapshot query block until the test ends.
// before runs first, outside the fake's lock.
func blockSnapshotQuery(t *testing.T, f *fakeSnapshotRun, before func()) {
	t.Helper()
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	f.onCall = func(method string) {
		if method == "pool.snapshot.query" || method == "zfs.snapshot.query" {
			if before != nil {
				before()
			}
			<-release
		}
	}
}

func TestSnapshotTaskRun_VerifyCancelled(t *testing.T) {
	f := newRunFake()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	blockSnapshotQuery(t, f, cancel)
	fastSnapshotRun(t, time.Second)
	_, err := runSnapshotTask(ctx, f.caller(), makeReq(t, map[string]any{"id": 7, "confirm": true}))
	if err == nil || !strings.Contains(err.Error(), "was not verified") || !strings.Contains(err.Error(), "truenas_snapshot_list") {
		t.Fatalf("error = %v, want a not verified error", err)
	}
	if f.runCalls() != 1 {
		t.Errorf("pool.snapshottask.run called %d times, want 1", f.runCalls())
	}
}

func TestSnapshotTaskRun_VerifyPastDeadline(t *testing.T) {
	f := newRunFake()
	blockSnapshotQuery(t, f, nil)
	fastSnapshotRun(t, 100*time.Millisecond)
	oldFloor := snapshotVerifyFloor
	snapshotVerifyFloor = 20 * time.Millisecond
	t.Cleanup(func() { snapshotVerifyFloor = oldFloor })
	result, err := callTool(t, f.caller(), false, "truenas_snapshot_task_run", map[string]any{"id": 7, "confirm": true})
	msg := toolError(t, result, err)
	if !strings.Contains(msg, "was not verified") || !strings.Contains(msg, "truenas_snapshot_list") {
		t.Errorf("error = %q", msg)
	}
	if f.runCalls() != 1 {
		t.Errorf("pool.snapshottask.run called %d times, want 1", f.runCalls())
	}
}
