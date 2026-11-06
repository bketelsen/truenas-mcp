package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// anyList turns a string slice into the []any a decoded JSON array gives.
func anyList(items []string) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	return out
}

// repCreateSnapshotTask builds a PeriodicSnapshotTaskEntry fixture.
func repCreateSnapshotTask(id float64, dataset string, recursive bool, exclude []string, enabled bool) map[string]any {
	return map[string]any{
		"id":             id,
		"dataset":        dataset,
		"recursive":      recursive,
		"exclude":        anyList(exclude),
		"enabled":        enabled,
		"naming_schema":  "auto-%Y-%m-%d_%H-%M",
		"lifetime_value": float64(2),
		"lifetime_unit":  "WEEK",
		"allow_empty":    true,
		"schedule": map[string]any{
			"minute": "0", "hour": "*/4", "dom": "*", "month": "*", "dow": "*", "begin": "00:00", "end": "23:59",
		},
		"state": map[string]any{
			"state":    "FINISHED",
			"datetime": map[string]any{"$date": float64(1761220800000)},
		},
	}
}

// repCreateReplicationTask builds a ReplicationEntry fixture, in the shape
// replication.query returns.
func repCreateReplicationTask(id float64, name, target string, sources, exclude []string, recursive bool, snapshotIDs []float64) map[string]any {
	tasks := make([]any, 0, len(snapshotIDs))
	for _, snapshotID := range snapshotIDs {
		tasks = append(tasks, map[string]any{"id": snapshotID, "dataset": "fast"})
	}
	return map[string]any{
		"id":                         id,
		"name":                       name,
		"direction":                  "PUSH",
		"transport":                  "LOCAL",
		"ssh_credentials":            nil,
		"source_datasets":            anyList(sources),
		"target_dataset":             target,
		"recursive":                  recursive,
		"exclude":                    anyList(exclude),
		"periodic_snapshot_tasks":    tasks,
		"naming_schema":              []any{"auto-%Y-%m-%d_%H-%M"},
		"also_include_naming_schema": []any{},
		"auto":                       true,
		"schedule":                   nil,
		"readonly":                   "SET",
		"retention_policy":           "SOURCE",
		"lifetime_value":             nil,
		"lifetime_unit":              nil,
		"encryption":                 false,
		"enabled":                    true,
		"state":                      map[string]any{"state": "FINISHED"},
	}
}

// repCreateCreatedTask is what replication.create returns. It carries secrets on
// purpose: the summary must not let them through.
const repCreateCreatedTask = `{
  "id": 12, "name": "fast/a - slow/aback", "direction": "PUSH", "transport": "LOCAL",
  "source_datasets": ["fast/a"], "target_dataset": "slow/aback", "recursive": false, "exclude": [],
  "periodic_snapshot_tasks": [{"id": 1, "dataset": "fast"}], "naming_schema": [], "also_include_naming_schema": [],
  "auto": true, "schedule": {"minute": "0", "hour": "2"}, "readonly": "SET", "retention_policy": "SOURCE",
  "lifetime_value": null, "lifetime_unit": null, "encryption": false,
  "encryption_key": "ENCRYPTIONKEYSECRET", "encryption_key_location": "/root/keys/tank.key",
  "ssh_credentials": {"id": 2, "name": "backup-box",
    "attributes": {"host": "backup.lan", "PRIVATE KEY": "-----BEGIN OPENSSH PRIVATE KEY-----\nSECRETKEYMATERIAL\n-----END OPENSSH PRIVATE KEY-----"}},
  "enabled": true,
  "state": {"state": "PENDING", "datetime": {"$date": 1761220800000}}}`

// repCreateFake answers the 3 reads and the create call of
// truenas_replication_task_create.
type repCreateFake struct {
	snapshotTasks    []map[string]any
	replicationTasks []map[string]any
	datasets         []string // the datasets pool.dataset.query finds
	createResult     string
	createErr        error
	readErrors       map[string]error // a method that must fail
}

// repCreateCalls records every middleware call the tool made.
type repCreateCalls struct {
	methods        []string
	datasetFilter  any
	datasetOptions any
	createCalls    [][]any // the parameter list of each replication.create call
}

// repCreateBase is the fixture most cases start from: 2 periodic snapshot tasks
// under the pool "fast" (task 1 recursive with 1 exclude), 1 unrelated
// replication task, and the datasets of the example plan: source fast/a into the
// new dataset slow/aback.
func repCreateBase() *repCreateFake {
	return &repCreateFake{
		snapshotTasks: []map[string]any{
			repCreateSnapshotTask(1, "fast", true, []string{"fast/tmp"}, true),
			repCreateSnapshotTask(2, "fast/b", false, nil, false),
		},
		replicationTasks: []map[string]any{
			repCreateReplicationTask(4, "fast to slow", "slow/old", []string{"fast/a"}, nil, false, []float64{1}),
		},
		datasets:     []string{"fast", "fast/a", "fast/b", "fast/tmp", "slow"},
		createResult: repCreateCreatedTask,
	}
}

// repCreateArgs is the example plan: fast/a replicated into the new dataset
// slow/aback, bound to periodic snapshot task 1.
func repCreateArgs() map[string]any {
	return map[string]any{
		"source_datasets":            []any{"fast/a"},
		"target_dataset":             "slow/aback",
		"periodic_snapshot_task_ids": []any{float64(1)},
	}
}

