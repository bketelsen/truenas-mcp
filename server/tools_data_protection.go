package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
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
			tasks = append(tasks, map[string]any{
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
			})
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
