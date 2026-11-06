package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"truenas-mcp/truenas"
)

const (
	replicationNameMaxRunes = 120
	replicationLifetimeMin  = 1
	replicationLifetimeMax  = 10000
	replicationCreateMethod = "replication.create"
)

// replicationCreateArgs are the argument names truenas_replication_task_create
// accepts. Any other key is refused before a middleware call.
var replicationCreateArgs = map[string]bool{
	"source_datasets":            true,
	"target_dataset":             true,
	"periodic_snapshot_task_ids": true,
	"name":                       true,
	"recursive":                  true,
	"exclude":                    true,
	"retention_policy":           true,
	"lifetime_value":             true,
	"lifetime_unit":              true,
	"readonly":                   true,
	"enabled":                    true,
	"transport":                  true,
	"dry_run":                    true,
	"expected_sha256":            true,
}

// replicationRetentionPolicies and replicationReadonlyModes hold the enum values
// replication.create accepts, checked against ReplicationCreate in the
// v25_10_0 and v27_0_0 API models of truenas/middleware.
var (
	replicationRetentionPolicies = map[string]bool{"SOURCE": true, "CUSTOM": true, "NONE": true}
	replicationReadonlyModes     = map[string]bool{"SET": true, "REQUIRE": true, "IGNORE": true}
)