// repCreateExpectedPayload is the payload of repCreateArgs with every default
// filled in.
const repCreateExpectedPayload = `{
  "name": "fast/a - slow/aback",
  "direction": "PUSH",
  "transport": "LOCAL",
  "source_datasets": ["fast/a"],
  "target_dataset": "slow/aback",
  "recursive": false,
  "exclude": [],
  "periodic_snapshot_tasks": [1],
  "auto": true,
  "retention_policy": "SOURCE",
  "readonly": "SET",
  "enabled": true,
  "allow_from_scratch": false,
  "replicate": false,
  "encryption": false
}`

// newRepCreateCaller returns a fake middleware for f, with its call log.
func newRepCreateCaller(f *repCreateFake) (*mockCaller, *repCreateCalls) {
	calls := &repCreateCalls{}
	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			calls.methods = append(calls.methods, method)
			if err := f.readErrors[method]; err != nil {
				return nil, err
			}
			switch method {
			case "pool.snapshottask.query":
				return json.Marshal(f.snapshotTasks)
			case "replication.query":
				return json.Marshal(f.replicationTasks)
			case "pool.dataset.query":
				if len(params) != 2 {
					return nil, fmt.Errorf("pool.dataset.query got %d params, want 2", len(params))
				}
				calls.datasetFilter, calls.datasetOptions = params[0], params[1]
				return f.datasetRows(params[0])
			case "replication.create":
				calls.createCalls = append(calls.createCalls, append([]any(nil), params...))
				if f.createErr != nil {
					return nil, f.createErr
				}
				if f.createResult == "" {
					return json.RawMessage(`{}`), nil
				}
				return json.RawMessage(f.createResult), nil
			}
			return nil, errors.New("unexpected method " + method)
		},
	}
	return mock, calls
}

// datasetRows answers pool.dataset.query by the "in" filter it gets, as the real
// middleware does: only the datasets f.datasets holds exist.
func (f *repCreateFake) datasetRows(filter any) (json.RawMessage, error) {
	conditions, ok := filter.([][]any)
	if !ok || len(conditions) != 1 || len(conditions[0]) != 3 {
		return nil, fmt.Errorf("pool.dataset.query filter = %v, want 1 condition", filter)
	}
	if conditions[0][0] != "id" || conditions[0][1] != "in" {
		return nil, fmt.Errorf("pool.dataset.query filter = %v, want [id in ...]", conditions[0])
	}
	wanted, ok := conditions[0][2].([]string)
	if !ok {
		return nil, fmt.Errorf("pool.dataset.query filter value is %T, want []string", conditions[0][2])
	}
	known := make(map[string]bool, len(f.datasets))
	for _, dataset := range f.datasets {
		known[dataset] = true
	}
	rows := make([]map[string]any, 0, len(wanted))
	for _, id := range wanted {
		if known[id] {
			rows = append(rows, map[string]any{"id": id, "name": id})
		}
	}
	return json.Marshal(rows)
}

// repCreateDryRunResult is the decoded dry run result of the tool.
type repCreateDryRunResult struct {
	DryRun  bool             `json:"dry_run"`
	Method  string           `json:"method"`
	Payload map[string]any   `json:"payload"`
	PlanSHA string           `json:"plan_sha256"`
	Bound   []map[string]any `json:"bound_snapshot_tasks"`
	Note    string           `json:"note"`
}

// repCreateDryRun runs the tool with dry_run set and returns the result.
func repCreateDryRun(t *testing.T, f *repCreateFake, args map[string]any) (repCreateDryRunResult, *repCreateCalls) {
	t.Helper()
	dry := make(map[string]any, len(args)+1)
	for key, value := range args {
		dry[key] = value
	}
	dry["dry_run"] = true
	mock, calls := newRepCreateCaller(f)
	result, err := callTool(t, mock, false, "truenas_replication_task_create", dry)
	if err != nil {
		t.Fatalf("dry run: transport error: %v", err)
	}
	if result.IsError {
		t.Fatalf("dry run: tool error: %s", resultText(t, result))
	}
	var decoded repCreateDryRunResult
	if err := json.Unmarshal([]byte(resultText(t, result)), &decoded); err != nil {
		t.Fatalf("dry run result is not JSON: %v\n%s", err, resultText(t, result))
	}
	return decoded, calls
}

// repCreatePlanHash runs a dry run and returns its plan_sha256.
func repCreatePlanHash(t *testing.T, f *repCreateFake, args map[string]any) string {
	t.Helper()
	decoded, _ := repCreateDryRun(t, f, args)
	if !isHexSHA256(decoded.PlanSHA) {
		t.Fatalf("plan_sha256 = %q, want 64 hex characters", decoded.PlanSHA)
	}
	return decoded.PlanSHA
}

