package server

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func fixedScrubClock(t *testing.T) time.Time {
	t.Helper()
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	old := scrubNow
	scrubNow = func() time.Time { return now }
	t.Cleanup(func() { scrubNow = old })
	return now
}

func scrubFixtureTask(id, pool int, name string, enabled bool, threshold int) map[string]any {
	return map[string]any{"id": id, "pool": pool, "pool_name": name, "threshold": threshold, "description": "monthly", "enabled": enabled,
		"schedule": map[string]any{"minute": "0", "hour": "3", "dom": "1", "month": "*", "dow": "*", "secret": "omit"}, "secret": "omit"}
}

func scrubFixtureScan(now time.Time, age time.Duration) map[string]any {
	return map[string]any{"function": "SCRUB", "state": "FINISHED", "start_time": map[string]any{"$date": now.Add(-age - time.Hour).UnixMilli()}, "end_time": map[string]any{"$date": now.Add(-age).UnixMilli()}, "errors": 0, "bytes_examined": 123, "secret": "omit"}
}

func callScrubFixture(t *testing.T, tasks, pools []map[string]any, args map[string]any) map[string]any {
	t.Helper()
	calls := []string{}
	mock := &mockCaller{CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
		if len(params) != 0 {
			t.Errorf("%s params = %v, want none", method, params)
		}
		calls = append(calls, method)
		var v any
		switch method {
		case "pool.scrub.query":
			v = tasks
		case "pool.query":
			v = pools
		default:
			t.Errorf("unexpected method %s", method)
			return nil, errors.New("unexpected method")
		}
		raw, err := json.Marshal(v)
		return raw, err
	}}
	result, err := callTool(t, mock, true, "truenas_scrub_task_list", args)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatal(resultText(t, result))
	}
	if !reflect.DeepEqual(calls, []string{"pool.scrub.query", "pool.query"}) {
		t.Fatalf("calls = %v", calls)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(resultText(t, result)), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestScrubTaskList_Normal(t *testing.T) {
	now := fixedScrubClock(t)
	tasks := []map[string]any{scrubFixtureTask(9, 2, "beta", false, 35), scrubFixtureTask(3, 1, "alpha", true, 35), scrubFixtureTask(1, 1, "alpha", true, 50)}
	pools := []map[string]any{{"id": 2, "name": "beta", "scan": nil}, {"id": 1, "name": "alpha", "scan": scrubFixtureScan(now, 36*24*time.Hour+8*time.Hour), "secret": "omit"}}
	got := callScrubFixture(t, tasks, pools, nil)
	expected := `{
 "summary":{"generated_at":"2026-11-20T12:00:00Z","tasks_total":3,"tasks_enabled":2,"pools_total":2,"pools_without_enabled_task":["beta"],"pools_past_threshold":["alpha"],"pools_without_finished_scrub":["beta"],"tasks_unknown_pool":[]},
 "tasks":[
 {"id":1,"pool":1,"pool_name":"alpha","threshold_days":50,"description":"monthly","enabled":true,"pool_found":true,"schedule":{"minute":"0","hour":"3","dom":"1","month":"*","dow":"*"}},
 {"id":3,"pool":1,"pool_name":"alpha","threshold_days":35,"description":"monthly","enabled":true,"pool_found":true,"schedule":{"minute":"0","hour":"3","dom":"1","month":"*","dow":"*"}},
 {"id":9,"pool":2,"pool_name":"beta","threshold_days":35,"description":"monthly","enabled":false,"pool_found":true,"schedule":{"minute":"0","hour":"3","dom":"1","month":"*","dow":"*"}}],
 "pools":[{"id":1,"name":"alpha","scrub_task_ids":[1,3],"last_scan":{"function":"SCRUB","state":"FINISHED","start_time":"2026-10-15T03:00:00Z","end_time":"2026-10-15T04:00:00Z","errors":0},"days_since_last_scrub":36.3,"past_threshold":true},
 {"id":2,"name":"beta","scrub_task_ids":[9],"last_scan":null,"days_since_last_scrub":null,"past_threshold":false}],
 "notes":["last_scan is the most recent scrub or resilver, so a resilver hides the last scrub date.","past_threshold uses the TrueNAS 27.0 rule: the next scheduled run of the task starts a scrub when the last finished scrub ended more than threshold - 1 days ago.","Schedules run in the Appliance's local time."]}`
	var want map[string]any
	if err := json.Unmarshal([]byte(expected), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("report = %#v, want %#v", got, want)
	}
	filtered := callScrubFixture(t, tasks, pools, map[string]any{"pool": "beta"})
	summary := filtered["summary"].(map[string]any)
	if summary["tasks_total"] != float64(1) || summary["pools_total"] != float64(1) || summary["tasks_enabled"] != float64(0) {
		t.Fatalf("filtered = %v", filtered)
	}
	if filtered["tasks"].([]any)[0].(map[string]any)["pool_name"] != "beta" || filtered["pools"].([]any)[0].(map[string]any)["name"] != "beta" {
		t.Fatal(filtered)
	}
}

func TestScrubTaskList_PoolStates(t *testing.T) {
	now := fixedScrubClock(t)
	for _, tc := range []struct {
		name    string
		scan    any
		task    bool
		enabled bool
		past    bool
		days    any
		missing bool
	}{
		{"no task", scrubFixtureScan(now, 40*24*time.Hour), false, false, false, float64(40), false},
		{"never scrubbed", nil, true, true, false, nil, true},
		{"resilver", map[string]any{"function": "RESILVER", "state": "FINISHED", "end_time": map[string]any{"$date": now.Add(-40 * 24 * time.Hour).UnixMilli()}}, true, true, false, nil, true},
		{"scanning", map[string]any{"function": "SCRUB", "state": "SCANNING", "end_time": nil}, true, true, false, nil, true},
		{"disabled", scrubFixtureScan(now, 40*24*time.Hour), true, false, false, float64(40), false},
		{"exact threshold", scrubFixtureScan(now, 34*24*time.Hour), true, true, false, float64(34), false},
		{"past threshold", scrubFixtureScan(now, 34*24*time.Hour+time.Second), true, true, true, float64(34), false},
		{"invalid end", map[string]any{"function": "SCRUB", "state": "FINISHED", "end_time": "invalid"}, true, true, false, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tasks := []map[string]any{}
			if tc.task {
				tasks = append(tasks, scrubFixtureTask(1, 1, "alpha", tc.enabled, 35))
			}
			got := callScrubFixture(t, tasks, []map[string]any{{"id": 1, "name": "alpha", "scan": tc.scan}}, map[string]any{"pool": "alpha"})
			p := got["pools"].([]any)[0].(map[string]any)
			if p["past_threshold"] != tc.past || p["days_since_last_scrub"] != tc.days {
				t.Fatal(p)
			}
			summary := got["summary"].(map[string]any)
			if (len(summary["pools_without_finished_scrub"].([]any)) == 1) != tc.missing {
				t.Fatal(summary)
			}
			if (len(summary["pools_without_enabled_task"].([]any)) == 1) != (!tc.task || !tc.enabled) {
				t.Fatal(summary)
			}
			if (len(summary["pools_past_threshold"].([]any)) == 1) != tc.past {
				t.Fatal(summary)
			}
			if !tc.task && len(p["scrub_task_ids"].([]any)) != 0 {
				t.Fatal(p)
			}
			if tc.scan == nil && p["last_scan"] != nil {
				t.Fatal(p)
			}
		})
	}
}