// registerReplicationTaskCreateTool registers truenas_replication_task_create,
// which creates a local PUSH replication task bound to existing periodic
// snapshot tasks.
//
// replication.create needs the REPLICATION_TASK_WRITE role (the role_prefix of
// plugins/replication.py is REPLICATION_TASK). Only the LOCAL transport is
// offered: source and target datasets are paths on the same appliance, so no
// SSH credential, keychain entry or encryption key is ever involved.
//
// The tool reads the snapshot tasks, the replication tasks and the datasets the
// plan names, checks the plan against all of them, and hashes the result. A
// write must repeat that hash, so the caller sees exactly the plan that is
// created. replication.create is called once and never again: a transport error
// leaves the task's state unknown (the no-replay rule in truenas/client.go).
func registerReplicationTaskCreateTool(s *mcp.Server, client truenas.Caller) {
	s.AddTool(&mcp.Tool{
		Name: "truenas_replication_task_create",
		Description: "Create a local replication task that copies datasets into a new dataset on this TrueNAS " +
			"appliance. The task is a PUSH with the LOCAL transport only: a remote transport is refused, because the " +
			"source and target datasets must be on this machine, so no SSH credential or encryption key is used. The " +
			"task is bound to existing periodic snapshot tasks, and zettarepl runs it after each run of those tasks; " +
			"every source dataset must be covered by 1 of the bound tasks. The target dataset must not exist yet and " +
			"must lie outside every other snapshot or replication task's tree: zettarepl creates it on the first run. " +
			"Call it with dry_run to get the exact payload, the bound snapshot tasks and plan_sha256; read them, then " +
			"call again with expected_sha256 set to that hash, which is refused when the arguments or the appliance " +
			"changed in between. A write needs the REPLICATION_TASK_WRITE role. replication.create is called once and " +
			"never replayed: when its result is unknown, check truenas_replication_task_list before you try again.",
		InputSchema: schema(map[string]any{
			"source_datasets":            arrayProp("source dataset paths on this appliance, 1 or more (e.g. fast/data)"),
			"target_dataset":             stringProp("new target dataset path below a pool (e.g. slow/backup); it must not exist yet"),
			"periodic_snapshot_task_ids": map[string]any{"type": "array", "items": map[string]any{"type": "number"}, "description": "ids of the periodic snapshot tasks the replication runs after, 1 or more; each source must be covered by 1 of them"},
			"name":                       stringProp("task name (default: the source datasets joined with \", \", then \" - \", then the target), at most 120 characters"),
			"recursive":                  boolProp("replicate child datasets too (default false); exclude is only allowed when true"),
			"exclude":                    arrayProp("datasets to skip; each must be below 1 of the source datasets, and only allowed when recursive is true"),
			"retention_policy":           stringProp("snapshot retention on the target: SOURCE (delete target snapshots absent on the source), CUSTOM, or NONE (default SOURCE; case-insensitive)"),
			"lifetime_value":             numberProp("retention lifetime, an integer from 1 to 10000; required with CUSTOM and refused otherwise"),
			"lifetime_unit":              stringProp("retention lifetime unit: HOUR, DAY, WEEK, MONTH, or YEAR; required with CUSTOM and refused otherwise"),
			"readonly":                   stringProp("readonly mode of the target datasets: SET, REQUIRE, or IGNORE (default SET; case-insensitive)"),
			"enabled":                    boolProp("create the task enabled (default true); an enabled task needs every bound snapshot task enabled"),
			"transport":                  stringProp("only LOCAL is accepted; any other transport is refused (a dry run too)"),
			"dry_run":                    boolProp("when true, return the payload, the bound snapshot tasks and plan_sha256, and do not call replication.create"),
			"expected_sha256":            stringProp("the plan_sha256 a dry run returned; required when dry_run is false"),
		}, "source_datasets", "target_dataset", "periodic_snapshot_task_ids"),
		Annotations: &mcp.ToolAnnotations{
			Title:           "Create Local Replication Task",
			ReadOnlyHint:    false,
			DestructiveHint: new(true), // each run of the task writes into the target dataset, and retention deletes target snapshots
			IdempotentHint:  false,     // every accepted call creates a new task, even when an identical one exists under another name
			OpenWorldHint:   new(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return createReplicationTask(ctx, client, req)
	})
}

// replicationCreatePlan is the checked form of the tool arguments.
type replicationCreatePlan struct {
	sources     []string
	target      string
	snapshotIDs []int
	name        string
	recursive   bool
	exclude     []string
	enabled     bool
	payload     map[string]any
	dryRun      bool
	expected    string
}

// createReplicationTask is the handler of truenas_replication_task_create. It is
// a separate function so tests can call it with a context they cancel.
func createReplicationTask(ctx context.Context, client truenas.Caller, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	plan, err := parseReplicationCreatePlan(req)
	if err != nil {
		return nil, err
	}

	// 1. Every periodic snapshot task: the bound ones and the trees to avoid.
	result, err := client.Call("pool.snapshottask.query")
	if err != nil {
		return nil, fmt.Errorf("pool.snapshottask.query: %w; nothing was created", err)
	}
	var snapshotTasks []map[string]any
	if err := json.Unmarshal(result, &snapshotTasks); err != nil {
		return nil, fmt.Errorf("parsing pool.snapshottask.query: %w; nothing was created", err)
	}
	sortEntriesByID(snapshotTasks)

	// 2. Every replication task: names and target trees that are taken.
	result, err = client.Call("replication.query")
	if err != nil {
		return nil, fmt.Errorf("replication.query: %w; nothing was created", err)
	}
	var replicationTasks []map[string]any
	if err := json.Unmarshal(result, &replicationTasks); err != nil {
		return nil, fmt.Errorf("parsing replication.query: %w; nothing was created", err)
	}
	sortEntriesByID(replicationTasks)

	// 3. The datasets the plan names: the sources, the target and the target's
	// parent. A flat query, so a nested child is not reported twice.
	datasetIDs := replicationCreateDatasetIDs(plan)
	result, err = client.Call("pool.dataset.query", [][]any{{"id", "in", datasetIDs}}, replicationCreateDatasetOptions())
	if err != nil {
		return nil, fmt.Errorf("pool.dataset.query: %w; nothing was created", err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(result, &rows); err != nil {
		return nil, fmt.Errorf("parsing pool.dataset.query: %w; nothing was created", err)
	}
	found := map[string]bool{}
	for _, row := range rows {
		for _, key := range []string{"id", "name"} {
			if name, ok := row[key].(string); ok && name != "" {
				found[name] = true
			}
		}
	}

	// 4. Every check runs on a dry run too, so the dry run gives the answer the
	// write would give.
	if err := checkReplicationCreatePlan(plan, snapshotTasks, replicationTasks, found); err != nil {
		return nil, err
	}

	// 5. The hash covers every input: the payload, the tasks and the datasets.
	planSHA, err := replicationCreatePlanHash(plan.payload, snapshotTasks, replicationTasks, found)
	if err != nil {
		return nil, fmt.Errorf("hashing the plan: %w; nothing was created", err)
	}

	// 6. Preview only.
	if plan.dryRun {
		return jsonValueResult(map[string]any{
			"dry_run":              true,
			"method":               replicationCreateMethod,
			"payload":              plan.payload,
			"plan_sha256":          planSHA,
			"bound_snapshot_tasks": replicationBoundTaskSummaries(plan, snapshotTasks),
			"note": "zettarepl runs this task after each run of the bound periodic snapshot tasks and creates the " +
				"target dataset on the first run. Retention SOURCE deletes target snapshots that are absent on the " +
				"source, and readonly SET sets readonly=on on the target datasets. Call again with expected_sha256 " +
				"set to plan_sha256 to create the task.",
		})
	}

	// 7. The caller must have seen this exact plan.
	if plan.expected == "" {
		return nil, fmt.Errorf("parameter %q is required when dry_run is false: run with dry_run first, then pass the plan_sha256 it returns (the current hash is %s)", "expected_sha256", planSHA)
	}
	if plan.expected != planSHA {
		return nil, fmt.Errorf("expected_sha256 %q does not match the plan hash %s: the arguments or the state of the appliance changed since the dry run; run with dry_run again, read the plan, then pass the plan_sha256 it returns", plan.expected, planSHA)
	}

	// 8. Just before the write: stop on a cancel.
	if ctx.Err() != nil {
		return nil, fmt.Errorf("cancelled before replication.create; nothing was created")
	}

	// 9. One call, never replayed: a transport error can still have created the
	// task, and a 2nd call would create a 2nd task.
	result, err = client.Call(replicationCreateMethod, plan.payload)
	if err != nil {
		return nil, fmt.Errorf("replication.create: %w; the result is unknown: check truenas_replication_task_list before you try again, and do not run this tool again blindly", err)
	}
	var created map[string]any
	if err := json.Unmarshal(result, &created); err != nil {
		return nil, fmt.Errorf("parsing replication.create: %w; the task may exist: check truenas_replication_task_list before you try again, and do not run this tool again blindly", err)
	}

	return jsonValueResult(map[string]any{
		"created": true,
		"task":    replicationTaskSummary(created),
		"note": "zettarepl runs the task after the next run of the bound periodic snapshot tasks; " +
			"truenas_snapshot_task_run on a bound task runs it now.",
	})
}

// parseReplicationCreatePlan checks every argument by hand and builds the exact
// payload. The SDK does not validate arguments against InputSchema, so a value of
// the wrong type must be refused here.
func parseReplicationCreatePlan(req *mcp.CallToolRequest) (*replicationCreatePlan, error) {
	if err := checkReplicationCreateArgNames(req); err != nil {
		return nil, err
	}

	sources, err := strictStringList(req, "source_datasets")
	if err != nil {
		return nil, err
	}
	if len(sources) == 0 {
		return nil, fmt.Errorf("parameter %q must hold at least 1 dataset", "source_datasets")
	}
	seen := make(map[string]bool, len(sources))
	for _, src := range sources {
		if err := checkDatasetPath(src); err != nil {
			return nil, fmt.Errorf("parameter %q: %w", "source_datasets", err)
		}
		if err := refuseHiddenDataset("source_datasets", src, "the source dataset exists"); err != nil {
			return nil, err
		}
		if seen[src] {
			return nil, fmt.Errorf("parameter %q lists %q more than once", "source_datasets", src)
		}
		seen[src] = true
	}

	target, err := strictOptionalString(req, "target_dataset")
	if err != nil {
		return nil, err
	}
	if target == "" {
		return nil, fmt.Errorf("required parameter %q missing", "target_dataset")
	}
	if err := checkDatasetPath(target); err != nil {
		return nil, fmt.Errorf("parameter %q: %w", "target_dataset", err)
	}
	if !strings.Contains(target, "/") {
		return nil, fmt.Errorf("parameter %q must name a dataset below a pool, not the pool root %q", "target_dataset", target)
	}
	if err := refuseHiddenDataset("target_dataset", target, "the target dataset is new"); err != nil {
		return nil, err
	}

	snapshotIDs, err := replicationSnapshotTaskIDs(req)
	if err != nil {
		return nil, err
	}

	name, err := strictOptionalString(req, "name")
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = strings.Join(sources, ", ") + " - " + target
	}
	if utf8.RuneCountInString(name) > replicationNameMaxRunes {
		return nil, fmt.Errorf("parameter %q must be at most %d characters, got %d", "name", replicationNameMaxRunes, utf8.RuneCountInString(name))
	}

	recursive, err := strictBool(req, "recursive", false)
	if err != nil {
		return nil, err
	}
	enabled, err := strictBool(req, "enabled", true)
	if err != nil {
		return nil, err
	}
	dryRun, err := strictBool(req, "dry_run", false)
	if err != nil {
		return nil, err
	}

	exclude, err := strictStringList(req, "exclude")
	if err != nil {
		return nil, err
	}
	if exclude == nil {
		exclude = []string{}
	}
	if len(exclude) > 0 && !recursive {
		return nil, fmt.Errorf("parameter %q is only allowed when %q is true", "exclude", "recursive")
	}
	for _, entry := range exclude {
		if err := checkDatasetPath(entry); err != nil {
			return nil, fmt.Errorf("parameter %q: %w", "exclude", err)
		}
		if err := refuseHiddenDataset("exclude", entry, "the excluded dataset exists"); err != nil {
			return nil, err
		}
		if !belowAnySource(entry, sources) {
			return nil, fmt.Errorf("parameter %q entry %q must be below 1 of the source datasets", "exclude", entry)
		}
	}

	policy, err := strictOptionalString(req, "retention_policy")
	if err != nil {
		return nil, err
	}
	policy = strings.ToUpper(policy)
	if policy == "" {
		policy = "SOURCE"
	}
	if !replicationRetentionPolicies[policy] {
		return nil, fmt.Errorf("parameter %q must be one of SOURCE, CUSTOM, NONE, got %q", "retention_policy", policy)
	}

	lifetime, hasLifetime, err := strictOptionalNumber(req, "lifetime_value")
	if err != nil {
		return nil, err
	}
	if hasLifetime && (lifetime < replicationLifetimeMin || lifetime > replicationLifetimeMax || lifetime != math.Trunc(lifetime)) {
		return nil, fmt.Errorf("parameter %q must be an integer from %d to %d, got %v", "lifetime_value", replicationLifetimeMin, replicationLifetimeMax, lifetime)
	}
	unit, err := strictOptionalString(req, "lifetime_unit")
	if err != nil {
		return nil, err
	}
	unit = strings.ToUpper(unit)
	if unit != "" && !snapshotLifetimeUnits[unit] {
		return nil, fmt.Errorf("parameter %q must be one of HOUR, DAY, WEEK, MONTH, YEAR, got %q", "lifetime_unit", unit)
	}
	if policy == "CUSTOM" {
		if !hasLifetime {
			return nil, fmt.Errorf("parameter %q is required when %q is CUSTOM", "lifetime_value", "retention_policy")
		}
		if unit == "" {
			return nil, fmt.Errorf("parameter %q is required when %q is CUSTOM", "lifetime_unit", "retention_policy")
		}
	} else if hasLifetime || unit != "" {
		return nil, fmt.Errorf("parameters %q and %q are only allowed when %q is CUSTOM", "lifetime_value", "lifetime_unit", "retention_policy")
	}

	readonly, err := strictOptionalString(req, "readonly")
	if err != nil {
		return nil, err
	}
	readonly = strings.ToUpper(readonly)
	if readonly == "" {
		readonly = "SET"
	}
	if !replicationReadonlyModes[readonly] {
		return nil, fmt.Errorf("parameter %q must be one of SET, REQUIRE, IGNORE, got %q", "readonly", readonly)
	}

	transport, err := strictOptionalString(req, "transport")
	if err != nil {
		return nil, err
	}
	if transport != "" && !strings.EqualFold(transport, "LOCAL") {
		return nil, fmt.Errorf("parameter %q is %q, but this tool creates local replication only: the source and target datasets must be on this appliance, so LOCAL is the only accepted transport", "transport", transport)
	}

	expected, err := strictOptionalString(req, "expected_sha256")
	if err != nil {
		return nil, err
	}
	expected = strings.TrimSpace(expected)

	payload := map[string]any{
		"name":                    name,
		"direction":               "PUSH",
		"transport":               "LOCAL",
		"source_datasets":         sources,
		"target_dataset":          target,
		"recursive":               recursive,
		"exclude":                 exclude,
		"periodic_snapshot_tasks": snapshotIDs,
		"auto":                    true,
		"retention_policy":        policy,
		"readonly":                readonly,
		"enabled":                 enabled,
		"allow_from_scratch":      false,
		"replicate":               false,
		"encryption":              false,
	}
	if policy == "CUSTOM" {
		payload["lifetime_value"] = int(lifetime)
		payload["lifetime_unit"] = unit
	}

	return &replicationCreatePlan{
		sources:     sources,
		target:      target,
		snapshotIDs: snapshotIDs,
		name:        name,
		recursive:   recursive,
		exclude:     exclude,
		enabled:     enabled,
		payload:     payload,
		dryRun:      dryRun,
		expected:    expected,
	}, nil
}

// checkReplicationCreateArgNames refuses every argument the tool does not
// implement, before a middleware call, so a typo or a remote-replication
// argument (ssh_credentials, direction, allow_from_scratch, encryption) cannot
// be dropped silently.
func checkReplicationCreateArgNames(req *mcp.CallToolRequest) error {
	args := parseArgs(req)
	keys := make([]string, 0, len(args))
	for key := range args {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !replicationCreateArgs[key] {
			return fmt.Errorf("unknown parameter %q", key)
		}
	}
	return nil
}

// replicationSnapshotTaskIDs reads periodic_snapshot_task_ids: 1 or more integer
// ids of at least 1, with no duplicates.
func replicationSnapshotTaskIDs(req *mcp.CallToolRequest) ([]int, error) {
	values, err := strictNumberList(req, "periodic_snapshot_task_ids")
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("parameter %q must hold at least 1 id", "periodic_snapshot_task_ids")
	}
	ids := make([]int, 0, len(values))
	seen := make(map[int]bool, len(values))
	for i, value := range values {
		if value < 1 || value > math.MaxInt32 || value != math.Trunc(value) {
			return nil, fmt.Errorf("parameter %q item %d must be an integer of at least 1, got %v", "periodic_snapshot_task_ids", i, value)
		}
		id := int(value)
		if seen[id] {
			return nil, fmt.Errorf("parameter %q lists id %d more than once", "periodic_snapshot_task_ids", id)
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, nil
}

// strictOptionalString returns a string field, or "" when the key is absent or
// null. A present value of another type is an error.
func strictOptionalString(req *mcp.CallToolRequest, field string) (string, error) {
	m := parseArgs(req)
	v, present := m[field]
	if !present || v == nil {
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("parameter %q must be a string, got %T", field, v)
	}
	return s, nil
}

// strictOptionalNumber returns a number field and whether the key is present. A
// present value of another type is an error.
func strictOptionalNumber(req *mcp.CallToolRequest, field string) (float64, bool, error) {
	m := parseArgs(req)
	v, present := m[field]
	if !present || v == nil {
		return 0, false, nil
	}
	n, ok := v.(float64)
	if !ok {
		return 0, false, fmt.Errorf("parameter %q must be a number, got %T", field, v)
	}
	return n, true, nil
}

// strictNumberList returns the numeric items of an array field, or nil when the
// key is absent or null. A present value that is not an array of numbers is an
// error, so a mistyped list cannot be dropped silently.
func strictNumberList(req *mcp.CallToolRequest, field string) ([]float64, error) {
	m := parseArgs(req)
	v, present := m[field]
	if !present || v == nil {
		return nil, nil
	}
	items, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("parameter %q must be an array of numbers, got %T", field, v)
	}
	out := make([]float64, 0, len(items))
	for i, item := range items {
		n, ok := item.(float64)
		if !ok {
			return nil, fmt.Errorf("parameter %q must contain only numbers (item %d is %T)", field, i, item)
		}
		out = append(out, n)
	}
	return out, nil
}

// checkDatasetPath refuses a path that is not a dataset path: empty, a leading
// or trailing "/", "//", "@", "#", or whitespace.
func checkDatasetPath(path string) error {
	switch {
	case path == "":
		return fmt.Errorf("a dataset path must not be empty")
	case strings.HasPrefix(path, "/") || strings.HasSuffix(path, "/"):
		return fmt.Errorf("%q must not start or end with \"/\"", path)
	case strings.Contains(path, "//"):
		return fmt.Errorf("%q must not contain \"//\"", path)
	case strings.ContainsAny(path, "@#"):
		return fmt.Errorf("%q must not contain \"@\" or \"#\"", path)
	case strings.ContainsFunc(path, unicode.IsSpace):
		return fmt.Errorf("%q must not contain whitespace", path)
	}
	return nil
}

// hiddenDatasetPath reports whether pool.dataset.query hides the path. The
// middleware drops internal datasets from that query (its
// internal_datasets_filters): a dataset of the boot pool (boot-pool or
// freenas-boot), 1 whose id contains "/.system/", 1 whose id contains
// "/ix-applications/", and 1 whose id contains "/ix-apps". The path gets a
// trailing "/" first, so a hidden dataset also matches when it is the last name
// in the path ("slow/.system", "slow/ix-applications").
func hiddenDatasetPath(path string) bool {
	if pool, _, _ := strings.Cut(path, "/"); pool == "boot-pool" || pool == "freenas-boot" {
		return true
	}
	padded := path + "/"
	for _, marker := range []string{"/.system/", "/ix-applications/", "/ix-apps"} {
		if strings.Contains(padded, marker) {
			return true
		}
	}
	return false
}

// refuseHiddenDataset refuses a path that the existence query cannot show, so
// the tool cannot prove that a dataset is new or that a source exists. It runs
// with the arguments, before any middleware call.
func refuseHiddenDataset(field, path, claim string) error {
	if !hiddenDatasetPath(path) {
		return nil
	}
	return fmt.Errorf("parameter %q: %q is an internal dataset path, which the pool.dataset.query this tool uses does not return (it hides the boot pool, .system, ix-applications and ix-apps), so the tool cannot prove that %s; nothing was created", field, path, claim)
}

// belowAnySource reports whether path is a strict child of 1 of the sources.
func belowAnySource(path string, sources []string) bool {
	for _, src := range sources {
		if strings.HasPrefix(path, src+"/") {
			return true
		}
	}
	return false
}

// within reports whether path is root itself or a dataset below it.
func within(path, root string) bool {
	return path == root || strings.HasPrefix(path, root+"/")
}

// replicationCreateDatasetIDs lists the datasets the plan needs: the sources,
// the target and the target's parent, sorted and without duplicates.
func replicationCreateDatasetIDs(plan *replicationCreatePlan) []string {
	ids := make([]string, 0, len(plan.sources)+2)
	ids = append(ids, plan.sources...)
	ids = append(ids, plan.target, plan.target[:strings.LastIndex(plan.target, "/")])
	sort.Strings(ids)
	return slices.Compact(ids)
}

// replicationCreateDatasetOptions is the flat, allow-listed query the tool needs:
// only the id and the name of each dataset, and no nesting.
func replicationCreateDatasetOptions() map[string]any {
	return map[string]any{
		"select": []string{"id", "name"},
		"extra": map[string]any{
			"flat":                true,
			"retrieve_children":   false,
			"properties":          []string{},
			"retrieve_user_props": false,
		},
	}
}

// checkReplicationCreatePlan runs checks a to f against the tasks and datasets
// the appliance reported. Every refusal says that nothing was created.
func checkReplicationCreatePlan(plan *replicationCreatePlan, snapshotTasks, replicationTasks []map[string]any, found map[string]bool) error {
	// a. The sources exist, the target does not, and the target's parent does.
	for _, src := range plan.sources {
		if !found[src] {
			return fmt.Errorf("source dataset %q does not exist; nothing was created", src)
		}
	}
	if found[plan.target] {
		return fmt.Errorf("target dataset %q already exists: zettarepl creates the target on the first run, and the tool replicates into a new dataset only; nothing was created", plan.target)
	}
	parent := plan.target[:strings.LastIndex(plan.target, "/")]
	if !found[parent] {
		return fmt.Errorf("the parent of target dataset %q, %q, does not exist; nothing was created", plan.target, parent)
	}

	// b. The source and target trees must not overlap.
	for _, src := range plan.sources {
		if within(plan.target, src) {
			return fmt.Errorf("target dataset %q is inside source dataset %q; nothing was created", plan.target, src)
		}
		if within(src, plan.target) {
			return fmt.Errorf("source dataset %q is inside target dataset %q, which does not exist yet; nothing was created", src, plan.target)
		}
	}

	// c. Names are how a person tells 2 tasks apart, so a name is used once.
	for _, task := range replicationTasks {
		if existing, _ := task["name"].(string); existing == plan.name {
			return fmt.Errorf("a replication task named %q already exists (id %v); nothing was created", plan.name, task["id"])
		}
	}

	// d. No other replication task may own a part of the target tree, or
	// replicate from a source that covers the target.
	for _, task := range replicationTasks {
		id, taskName := task["id"], taskStringField(task, "name")
		target, _ := task["target_dataset"].(string)
		if within(plan.target, target) || within(target, plan.target) {
			return fmt.Errorf("replication task %v (%q) has target dataset %q, which overlaps target dataset %q; nothing was created", id, taskName, target, plan.target)
		}
		recursive, _ := task["recursive"].(bool)
		exclude := stringListField(task["exclude"])
		for _, src := range stringListField(task["source_datasets"]) {
			if covered, _ := taskCovers(src, recursive, exclude, plan.target); covered {
				return fmt.Errorf("replication task %v (%q) replicates source dataset %q, which covers target dataset %q; nothing was created", id, taskName, src, plan.target)
			}
		}
	}

	// e. The target must be outside every snapshot task's tree.
	for _, task := range snapshotTasks {
		dataset, _ := task["dataset"].(string)
		recursive, _ := task["recursive"].(bool)
		if covered, _ := taskCovers(dataset, recursive, stringListField(task["exclude"]), plan.target); covered {
			return fmt.Errorf("periodic snapshot task %v snapshots dataset %q, which covers target dataset %q, so the target must be outside every snapshot task's tree; nothing was created", task["id"], dataset, plan.target)
		}
	}

	// f. The bound snapshot tasks must exist, be enabled, and snapshot what the
	// replication copies. The rule mirrors _validate_source_datasets in
	// plugins/replication.py of the middleware.
	return checkReplicationBoundTasks(plan, snapshotTasks)
}

// checkReplicationBoundTasks runs check f: the bound periodic snapshot tasks are
// the ones zettarepl waits for, so every source must be theirs to snapshot.
func checkReplicationBoundTasks(plan *replicationCreatePlan, snapshotTasks []map[string]any) error {
	byID := make(map[int64]map[string]any, len(snapshotTasks))
	for _, task := range snapshotTasks {
		if id, ok := int64Value(task["id"]); ok {
			byID[id] = task
		}
	}
	bound := make([]map[string]any, 0, len(plan.snapshotIDs))
	for _, id := range plan.snapshotIDs {
		task, ok := byID[int64(id)]
		if !ok {
			return fmt.Errorf("periodic snapshot task %d does not exist; nothing was created", id)
		}
		// zettarepl runs an enabled replication only after an enabled snapshot
		// task, so an enabled plan needs enabled tasks.
		if plan.enabled {
			if enabled, _ := task["enabled"].(bool); !enabled {
				return fmt.Errorf("periodic snapshot task %d (dataset %q) is disabled, and the replication would be enabled: zettarepl runs an enabled replication only after an enabled snapshot task; nothing was created", id, taskStringField(task, "dataset"))
			}
		}
		bound = append(bound, task)
	}

	excluded := make(map[string]bool, len(plan.exclude))
	for _, entry := range plan.exclude {
		excluded[entry] = true
	}

	for _, src := range plan.sources {
		covered := false
		for _, task := range bound {
			dataset, _ := task["dataset"].(string)
			recursive, _ := task["recursive"].(bool)
			taskExclude := stringListField(task["exclude"])
			if covers, _ := taskCovers(dataset, recursive, taskExclude, src); covers {
				covered = true
			}
			if !within(src, dataset) {
				continue
			}
			if plan.recursive {
				for _, entry := range taskExclude {
					if within(entry, src) && !excluded[entry] {
						return fmt.Errorf("periodic snapshot task %v (dataset %q) excludes %q, which is inside source dataset %q: a recursive replication must exclude it too; nothing was created", task["id"], dataset, entry, src)
					}
				}
				continue
			}
			for _, entry := range taskExclude {
				if entry == src {
					return fmt.Errorf("periodic snapshot task %v (dataset %q) excludes source dataset %q, so the replication would copy nothing; nothing was created", task["id"], dataset, src)
				}
			}
		}
		if !covered {
			return fmt.Errorf("source dataset %q is not covered by any of the bound periodic snapshot tasks %v; nothing was created", src, plan.snapshotIDs)
		}
	}
	return nil
}

// replicationBoundTaskSummaries projects the bound snapshot tasks onto the list
// tool's fields, in the order the caller gave the ids.
func replicationBoundTaskSummaries(plan *replicationCreatePlan, snapshotTasks []map[string]any) []map[string]any {
	byID := make(map[int64]map[string]any, len(snapshotTasks))
	for _, task := range snapshotTasks {
		if id, ok := int64Value(task["id"]); ok {
			byID[id] = task
		}
	}
	out := make([]map[string]any, 0, len(plan.snapshotIDs))
	for _, id := range plan.snapshotIDs {
		if task, ok := byID[int64(id)]; ok {
			out = append(out, snapshotTaskSummary(task))
		}
	}
	return out
}

// replicationCreatePlanHash hashes every input of replication.create, so a write
// can prove that it creates the plan the caller reviewed. Every configuration
// field the dry run shows is covered, and so is every field of the tasks and the
// datasets the checks read. json.Marshal sorts map keys, so the text of a given
// plan is stable.
func replicationCreatePlanHash(payload map[string]any, snapshotTasks, replicationTasks []map[string]any, found map[string]bool) (string, error) {
	snapshotPlan := make([]map[string]any, 0, len(snapshotTasks))
	for _, task := range snapshotTasks {
		// Only state is left out: a run of the task must not change the hash.
		snapshotPlan = append(snapshotPlan, map[string]any{
			"id":             task["id"],
			"dataset":        task["dataset"],
			"recursive":      task["recursive"],
			"exclude":        listOrEmpty(task["exclude"]),
			"enabled":        task["enabled"],
			"naming_schema":  task["naming_schema"],
			"lifetime_value": task["lifetime_value"],
			"lifetime_unit":  task["lifetime_unit"],
			"schedule":       task["schedule"],
			"allow_empty":    task["allow_empty"],
		})
	}
	replicationPlan := make([]map[string]any, 0, len(replicationTasks))
	for _, task := range replicationTasks {
		replicationPlan = append(replicationPlan, map[string]any{
			"id":                         task["id"],
			"name":                       task["name"],
			"direction":                  task["direction"],
			"transport":                  task["transport"],
			"source_datasets":            listOrEmpty(task["source_datasets"]),
			"target_dataset":             task["target_dataset"],
			"recursive":                  task["recursive"],
			"exclude":                    listOrEmpty(task["exclude"]),
			"enabled":                    task["enabled"],
			"auto":                       task["auto"],
			"periodic_snapshot_task_ids": periodicSnapshotTaskIDs(task["periodic_snapshot_tasks"]),
		})
	}
	foundIDs := make([]string, 0, len(found))
	for id := range found {
		foundIDs = append(foundIDs, id)
	}
	sort.Strings(foundIDs)

	encoded, err := json.Marshal(map[string]any{
		"payload":           payload,
		"snapshot_tasks":    snapshotPlan,
		"replication_tasks": replicationPlan,
		"datasets_found":    foundIDs,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// taskStringField returns a string field of a task entry, or "" when it is not a
// string.
func taskStringField(task map[string]any, key string) string {
	value, _ := task[key].(string)
	return value
}

// stringListField returns the string items of an entry's list field.
func stringListField(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// int64Value returns an entry's id as an integer, and false when the id is not
// an integral JSON number.
func int64Value(v any) (int64, bool) {
	n, ok := v.(float64)
	if !ok || n != math.Trunc(n) {
		return 0, false
	}
	return int64(n), true
}