// isHexSHA256 reports whether s is 64 lower-case hex characters.
func isHexSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func TestReplicationTaskCreate_DryRun(t *testing.T) {
	f := repCreateBase()
	decoded, calls := repCreateDryRun(t, f, repCreateArgs())

	if !decoded.DryRun || decoded.Method != "replication.create" {
		t.Errorf("dry run answer = %v, want dry_run true and method replication.create", decoded)
	}
	if want := []string{"pool.snapshottask.query", "replication.query", "pool.dataset.query"}; !reflect.DeepEqual(calls.methods, want) {
		t.Errorf("methods = %v, want %v", calls.methods, want)
	}
	if len(calls.createCalls) != 0 {
		t.Fatalf("dry run made %d replication.create calls, want 0", len(calls.createCalls))
	}

	// The default name, and the payload exactly as the middleware gets it.
	assertSameJSON(t, decoded.Payload, repCreateExpectedPayload)
	if decoded.Payload["name"] != "fast/a - slow/aback" {
		t.Errorf("name = %v, want the default name fast/a - slow/aback", decoded.Payload["name"])
	}
	if !isHexSHA256(decoded.PlanSHA) {
		t.Errorf("plan_sha256 = %q, want 64 hex characters", decoded.PlanSHA)
	}

	// The dataset query names the sources, the target and the target's parent.
	wantFilter := [][]any{{"id", "in", []string{"fast/a", "slow", "slow/aback"}}}
	if !reflect.DeepEqual(calls.datasetFilter, wantFilter) {
		t.Errorf("pool.dataset.query filter = %v, want %v", calls.datasetFilter, wantFilter)
	}
	wantOptions := map[string]any{
		"select": []string{"id", "name"},
		"extra": map[string]any{
			"flat":                true,
			"retrieve_children":   false,
			"properties":          []string{},
			"retrieve_user_props": false,
		},
	}
	if !reflect.DeepEqual(calls.datasetOptions, wantOptions) {
		t.Errorf("pool.dataset.query options = %v, want %v", calls.datasetOptions, wantOptions)
	}

	// The bound task is reported with the list tool's fields.
	if len(decoded.Bound) != 1 || decoded.Bound[0]["id"] != float64(1) || decoded.Bound[0]["dataset"] != "fast" {
		t.Fatalf("bound_snapshot_tasks = %v, want task 1 on fast", decoded.Bound)
	}
	if lr := decoded.Bound[0]["last_run"].(map[string]any); lr["state"] != "FINISHED" {
		t.Errorf("bound task last_run = %v, want state FINISHED", lr)
	}
	if !strings.Contains(decoded.Note, "plan_sha256") || !strings.Contains(decoded.Note, "first run") {
		t.Errorf("note = %q, want the zettarepl and plan_sha256 text", decoded.Note)
	}
}

func TestReplicationTaskCreate_Write(t *testing.T) {
	f := repCreateBase()
	hash := repCreatePlanHash(t, f, repCreateArgs())

	mock, calls := newRepCreateCaller(f)
	args := repCreateArgs()
	args["expected_sha256"] = hash
	result, err := callTool(t, mock, false, "truenas_replication_task_create", args)
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", resultText(t, result))
	}

	if len(calls.createCalls) != 1 {
		t.Fatalf("replication.create calls = %d, want exactly 1", len(calls.createCalls))
	}
	if params := calls.createCalls[0]; len(params) != 1 {
		t.Fatalf("replication.create params = %v, want the payload as the only param", params)
	}
	assertSameJSON(t, calls.createCalls[0][0], repCreateExpectedPayload)

	var got struct {
		Created bool           `json:"created"`
		Task    map[string]any `json:"task"`
		Note    string         `json:"note"`
	}
	text := resultText(t, result)
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("result is not a JSON object: %v\n%s", err, text)
	}
	if !got.Created || got.Task["id"] != float64(12) || got.Task["name"] != "fast/a - slow/aback" ||
		got.Task["target_dataset"] != "slow/aback" || got.Task["transport"] != "LOCAL" {
		t.Errorf("created task = %v", got.Task)
	}
	if retention := got.Task["retention"].(map[string]any); retention["policy"] != "SOURCE" {
		t.Errorf("retention = %v, want policy SOURCE", retention)
	}
	if !strings.Contains(got.Note, "truenas_snapshot_task_run") {
		t.Errorf("note = %q, want the run-now hint", got.Note)
	}
	// The credential's id and name are reported on purpose, as the list tool
	// does; its attributes and the encryption key are not.
	for _, leaked := range []string{
		"ENCRYPTIONKEYSECRET", "encryption_key", "/root/keys/tank.key",
		"SECRETKEYMATERIAL", "PRIVATE KEY", "attributes",
	} {
		if strings.Contains(text, leaked) {
			t.Errorf("result leaks %q:\n%s", leaked, text)
		}
	}
}

