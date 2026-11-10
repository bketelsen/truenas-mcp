package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"truenas-mcp/truenas"
)

// scrubNow is the clock for the scrub ages. Tests replace it with a fixed instant.
var scrubNow = time.Now

// registerScrubReadTools registers the read-only view of pool scrub tasks and
// the last scan of each pool. Field names were checked against PoolScrubEntry
// and PoolScan in the v27_0_0 API models of truenas/middleware.
func registerScrubReadTools(s *mcp.Server, client truenas.Caller) {
	s.AddTool(&mcp.Tool{
		Name: "truenas_scrub_task_list",
		Description: "List pool scrub tasks with pool, schedule, threshold (days), description, and enabled state, plus each " +
			"pool's last scan (scrub or resilver) and the days since its last finished scrub. The summary lists pools " +
			"without an enabled task, pools past threshold (the next scheduled run starts a scrub), pools without a " +
			"finished scrub, and tasks whose pool is unknown. Optionally filter by pool name.",
		InputSchema: schema(map[string]any{"pool": stringProp("optional pool name; only tasks and pools with exactly this name are returned")}),
		Annotations: &mcp.ToolAnnotations{Title: "List Scrub Tasks", ReadOnlyHint: true, OpenWorldHint: new(false)},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, err := client.Call("pool.scrub.query")
		if err != nil {
			return nil, fmt.Errorf("pool.scrub.query: %w", err)
		}
		var tasks []map[string]any
		if err := json.Unmarshal(raw, &tasks); err != nil {
			return nil, fmt.Errorf("parsing pool.scrub.query: %w", err)
		}
		raw, err = client.Call("pool.query")
		if err != nil {
			return nil, fmt.Errorf("pool.query: %w", err)
		}
		var pools []map[string]any
		if err := json.Unmarshal(raw, &pools); err != nil {
			return nil, fmt.Errorf("parsing pool.query: %w", err)
		}
		poolIDs := make(map[any]bool, len(pools))
		for _, p := range pools {
			poolIDs[p["id"]] = true
		}
		if name := optionalString(req, "pool"); name != "" {
			filteredTasks, filteredPools := []map[string]any{}, []map[string]any{}
			for _, task := range tasks {
				if task["pool_name"] == name {
					filteredTasks = append(filteredTasks, task)
				}
			}
			for _, pool := range pools {
				if pool["name"] == name {
					filteredPools = append(filteredPools, pool)
				}
			}
			if len(filteredTasks) == 0 && len(filteredPools) == 0 {
				return nil, fmt.Errorf("pool %q not found", name)
			}
			tasks, pools = filteredTasks, filteredPools
		}
		sortEntriesByID(tasks)
		sortEntriesByID(pools)
		return jsonValueResult(scrubReport(tasks, pools, poolIDs, scrubNow().UTC()))
	})
}

// scrubReport projects only the documented fields. The complete Pool id set
// keeps pool_found independent of the optional name filter.
func scrubReport(tasks, pools []map[string]any, poolIDs map[any]bool, now time.Time) map[string]any {
	taskOut, poolOut := []map[string]any{}, []map[string]any{}
	withoutTask, past, withoutScrub := []any{}, []any{}, []any{}
	unknown := []any{}
	enabled := 0
	for _, task := range tasks {
		found := poolIDs[task["pool"]]
		if !found {
			unknown = append(unknown, task["id"])
		}
		if task["enabled"] == true {
			enabled++
		}
		schedule, _ := task["schedule"].(map[string]any)
		taskOut = append(taskOut, map[string]any{
			"id": task["id"], "pool": task["pool"], "pool_name": task["pool_name"],
			"threshold_days": task["threshold"], "description": task["description"],
			"enabled": task["enabled"], "pool_found": found,
			"schedule": map[string]any{"minute": schedule["minute"], "hour": schedule["hour"], "dom": schedule["dom"], "month": schedule["month"], "dow": schedule["dow"]},
		})
	}
	for _, pool := range pools {
		ids := []any{}
		hasEnabled := false
		threshold := math.Inf(1)
		for _, task := range tasks {
			if task["pool"] != pool["id"] {
				continue
			}
			ids = append(ids, task["id"])
			if task["enabled"] == true {
				hasEnabled = true
				if days, ok := task["threshold"].(float64); ok {
					threshold = math.Min(threshold, days)
				}
			}
		}
		if !hasEnabled {
			withoutTask = append(withoutTask, pool["name"])
		}
		var lastScan, daysSince any
		finished, isPast := false, false
		if scan, ok := pool["scan"].(map[string]any); ok {
			end := wireDatetime(scan["end_time"])
			lastScan = map[string]any{"function": scan["function"], "state": scan["state"], "start_time": wireDatetime(scan["start_time"]), "end_time": end, "errors": scan["errors"]}
			if text, ok := end.(string); ok && scan["function"] == "SCRUB" && scan["state"] == "FINISHED" {
				if ended, err := time.Parse(time.RFC3339, text); err == nil {
					finished = true
					days := now.Sub(ended).Hours() / 24
					daysSince = math.Round(days*10) / 10
					isPast = hasEnabled && days > threshold-1
				}
			}
		}
		if !finished {
			withoutScrub = append(withoutScrub, pool["name"])
		}
		if isPast {
			past = append(past, pool["name"])
		}
		poolOut = append(poolOut, map[string]any{"id": pool["id"], "name": pool["name"], "scrub_task_ids": ids, "last_scan": lastScan, "days_since_last_scrub": daysSince, "past_threshold": isPast})
	}
	return map[string]any{
		"summary": map[string]any{"generated_at": now.Format(time.RFC3339), "tasks_total": len(tasks), "tasks_enabled": enabled, "pools_total": len(pools), "pools_without_enabled_task": withoutTask, "pools_past_threshold": past, "pools_without_finished_scrub": withoutScrub, "tasks_unknown_pool": unknown},
		"tasks":   taskOut, "pools": poolOut,
		"notes": []string{
			"last_scan is the most recent scrub or resilver, so a resilver hides the last scrub date.",
			"past_threshold uses the TrueNAS 27.0 rule: the next scheduled run of the task starts a scrub when the last finished scrub ended more than threshold - 1 days ago.",
			"Schedules run in the Appliance's local time.",
		},
	}
}
