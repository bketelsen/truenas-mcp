package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"truenas-mcp/truenas"
)

// registerDataProtectionReadTools registers the read-only views of periodic
// snapshot tasks and replication tasks.
//
// Both tools build their output from an allow-list of fields instead of passing
// the middleware entry through. A replication entry carries `encryption_key`,
// `encryption_key_location`, and an `ssh_credentials` keychain credential whose
// `attributes` can hold a PRIVATE KEY; none of those may reach the result.
// Field names were checked against PeriodicSnapshotTaskEntry and
// ReplicationEntry in the v25_10_0 and v27_0_0 API models of truenas/middleware.
func registerDataProtectionReadTools(s *mcp.Server, client truenas.Caller) {
	s.AddTool(&mcp.Tool{
		Name: "truenas_snapshot_task_list",
		Description: "List periodic snapshot tasks with dataset, schedule, retention, naming schema, enabled state, " +
			"and the result of the last run. The summary counts total, enabled, and failing tasks " +
			"(last run state ERROR). Optionally filter by dataset.",
		InputSchema: schema(map[string]any{
			"dataset": stringProp("optional dataset path; only tasks for exactly this dataset are returned"),
		}),
		Annotations: &mcp.ToolAnnotations{
			Title:         "List Periodic Snapshot Tasks",
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var params []any
		if dataset := optionalString(req, "dataset"); dataset != "" {
			params = append(params, [][]any{{"dataset", "=", dataset}})
		}
		result, err := client.Call("pool.snapshottask.query", params...)
		if err != nil {
			return nil, fmt.Errorf("pool.snapshottask.query: %w", err)
		}
		var entries []map[string]any
		if err := json.Unmarshal(result, &entries); err != nil {
			return nil, fmt.Errorf("parsing pool.snapshottask.query: %w", err)
		}
		sortEntriesByID(entries)

		tasks := make([]map[string]any, 0, len(entries))
		for _, e := range entries {
			tasks = append(tasks, snapshotTaskSummary(e))
		}
		return jsonValueResult(taskReport(entries, tasks))
	})

	s.AddTool(&mcp.Tool{
		Name: "truenas_replication_task_list",
		Description: "List replication tasks with direction, transport, source and target datasets, schedule, " +
			"retention, and the result of the last run. The summary counts total, enabled, and failing tasks " +
			"(last run state ERROR). Encryption keys and SSH credential secrets are never returned: " +
			"an SSH credential shows only its id and name.",
		InputSchema: noArgs(),
		Annotations: &mcp.ToolAnnotations{
			Title:         "List Replication Tasks",
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := client.Call("replication.query")
		if err != nil {
			return nil, fmt.Errorf("replication.query: %w", err)
		}
		var entries []map[string]any
		if err := json.Unmarshal(result, &entries); err != nil {
			return nil, fmt.Errorf("parsing replication.query: %w", err)
		}
		sortEntriesByID(entries)

		tasks := make([]map[string]any, 0, len(entries))
		for _, e := range entries {
			encryption, _ := e["encryption"].(bool)
			tasks = append(tasks, map[string]any{
				"id":                         e["id"],
				"name":                       e["name"],
				"direction":                  e["direction"],
				"transport":                  e["transport"],
				"source_datasets":            listOrEmpty(e["source_datasets"]),
				"target_dataset":             e["target_dataset"],
				"recursive":                  e["recursive"],
				"exclude":                    listOrEmpty(e["exclude"]),
				"enabled":                    e["enabled"],
				"auto":                       e["auto"],
				"schedule":                   e["schedule"],
				"ssh_credentials":            sshCredentialSummary(e["ssh_credentials"]),
				"periodic_snapshot_task_ids": periodicSnapshotTaskIDs(e["periodic_snapshot_tasks"]),
				"naming_schema":              listOrEmpty(e["naming_schema"]),
				"also_include_naming_schema": listOrEmpty(e["also_include_naming_schema"]),
				"readonly":                   e["readonly"],
				"encryption":                 encryption, // the flag only; encryption_key never leaves the middleware
				"retention": map[string]any{
					"policy": e["retention_policy"],
					"value":  e["lifetime_value"],
					"unit":   e["lifetime_unit"],
				},
				"last_run": lastRun(e["state"], e["job"], true),
			})
		}
		return jsonValueResult(taskReport(entries, tasks))
	})
}

// registerDataProtectionWriteTools registers the write tool that creates a
// periodic snapshot task.
//
// The client never replays a call after a transport error, and an AI client may
// retry a call whose result it did not see. The tool therefore queries the
// existing tasks first and refuses to create a second task for the same dataset
// and naming schema, so a retry cannot add a duplicate schedule.
func registerDataProtectionWriteTools(s *mcp.Server, client truenas.Caller) {
	s.AddTool(&mcp.Tool{
		Name: "truenas_snapshot_task_create",
		Description: "Create a periodic snapshot task: TrueNAS snapshots the dataset on the given cron schedule and " +
			"deletes the task's own expired snapshots by the retention lifetime. Retention can also delete existing " +
			"snapshots on the dataset whose names match the naming schema. Before creating, the tool queries the " +
			"existing tasks and returns an error naming the task id when one for the same dataset and naming schema " +
			"exists. Set dry_run to return the exact payload without calling pool.snapshottask.create.",
		InputSchema: schema(map[string]any{
			"dataset":        stringProp("dataset path to snapshot (e.g. tank/data)"),
			"recursive":      boolProp("snapshot child datasets too (default false)"),
			"exclude":        arrayProp("datasets to skip; only allowed when recursive is true"),
			"lifetime_value": numberProp("retention lifetime, an integer of at least 1"),
			"lifetime_unit":  stringProp("retention lifetime unit: HOUR, DAY, WEEK, MONTH, or YEAR (case-insensitive)"),
			"naming_schema":  stringProp("snapshot name schema (default auto-%Y-%m-%d_%H-%M)"),
			"schedule": objectProp("cron schedule: minute and hour required; dom, month, and dow default to *; " +
				"begin (default 00:00) and end (default 23:59) are HH:MM"),
			"allow_empty": boolProp("take a snapshot even when the dataset holds no data (default true)"),
			"enabled":     boolProp("create the task enabled (default true)"),
			"dry_run":     boolProp("when true, return the exact payload without calling pool.snapshottask.create"),
		}, "dataset", "lifetime_value", "lifetime_unit", "schedule"),
		Annotations: &mcp.ToolAnnotations{
			Title:           "Create Periodic Snapshot Task",
			ReadOnlyHint:    false,
			DestructiveHint: new(true), // retention selects snapshots by dataset and naming schema, so it can delete snapshots this task did not make
			IdempotentHint:  false,     // the duplicate guard is a read-then-write race, so a repeat can add a task
			OpenWorldHint:   new(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		dataset, err := requireString(req, "dataset")
		if err != nil {
			return nil, err
		}
		recursive, err := strictBool(req, "recursive", false)
		if err != nil {
			return nil, err
		}
		allowEmpty, err := strictBool(req, "allow_empty", true)
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

		paths, err := strictStringList(req, "exclude")
		if err != nil {
			return nil, err
		}
		exclude := paths
		if exclude == nil {
			exclude = []string{}
		}
		if len(exclude) > 0 && !recursive {
			return nil, fmt.Errorf("parameter %q is only allowed when %q is true", "exclude", "recursive")
		}

		lifetime, err := requireFloat64(req, "lifetime_value")
		if err != nil {
			return nil, err
		}
		// The upper bound keeps the value far inside the middleware Integer field
		// and inside an int, which the conversion below needs.
		if lifetime < 1 || lifetime > 10000 || lifetime != math.Trunc(lifetime) {
			return nil, fmt.Errorf("parameter %q must be an integer from 1 to 10000, got %v", "lifetime_value", lifetime)
		}
		unit, err := snapshotLifetimeUnit(req)
		if err != nil {
			return nil, err
		}
		schedule, err := snapshotTaskSchedule(req)
		if err != nil {
			return nil, err
		}
		namingSchema := optionalString(req, "naming_schema")
		if namingSchema == "" {
			namingSchema = defaultSnapshotNamingSchema
		}

		payload := map[string]any{
			"dataset":        dataset,
			"recursive":      recursive,
			"exclude":        exclude,
			"lifetime_value": int(lifetime),
			"lifetime_unit":  unit,
			"naming_schema":  namingSchema,
			"schedule":       schedule,
			"allow_empty":    allowEmpty,
			"enabled":        enabled,
		}

		// Duplicate guard: a retried call must not add a second task with the
		// same dataset and naming schema. The check also runs on a dry run, so
		// the answer matches what a real call would do.
		result, err := client.Call("pool.snapshottask.query", [][]any{{"dataset", "=", dataset}})
		if err != nil {
			return nil, fmt.Errorf("pool.snapshottask.query: %w", err)
		}
		var existing []map[string]any
		if err := json.Unmarshal(result, &existing); err != nil {
			return nil, fmt.Errorf("parsing pool.snapshottask.query: %w", err)
		}
		for _, task := range existing {
			if existingSchema, _ := task["naming_schema"].(string); existingSchema == namingSchema {
				return nil, fmt.Errorf("a periodic snapshot task for dataset %q with naming schema %q already exists (id %v); "+
					"nothing was created", dataset, namingSchema, task["id"])
			}
		}

		if dryRun {
			return jsonValueResult(map[string]any{
				"dry_run": true,
				"method":  "pool.snapshottask.create",
				"payload": payload,
			})
		}

		result, err = client.Call("pool.snapshottask.create", payload)
		if err != nil {
			return nil, fmt.Errorf("pool.snapshottask.create: %w", err)
		}
		var created map[string]any
		if err := json.Unmarshal(result, &created); err != nil {
			return nil, fmt.Errorf("parsing pool.snapshottask.create: %w", err)
		}
		return jsonValueResult(snapshotTaskSummary(created))
	})
}

// defaultSnapshotNamingSchema is the naming schema the create tool sends when
// the caller omits one; it is the TrueNAS default for a periodic snapshot task.
const defaultSnapshotNamingSchema = "auto-%Y-%m-%d_%H-%M"

// snapshotLifetimeUnits holds the retention units pool.snapshottask.create
// accepts.
var snapshotLifetimeUnits = map[string]bool{
	"HOUR":  true,
	"DAY":   true,
	"WEEK":  true,
	"MONTH": true,
	"YEAR":  true,
}

// snapshotLifetimeUnit reads lifetime_unit and returns it upper-cased, the form
// the middleware enum expects, or an error when it is missing or unknown.
func snapshotLifetimeUnit(req *mcp.CallToolRequest) (string, error) {
	unit, err := requireString(req, "lifetime_unit")
	if err != nil {
		return "", err
	}
	unit = strings.ToUpper(unit)
	if !snapshotLifetimeUnits[unit] {
		return "", fmt.Errorf("parameter %q must be one of HOUR, DAY, WEEK, MONTH, YEAR, got %q", "lifetime_unit", unit)
	}
	return unit, nil
}

// snapshotTaskSchedule builds the complete 7-key schedule object from the
// "schedule" argument. Unknown keys are refused, so a typo cannot fall back to
// a default silently.
func snapshotTaskSchedule(req *mcp.CallToolRequest) (map[string]any, error) {
	raw, err := requireObject(req, "schedule")
	if err != nil {
		return nil, err
	}
	allowed := []string{"minute", "hour", "dom", "month", "dow", "begin", "end"}
	isAllowed := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		isAllowed[key] = true
	}

	values := make(map[string]string, len(raw))
	for key, v := range raw {
		if !isAllowed[key] {
			return nil, fmt.Errorf("unknown schedule key %q; allowed keys are %s", key, strings.Join(allowed, ", "))
		}
		value, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("schedule key %q must be a string, got %T", key, v)
		}
		values[key] = value
	}

	out := make(map[string]any, len(allowed))
	for _, key := range []string{"minute", "hour"} {
		if values[key] == "" {
			return nil, fmt.Errorf("required parameter %q missing from schedule", key)
		}
		out[key] = values[key]
	}
	for _, key := range []string{"dom", "month", "dow"} {
		value := values[key]
		if value == "" {
			value = "*"
		}
		out[key] = value
	}
	defaults := map[string]string{"begin": "00:00", "end": "23:59"}
	for _, key := range []string{"begin", "end"} {
		value := values[key]
		if value == "" {
			value = defaults[key]
		}
		// time.Parse accepts "1:30"; the middleware and the docs use HH:MM, so
		// the canonical form must equal the input.
		parsed, err := time.Parse("15:04", value)
		if err != nil || parsed.Format("15:04") != value {
			return nil, fmt.Errorf("schedule %s must be HH:MM, got %q", key, value)
		}
		out[key] = value
	}
	return out, nil
}

// snapshotTaskSummary projects a PeriodicSnapshotTaskEntry onto the fields both
// snapshot task tools report. It is an allow-list: a field that is not listed
// here never reaches a result.
func snapshotTaskSummary(e map[string]any) map[string]any {
	return map[string]any{
		"id":            e["id"],
		"dataset":       e["dataset"],
		"recursive":     e["recursive"],
		"exclude":       listOrEmpty(e["exclude"]),
		"enabled":       e["enabled"],
		"naming_schema": e["naming_schema"],
		"allow_empty":   e["allow_empty"],
		"schedule":      e["schedule"],
		"retention": map[string]any{
			"value": e["lifetime_value"],
			"unit":  e["lifetime_unit"],
		},
		"last_run": lastRun(e["state"], nil, false),
	}
}

// taskReport wraps built tasks with the summary counts. entries and tasks are
// parallel: failing is read from the already built last_run of each task.
func taskReport(entries []map[string]any, tasks []map[string]any) map[string]any {
	enabled, failing := 0, 0
	for i, e := range entries {
		if on, _ := e["enabled"].(bool); on {
			enabled++
		}
		if lr, ok := tasks[i]["last_run"].(map[string]any); ok && lr["state"] == "ERROR" {
			failing++
		}
	}
	return map[string]any{
		"summary": map[string]any{
			"total":   len(entries),
			"enabled": enabled,
			"failing": failing,
		},
		"tasks": tasks,
	}
}

// sortEntriesByID orders entries by their numeric id. Entries without a numeric
// id keep their relative order after the ones that have one.
func sortEntriesByID(entries []map[string]any) {
	sort.SliceStable(entries, func(i, j int) bool {
		a, aok := entries[i]["id"].(float64)
		b, bok := entries[j]["id"].(float64)
		if aok != bok {
			return aok
		}
		return a < b
	})
}

// listOrEmpty returns v when it is a JSON array and an empty array otherwise,
// so list fields never come out as null.
func listOrEmpty(v any) any {
	if l, ok := v.([]any); ok {
		return l
	}
	return []any{}
}

// sshCredentialSummary reduces a keychain credential to its id and name. The
// credential's attributes can hold a private key and must never be returned.
func sshCredentialSummary(v any) any {
	cred, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	return map[string]any{"id": cred["id"], "name": cred["name"]}
}

// periodicSnapshotTaskIDs lists the ids of the snapshot tasks a replication uses.
func periodicSnapshotTaskIDs(v any) []any {
	ids := []any{}
	list, _ := v.([]any)
	for _, item := range list {
		if task, ok := item.(map[string]any); ok {
			if id, ok := task["id"]; ok {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// wireDatetime converts a middleware datetime, {"$date": unix millis}, to an
// RFC 3339 UTC string. Any other form is returned unchanged; this never fails.
func wireDatetime(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	ms, ok := m["$date"].(float64)
	if !ok {
		return v
	}
	return time.UnixMilli(int64(ms)).UTC().Format(time.RFC3339)
}

// lastRun builds the last_run object from a zettarepl task state. state is
// normally {"state", "datetime", "last_snapshot", "error", "job"}, every key
// but "state" optional; some releases send just a string. Absent keys are
// omitted. When withJob is set and topJob (a replication's top-level job) or
// state.job is an object, only its id and state are added, as "job".
func lastRun(state any, topJob any, withJob bool) map[string]any {
	out := map[string]any{}
	var stateJob any
	switch st := state.(type) {
	case string:
		out["state"] = st
	case map[string]any:
		if v, ok := st["state"]; ok && v != nil {
			out["state"] = v
		}
		if v, ok := st["datetime"]; ok && v != nil {
			out["datetime"] = wireDatetime(v)
		}
		for _, key := range []string{"last_snapshot", "error"} {
			if v, ok := st[key]; ok && v != nil {
				out[key] = v
			}
		}
		stateJob = st["job"]
	}
	if !withJob {
		return out
	}
	job, ok := topJob.(map[string]any)
	if !ok {
		job, ok = stateJob.(map[string]any)
	}
	if ok {
		summary := map[string]any{}
		for _, key := range []string{"id", "state"} {
			if v, present := job[key]; present {
				summary[key] = v
			}
		}
		out["job"] = summary
	}
	return out
}