func TestReplicationTaskCreate_HashGuards(t *testing.T) {
	t.Run("no expected_sha256", func(t *testing.T) {
		f := repCreateBase()
		hash := repCreatePlanHash(t, f, repCreateArgs())
		mock, calls := newRepCreateCaller(f)
		result, err := callTool(t, mock, false, "truenas_replication_task_create", repCreateArgs())
		text := toolError(t, result, err)
		if !strings.Contains(text, "dry_run") || !strings.Contains(text, hash) {
			t.Errorf("error text = %q, want the dry-run hint and the current hash", text)
		}
		if len(calls.createCalls) != 0 {
			t.Errorf("replication.create calls = %d, want 0", len(calls.createCalls))
		}
	})

	t.Run("wrong hash", func(t *testing.T) {
		f := repCreateBase()
		hash := repCreatePlanHash(t, f, repCreateArgs())
		mock, calls := newRepCreateCaller(f)
		args := repCreateArgs()
		args["expected_sha256"] = strings.Repeat("0", 64)
		result, err := callTool(t, mock, false, "truenas_replication_task_create", args)
		text := toolError(t, result, err)
		if !strings.Contains(text, "changed since the dry run") || !strings.Contains(text, hash) {
			t.Errorf("error text = %q, want the changed-plan text and the current hash", text)
		}
		if len(calls.createCalls) != 0 {
			t.Errorf("replication.create calls = %d, want 0", len(calls.createCalls))
		}
	})

	t.Run("appliance changed since the dry run", func(t *testing.T) {
		f := repCreateBase()
		hash := repCreatePlanHash(t, f, repCreateArgs())
		// A new replication task appears; it overlaps nothing, so only the hash
		// can catch the change.
		f.replicationTasks = append(f.replicationTasks,
			repCreateReplicationTask(9, "fast/b to elsewhere", "elsewhere/x", []string{"fast/b"}, nil, false, nil))
		mock, calls := newRepCreateCaller(f)
		args := repCreateArgs()
		args["expected_sha256"] = hash
		result, err := callTool(t, mock, false, "truenas_replication_task_create", args)
		text := toolError(t, result, err)
		if !strings.Contains(text, "changed since the dry run") {
			t.Errorf("error text = %q, want the changed-plan text", text)
		}
		if len(calls.createCalls) != 0 {
			t.Errorf("replication.create calls = %d, want 0", len(calls.createCalls))
		}
	})
}

func TestReplicationTaskCreate_RefusedArguments(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{
			name: "ssh transport",
			args: map[string]any{"transport": "SSH"},
			want: "local replication only",
		},
		{
			name: "ssh+netcat transport",
			args: map[string]any{"transport": "ssh+netcat"},
			want: "local replication only",
		},
		{
			name: "ssh_credentials",
			args: map[string]any{"ssh_credentials": map[string]any{"id": float64(2)}},
			want: `unknown parameter "ssh_credentials"`,
		},
		{
			name: "direction",
			args: map[string]any{"direction": "PULL"},
			want: `unknown parameter "direction"`,
		},
		{
			name: "allow_from_scratch",
			args: map[string]any{"allow_from_scratch": true},
			want: `unknown parameter "allow_from_scratch"`,
		},
		{
			name: "encryption",
			args: map[string]any{"encryption": true},
			want: `unknown parameter "encryption"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := repCreateBase()
			mock, calls := newRepCreateCaller(f)
			args := repCreateArgs()
			args["dry_run"] = true
			for key, value := range tc.args {
				args[key] = value
			}
			result, err := callTool(t, mock, false, "truenas_replication_task_create", args)
			text := toolError(t, result, err)
			if !strings.Contains(text, tc.want) {
				t.Errorf("error text = %q, want it to contain %q", text, tc.want)
			}
			if len(calls.methods) != 0 {
				t.Errorf("middleware calls %v, want none", calls.methods)
			}
		})
	}
}

// TestReplicationTaskCreate_Checks covers checks a to f: each one has at least 1
// case that is refused, and nothing is created in any of them.
func TestReplicationTaskCreate_Checks(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(f *repCreateFake, args map[string]any)
		want    string
		notWant string
	}{
		{
			name: "a source does not exist",
			mutate: func(f *repCreateFake, args map[string]any) {
				f.datasets = []string{"fast", "fast/b", "fast/tmp", "slow"}
			},
			want: `source dataset "fast/a" does not exist`,
		},
		{
			name: "a target that already exists",
			mutate: func(f *repCreateFake, args map[string]any) {
				f.datasets = append(f.datasets, "slow/aback")
			},
			want: `target dataset "slow/aback" already exists`,
		},
		{
			name: "a missing target parent",
			mutate: func(f *repCreateFake, args map[string]any) {
				f.datasets = []string{"fast", "fast/a", "fast/b", "fast/tmp"}
			},
			want: "does not exist",
		},
		{
			name: "b target inside a source",
			mutate: func(f *repCreateFake, args map[string]any) {
				args["source_datasets"] = []any{"slow"}
			},
			want: `target dataset "slow/aback" is inside source dataset "slow"`,
		},
		{
			name: "b source inside the target",
			mutate: func(f *repCreateFake, args map[string]any) {
				args["source_datasets"] = []any{"slow/aback/deep"}
				f.datasets = append(f.datasets, "slow/aback/deep")
			},
			want: `source dataset "slow/aback/deep" is inside target dataset "slow/aback"`,
		},
		{
			name: "c name already used",
			mutate: func(f *repCreateFake, args map[string]any) {
				args["name"] = "fast to slow"
			},
			want: `a replication task named "fast to slow" already exists (id 4)`,
		},
		{
			name: "d target overlaps another task's target",
			mutate: func(f *repCreateFake, args map[string]any) {
				f.replicationTasks = append(f.replicationTasks,
					repCreateReplicationTask(9, "slow to elsewhere", "slow", []string{"fast/b"}, nil, false, nil))
			},
			want: `has target dataset "slow", which overlaps target dataset "slow/aback"`,
		},
		{
			name: "d target covered by another task's source",
			mutate: func(f *repCreateFake, args map[string]any) {
				f.replicationTasks = append(f.replicationTasks,
					repCreateReplicationTask(9, "slow to elsewhere", "elsewhere/x", []string{"slow"}, nil, true, nil))
			},
			want: `replicates source dataset "slow", which covers target dataset "slow/aback"`,
		},
		{
			name: "e target covered by a snapshot task",
			mutate: func(f *repCreateFake, args map[string]any) {
				f.snapshotTasks = append(f.snapshotTasks, repCreateSnapshotTask(5, "slow", true, nil, true))
			},
			want: `periodic snapshot task 5 snapshots dataset "slow", which covers target dataset "slow/aback"`,
		},
		{
			name: "f bound task does not exist",
			mutate: func(f *repCreateFake, args map[string]any) {
				args["periodic_snapshot_task_ids"] = []any{float64(99)}
			},
			want: "periodic snapshot task 99 does not exist",
		},
		{
			name: "f bound task is disabled",
			mutate: func(f *repCreateFake, args map[string]any) {
				args["periodic_snapshot_task_ids"] = []any{float64(2)}
			},
			want: "periodic snapshot task 2 (dataset \"fast/b\") is disabled",
		},
		{
			name: "f source not covered by a bound task",
			mutate: func(f *repCreateFake, args map[string]any) {
				args["periodic_snapshot_task_ids"] = []any{float64(2)}
				args["enabled"] = false
			},
			want: `source dataset "fast/a" is not covered by any of the bound periodic snapshot tasks [2]`,
		},
		{
			name: "f recursive plan misses a bound task exclude",
			mutate: func(f *repCreateFake, args map[string]any) {
				f.snapshotTasks[0] = repCreateSnapshotTask(1, "fast", true, []string{"fast/a/tmp"}, true)
				args["recursive"] = true
			},
			want: `excludes "fast/a/tmp", which is inside source dataset "fast/a"`,
		},
		{
			name: "f non-recursive plan copies an excluded source",
			mutate: func(f *repCreateFake, args map[string]any) {
				f.snapshotTasks = append(f.snapshotTasks, repCreateSnapshotTask(3, "fast", false, []string{"fast/a"}, true))
				args["periodic_snapshot_task_ids"] = []any{float64(3)}
			},
			want: `excludes source dataset "fast/a"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := repCreateBase()
			args := repCreateArgs()
			tc.mutate(f, args)
			mock, calls := newRepCreateCaller(f)
			args["dry_run"] = true
			result, err := callTool(t, mock, false, "truenas_replication_task_create", args)
			text := toolError(t, result, err)
			if !strings.Contains(text, tc.want) {
				t.Errorf("error text = %q, want it to contain %q", text, tc.want)
			}
			if !strings.Contains(text, "nothing was created") {
				t.Errorf("error text = %q, want the nothing-was-created note", text)
			}
			if len(calls.createCalls) != 0 {
				t.Errorf("replication.create calls = %d, want 0", len(calls.createCalls))
			}
		})
	}
}