func TestScrubTaskList_UnknownPoolAndEmptyLists(t *testing.T) {
	fixedScrubClock(t)
	got := callScrubFixture(t, []map[string]any{scrubFixtureTask(7, 99, "gone", true, 35)}, nil, map[string]any{"pool": "gone"})
	if got["tasks"].([]any)[0].(map[string]any)["pool_found"] != false {
		t.Fatal(got)
	}
	if !reflect.DeepEqual(got["summary"].(map[string]any)["tasks_unknown_pool"], []any{float64(7)}) {
		t.Fatal(got)
	}
	empty := callScrubFixture(t, nil, nil, nil)
	if !reflect.DeepEqual(empty["tasks"], []any{}) || !reflect.DeepEqual(empty["pools"], []any{}) {
		t.Fatal(empty)
	}
}

func TestScrubTaskList_Errors(t *testing.T) {
	fixedScrubClock(t)
	for _, tc := range []struct {
		name, method, raw, want string
		failure                 bool
		filter                  string
	}{
		{name: "task error", method: "pool.scrub.query", failure: true, want: "pool.scrub.query: broken"},
		{name: "pool error", method: "pool.query", failure: true, want: "pool.query: broken"},
		{name: "invalid tasks", method: "pool.scrub.query", raw: "{", want: "parsing pool.scrub.query:"},
		{name: "invalid pools", method: "pool.query", raw: "{", want: "parsing pool.query:"},
		{name: "unknown filter", filter: "missing", want: `pool "missing" not found`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := []string{}
			mock := &mockCaller{CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
				if len(params) != 0 {
					t.Errorf("%s params = %v, want none", method, params)
				}
				calls = append(calls, method)
				if method == tc.method {
					if tc.failure {
						return nil, errors.New("broken")
					}
					return json.RawMessage(tc.raw), nil
				}
				return json.RawMessage(`[]`), nil
			}}
			result, err := callTool(t, mock, true, "truenas_scrub_task_list", map[string]any{"pool": tc.filter})
			if err == nil {
				if !result.IsError || !strings.Contains(resultText(t, result), tc.want) {
					t.Fatalf("expected error containing %q, result = %v", tc.want, result)
				}
			} else if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			wantCalls := []string{"pool.scrub.query", "pool.query"}
			if tc.method == "pool.scrub.query" {
				wantCalls = wantCalls[:1]
			}
			if !reflect.DeepEqual(calls, wantCalls) {
				t.Fatalf("calls = %v", calls)
			}
		})
	}
}
