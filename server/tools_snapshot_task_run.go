package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"truenas-mcp/truenas"
)

// snapshotRunWait, when not zero, replaces the timeout_seconds argument of
// truenas_snapshot_task_run. Tests set it, because the smallest allowed
// timeout_seconds is 10 seconds and a run that never finishes would stall them.
// Nothing sets it outside tests.
var snapshotRunWait time.Duration

const (
	snapshotRunTimeoutDefault = 120
	snapshotRunTimeoutMin     = 10
	snapshotRunTimeoutMax     = 600
)

// registerSnapshotTaskRunTool registers truenas_snapshot_task_run, which runs
// one existing periodic snapshot task now ("Run Now" in the TrueNAS UI).
//
// pool.snapshottask.run needs SNAPSHOT_TASK_WRITE. It has 2 forms:
//   - TS-25.10.x and TS-27.0.0-RC.1: not a job. It queues the run in zettarepl
//     and returns null at once; the snapshot is made later.
//   - middleware master: a job. Over the websocket it returns the job id, and the
//     job fails with EEXIST when the snapshot name already exists.
//
// In both forms zettarepl records the run in the task's state. Just before the
// call the tool reads the task again and keeps that state as the baseline; it
// refuses a task that is RUNNING then. After the call it polls
// pool.snapshottask.query until a FINISHED or ERROR state with a datetime after
// that baseline appears, then verifies the reported snapshot. The deadline
// bounds the run wait; the verification gets the time left, or at least
// snapshotVerifyFloor, and the context bounds both. A scheduled run that finishes after the call is reported too: its
// snapshot is newer than the call.
//
// pool.snapshottask.run is called exactly once and never again in any path: a
// transport error leaves the run state unknown (the no-replay rule in
// truenas/client.go).
func registerSnapshotTaskRunTool(s *mcp.Server, client truenas.Caller) {
	s.AddTool(&mcp.Tool{
		Name: "truenas_snapshot_task_run",
		Description: "Run 1 existing periodic snapshot task now (TrueNAS \"Run Now\", pool.snapshottask.run, role " +
			"SNAPSHOT_TASK_WRITE). The default is a dry run: it returns the task and the replication tasks the run " +
			"also starts, and runs nothing. Set confirm to true to run. A run does what a scheduled run does: it makes " +
			"the snapshot (recursive when the task is recursive), starts every enabled replication task that runs after " +
			"this task (auto, no schedule), and runs retention, which can delete expired snapshots. The task's " +
			"retention never deletes the snapshot of this run when its time is off the task's schedule: zettarepl " +
			"prunes only the snapshots whose time matches the schedule of a task on that dataset with that naming " +
			"schema, so the snapshot stays until someone deletes it. A disabled task is " +
			"refused. The tool calls pool.snapshottask.run once and never again, waits up to timeout_seconds for the " +
			"task state to show a finished run, and verifies that the reported snapshot exists. When the state is " +
			"unknown, check truenas_snapshot_task_list before you try again.",
		InputSchema: schema(map[string]any{
			"id":              numberProp("periodic snapshot task id, an integer of at least 1"),
			"confirm":         boolProp("when true, run the task; any other value returns a dry run (default false)"),
			"timeout_seconds": numberProp("how long to wait for the run to finish, 10 to 600, default 120"),
		}, "id"),
		Annotations: &mcp.ToolAnnotations{
			Title:           "Run Periodic Snapshot Task",
			ReadOnlyHint:    false,
			DestructiveHint: new(true), // the run starts retention, which can delete expired snapshots, and it can start replication
			IdempotentHint:  false,     // every run makes a new snapshot, or fails when the name of this minute exists
			OpenWorldHint:   new(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return runSnapshotTask(ctx, client, req)
	})
}

// snapshotVerifyFloor is the shortest time the snapshot verification gets when
// the run deadline is near or past, so a run seen just before the deadline can
// still be verified. Tests shrink it.
var snapshotVerifyFloor = 5 * time.Second

// runSnapshotTask is the handler of truenas_snapshot_task_run. It is a separate
// function so tests can call it with a context they cancel.
func runSnapshotTask(ctx context.Context, client truenas.Caller, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := snapshotTaskRunID(req)
	if err != nil {
		return nil, err
	}
	confirm, err := strictBool(req, "confirm", false)
	if err != nil {
		return nil, err
	}
	wait, err := snapshotRunWaitFor(req)
	if err != nil {
		return nil, err
	}

	// 1. The task must exist.
	task, err := querySnapshotTask(client, id)
	if err != nil {
		return nil, fmt.Errorf("%w; nothing was run", err)
	}
	if task == nil {
		return nil, fmt.Errorf("no periodic snapshot task with id %d; nothing was run", id)
	}
	dataset, _ := task["dataset"].(string)

	// 2. TrueNAS refuses a disabled task. The check also runs on a dry run,
	// so the dry run gives the answer the real call would give.
	if enabled, _ := task["enabled"].(bool); !enabled {
		return nil, fmt.Errorf("task %d (dataset %s) is disabled; TrueNAS refuses to run a disabled task; nothing was run", id, dataset)
	}

	// 3. The replication tasks zettarepl starts after this task.
	replications, err := replicationsStartedBy(client, id)
	if err != nil {
		return nil, fmt.Errorf("%w; nothing was run", err)
	}

	// 4. Preview only.
	if !confirm {
		return jsonValueResult(map[string]any{
			"dry_run":            true,
			"method":             "pool.snapshottask.run",
			"params":             []any{id},
			"task":               snapshotTaskSummary(task),
			"starts_replication": replications,
			"note": "A run makes the snapshot now, starts the listed replication tasks, and runs retention " +
				"like a scheduled run. The task's retention does not delete the new snapshot when its time is off the " +
				"task's schedule: it stays until someone deletes it. Call again with confirm true to run.",
		})
	}

	// 5. Just before the write: stop on a cancel, and read the task again
	// for a fresh baseline. A run that started or finished since step 1
	// must not count as the result of this call.
	if ctx.Err() != nil {
		return nil, fmt.Errorf("cancelled before the run of task %d; nothing was run", id)
	}
	task, err = querySnapshotTask(client, id)
	if err != nil {
		return nil, fmt.Errorf("%w; nothing was run", err)
	}
	if task == nil {
		return nil, fmt.Errorf("no periodic snapshot task with id %d; nothing was run", id)
	}
	if enabled, _ := task["enabled"].(bool); !enabled {
		return nil, fmt.Errorf("task %d (dataset %s) is disabled; TrueNAS refuses to run a disabled task; nothing was run", id, dataset)
	}
	before, _ := task["state"].(map[string]any)
	if before["state"] == "RUNNING" {
		return nil, fmt.Errorf("task %d is running now (a scheduled or earlier run); nothing was run; try again after it finishes", id)
	}
	beforeDatetime := before["datetime"]
	beforeSnapshot, _ := before["last_snapshot"].(string)
	if ctx.Err() != nil {
		return nil, fmt.Errorf("cancelled before the run of task %d; nothing was run", id)
	}
	deadline := time.Now().Add(wait)

	// 6. One call, never replayed.
	result, err := client.Call("pool.snapshottask.run", id)
	if err != nil {
		return nil, fmt.Errorf("pool.snapshottask.run failed: %w; the state of the run is not known: "+
			"check truenas_snapshot_task_list before a new try, and do not run this tool again blindly", err)
	}

	// 7. Middleware master returns a job id; 25.10 and 27.0 return null.
	if jobID, ok := snapshotRunJobID(result); ok {
		outcome := waitForJob(ctx, client, jobID, time.Until(deadline))
		switch outcome.State {
		case "SUCCESS":
		case "FAILED", "ABORTED":
			if strings.Contains(outcome.Error, "already existed") {
				return nil, fmt.Errorf("pool.snapshottask.run job %d ended %s: %s; the snapshot name already "+
					"existed, so nothing new was made (the task probably ran on schedule in this minute)",
					jobID, outcome.State, jobErrorText(outcome.Error))
			}
			return nil, fmt.Errorf("pool.snapshottask.run job %d ended %s: %s; the run failed",
				jobID, outcome.State, jobErrorText(outcome.Error))
		default:
			reason := fmt.Sprintf("did not finish within %s", wait)
			if outcome.Cancelled {
				reason = "was cancelled while waiting"
			}
			return nil, fmt.Errorf("pool.snapshottask.run job %d %s and its last state was %s%s; the state of "+
				"the run is unknown: check truenas_snapshot_task_list and do not run this tool again blindly",
				jobID, reason, outcome.State, pollErrSuffix(outcome.PollErr))
		}
	}

	// 8. Wait for zettarepl to record the run in the task state.
	run := waitForSnapshotTaskState(ctx, client, id, beforeDatetime, deadline)
	if run.Cancelled {
		return nil, fmt.Errorf("cancelled while waiting for task %d to finish; the run was accepted and can "+
			"still run: check truenas_snapshot_task_list and do not run this tool again blindly%s",
			id, snapshotPollErrSuffix(run.PollErr))
	}
	if run.State == nil {
		return nil, fmt.Errorf("the run of task %d was accepted, but no finished run was seen in %s; "+
			"do not run this tool again: check truenas_snapshot_task_list%s",
			id, wait, snapshotPollErrSuffix(run.PollErr))
	}

	if run.State["state"] == "ERROR" {
		return nil, fmt.Errorf("the run of task %d (dataset %s) ended ERROR: %v; no verified snapshot exists from this run",
			id, dataset, stateErrorText(run.State["error"]))
	}

	snapshot, _ := run.State["last_snapshot"].(string)
	if snapshot == "" {
		return nil, fmt.Errorf("the run of task %d finished, but the task state names no snapshot; "+
			"no snapshot was verified: check truenas_snapshot_task_list", id)
	}
	alreadyExisted := snapshot == beforeSnapshot

	// 9. Verify the snapshot, bounded by the deadline and the context.
	verifyWait := max(time.Until(deadline), snapshotVerifyFloor)
	entry, ok, err := findSnapshotBounded(ctx, client, snapshot, verifyWait)
	if !ok {
		return nil, fmt.Errorf("the run of task %d finished and reported snapshot %s, but the snapshot %s was not "+
			"verified: the check was cancelled or took longer than %s; check truenas_snapshot_list", id, snapshot, snapshot, verifyWait)
	}
	if err != nil || entry == nil {
		cause := "the snapshot query found no such snapshot"
		if err != nil {
			cause = fmt.Sprintf("the snapshot query failed: %v", err)
		}
		hint := ""
		if allowEmpty, ok := task["allow_empty"].(bool); ok && !allowEmpty {
			hint = "; the task has allow_empty false, so TrueNAS can skip or remove the snapshot of an unchanged dataset"
		}
		return nil, fmt.Errorf("the run of task %d finished and reported snapshot %s, but the snapshot was not verified: %s%s",
			id, snapshot, cause, hint)
	}

	if ctx.Err() != nil {
		return nil, fmt.Errorf("cancelled after the run of task %d; the snapshot %s was not verified; check truenas_snapshot_list", id, snapshot)
	}

	out := map[string]any{
		"ran":                true,
		"task_id":            id,
		"dataset":            task["dataset"],
		"recursive":          task["recursive"],
		"snapshot":           snapshot,
		"already_existed":    alreadyExisted,
		"verified":           true,
		"last_run":           lastRun(run.State, nil, false),
		"starts_replication": replications,
	}
	if created, ok := snapshotCreation(entry); ok {
		out["created"] = created
	}
	var notes []string
	if alreadyExisted {
		notes = append(notes, "The snapshot name exists already: the task probably ran on schedule in this "+
			"minute. This run made no new snapshot.")
	}
	if recursive, _ := task["recursive"].(bool); recursive {
		notes = append(notes, "The task is recursive: child datasets got the same snapshot name.")
	}
	if len(notes) > 0 {
		out["note"] = strings.Join(notes, " ")
	}
	return jsonValueResult(out)
}

// snapshotTaskRunID reads the id argument: an integer of at least 1.
func snapshotTaskRunID(req *mcp.CallToolRequest) (int64, error) {
	raw, err := requireFloat64(req, "id")
	if err != nil {
		return 0, err
	}
	if raw < 1 || raw > math.MaxInt32 || raw != math.Trunc(raw) {
		return 0, fmt.Errorf("parameter %q must be an integer of at least 1, got %v", "id", raw)
	}
	return int64(raw), nil
}

// snapshotRunWaitFor returns how long the tool waits for the run. The
// timeout_seconds argument must be a number from 10 to 600.
func snapshotRunWaitFor(req *mcp.CallToolRequest) (time.Duration, error) {
	seconds := float64(snapshotRunTimeoutDefault)
	if raw, present := parseArgs(req)["timeout_seconds"]; present && raw != nil {
		v, ok := raw.(float64)
		if !ok {
			return 0, fmt.Errorf("parameter %q must be a number, got %T", "timeout_seconds", raw)
		}
		seconds = v
	}
	if seconds < snapshotRunTimeoutMin || seconds > snapshotRunTimeoutMax {
		return 0, fmt.Errorf("parameter %q must be between %d and %d seconds, got %v",
			"timeout_seconds", snapshotRunTimeoutMin, snapshotRunTimeoutMax, seconds)
	}
	if snapshotRunWait > 0 {
		return snapshotRunWait, nil
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

// querySnapshotTask returns the periodic snapshot task with the id, or nil when
// there is none.
func querySnapshotTask(client truenas.Caller, id int64) (map[string]any, error) {
	result, err := client.Call("pool.snapshottask.query", [][]any{{"id", "=", id}})
	if err != nil {
		return nil, fmt.Errorf("pool.snapshottask.query: %w", err)
	}
	var tasks []map[string]any
	if err := json.Unmarshal(result, &tasks); err != nil {
		return nil, fmt.Errorf("parsing pool.snapshottask.query: %w", err)
	}
	if len(tasks) == 0 {
		return nil, nil
	}
	return tasks[0], nil
}

// replicationsStartedBy lists the replication tasks zettarepl starts after a
// run of the periodic snapshot task: enabled, auto, no schedule of their own,
// and linked to the task. The output is an allow-list, so encryption keys and
// SSH credentials never reach a result.
func replicationsStartedBy(client truenas.Caller, taskID int64) ([]map[string]any, error) {
	result, err := client.Call("replication.query")
	if err != nil {
		return nil, fmt.Errorf("replication.query: %w", err)
	}
	var entries []map[string]any
	if err := json.Unmarshal(result, &entries); err != nil {
		return nil, fmt.Errorf("parsing replication.query: %w", err)
	}
	sortEntriesByID(entries)

	out := []map[string]any{}
	for _, e := range entries {
		enabled, _ := e["enabled"].(bool)
		auto, _ := e["auto"].(bool)
		if !enabled || !auto || e["schedule"] != nil {
			continue
		}
		linked := false
		for _, id := range periodicSnapshotTaskIDs(e["periodic_snapshot_tasks"]) {
			if n, ok := id.(float64); ok && n == float64(taskID) {
				linked = true
				break
			}
		}
		if !linked {
			continue
		}
		out = append(out, map[string]any{
			"id":             e["id"],
			"name":           e["name"],
			"direction":      e["direction"],
			"transport":      e["transport"],
			"target_dataset": e["target_dataset"],
		})
	}
	return out, nil
}

// snapshotRunJobID returns the job id when the run result is a JSON integer
// (middleware master). null and any other value return false.
func snapshotRunJobID(result json.RawMessage) (int64, bool) {
	trimmed := bytes.TrimSpace(result)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return 0, false
	}
	var id int64
	if err := json.Unmarshal(trimmed, &id); err != nil {
		return 0, false
	}
	return id, true
}

// snapshotRunState is the outcome of waiting for the task state.
type snapshotRunState struct {
	State     map[string]any // the new FINISHED or ERROR state; nil when none was seen
	PollErr   error          // the last failed poll, if any
	Cancelled bool           // the request context ended first
}

// waitForSnapshotTaskState polls pool.snapshottask.query until the task state
// shows a run newer than before (a different raw datetime) that is FINISHED or
// ERROR, the deadline passes, or the request is cancelled. A RUNNING state or a
// failed poll keeps the wait going; the last poll error is kept for the report.
//
// As in waitForJob, each poll runs in its own goroutine with a 1-slot channel,
// because truenas.Caller takes no context: the wait can return at the deadline
// or on a cancel, and a late poll result is dropped. A poll is a read only.
func waitForSnapshotTaskState(ctx context.Context, client truenas.Caller, id int64, before any, deadline time.Time) snapshotRunState {
	type poll struct {
		task map[string]any
		err  error
	}
	var outcome snapshotRunState
	for {
		if ctx.Err() != nil {
			outcome.Cancelled = true
			return outcome
		}
		left := time.Until(deadline)
		if left <= 0 {
			return outcome
		}

		results := make(chan poll, 1)
		go func() {
			task, err := querySnapshotTask(client, id)
			if err == nil && task == nil {
				err = fmt.Errorf("task %d not found in pool.snapshottask.query", id)
			}
			results <- poll{task: task, err: err}
		}()

		timer := time.NewTimer(left)
		select {
		case result := <-results:
			timer.Stop()
			if result.err != nil {
				outcome.PollErr = result.err
				break
			}
			if state, ok := result.task["state"].(map[string]any); ok && snapshotRunFinished(state, before) {
				outcome.State = state
				return outcome
			}
		case <-ctx.Done():
			timer.Stop()
			outcome.Cancelled = true
			return outcome
		case <-timer.C:
			return outcome
		}

		pause := min(jobPollInterval, time.Until(deadline))
		if pause <= 0 {
			return outcome
		}
		timer = time.NewTimer(pause)
		select {
		case <-ctx.Done():
			timer.Stop()
			outcome.Cancelled = true
			return outcome
		case <-timer.C:
		}
	}
}

// snapshotRunFinished reports whether state records a finished run newer than
// the before datetime. A nil before means the task never ran, so any datetime
// is new.
func snapshotRunFinished(state map[string]any, before any) bool {
	now, ok := state["datetime"]
	if !ok || now == nil || reflect.DeepEqual(now, before) {
		return false
	}
	switch state["state"] {
	case "FINISHED", "ERROR":
		return true
	}
	return false
}

// findSnapshot returns the snapshot entry with the full name, or nil when the
// query finds none.
func findSnapshot(client truenas.Caller, name string) (map[string]any, error) {
	result, err := callSnapshot(client, "query", [][]any{{"id", "=", name}}, snapshotQueryOptions())
	if err != nil {
		return nil, err
	}
	var items []map[string]any
	if err := json.Unmarshal(result, &items); err != nil {
		return nil, fmt.Errorf("parsing snapshot query: %w", err)
	}
	for _, item := range items {
		if item["id"] == name || item["name"] == name {
			return item, nil
		}
	}
	return nil, nil
}

// findSnapshotBounded runs findSnapshot in its own goroutine with a 1-slot
// channel, as the polls do, because truenas.Caller takes no context. ok is false
// when the context ends or wait runs out first; the call then finishes in the
// background and its result is dropped. It is a read only.
func findSnapshotBounded(ctx context.Context, client truenas.Caller, name string, wait time.Duration) (map[string]any, bool, error) {
	type found struct {
		entry map[string]any
		err   error
	}
	results := make(chan found, 1)
	go func() {
		entry, err := findSnapshot(client, name)
		results <- found{entry: entry, err: err}
	}()
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case r := <-results:
		return r.entry, true, r.err
	case <-ctx.Done():
		return nil, false, nil
	case <-timer.C:
		return nil, false, nil
	}
}

// stateErrorText gives the error of an ERROR state, or says it has none.
func stateErrorText(v any) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	if v == nil {
		return "the task state carries no error text"
	}
	return fmt.Sprint(v)
}

// snapshotPollErrSuffix reports the last failed state poll, when there was one.
func snapshotPollErrSuffix(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf(" (the last pool.snapshottask.query call failed: %v)", err)
}