// TestReplicationTaskCreate_ExcludedTargetAccepted is the accepting case of check
// d: the target lies under the source tree of an existing replication task, but
// that task excludes it, so it does not cover the target.
func TestReplicationTaskCreate_ExcludedTargetAccepted(t *testing.T) {
	f := repCreateBase()
	f.replicationTasks = append(f.replicationTasks,
		repCreateReplicationTask(9, "slow to elsewhere", "elsewhere/x", []string{"slow"}, []string{"slow/aback"}, true, nil))

	decoded, calls := repCreateDryRun(t, f, repCreateArgs())
	if len(calls.createCalls) != 0 {
		t.Fatalf("dry run made %d replication.create calls, want 0", len(calls.createCalls))
	}
	if decoded.Payload["target_dataset"] != "slow/aback" {
		t.Errorf("payload target = %v, want slow/aback", decoded.Payload["target_dataset"])
	}

	// The same plan is accepted as a write.
	hash := decoded.PlanSHA
	mock, written := newRepCreateCaller(f)
	args := repCreateArgs()
	args["expected_sha256"] = hash
	result, err := callTool(t, mock, false, "truenas_replication_task_create", args)
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", resultText(t, result))
	}
	if len(written.createCalls) != 1 {
		t.Errorf("replication.create calls = %d, want 1", len(written.createCalls))
	}
}

