package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"truenas-mcp/truenas"
)

// maxRsyncTaskID bounds the id filter. The value keeps the filter inside the
// middleware Integer field and inside an int, which the conversion below needs.
const maxRsyncTaskID = 2147483647

// registerRsyncReadTools registers the read-only view of rsync tasks.
//
// The tool builds its output from an allow-list of fields instead of passing the
// middleware entry through: an entry carries `ssh_credentials`, a full keychain
// credential whose `attributes` can hold a private key, a `job` dict with the
// job arguments, logs and traceback, and `extra`, which can hold a password
// file path. None of those may reach the result.
//
// Field names were checked against `RsyncTaskEntry` in the v27_0_0 API model of
// truenas/middleware (`api/v27_0_0/rsync_task.py`). The method is
// `rsynctask.query`, the service `RsyncTaskService` in `plugins/rsync.py`; its
// role prefix is `SNAPSHOT_TASK`, so a read needs `SNAPSHOT_TASK_READ`. Of the job, `utils/service/task_state.py`
// keeps the last finished job in the task row. `locked` comes from
// `pool.dataset.path_in_locked_datasets` for the task path
// (`service/sharing_service.py`), not from a run, although the API text says
// "currently locked (running)".
func registerRsyncReadTools(s *mcp.Server, client truenas.Caller) {
	s.AddTool(&mcp.Tool{
		Name: "truenas_rsync_task_list",
		Description: "List rsync tasks with direction (PULL or PUSH), mode (SSH or MODULE), local path and dataset, " +
			"remote host, port, module and path, user, schedule, flags, enabled and locked state, and the running, " +
			"queued or last finished job with its state and times. The summary lists the ids of disabled, failing, " +
			"running, locked and never-run tasks. An SSH credential shows only its id and name. Optionally filter " +
			"by task id.",
		InputSchema: schema(map[string]any{
			"id": numberProp("optional task id; only the task with exactly this id is returned"),
		}),
		Annotations: &mcp.ToolAnnotations{
			Title:         "List Rsync Tasks",
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, filtered, err := rsyncTaskID(req)
		if err != nil {
			return nil, err
		}
		var params []any
		if filtered {
			params = append(params, [][]any{{"id", "=", id}})
		}
		raw, callErr := client.Call("rsynctask.query", params...)
		entries, err := queryRows("rsynctask.query", raw, callErr)
		if err != nil {
			return nil, err
		}
		if filtered && len(entries) == 0 {
			return nil, fmt.Errorf("rsync task %d not found", id)
		}
		sortEntriesByID(entries)

		tasks := make([]map[string]any, 0, len(entries))
		for _, e := range entries {
			tasks = append(tasks, rsyncTaskSummary(e))
		}
		return jsonValueResult(rsyncTaskReport(tasks))
	})
}

// rsyncTaskID reads the optional id filter. It reports whether the caller sent
// one, and refuses anything but a whole JSON number from 1 to maxRsyncTaskID, so
// a string "3", 0, a negative value or 1.5 is an error before any middleware
// call. An absent key and a null value both mean no filter.
//
// It decodes req.Params.Arguments here instead of through parseArgs: parseArgs
// returns an empty map on a decode error, so a number that json cannot put in a
// float64 (1e309) would look like an absent id, and rsynctask.query would return
// every task. rejectUndecodableArguments refuses such a call before the handler
// runs; this check stays as the Tool's own guard.
func rsyncTaskID(req *mcp.CallToolRequest) (int, bool, error) {
	arguments := bytes.TrimSpace(req.Params.Arguments)
	if len(arguments) == 0 || string(arguments) == "null" {
		return 0, false, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(arguments, &fields); err != nil {
		return 0, false, fmt.Errorf("arguments must be a JSON object: %w", err)
	}
	raw, present := fields["id"]
	if !present || string(bytes.TrimSpace(raw)) == "null" {
		return 0, false, nil
	}
	// A value that is not a number json can hold, such as 1e309, fails here.
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, false, fmt.Errorf("parameter %q must be a whole number from 1 to %d: %w", "id", maxRsyncTaskID, err)
	}
	number, ok := value.(float64)
	if !ok {
		return 0, false, fmt.Errorf("parameter %q must be a number, got %T", "id", value)
	}
	if number != math.Trunc(number) || number < 1 || number > maxRsyncTaskID {
		return 0, false, fmt.Errorf("parameter %q must be a whole number from 1 to %d, got %v", "id", maxRsyncTaskID, number)
	}
	return int(number), true, nil
}

// rsyncTaskSummary projects an RsyncTaskEntry onto the documented fields. It is
// an allow-list: a field it does not name never reaches a result, so
// ssh_credentials.attributes, `extra`, the times/compress/archive family and
// `relative_path` stay with the middleware.
func rsyncTaskSummary(e map[string]any) map[string]any {
	schedule, _ := e["schedule"].(map[string]any)
	return map[string]any{
		"id":              e["id"],
		"path":            e["path"],
		"dataset":         e["dataset"],
		"direction":       e["direction"],
		"mode":            e["mode"],
		"remotehost":      e["remotehost"],
		"remoteport":      e["remoteport"],
		"remotemodule":    e["remotemodule"],
		"remotepath":      e["remotepath"],
		"user":            e["user"],
		"desc":            e["desc"],
		"schedule":        map[string]any{"minute": schedule["minute"], "hour": schedule["hour"], "dom": schedule["dom"], "month": schedule["month"], "dow": schedule["dow"]},
		"enabled":         e["enabled"],
		"locked":          e["locked"],
		"recursive":       e["recursive"],
		"delete":          e["delete"],
		"archive":         e["archive"],
		"ssh_credentials": sshCredentialSummary(e["ssh_credentials"]),
		"job":             rsyncJobSummary(e["job"]),
	}
}

// rsyncJobSummary reduces the task's job to the keys a caller may see. The other
// job keys (arguments, logs_excerpt, logs_path, exception, exc_info, result,
// progress) carry output, paths and tracebacks, so they are dropped. A job that
// is not an object gives null.
func rsyncJobSummary(v any) any {
	job, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	return map[string]any{
		"id":            job["id"],
		"state":         job["state"],
		"time_started":  wireDatetime(job["time_started"]),
		"time_finished": wireDatetime(job["time_finished"]),
		"error":         job["error"],
	}
}

// rsyncTaskReport wraps the built tasks with the summary id lists and the fixed
// notes. It reads the already projected tasks, so the summary always agrees with
// the output. The tasks arrive sorted by id, which keeps every list in id order.
func rsyncTaskReport(tasks []map[string]any) map[string]any {
	disabled, failing, running, locked, neverRun := []any{}, []any{}, []any{}, []any{}, []any{}
	enabled := 0
	for _, task := range tasks {
		id := task["id"]
		if task["enabled"] == true {
			enabled++
		} else {
			disabled = append(disabled, id)
		}
		if task["locked"] == true {
			locked = append(locked, id)
		}
		job, ok := task["job"].(map[string]any)
		if !ok {
			neverRun = append(neverRun, id)
			continue
		}
		switch job["state"] {
		case "FAILED", "ABORTED":
			failing = append(failing, id)
		case "RUNNING", "WAITING":
			running = append(running, id)
		}
	}
	return map[string]any{
		"summary": map[string]any{
			"tasks_total":        len(tasks),
			"tasks_enabled":      enabled,
			"disabled_task_ids":  disabled,
			"failing_task_ids":   failing,
			"running_task_ids":   running,
			"locked_task_ids":    locked,
			"never_run_task_ids": neverRun,
		},
		"tasks": tasks,
		"notes": []string{
			"Schedules run in the Appliance's local time.",
			"job is the running or queued job of the task, else the last finished job TrueNAS stored when the job ended; null means no run is recorded.",
			"locked true means the task path is in a locked encrypted dataset, so TrueNAS skips the run.",
			"An SSH credential shows only its id and name, and extra options are not returned.",
		},
	}
}