func TestReplicationTaskCreate_Retention(t *testing.T) {
	refused := []struct {
		name string
		args map[string]any
		want string
	}{
		{
			name: "custom without lifetime_value",
			args: map[string]any{"retention_policy": "custom", "lifetime_unit": "day"},
			want: `parameter "lifetime_value" is required when "retention_policy" is CUSTOM`,
		},
		{
			name: "custom without lifetime_unit",
			args: map[string]any{"retention_policy": "CUSTOM", "lifetime_value": float64(2)},
			want: `parameter "lifetime_unit" is required when "retention_policy" is CUSTOM`,
		},
		{
			name: "custom without any lifetime",
			args: map[string]any{"retention_policy": "CUSTOM"},
			want: "is required when",
		},
		{
			name: "source with a lifetime",
			args: map[string]any{"retention_policy": "SOURCE", "lifetime_value": float64(2), "lifetime_unit": "DAY"},
			want: "are only allowed when",
		},
		{
			name: "none with a lifetime",
			args: map[string]any{"retention_policy": "none", "lifetime_unit": "WEEK"},
			want: "are only allowed when",
		},
		{
			name: "custom with a lifetime of 0",
			args: map[string]any{"retention_policy": "CUSTOM", "lifetime_value": float64(0), "lifetime_unit": "DAY"},
			want: "integer from 1 to 10000",
		},
		{
			name: "custom with a fractional lifetime",
			args: map[string]any{"retention_policy": "CUSTOM", "lifetime_value": 1.5, "lifetime_unit": "DAY"},
			want: "integer from 1 to 10000",
		},
		{
			name: "custom with an unknown unit",
			args: map[string]any{"retention_policy": "CUSTOM", "lifetime_value": float64(2), "lifetime_unit": "FORTNIGHT"},
			want: `parameter "lifetime_unit"`,
		},
		{
			name: "unknown policy",
			args: map[string]any{"retention_policy": "WEEKLY"},
			want: `parameter "retention_policy" must be one of SOURCE, CUSTOM, NONE`,
		},
	}

	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			f := repCreateBase()
			mock, calls := newRepCreateCaller(f)
			args := repCreateArgs()
			args["dry_run"] = true
			for key, value := range tc.args {
				args[key] = value
			}
			result, err := callTool(t, mock, false, "truenas_replication_task_create", args)
			text := toolError(t, result, err)
			if !strings.Contains(text, tc.want) {
				t.Errorf("error text = %q, want it to contain %q", text, tc.want)
			}
			if len(calls.methods) != 0 {
				t.Errorf("middleware calls %v, want none", calls.methods)
			}
		})
	}

	t.Run("custom with a lifetime", func(t *testing.T) {
		f := repCreateBase()
		args := repCreateArgs()
		args["retention_policy"] = "Custom"
		args["lifetime_value"] = float64(4)
		args["lifetime_unit"] = "day"
		decoded, _ := repCreateDryRun(t, f, args)
		if decoded.Payload["retention_policy"] != "CUSTOM" {
			t.Errorf("retention_policy = %v, want CUSTOM", decoded.Payload["retention_policy"])
		}
		if decoded.Payload["lifetime_value"] != float64(4) || decoded.Payload["lifetime_unit"] != "DAY" {
			t.Errorf("lifetime = %v %v, want 4 DAY", decoded.Payload["lifetime_value"], decoded.Payload["lifetime_unit"])
		}
	})
}

func TestReplicationTaskCreate_BadArguments(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"bad readonly", map[string]any{"readonly": "always"}, `parameter "readonly" must be one of SET, REQUIRE, IGNORE`},
		{"pool root target", map[string]any{"target_dataset": "slow"}, "not the pool root"},
		{"trailing slash target", map[string]any{"target_dataset": "slow/aback/"}, "must not start or end with"},
		{"leading slash source", map[string]any{"source_datasets": []any{"/fast/a"}}, "must not start or end with"},
		{"double slash source", map[string]any{"source_datasets": []any{"fast//a"}}, `must not contain "//"`},
		{"snapshot marker source", map[string]any{"source_datasets": []any{"fast/a@snap"}}, `must not contain "@"`},
		{"hash character source", map[string]any{"source_datasets": []any{"fast/a#1"}}, `must not contain "@"`},
		{"whitespace source", map[string]any{"source_datasets": []any{"fast/a b"}}, "must not contain whitespace"},
		{"empty source", map[string]any{"source_datasets": []any{""}}, "must not be empty"},
		{"no sources", map[string]any{"source_datasets": []any{}}, "must hold at least 1 dataset"},
		{"duplicate source", map[string]any{"source_datasets": []any{"fast/a", "fast/a"}}, "more than once"},
		{"duplicate id", map[string]any{"periodic_snapshot_task_ids": []any{float64(1), float64(1)}}, "more than once"},
		{"fractional id", map[string]any{"periodic_snapshot_task_ids": []any{1.5}}, "integer of at least 1"},
		{"zero id", map[string]any{"periodic_snapshot_task_ids": []any{float64(0)}}, "integer of at least 1"},
		{"negative id", map[string]any{"periodic_snapshot_task_ids": []any{float64(-3)}}, "integer of at least 1"},
		{"string id", map[string]any{"periodic_snapshot_task_ids": []any{"1"}}, "only numbers"},
		{"ids not an array", map[string]any{"periodic_snapshot_task_ids": float64(1)}, "array of numbers"},
		{"no ids", map[string]any{"periodic_snapshot_task_ids": []any{}}, "must hold at least 1 id"},
		{"exclude without recursive", map[string]any{"exclude": []any{"fast/a/tmp"}}, `only allowed when "recursive" is true`},
		{"exclude outside the sources", map[string]any{"recursive": true, "exclude": []any{"slow/x"}}, "must be below 1 of the source datasets"},
		{"name too long", map[string]any{"name": strings.Repeat("n", 121)}, "at most 120 characters"},
		{"string dry_run", map[string]any{"dry_run": "true"}, `parameter "dry_run" must be a boolean`},
		{"string recursive", map[string]any{"recursive": "true"}, `parameter "recursive" must be a boolean`},
		{"string enabled", map[string]any{"enabled": "false"}, `parameter "enabled" must be a boolean`},
		{"number name", map[string]any{"name": float64(1)}, `parameter "name" must be a string`},
		{"missing target", map[string]any{"target_dataset": nil}, `required parameter "target_dataset" missing`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := repCreateBase()
			mock, calls := newRepCreateCaller(f)
			args := repCreateArgs()
			for key, value := range tc.args {
				args[key] = value
			}
			result, err := callTool(t, mock, false, "truenas_replication_task_create", args)
			text := toolError(t, result, err)
			if !strings.Contains(text, tc.want) {
				t.Errorf("error text = %q, want it to contain %q", text, tc.want)
			}
			if len(calls.methods) != 0 {
				t.Errorf("middleware calls %v, want none", calls.methods)
			}
		})
	}

	// A name of exactly 120 characters is accepted.
	f := repCreateBase()
	args := repCreateArgs()
	args["name"] = strings.Repeat("n", 120)
	decoded, _ := repCreateDryRun(t, f, args)
	if decoded.Payload["name"] != strings.Repeat("n", 120) {
		t.Errorf("name = %v, want the 120-character name", decoded.Payload["name"])
	}
}

func TestReplicationTaskCreate_ReadErrors(t *testing.T) {
	for _, method := range []string{"pool.snapshottask.query", "replication.query", "pool.dataset.query"} {
		t.Run(method, func(t *testing.T) {
			f := repCreateBase()
			f.readErrors = map[string]error{method: errors.New("boom")}
			mock, calls := newRepCreateCaller(f)
			args := repCreateArgs()
			args["dry_run"] = true
			result, err := callTool(t, mock, false, "truenas_replication_task_create", args)
			text := toolError(t, result, err)
			if !strings.Contains(text, method) || !strings.Contains(text, "boom") {
				t.Errorf("error text = %q, want the method name and the cause", text)
			}
			if !strings.HasSuffix(text, "; nothing was created") {
				t.Errorf("error text = %q, want it to end with the nothing-was-created note", text)
			}
			if len(calls.createCalls) != 0 {
				t.Errorf("replication.create calls = %d, want 0", len(calls.createCalls))
			}
		})
	}
}

func TestReplicationTaskCreate_APIError(t *testing.T) {
	f := repCreateBase()
	hash := repCreatePlanHash(t, f, repCreateArgs())
	f.createErr = errors.New("boom")
	mock, calls := newRepCreateCaller(f)
	args := repCreateArgs()
	args["expected_sha256"] = hash
	result, err := callTool(t, mock, false, "truenas_replication_task_create", args)
	text := toolError(t, result, err)
	if !strings.Contains(text, "replication.create:") || !strings.Contains(text, "boom") {
		t.Errorf("error text = %q, want the wrapped method name and the cause", text)
	}
	if !strings.Contains(text, "unknown") || !strings.Contains(text, "truenas_replication_task_list") {
		t.Errorf("error text = %q, want the unknown-result text and the check hint", text)
	}
	if len(calls.createCalls) != 1 {
		t.Errorf("replication.create calls = %d, want exactly 1", len(calls.createCalls))
	}
}

func TestReplicationTaskCreate_Cancelled(t *testing.T) {
	f := repCreateBase()
	hash := repCreatePlanHash(t, f, repCreateArgs())
	mock, calls := newRepCreateCaller(f)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	args := repCreateArgs()
	args["expected_sha256"] = hash
	if _, err := createReplicationTask(ctx, mock, makeReq(t, args)); err == nil {
		t.Fatal("expected the cancelled call to fail")
	} else if !strings.Contains(err.Error(), "nothing was created") {
		t.Errorf("error = %v, want the nothing-was-created note", err)
	}
	if len(calls.createCalls) != 0 {
		t.Errorf("replication.create calls = %d, want 0", len(calls.createCalls))
	}
}

// TestReplicationTaskCreate_SnapshotRunDoesNotChangeTheHash pins the reason the
// snapshot task projection leaves `state` out: a scheduled run between the dry
// run and the write must not change plan_sha256.
func TestReplicationTaskCreate_SnapshotRunDoesNotChangeTheHash(t *testing.T) {
	f := repCreateBase()
	hash := repCreatePlanHash(t, f, repCreateArgs())

	f.snapshotTasks[0]["state"] = map[string]any{
		"state":         "FINISHED",
		"datetime":      map[string]any{"$date": float64(1761224400000)},
		"last_snapshot": "fast@auto-2025-10-23_13-00",
	}
	if again := repCreatePlanHash(t, f, repCreateArgs()); again != hash {
		t.Errorf("plan_sha256 changed to %s after a run of a bound task, want %s", again, hash)
	}
}

// TestReplicationTaskCreate_HiddenDatasetRefused covers the paths that
// pool.dataset.query hides (its internal_datasets_filters): the existence check
// cannot see them, so the tool cannot prove that a target is new or that a
// source exists. The query is answered by the "in" filter, and slow/ix-apps is
// never in the fixture's dataset list, as the middleware would not return it.
func TestReplicationTaskCreate_HiddenDatasetRefused(t *testing.T) {
	hiddenTargets := []string{"slow/ix-apps", "slow/.system/x", "slow/ix-applications/x", "boot-pool/x"}
	for _, target := range hiddenTargets {
		t.Run("target "+target, func(t *testing.T) {
			f := repCreateBase()
			mock, calls := newRepCreateCaller(f)
			args := repCreateArgs()
			args["target_dataset"] = target
			args["dry_run"] = true
			result, err := callTool(t, mock, false, "truenas_replication_task_create", args)
			text := toolError(t, result, err)
			if !strings.Contains(text, `parameter "target_dataset"`) ||
				!strings.Contains(text, "cannot prove that the target dataset is new") {
				t.Errorf("dry run error = %q, want the hidden-target refusal", text)
			}
			if !strings.Contains(text, "nothing was created") {
				t.Errorf("error = %q, want the nothing-was-created note", text)
			}
			if len(calls.methods) != 0 {
				t.Errorf("middleware calls %v, want none", calls.methods)
			}
		})
	}

	// The same refusal on a write, before the hash is compared.
	t.Run("write", func(t *testing.T) {
		f := repCreateBase()
		mock, calls := newRepCreateCaller(f)
		args := repCreateArgs()
		args["target_dataset"] = "slow/ix-apps"
		args["expected_sha256"] = strings.Repeat("0", 64)
		result, err := callTool(t, mock, false, "truenas_replication_task_create", args)
		text := toolError(t, result, err)
		if !strings.Contains(text, "cannot prove that the target dataset is new") {
			t.Errorf("error = %q, want the hidden-target refusal", text)
		}
		if len(calls.methods) != 0 {
			t.Errorf("middleware calls %v, want none", calls.methods)
		}
		if len(calls.createCalls) != 0 {
			t.Errorf("replication.create calls = %d, want 0", len(calls.createCalls))
		}
	})

	t.Run("hidden source", func(t *testing.T) {
		f := repCreateBase()
		f.datasets = append(f.datasets, "slow/ix-apps")
		mock, calls := newRepCreateCaller(f)
		args := repCreateArgs()
		args["source_datasets"] = []any{"slow/ix-apps"}
		args["dry_run"] = true
		result, err := callTool(t, mock, false, "truenas_replication_task_create", args)
		text := toolError(t, result, err)
		if !strings.Contains(text, `parameter "source_datasets"`) ||
			!strings.Contains(text, "cannot prove that the source dataset exists") {
			t.Errorf("error = %q, want the hidden-source refusal", text)
		}
		if len(calls.methods) != 0 {
			t.Errorf("middleware calls %v, want none", calls.methods)
		}
	})

	t.Run("hidden exclude", func(t *testing.T) {
		f := repCreateBase()
		mock, calls := newRepCreateCaller(f)
		args := repCreateArgs()
		args["recursive"] = true
		args["exclude"] = []any{"fast/a/.system"}
		args["dry_run"] = true
		result, err := callTool(t, mock, false, "truenas_replication_task_create", args)
		text := toolError(t, result, err)
		if !strings.Contains(text, `parameter "exclude"`) ||
			!strings.Contains(text, "cannot prove that the excluded dataset exists") {
			t.Errorf("error = %q, want the hidden-exclude refusal", text)
		}
		if len(calls.methods) != 0 {
			t.Errorf("middleware calls %v, want none", calls.methods)
		}
	})
}

// TestReplicationTaskCreate_BoundTaskConfigChangesThePlan pins finding 2 of the
// PR #54 review: the plan hash covers the bound task's whole configuration that
// the dry run shows, so a change to its schedule or allow_empty between the dry
// run and the write is refused.
func TestReplicationTaskCreate_BoundTaskConfigChangesThePlan(t *testing.T) {
	cases := []struct {
		name   string
		change func(task map[string]any)
	}{
		{
			name: "schedule",
			change: func(task map[string]any) {
				task["schedule"] = map[string]any{
					"minute": "30", "hour": "1", "dom": "*", "month": "*", "dow": "*", "begin": "00:00", "end": "23:59",
				}
			},
		},
		{
			name: "allow_empty",
			change: func(task map[string]any) {
				task["allow_empty"] = false
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := repCreateBase()
			hash := repCreatePlanHash(t, f, repCreateArgs())
			tc.change(f.snapshotTasks[0])

			mock, calls := newRepCreateCaller(f)
			args := repCreateArgs()
			args["expected_sha256"] = hash
			result, err := callTool(t, mock, false, "truenas_replication_task_create", args)
			text := toolError(t, result, err)
			if !strings.Contains(text, "changed since the dry run") {
				t.Errorf("error = %q, want the changed-plan refusal", text)
			}
			if len(calls.createCalls) != 0 {
				t.Errorf("replication.create calls = %d, want 0", len(calls.createCalls))
			}
		})
	}
}

// TestReplicationTaskCreate_RegistrationMode checks that the tool exists only
// with writes enabled.
func TestReplicationTaskCreate_RegistrationMode(t *testing.T) {
	f := repCreateBase()
	mock, _ := newRepCreateCaller(f)
	for _, name := range listTools(t, mock, true) {
		if name == "truenas_replication_task_create" {
			t.Fatal("read-only server registered truenas_replication_task_create")
		}
	}
	found := false
	for _, name := range listTools(t, mock, false) {
		if name == "truenas_replication_task_create" {
			found = true
		}
	}
	if !found {
		t.Fatal("server with writes did not register truenas_replication_task_create")
	}
}
