package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// protectionAutoSchema is the naming schema every fixture task uses.
const protectionAutoSchema = "auto-%Y-%m-%d_%H-%M"

// protectionTestNow is the fixed clock of the end-to-end tests.
var protectionTestNow = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

// fixProtectionNow replaces the report's clock for one test.
func fixProtectionNow(t *testing.T, now time.Time) {
	t.Helper()
	previous := protectionNow
	protectionNow = func() time.Time { return now }
	t.Cleanup(func() { protectionNow = previous })
}

func TestTaskCovers(t *testing.T) {
	cases := []struct {
		name         string
		root         string
		recursive    bool
		exclude      []string
		dataset      string
		wantCovered  bool
		wantExcluded bool
	}{
		{"root itself", "tank/data", true, nil, "tank/data", true, false},
		{"recursive child", "tank/data", true, nil, "tank/data/sub", true, false},
		{"recursive grandchild", "tank/data", true, nil, "tank/data/sub/deep", true, false},
		{"non-recursive child", "tank/data", false, nil, "tank/data/sub", false, false},
		{"excluded child", "tank/data", true, []string{"tank/data/tmp"}, "tank/data/tmp", false, true},
		{"child of an excluded child", "tank/data", true, []string{"tank/data/tmp"}, "tank/data/tmp/deep", false, true},
		{"sibling with a shared prefix", "tank/data", true, nil, "tank/data2", false, false},
		{"unrelated dataset", "tank/data", true, nil, "other/data", false, false},
		{"ancestor is not covered", "tank/data", true, nil, "tank", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			covered, excluded := taskCovers(tc.root, tc.recursive, tc.exclude, tc.dataset)
			if covered != tc.wantCovered || excluded != tc.wantExcluded {
				t.Errorf("taskCovers(%q, %v, %v, %q) = (%v, %v), want (%v, %v)",
					tc.root, tc.recursive, tc.exclude, tc.dataset, covered, excluded, tc.wantCovered, tc.wantExcluded)
			}
		})
	}
}

func TestNamingSchemaRegexp(t *testing.T) {
	cases := []struct {
		name   string
		schema string
		input  string
		want   bool
	}{
		{"matches the auto default", protectionAutoSchema, "auto-2026-11-02_00-00", true},
		{"anchored at the end", protectionAutoSchema, "auto-2026-11-02_00-00x", false},
		{"anchored at the start", protectionAutoSchema, "manual-2026-11-02_00-00", false},
		{"4-digit year required", protectionAutoSchema, "auto-26-11-02_00-00", false},
		{"dot is a literal", "a.b-%Y", "a.b-2026", true},
		{"dot does not match any character", "a.b-%Y", "axb-2026", false},
		{"percent escape", "100%%-%Y", "100%-2026", true},
		{"percent escape is literal", "100%%-%Y", "100x-2026", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			re, err := namingSchemaRegexp(tc.schema)
			if err != nil {
				t.Fatalf("namingSchemaRegexp(%q): %v", tc.schema, err)
			}
			if got := re.MatchString(tc.input); got != tc.want {
				t.Errorf("%q matches %q = %v, want %v (pattern %s)", tc.schema, tc.input, got, tc.want, re)
			}
		})
	}
}

func TestScheduleIntervalMinutes(t *testing.T) {
	cases := []struct {
		name         string
		schedule     any
		want         int
		wantLessThan int // when set, the result must be positive and below this
		wantOK       bool
	}{
		{
			name:     "daily at midnight",
			schedule: map[string]any{"minute": "0", "hour": "0"},
			want:     1440, wantOK: true,
		},
		{
			name:     "hourly",
			schedule: map[string]any{"minute": "0", "hour": "*"},
			want:     60, wantOK: true,
		},
		{
			name:     "every 4 hours",
			schedule: map[string]any{"minute": "0", "hour": "*/4"},
			want:     240, wantOK: true,
		},
		{
			name:     "every 15 minutes",
			schedule: map[string]any{"minute": "*/15", "hour": "*"},
			want:     15, wantOK: true,
		},
		{
			name:     "daily inside an 08:00 to 18:00 window",
			schedule: map[string]any{"minute": "0", "hour": "*", "begin": "08:00", "end": "18:00"},
			want:     840, wantOK: true,
		},
		{
			name:     "weekly on monday",
			schedule: map[string]any{"minute": "0", "hour": "1", "dow": "mon"},
			want:     10080, wantOK: true,
		},
		{
			name:     "dow 7 is sunday",
			schedule: map[string]any{"minute": "0", "hour": "0", "dow": "7"},
			want:     10080, wantOK: true,
		},
		{
			name:     "monthly on the 1st",
			schedule: map[string]any{"minute": "0", "hour": "0", "dom": "1"},
			want:     31 * 1440, wantOK: true,
		},
		{
			name:         "dom 1 or dow mon",
			schedule:     map[string]any{"minute": "0", "hour": "0", "dom": "1", "dow": "mon"},
			wantLessThan: 31 * 1440, wantOK: true,
		},
		{
			name:     "minute out of range",
			schedule: map[string]any{"minute": "61", "hour": "0"},
		},
		{
			name:     "minute is not a number",
			schedule: map[string]any{"minute": "x", "hour": "0"},
		},
		{
			name:     "missing minute",
			schedule: map[string]any{"hour": "0"},
		},
		{
			name:     "not an object",
			schedule: "0 0 * * *",
		},
		{
			name:     "two minutes each hour",
			schedule: map[string]any{"minute": "0,30", "hour": "*"},
			want:     30, wantOK: true,
		},
		{
			// 08:00, 09:00, 10:00; the longest gap is 10:00 to the next 08:00,
			// which is 22 hours.
			name:     "three hours a day",
			schedule: map[string]any{"minute": "0", "hour": "8-10"},
			want:     22 * 60, wantOK: true,
		},
		{
			// 00:00, 06:00, 12:00; the longest gap is 12:00 to midnight, 12 hours.
			name:     "stepped hour range",
			schedule: map[string]any{"minute": "0", "hour": "0-12/6"},
			want:     12 * 60, wantOK: true,
		},
		{
			// The 1st of January and July; July 1 to January 1 is 184 days
			// (31+31+30+31+30+31).
			name:     "two months by name",
			schedule: map[string]any{"minute": "0", "hour": "0", "dom": "1", "month": "jan,jul"},
			want:     184 * 24 * 60, wantOK: true,
		},
		{
			// A restricted day-of-month OR a restricted day-of-week: every
			// Sunday fires, so no gap exceeds 7 days.
			name:     "day of month or Sunday",
			schedule: map[string]any{"minute": "0", "hour": "0", "dom": "15", "dow": "sun"},
			want:     7 * 24 * 60, wantOK: true,
		},
		{
			// The 15th alone waits up to 31 days; with the OR above it is 7.
			name:     "day of month alone",
			schedule: map[string]any{"minute": "0", "hour": "0", "dom": "15"},
			want:     31 * 24 * 60, wantOK: true,
		},
		{
			// Days 1-7 OR Sunday: every Sunday fires, so 7 days again.
			name:     "days 1-7 or dow 7",
			schedule: map[string]any{"minute": "0", "hour": "0", "dom": "1-7", "dow": "7"},
			want:     7 * 24 * 60, wantOK: true,
		},
		{
			// A step of maxint64 must not overflow: only minute 59 fires.
			name:     "step larger than the range from 59",
			schedule: map[string]any{"minute": "59/9223372036854775807", "hour": "0"},
			want:     1440, wantOK: true,
		},
		{
			name:     "step larger than the range from 0",
			schedule: map[string]any{"minute": "0/9223372036854775807", "hour": "*"},
			want:     60, wantOK: true,
		},
		{
			// A present optional key fails closed; only an absent key gets a default.
			name:     "dom is an empty string",
			schedule: map[string]any{"minute": "0", "hour": "0", "dom": ""},
		},
		{
			name:     "dow is null",
			schedule: map[string]any{"minute": "0", "hour": "0", "dow": nil},
		},
		{
			name:     "begin is an empty string",
			schedule: map[string]any{"minute": "0", "hour": "0", "begin": ""},
		},
		{
			name:     "end is a number",
			schedule: map[string]any{"minute": "0", "hour": "0", "end": float64(5)},
		},
		{
			name:     "month is blank",
			schedule: map[string]any{"minute": "0", "hour": "0", "month": "  "},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := scheduleIntervalMinutes(tc.schedule)
			if ok != tc.wantOK {
				t.Fatalf("scheduleIntervalMinutes(%v) ok = %v, want %v (got %d)", tc.schedule, ok, tc.wantOK, got)
			}
			if !ok {
				if got != 0 {
					t.Errorf("interval = %d on failure, want 0", got)
				}
				return
			}
			if tc.wantLessThan > 0 {
				if got <= 0 || got >= tc.wantLessThan {
					t.Errorf("interval = %d, want a value in 1..%d", got, tc.wantLessThan-1)
				}
				return
			}
			if got != tc.want {
				t.Errorf("interval = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestFormatBytesIEC(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{512, "512 B"},
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1024 * 1024 * 1024 * 1.5, "1.5 GiB"},
		{2.3 * 1024 * 1024 * 1024 * 1024, "2.3 TiB"},
	}
	for _, tc := range cases {
		if got := formatBytesIEC(tc.in); got != tc.want {
			t.Errorf("formatBytesIEC(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestProtectionPeriodicMatchers(t *testing.T) {
	// No task and no replication names a schema: the TrueNAS default applies.
	matchers := protectionPeriodicMatchers(nil, nil)
	if len(matchers) != 1 {
		t.Fatalf("%d matchers, want the default schema only", len(matchers))
	}
	if !protectionMatchesAny(matchers, protectionAutoName(protectionTestNow)) {
		t.Error("the default schema does not match an auto-<timestamp> name")
	}

	// A task schema replaces the default.
	matchers = protectionPeriodicMatchers([]protectionSnapshotTask{{NamingSchema: "nightly-%Y"}}, nil)
	if protectionMatchesAny(matchers, protectionAutoName(protectionTestNow)) {
		t.Error("a task schema must not fall back to the default schema")
	}
	if !protectionMatchesAny(matchers, "nightly-2026") {
		t.Error("the task schema does not match its own name")
	}

	// A replication's also_include_naming_schema counts too.
	matchers = protectionPeriodicMatchers(nil, []protectionReplication{{
		NamingSchemas: []string{"keep-%Y", "manual-%Y"},
	}})
	if !protectionMatchesAny(matchers, "manual-2026") {
		t.Error("a replication schema does not match its own name")
	}
}

func TestProtectionDatasetsFromRows(t *testing.T) {
	flat := []map[string]any{
		{"name": "tank", "type": "FILESYSTEM", "usedbydataset": map[string]any{"parsed": 0.0, "rawvalue": "0"}},
		{"name": "tank/data", "type": "FILESYSTEM", "usedbydataset": map[string]any{"rawvalue": "2048"}},
		{"name": "tank/data/app", "type": "VOLUME", "usedbydataset": map[string]any{"parsed": 1610612736.0}},
	}
	want := []protectionDataset{
		{Name: "tank", Type: "FILESYSTEM", Used: 0, HasUsed: true},
		{Name: "tank/data", Type: "FILESYSTEM", Used: 2048, HasUsed: true},
		{Name: "tank/data/app", Type: "VOLUME", Used: 1610612736, HasUsed: true},
	}

	if got := protectionDatasetsFromRows(flat); !reflect.DeepEqual(got, want) {
		t.Errorf("flat rows = %+v, want %+v", got, want)
	}

	nested := []map[string]any{
		{
			"name": "tank", "type": "FILESYSTEM", "usedbydataset": map[string]any{"parsed": 0.0, "rawvalue": "0"},
			"children": []any{
				map[string]any{
					"name": "tank/data", "type": "FILESYSTEM", "usedbydataset": map[string]any{"rawvalue": "2048"},
					"children": []any{
						map[string]any{
							"name": "tank/data/app", "type": "VOLUME",
							"usedbydataset": map[string]any{"parsed": 1610612736.0},
						},
					},
				},
			},
		},
	}
	if got := protectionDatasetsFromRows(nested); !reflect.DeepEqual(got, want) {
		t.Errorf("nested rows = %+v, want %+v", got, want)
	}

	// A row that appears twice keeps its first appearance only.
	duplicated := append(append([]map[string]any{}, flat...), flat[1])
	if got := protectionDatasetsFromRows(duplicated); !reflect.DeepEqual(got, want) {
		t.Errorf("duplicated rows = %+v, want %+v", got, want)
	}
}

// --- end-to-end fixtures -------------------------------------------------

// protectionFixture is the middleware answer a test feeds the tool.
type protectionFixture struct {
	datasets     []map[string]any
	tasks        []map[string]any
	replications []map[string]any
	snapshots    []map[string]any
	appsPool     string
	dockerErr    bool
}

// protectionCalls records what the tool asked the middleware.
type protectionCalls struct {
	methods []string
	params  map[string][]interface{}
}

func (c *protectionCalls) record(method string, params []interface{}) {
	c.methods = append(c.methods, method)
	if c.params == nil {
		c.params = map[string][]interface{}{}
	}
	c.params[method] = params
}

func protectionJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return raw
}

// mock builds a caller that answers every method from the fixture.
func (f protectionFixture) mock(t *testing.T) *mockCaller {
	t.Helper()
	return f.mockRecording(t, nil)
}

// mockRecording also records each call when calls is not nil.
func (f protectionFixture) mockRecording(t *testing.T, calls *protectionCalls) *mockCaller {
	t.Helper()
	return &mockCaller{CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
		if calls != nil {
			calls.record(method, params)
		}
		switch method {
		case "pool.dataset.query":
			return protectionJSON(t, f.datasets), nil
		case "pool.snapshottask.query":
			return protectionJSON(t, f.tasks), nil
		case "replication.query":
			return protectionJSON(t, f.replications), nil
		case "pool.snapshot.query":
			return protectionJSON(t, f.snapshots), nil
		case "docker.config":
			if f.dockerErr {
				return nil, errors.New("docker.config is not available")
			}
			if f.appsPool == "" {
				return json.RawMessage(`{}`), nil
			}
			return json.RawMessage(`{"pool": ` + strconv.Quote(f.appsPool) + `}`), nil
		}
		return nil, errors.New("unexpected method " + method)
	}}
}

// protectionAutoName is the snapshot name the auto schema makes for a time.
func protectionAutoName(at time.Time) string {
	return "auto-" + at.Format("2006-01-02_15-04")
}

// protectionSnapshotRow builds one snapshot entry.
func protectionSnapshotRow(dataset, name string, at time.Time) map[string]any {
	return map[string]any{
		"id":            dataset + "@" + name,
		"name":          dataset + "@" + name,
		"dataset":       dataset,
		"snapshot_name": name,
		"properties": map[string]any{
			"creation": map[string]any{"parsed": float64(at.Unix()), "rawvalue": strconv.FormatInt(at.Unix(), 10)},
		},
	}
}

// protectionTask builds one periodic snapshot task entry.
func protectionTask(id int, dataset string, recursive bool, exclude []string, hour, minute string,
	enabled bool, state string) map[string]any {
	if exclude == nil {
		exclude = []string{}
	}
	return map[string]any{
		"id":             float64(id),
		"dataset":        dataset,
		"recursive":      recursive,
		"exclude":        exclude,
		"enabled":        enabled,
		"naming_schema":  protectionAutoSchema,
		"allow_empty":    true,
		"lifetime_value": float64(2),
		"lifetime_unit":  "WEEK",
		"schedule": map[string]any{
			"minute": minute, "hour": hour, "dom": "*", "month": "*", "dow": "*",
			"begin": "00:00", "end": "23:59",
		},
		"vmware_sync": false,
		"state":       map[string]any{"state": state},
	}
}

// protectionReplicationFixture builds one replication task entry. The entries carry
// secrets on purpose: the report must never echo them.
func protectionReplicationFixture(id int, name string, source []string, target string, recursive bool,
	exclude []string, periodicTaskID int, state string) map[string]any {
	if exclude == nil {
		exclude = []string{}
	}
	return map[string]any{
		"id":                         float64(id),
		"name":                       name,
		"direction":                  "PUSH",
		"transport":                  "LOCAL",
		"source_datasets":            source,
		"target_dataset":             target,
		"recursive":                  recursive,
		"exclude":                    exclude,
		"enabled":                    true,
		"auto":                       true,
		"periodic_snapshot_tasks":    []any{map[string]any{"id": float64(periodicTaskID), "dataset": source[0]}},
		"naming_schema":              protectionAutoSchema,
		"also_include_naming_schema": []any{},
		"readonly":                   "IGNORE",
		"retention_policy":           "SOURCE",
		"encryption":                 true,
		"encryption_key":             "ENCRYPTIONKEYMATERIAL1888310D",
		"encryption_key_location":    "/root/keys/secret.key",
		"ssh_credentials": map[string]any{
			"id": float64(2), "name": "backup-box", "type": "SSH_CREDENTIALS",
			"attributes": map[string]any{
				"host":        "backup.lan",
				"private_key": "-----BEGIN PRIVATE KEY-----SECRET",
			},
		},
		"schedule": map[string]any{"minute": "0", "hour": "0", "dom": "*", "month": "*", "dow": "*"},
		"state":    map[string]any{"state": state, "job": map[string]any{"id": float64(9), "state": state}},
		"job":      map[string]any{"id": float64(9), "state": state, "logs_excerpt": "job log text"},
	}
}

// protectionDatasetRowFixture builds one dataset entry.
func protectionDatasetRowFixture(name, kind string, used float64) map[string]any {
	return map[string]any{
		"id":            name,
		"name":          name,
		"type":          kind,
		"pool":          protectionPoolOf(name),
		"usedbydataset": map[string]any{"parsed": used, "rawvalue": strconv.FormatFloat(used, 'f', -1, 64)},
	}
}

// protectionNestedRows restates a flat dataset list as a children tree.
func protectionNestedRows(flat []map[string]any) []map[string]any {
	type node struct {
		row      map[string]any
		children []*node
	}
	nodes := make(map[string]*node, len(flat))
	order := make([]string, 0, len(flat))
	for _, row := range flat {
		name := protectionString(row["name"])
		copied := make(map[string]any, len(row))
		for k, v := range row {
			copied[k] = v
		}
		nodes[name] = &node{row: copied}
		order = append(order, name)
	}

	roots := make([]*node, 0, len(flat))
	for _, name := range order {
		if idx := strings.LastIndex(name, "/"); idx > 0 {
			if parent, ok := nodes[name[:idx]]; ok {
				parent.children = append(parent.children, nodes[name])
				continue
			}
		}
		roots = append(roots, nodes[name])
	}

	var build func(n *node) map[string]any
	build = func(n *node) map[string]any {
		if len(n.children) == 0 {
			return n.row
		}
		kids := make([]any, 0, len(n.children))
		for _, child := range n.children {
			kids = append(kids, build(child))
		}
		n.row["children"] = kids
		return n.row
	}

	out := make([]map[string]any, 0, len(roots))
	for _, root := range roots {
		out = append(out, build(root))
	}
	return out
}

// protectionBaseFixture copies the real layout of the NAS: the tasks, the
// replications, the datasets, and snapshots relative to now.
func protectionBaseFixture(now time.Time) protectionFixture {
	fresh := now.Add(-1 * time.Hour)
	fresh2 := now.Add(-2 * time.Hour)
	stale := now.Add(-120 * time.Hour)

	tasks := []map[string]any{
		protectionTask(2, "fast/configs", true, nil, "0", "0", true, "FINISHED"),
		protectionTask(3, "fast/paperless", true, nil, "0", "0", true, "FINISHED"),
		protectionTask(4, "big/media", false, nil, "0", "0", true, "FINISHED"),
		protectionTask(5, "slow", true, []string{
			"slow/configback", "slow/paperlessback", "slow/timemachine", "slow/vmback",
		}, "1", "0", true, "FINISHED"),
		protectionTask(6, "fast/caddy", false, nil, "0", "0", true, "FINISHED"),
		protectionTask(8, "fast/automation/semaphore", true, nil, "0", "0", true, "FINISHED"),
		// big/old has only the disabled task 9, so it is uncovered.
		protectionTask(9, "big/old", true, nil, "0", "0", false, "FINISHED"),
	}

	replications := []map[string]any{
		protectionReplicationFixture(1, "configs to backup", []string{"fast/configs"}, "slow/configback", true, nil, 2, "FINISHED"),
		protectionReplicationFixture(3, "slow to big", []string{"slow"}, "big/replica", true, []string{
			"slow/configback", "slow/paperlessback", "slow/timemachine", "slow/vmback",
		}, 5, "FINISHED"),
		protectionReplicationFixture(4, "caddy to vmback", []string{"fast/caddy"}, "slow/vmback/caddy", false, nil, 6, "FINISHED"),
	}

	datasets := []map[string]any{
		protectionDatasetRowFixture("fast", "FILESYSTEM", 0),
		protectionDatasetRowFixture("fast/configs", "FILESYSTEM", 1048576),
		protectionDatasetRowFixture("fast/configs/app", "FILESYSTEM", 2048),
		protectionDatasetRowFixture("fast/paperless", "FILESYSTEM", 5368709120),
		protectionDatasetRowFixture("fast/automation", "FILESYSTEM", 4096),
		protectionDatasetRowFixture("fast/automation/semaphore", "FILESYSTEM", 8192),
		protectionDatasetRowFixture("fast/caddy", "VOLUME", 1610612736),
		protectionDatasetRowFixture("fast/scratch", "FILESYSTEM", 0),
		protectionDatasetRowFixture("slow", "FILESYSTEM", 0),
		protectionDatasetRowFixture("slow/life", "FILESYSTEM", 1073741824),
		protectionDatasetRowFixture("slow/configback", "FILESYSTEM", 1048576),
		protectionDatasetRowFixture("slow/configback/app", "FILESYSTEM", 4096),
		protectionDatasetRowFixture("slow/timemachine", "FILESYSTEM", 0),
		protectionDatasetRowFixture("slow/vmback", "FILESYSTEM", 0),
		protectionDatasetRowFixture("slow/vmback/caddy", "VOLUME", 1610612736),
		protectionDatasetRowFixture("big", "FILESYSTEM", 0),
		protectionDatasetRowFixture("big/media", "FILESYSTEM", 1099511627776),
		protectionDatasetRowFixture("big/old", "FILESYSTEM", 0),
		protectionDatasetRowFixture("big/replica", "FILESYSTEM", 0),
		protectionDatasetRowFixture("big/replica/life", "FILESYSTEM", 1073741824),
	}

	snapshots := []map[string]any{
		protectionSnapshotRow("fast/configs", protectionAutoName(fresh), fresh),
		protectionSnapshotRow("fast/configs/app", protectionAutoName(fresh2), fresh2),
		protectionSnapshotRow("fast/paperless", protectionAutoName(fresh), fresh),
		protectionSnapshotRow("fast/caddy", protectionAutoName(fresh), fresh),
		// A manual snapshot is not a periodic one.
		protectionSnapshotRow("fast/automation/semaphore", "manual-x", now.Add(-3*time.Hour)),
		protectionSnapshotRow("slow", protectionAutoName(fresh), fresh),
		protectionSnapshotRow("slow/life", protectionAutoName(stale), stale),
		protectionSnapshotRow("slow/configback", protectionAutoName(fresh), fresh),
		protectionSnapshotRow("slow/configback/app", protectionAutoName(fresh2), fresh2),
		protectionSnapshotRow("slow/vmback/caddy", protectionAutoName(fresh), fresh),
		protectionSnapshotRow("big/media", protectionAutoName(fresh), fresh),
		protectionSnapshotRow("big/replica", protectionAutoName(fresh), fresh),
		protectionSnapshotRow("big/replica/life", protectionAutoName(fresh), fresh),
	}

	return protectionFixture{
		datasets:     datasets,
		tasks:        tasks,
		replications: replications,
		snapshots:    snapshots,
		appsPool:     "fast",
	}
}

// --- end-to-end assertions -----------------------------------------------

type protectionSummaryJSON struct {
	GeneratedAt  string `json:"generated_at"`
	Status       string `json:"status"`
	Datasets     int    `json:"datasets"`
	Covered      int    `json:"covered"`
	Uncovered    int    `json:"uncovered"`
	Stale        int    `json:"stale"`
	NoSnapshot   int    `json:"no_snapshot"`
	Replicated   int    `json:"replicated"`
	Replicas     int    `json:"replicas"`
	Flagged      int    `json:"flagged"`
	FailingTasks int    `json:"failing_tasks"`
}

type protectionDatasetJSON struct {
	Name             string   `json:"name"`
	Type             string   `json:"type"`
	UsedByDataset    string   `json:"used_by_dataset"`
	SnapshotTasks    []int    `json:"snapshot_tasks"`
	ExcludedBy       []int    `json:"excluded_by"`
	NewestSnapshot   string   `json:"newest_snapshot"`
	NewestAgeHours   *float64 `json:"newest_age_hours"`
	MaxAgeHours      *float64 `json:"max_age_hours"`
	ReplicationTasks []int    `json:"replication_tasks"`
	ReplicaOf        []int    `json:"replica_of"`
	Flags            []string `json:"flags"`
}

type protectionReportJSON struct {
	Summary          protectionSummaryJSON   `json:"summary"`
	Datasets         []protectionDatasetJSON `json:"datasets"`
	SnapshotTasks    []map[string]any        `json:"snapshot_tasks"`
	ReplicationTasks []map[string]any        `json:"replication_tasks"`
	Limits           []string                `json:"limits"`
	Truncated        string                  `json:"truncated"`
}

// decodeProtectionReport parses a result text.
func decodeProtectionReport(t *testing.T, text string) protectionReportJSON {
	t.Helper()
	var got protectionReportJSON
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("result is not JSON: %v\n%s", err, text)
	}
	return got
}

// protectionWant is one expected dataset row.
type protectionWant struct {
	name             string
	snapshotTasks    []int
	excludedBy       []int
	replicationTasks []int
	replicaOf        []int
	flags            []string
}

func TestProtectionReport_Nested(t *testing.T) {
	fixProtectionNow(t, protectionTestNow)
	fixture := protectionBaseFixture(protectionTestNow)
	fixture.datasets = protectionNestedRows(fixture.datasets)

	result, err := callTool(t, fixture.mock(t), true, "truenas_protection_report", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", resultText(t, result))
	}
	assertProtectionReport(t, resultText(t, result))
}

func TestProtectionReport_Flat(t *testing.T) {
	fixProtectionNow(t, protectionTestNow)
	fixture := protectionBaseFixture(protectionTestNow)

	result, err := callTool(t, fixture.mock(t), true, "truenas_protection_report", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", resultText(t, result))
	}
	assertProtectionReport(t, resultText(t, result))
}

// assertProtectionReport checks every rule against the fixture layout.
func assertProtectionReport(t *testing.T, text string) {
	t.Helper()
	if r := []rune(text); len(r) != len(text) {
		t.Logf("result contains %d non-ASCII bytes", len(text)-len(r))
	}
	got := decodeProtectionReport(t, text)

	want := protectionSummaryJSON{
		GeneratedAt:  "2026-11-20T12:00:00Z",
		Status:       "warning",
		Datasets:     20,
		Covered:      8,
		Uncovered:    7,
		Stale:        1,
		NoSnapshot:   1,
		Replicated:   5,
		Replicas:     5,
		Flagged:      9,
		FailingTasks: 0,
	}
	if got.Summary != want {
		t.Errorf("summary = %+v, want %+v", got.Summary, want)
	}

	wantRows := []protectionWant{
		{name: "big", flags: []string{"uncovered"}},
		{name: "big/media", snapshotTasks: []int{4}},
		{name: "big/old", flags: []string{"uncovered"}},
		{name: "big/replica", replicaOf: []int{3}},
		{name: "big/replica/life", replicaOf: []int{3}},
		{name: "fast", flags: []string{"uncovered"}},
		{name: "fast/automation", flags: []string{"uncovered"}},
		{name: "fast/automation/semaphore", snapshotTasks: []int{8}, flags: []string{"no_snapshot"}},
		{name: "fast/caddy", snapshotTasks: []int{6}, replicationTasks: []int{4}},
		{name: "fast/configs", snapshotTasks: []int{2}, replicationTasks: []int{1}},
		{name: "fast/configs/app", snapshotTasks: []int{2}, replicationTasks: []int{1}},
		{name: "fast/paperless", snapshotTasks: []int{3}},
		{name: "fast/scratch", flags: []string{"uncovered"}},
		{name: "slow", snapshotTasks: []int{5}, replicationTasks: []int{3}},
		{name: "slow/configback", excludedBy: []int{5}, replicaOf: []int{1}},
		{name: "slow/configback/app", excludedBy: []int{5}, replicaOf: []int{1}},
		{name: "slow/life", snapshotTasks: []int{5}, replicationTasks: []int{3}, flags: []string{"stale"}},
		{name: "slow/timemachine", excludedBy: []int{5}, flags: []string{"uncovered"}},
		{name: "slow/vmback", excludedBy: []int{5}, flags: []string{"uncovered"}},
		{name: "slow/vmback/caddy", excludedBy: []int{5}, replicaOf: []int{4}},
	}

	if len(got.Datasets) != len(wantRows) {
		t.Fatalf("%d dataset rows, want %d: %+v", len(got.Datasets), len(wantRows), got.Datasets)
	}
	for i, want := range wantRows {
		row := got.Datasets[i]
		if row.Name != want.name {
			t.Fatalf("row %d is %q, want %q (rows must be sorted by name)", i, row.Name, want.name)
		}
		if !slices.Equal(row.SnapshotTasks, want.snapshotTasks) {
			t.Errorf("%s snapshot_tasks = %v, want %v", row.Name, row.SnapshotTasks, want.snapshotTasks)
		}
		if !slices.Equal(row.ExcludedBy, want.excludedBy) {
			t.Errorf("%s excluded_by = %v, want %v", row.Name, row.ExcludedBy, want.excludedBy)
		}
		if !slices.Equal(row.ReplicationTasks, want.replicationTasks) {
			t.Errorf("%s replication_tasks = %v, want %v", row.Name, row.ReplicationTasks, want.replicationTasks)
		}
		if !slices.Equal(row.ReplicaOf, want.replicaOf) {
			t.Errorf("%s replica_of = %v, want %v", row.Name, row.ReplicaOf, want.replicaOf)
		}
		if !slices.Equal(row.Flags, want.flags) {
			t.Errorf("%s flags = %v, want %v", row.Name, row.Flags, want.flags)
		}
	}

	byName := map[string]protectionDatasetJSON{}
	for _, row := range got.Datasets {
		byName[row.Name] = row
	}

	// The type is named for a VOLUME only.
	if row := byName["fast/caddy"]; row.Type != "VOLUME" {
		t.Errorf("fast/caddy type = %q, want VOLUME", row.Type)
	}
	if row := byName["fast/configs"]; row.Type != "" {
		t.Errorf("fast/configs type = %q, want it omitted for a FILESYSTEM", row.Type)
	}

	// Ages: a fresh snapshot, the stale one, and the threshold of 2 daily intervals.
	configs := byName["fast/configs"]
	if configs.NewestSnapshot != protectionAutoName(protectionTestNow.Add(-time.Hour)) {
		t.Errorf("fast/configs newest_snapshot = %q", configs.NewestSnapshot)
	}
	if configs.NewestAgeHours == nil || *configs.NewestAgeHours != 1 {
		t.Errorf("fast/configs newest_age_hours = %v, want 1", configs.NewestAgeHours)
	}
	if configs.MaxAgeHours == nil || *configs.MaxAgeHours != 48 {
		t.Errorf("fast/configs max_age_hours = %v, want 48", configs.MaxAgeHours)
	}
	if configs.UsedByDataset != "1.0 MiB" {
		t.Errorf("fast/configs used_by_dataset = %q, want 1.0 MiB", configs.UsedByDataset)
	}
	life := byName["slow/life"]
	if life.NewestAgeHours == nil || *life.NewestAgeHours != 120 {
		t.Errorf("slow/life newest_age_hours = %v, want 120", life.NewestAgeHours)
	}
	if life.MaxAgeHours == nil || *life.MaxAgeHours != 48 {
		t.Errorf("slow/life max_age_hours = %v, want 48", life.MaxAgeHours)
	}

	// Task rows: sorted by id, the disabled task kept, no schedule echoed.
	if len(got.SnapshotTasks) != 7 {
		t.Fatalf("%d snapshot task rows, want 7", len(got.SnapshotTasks))
	}
	for i, id := range []float64{2, 3, 4, 5, 6, 8, 9} {
		if got.SnapshotTasks[i]["id"] != id {
			t.Errorf("snapshot task row %d id = %v, want %v", i, got.SnapshotTasks[i]["id"], id)
		}
	}
	disabled := got.SnapshotTasks[6]
	if disabled["enabled"] != false || disabled["dataset"] != "big/old" {
		t.Errorf("disabled task row = %v", disabled)
	}
	if hours, ok := disabled["interval_hours"].(float64); !ok || hours != 24 {
		t.Errorf("task 9 interval_hours = %v, want 24", disabled["interval_hours"])
	}

	// Replication rows are an allow-list.
	if len(got.ReplicationTasks) != 3 {
		t.Fatalf("%d replication rows, want 3", len(got.ReplicationTasks))
	}
	allowed := []string{
		"id", "name", "direction", "transport", "source_datasets", "target_dataset",
		"recursive", "exclude", "enabled", "periodic_snapshot_task_ids", "last_run_state",
	}
	for _, row := range got.ReplicationTasks {
		for key := range row {
			if !slices.Contains(allowed, key) {
				t.Errorf("replication row has unexpected key %q: %v", key, row)
			}
		}
	}

	// Limits name the apps pool.
	if len(got.Limits) != 3 {
		t.Fatalf("%d limits, want 3", len(got.Limits))
	}
	if !strings.Contains(got.Limits[0], "fast/ix-apps") {
		t.Errorf("limits[0] = %q, want it to name fast/ix-apps", got.Limits[0])
	}
	if got.Truncated != "" {
		t.Errorf("truncated = %q, want it empty for a small report", got.Truncated)
	}
}

func TestProtectionReport_NoSecretLeak(t *testing.T) {
	fixProtectionNow(t, protectionTestNow)
	fixture := protectionBaseFixture(protectionTestNow)

	result, err := callTool(t, fixture.mock(t), true, "truenas_protection_report", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, result)
	for _, secret := range []string{
		"-----BEGIN PRIVATE KEY-----SECRET",
		"PRIVATE KEY",
		"private_key",
		"ENCRYPTIONKEYMATERIAL1888310D",
		"encryption_key",
		"/root/keys/secret.key",
		"ssh_credentials",
		"attributes",
		"backup.lan",
		"job log text",
	} {
		if strings.Contains(text, secret) {
			t.Errorf("result leaks %q:\n%s", secret, text)
		}
	}
}

func TestProtectionReport_FailingTask(t *testing.T) {
	fixProtectionNow(t, protectionTestNow)

	t.Run("failing snapshot task", func(t *testing.T) {
		fixture := protectionBaseFixture(protectionTestNow)
		fixture.tasks[0] = protectionTask(2, "fast/configs", true, nil, "0", "0", true, "ERROR")

		result, err := callTool(t, fixture.mock(t), true, "truenas_protection_report", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := decodeProtectionReport(t, resultText(t, result))
		if got.Summary.FailingTasks != 1 {
			t.Errorf("failing_tasks = %d, want 1", got.Summary.FailingTasks)
		}
		for _, row := range got.Datasets {
			wantFailing := row.Name == "fast/configs" || row.Name == "fast/configs/app"
			if slices.Contains(row.Flags, "task_failing") != wantFailing {
				t.Errorf("%s flags = %v, want task_failing %v", row.Name, row.Flags, wantFailing)
			}
		}
	})

	t.Run("failing replication", func(t *testing.T) {
		fixture := protectionBaseFixture(protectionTestNow)
		fixture.replications[0] = protectionReplicationFixture(1, "configs to backup", []string{"fast/configs"},
			"slow/configback", true, nil, 2, "ERROR")

		result, err := callTool(t, fixture.mock(t), true, "truenas_protection_report", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := decodeProtectionReport(t, resultText(t, result))
		if got.Summary.FailingTasks != 1 {
			t.Errorf("failing_tasks = %d, want 1", got.Summary.FailingTasks)
		}
		want := map[string]bool{
			"fast/configs":        true,
			"fast/configs/app":    true,
			"slow/configback":     true,
			"slow/configback/app": true, // R1 is recursive, so the descendant is a replica too
		}
		for _, row := range got.Datasets {
			if slices.Contains(row.Flags, "replication_failing") != want[row.Name] {
				t.Errorf("%s flags = %v, want replication_failing %v", row.Name, row.Flags, want[row.Name])
			}
		}
	})
}

func TestProtectionReport_Arguments(t *testing.T) {
	fixProtectionNow(t, protectionTestNow)

	names := func(got protectionReportJSON) []string {
		out := make([]string, 0, len(got.Datasets))
		for _, row := range got.Datasets {
			out = append(out, row.Name)
		}
		return out
	}

	// The pool filter goes to pool.dataset.query and the snapshot query.
	calls := &protectionCalls{}
	fixture := protectionBaseFixture(protectionTestNow)
	result, err := callTool(t, fixture.mockRecording(t, calls), true, "truenas_protection_report",
		map[string]any{"pool": "big"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := decodeProtectionReport(t, resultText(t, result))
	if want := []string{"big", "big/media", "big/old", "big/replica", "big/replica/life"}; !slices.Equal(names(got), want) {
		t.Errorf("pool big rows = %v, want %v", names(got), want)
	}
	if got.Summary.Datasets != 5 {
		t.Errorf("summary.datasets = %d, want 5", got.Summary.Datasets)
	}

	wantOrder := []string{"pool.dataset.query", "pool.snapshottask.query", "replication.query", "pool.snapshot.query", "docker.config"}
	if !slices.Equal(calls.methods, wantOrder) {
		t.Errorf("methods = %v, want %v", calls.methods, wantOrder)
	}
	wantFilter := [][]any{{"pool", "=", "big"}}
	for _, method := range []string{"pool.dataset.query", "pool.snapshot.query"} {
		params := calls.params[method]
		if len(params) != 2 {
			t.Fatalf("%s params = %v, want filters and options", method, params)
		}
		filter, ok := params[0].([][]any)
		if !ok || !reflect.DeepEqual(filter, wantFilter) {
			t.Errorf("%s filter = %v, want %v", method, params[0], wantFilter)
		}
	}
	options, ok := calls.params["pool.dataset.query"][1].(map[string]any)
	if !ok {
		t.Fatalf("dataset options = %v, want an object", calls.params["pool.dataset.query"][1])
	}
	if extra, ok := options["extra"].(map[string]any); !ok || extra["flat"] != true {
		t.Errorf("dataset options extra = %v, want flat true", options["extra"])
	}

	// Without a pool the filters are an empty list.
	calls = &protectionCalls{}
	result, err = callTool(t, fixture.mockRecording(t, calls), true, "truenas_protection_report", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", resultText(t, result))
	}
	filter, ok := calls.params["pool.dataset.query"][0].([][]any)
	if !ok || len(filter) != 0 {
		t.Errorf("no-pool dataset filter = %v, want an empty list", calls.params["pool.dataset.query"][0])
	}

	// A dataset argument narrows to that dataset and its descendants, and its
	// pool is the query filter.
	calls = &protectionCalls{}
	result, err = callTool(t, fixture.mockRecording(t, calls), true, "truenas_protection_report",
		map[string]any{"dataset": "slow/vmback"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got = decodeProtectionReport(t, resultText(t, result))
	if want := []string{"slow/vmback", "slow/vmback/caddy"}; !slices.Equal(names(got), want) {
		t.Errorf("dataset slow/vmback rows = %v, want %v", names(got), want)
	}
	filter, ok = calls.params["pool.dataset.query"][0].([][]any)
	if !ok || !reflect.DeepEqual(filter, [][]any{{"pool", "=", "slow"}}) {
		t.Errorf("dataset argument filter = %v, want [[pool = slow]]", calls.params["pool.dataset.query"][0])
	}

	// flagged_only keeps the flagged rows but the summary covers the scope.
	result, err = callTool(t, fixture.mock(t), true, "truenas_protection_report",
		map[string]any{"flagged_only": true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got = decodeProtectionReport(t, resultText(t, result))
	if len(got.Datasets) != got.Summary.Flagged {
		t.Errorf("%d rows with flagged_only, want %d", len(got.Datasets), got.Summary.Flagged)
	}
	for _, row := range got.Datasets {
		if len(row.Flags) == 0 {
			t.Errorf("flagged_only returned %q without flags", row.Name)
		}
	}
	if got.Summary.Datasets != 20 {
		t.Errorf("summary.datasets = %d, want the whole scope 20", got.Summary.Datasets)
	}

	// A pool and a dataset in different pools are refused.
	result, err = callTool(t, fixture.mock(t), true, "truenas_protection_report",
		map[string]any{"pool": "fast", "dataset": "slow/life"})
	text := toolError(t, result, err)
	if !strings.Contains(text, "fast") || !strings.Contains(text, "slow") {
		t.Errorf("conflict error = %q, want both pool names", text)
	}

	// A wrong type for flagged_only is refused before any call.
	calls = &protectionCalls{}
	result, err = callTool(t, fixture.mockRecording(t, calls), true, "truenas_protection_report",
		map[string]any{"flagged_only": "true"})
	text = toolError(t, result, err)
	if !strings.Contains(text, `"flagged_only"`) {
		t.Errorf("type error = %q, want the parameter name", text)
	}
	if len(calls.methods) != 0 {
		t.Errorf("type error made calls %v, want none", calls.methods)
	}
}

// protectionSizeFixture builds count datasets under one recursive task that
// covers them, with a fresh snapshot for every dataset when snapshots is true.
func protectionSizeFixture(now time.Time, count int, withSnapshots bool) protectionFixture {
	datasets := make([]map[string]any, 0, count)
	snapshots := make([]map[string]any, 0, count)
	fresh := now.Add(-1 * time.Hour)
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("tank/ds%04d", i)
		datasets = append(datasets, map[string]any{"id": name, "name": name, "type": "FILESYSTEM", "pool": "tank"})
		if withSnapshots {
			snapshots = append(snapshots, protectionSnapshotRow(name, protectionAutoName(fresh), fresh))
		}
	}
	return protectionFixture{
		datasets:  datasets,
		tasks:     []map[string]any{protectionTask(1, "tank", true, nil, "0", "0", true, "FINISHED")},
		snapshots: snapshots,
		appsPool:  "tank",
	}
}

func TestProtectionReport_SizeLimit(t *testing.T) {
	fixProtectionNow(t, protectionTestNow)

	cases := []struct {
		name          string
		withSnapshots bool
		wantUnflagged string
	}{
		{name: "most datasets have no flags", withSnapshots: true, wantUnflagged: "datasets without flags are left out"},
		{name: "most datasets are flagged", withSnapshots: false, wantUnflagged: "flagged datasets are left out"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := protectionSizeFixture(protectionTestNow, 3000, tc.withSnapshots)
			result, err := callTool(t, fixture.mock(t), true, "truenas_protection_report", nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.IsError {
				t.Fatalf("tool error: %s", resultText(t, result))
			}
			text := resultText(t, result)
			if len(text) > protectionReportMaxChars {
				t.Fatalf("result is %d characters, want at most %d", len(text), protectionReportMaxChars)
			}
			got := decodeProtectionReport(t, text)
			if got.Summary.Datasets != 3000 {
				t.Errorf("summary.datasets = %d, want 3000", got.Summary.Datasets)
			}
			if len(got.Datasets) >= 3000 {
				t.Errorf("%d dataset rows kept, want fewer than 3000", len(got.Datasets))
			}
			if got.Truncated == "" {
				t.Fatal("truncated is empty, want it to say what was left out")
			}
			for _, want := range []string{tc.wantUnflagged, "use the pool or dataset argument, or flagged_only"} {
				if !strings.Contains(got.Truncated, want) {
					t.Errorf("truncated = %q, want it to contain %q", got.Truncated, want)
				}
			}
		})
	}
}

func TestProtectionReport_Errors(t *testing.T) {
	fixProtectionNow(t, protectionTestNow)

	// The snapshot call may also run as zfs.snapshot.query; its error names the
	// logical call.
	callErrors := []struct {
		method string
		label  string
	}{
		{"pool.dataset.query", "pool.dataset.query:"},
		{"pool.snapshottask.query", "pool.snapshottask.query:"},
		{"replication.query", "replication.query:"},
		{"pool.snapshot.query", "snapshot.query:"},
	}
	for _, tc := range callErrors {
		t.Run(tc.method, func(t *testing.T) {
			fixture := protectionBaseFixture(protectionTestNow)
			mock := &mockCaller{CallFunc: func(called string, params ...interface{}) (json.RawMessage, error) {
				if called == tc.method {
					return nil, errors.New("boom")
				}
				return fixture.mock(t).Call(called, params...)
			}}
			result, err := callTool(t, mock, true, "truenas_protection_report", nil)
			text := toolError(t, result, err)
			if !strings.Contains(text, tc.label) || !strings.Contains(text, "boom") {
				t.Errorf("error text = %q, want %q and boom", text, tc.label)
			}
		})
	}

	t.Run("docker.config fails", func(t *testing.T) {
		fixture := protectionBaseFixture(protectionTestNow)
		fixture.dockerErr = true
		result, err := callTool(t, fixture.mock(t), true, "truenas_protection_report", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("tool error: %s", resultText(t, result))
		}
		got := decodeProtectionReport(t, resultText(t, result))
		if !strings.Contains(got.Limits[0], "the apps pool's ix-apps dataset") {
			t.Errorf("limits[0] = %q, want the generic apps pool wording", got.Limits[0])
		}
		if strings.Contains(got.Limits[0], "fast/ix-apps") {
			t.Errorf("limits[0] = %q, want no pool name when docker.config fails", got.Limits[0])
		}
		if got.Summary.Datasets != 20 {
			t.Errorf("summary.datasets = %d, want 20", got.Summary.Datasets)
		}
	})

	t.Run("docker.config names the apps pool", func(t *testing.T) {
		fixture := protectionBaseFixture(protectionTestNow)
		fixture.appsPool = "fast"
		result, err := callTool(t, fixture.mock(t), true, "truenas_protection_report", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := decodeProtectionReport(t, resultText(t, result))
		if !strings.Contains(got.Limits[0], "fast/ix-apps") {
			t.Errorf("limits[0] = %q, want fast/ix-apps", got.Limits[0])
		}
	})

	// A pool name that is not a plausible pool name is not repeated in the
	// text: a 40,000-character answer must not push the report over the limit.
	rejected := []struct {
		name string
		pool string
	}{
		{"40,000 characters", strings.Repeat("x", 40000)},
		{"a slash", "fast/slow"},
		{"a leading dash", "-fast"},
		{"whitespace", "fast slow"},
	}
	for _, tc := range rejected {
		t.Run("docker.config pool name with "+tc.name, func(t *testing.T) {
			fixture := protectionBaseFixture(protectionTestNow)
			fixture.appsPool = tc.pool
			result, err := callTool(t, fixture.mock(t), true, "truenas_protection_report", nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.IsError {
				t.Fatalf("tool error: %s", resultText(t, result))
			}
			text := resultText(t, result)
			if len(text) > protectionReportMaxChars {
				t.Fatalf("result is %d characters, want at most %d", len(text), protectionReportMaxChars)
			}
			got := decodeProtectionReport(t, text)
			if !strings.Contains(got.Limits[0], "the apps pool's ix-apps dataset") {
				t.Errorf("limits[0] = %q, want the generic apps pool wording", got.Limits[0])
			}
			if strings.Contains(strings.Join(got.Limits, " "), tc.pool) {
				t.Errorf("limits repeat the rejected pool name")
			}
			if got.Summary.Datasets != 20 {
				t.Errorf("summary.datasets = %d, want 20", got.Summary.Datasets)
			}
		})
	}
}

// protectionAgeFixture is one dataset under one task, with one periodic snapshot
// of the given age.
func protectionAgeFixture(now time.Time, schedule map[string]any, age time.Duration) protectionFixture {
	when := now.Add(-age)
	task := protectionTask(1, "tank", true, nil, "0", "0", true, "FINISHED")
	task["schedule"] = schedule
	return protectionFixture{
		datasets:  []map[string]any{protectionDatasetRowFixture("tank/data", "FILESYSTEM", 1024)},
		tasks:     []map[string]any{task},
		snapshots: []map[string]any{protectionSnapshotRow("tank/data", protectionAutoName(when), when)},
	}
}

// TestProtectionReport_StaleThreshold checks that the stale flag compares the
// exact age in minutes with the exact threshold, and rounds only what it shows.
func TestProtectionReport_StaleThreshold(t *testing.T) {
	fixProtectionNow(t, protectionTestNow)

	every2 := map[string]any{"minute": "*/2", "hour": "*"} // interval 2 min, threshold 4 min
	daily := map[string]any{"minute": "0", "hour": "0"}    // interval 1440 min, threshold 48 h

	cases := []struct {
		name         string
		schedule     map[string]any
		age          time.Duration
		wantStale    bool
		wantAgeHours float64
		wantMaxHours float64
	}{
		{"8 minutes on a 2-minute task", every2, 8 * time.Minute, true, 0.1, 0.1},
		{"3 minutes on a 2-minute task", every2, 3 * time.Minute, false, 0.1, 0.1},
		{"exactly 48 h on a daily task", daily, 48 * time.Hour, false, 48, 48},
		{"48 h and 1 minute on a daily task", daily, 48*time.Hour + time.Minute, true, 48, 48},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := protectionAgeFixture(protectionTestNow, tc.schedule, tc.age)
			result, err := callTool(t, fixture.mock(t), true, "truenas_protection_report", nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.IsError {
				t.Fatalf("tool error: %s", resultText(t, result))
			}
			got := decodeProtectionReport(t, resultText(t, result))
			if len(got.Datasets) != 1 {
				t.Fatalf("%d rows, want 1", len(got.Datasets))
			}
			row := got.Datasets[0]
			if slices.Contains(row.Flags, "stale") != tc.wantStale {
				t.Errorf("flags = %v, want stale %v", row.Flags, tc.wantStale)
			}
			// The shown values are rounded; the flag must not use them.
			if row.NewestAgeHours == nil || *row.NewestAgeHours != tc.wantAgeHours {
				t.Errorf("newest_age_hours = %v, want %v", row.NewestAgeHours, tc.wantAgeHours)
			}
			if row.MaxAgeHours == nil || *row.MaxAgeHours != tc.wantMaxHours {
				t.Errorf("max_age_hours = %v, want %v", row.MaxAgeHours, tc.wantMaxHours)
			}
		})
	}
}

// TestRenderProtectionReport_FallbackCounts forces the last size fallback and
// checks that it counts each dataset once and stays under the limit.
func TestRenderProtectionReport_FallbackCounts(t *testing.T) {
	report := protectionReport{
		Summary: protectionSummary{
			GeneratedAt: "2026-11-20T12:00:00Z", Status: "warning", Datasets: 6, Flagged: 2,
		},
		Datasets: []protectionDatasetRow{
			{Name: "tank/a", Flags: []string{"uncovered"}},
			{Name: "tank/b", Flags: []string{"uncovered"}},
			{Name: "tank/c"},
			{Name: "tank/d"},
			{Name: "tank/e"},
			{Name: "tank/f"},
		},
		// An oversized limits text defeats every step before the last fallback.
		Limits: []string{strings.Repeat("x", protectionReportMaxChars)},
	}

	text, err := renderProtectionReport(report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(text) > protectionReportMaxChars {
		t.Fatalf("text is %d characters, want at most %d", len(text), protectionReportMaxChars)
	}

	var got struct {
		Summary   protectionSummary `json:"summary"`
		Limits    []string          `json:"limits"`
		Truncated string            `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("text is not JSON: %v\n%s", err, text)
	}
	if got.Summary != report.Summary {
		t.Errorf("summary = %+v, want %+v", got.Summary, report.Summary)
	}
	if !slices.Equal(got.Limits, protectionLimitTexts("")) {
		t.Errorf("limits = %v, want the generic texts", got.Limits)
	}
	for _, want := range []string{
		"4 datasets without flags are left out",
		"2 flagged datasets are left out",
		"of 6 datasets",
	} {
		if !strings.Contains(got.Truncated, want) {
			t.Errorf("truncated = %q, want it to contain %q", got.Truncated, want)
		}
	}
	if strings.Contains(got.Truncated, "6 datasets without flags") {
		t.Errorf("truncated = %q, the unflagged count is double counted", got.Truncated)
	}
}

func TestProtectionReport_Registration(t *testing.T) {
	fixture := protectionBaseFixture(protectionTestNow)
	if names := listTools(t, fixture.mock(t), true); !slices.Contains(names, "truenas_protection_report") {
		t.Error("read-only server did not register truenas_protection_report")
	}
}

// TestProtectionCronValues pins the expansion of one cron field.
func TestProtectionCronValues(t *testing.T) {
	set := func(values ...int) map[int]bool {
		out := make(map[int]bool, len(values))
		for _, v := range values {
			out[v] = true
		}
		return out
	}

	cases := []struct {
		name  string
		field string
		min   int
		max   int
		names map[string]int
		want  map[int]bool
	}{
		{"step over the whole range", "*/20", 0, 59, nil, set(0, 20, 40)},
		{"stepped range", "1-5/2", 0, 59, nil, set(1, 3, 5)},
		{"weekday names across a range", "mon-fri", 0, 7, protectionWeekdayNames, set(1, 2, 3, 4, 5)},
		{"month names in a list", "jan,dec", 1, 12, protectionMonthNames, set(1, 12)},
		{"dow 7 is a value of the field", "7", 0, 7, protectionWeekdayNames, set(7)},
		{"dow 0 is Sunday", "sun", 0, 7, protectionWeekdayNames, set(0)},
		{"step from a value", "50/5", 0, 59, nil, set(50, 55)},
		{"single value", "30", 0, 59, nil, set(30)},
		{"reversed range", "5-1", 0, 59, nil, nil},
		{"zero step", "0/0", 0, 59, nil, nil},
		{"not a number", "abc", 0, 59, nil, nil},
		{"above the maximum", "60", 0, 59, nil, nil},
		{"empty list item", "1,,2", 0, 59, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := protectionCronValues(tc.field, tc.min, tc.max, tc.names)
			if tc.want == nil {
				if ok {
					t.Fatalf("%q parsed to %v, want a refusal", tc.field, got)
				}
				return
			}
			if !ok {
				t.Fatalf("%q was refused, want %v", tc.field, tc.want)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("%q = %v, want %v", tc.field, got, tc.want)
			}
		})
	}
}

// TestScheduleIntervalMinutes_SundayAndDayOfWeek checks that dow 7 is Sunday and
// that a restricted day-of-month with a restricted day-of-week fires on either.
func TestScheduleIntervalMinutes_SundayAndDayOfWeek(t *testing.T) {
	schedule := func(dow string) map[string]any {
		return map[string]any{"minute": "0", "hour": "0", "dom": "1-7", "dow": dow}
	}

	seven, okSeven := scheduleIntervalMinutes(schedule("7"))
	zero, okZero := scheduleIntervalMinutes(schedule("0"))
	if !okSeven || !okZero {
		t.Fatalf("dow 7 and dow 0 must both parse: %v, %v", okSeven, okZero)
	}
	if seven != zero {
		t.Errorf("dow 7 gives %d and dow 0 gives %d; 7 must mean Sunday", seven, zero)
	}

	// The maximum gap alone cannot separate two weekdays, because the monthly
	// day range already fires every week. The day expansion can: 7, 0, and 1
	// must be different days.
	for _, tc := range []struct {
		field string
		want  int
	}{{"0", 0}, {"1", 1}, {"7", 7}} {
		got, ok := protectionCronValues(tc.field, 0, 7, protectionWeekdayNames)
		if !ok {
			t.Fatalf("protectionCronValues(%q) was refused", tc.field)
		}
		if !reflect.DeepEqual(got, map[int]bool{tc.want: true}) {
			t.Errorf("protectionCronValues(%q) = %v, want only %d", tc.field, got, tc.want)
		}
	}
}

// protectionOrderFixture is one dataset under one daily task, with the given
// snapshots in the given order.
func protectionOrderFixture(now time.Time, ages ...time.Duration) protectionFixture {
	snapshots := make([]map[string]any, 0, len(ages))
	for _, age := range ages {
		when := now.Add(-age)
		snapshots = append(snapshots, protectionSnapshotRow("tank/data", protectionAutoName(when), when))
	}
	return protectionFixture{
		datasets:  []map[string]any{protectionDatasetRowFixture("tank/data", "FILESYSTEM", 1024)},
		tasks:     []map[string]any{protectionTask(1, "tank", true, nil, "0", "0", true, "FINISHED")},
		snapshots: snapshots,
	}
}

// protectionOnlyRow runs the tool on one fixture and returns its only row.
func protectionOnlyRow(t *testing.T, fixture protectionFixture) protectionDatasetJSON {
	t.Helper()
	result, err := callTool(t, fixture.mock(t), true, "truenas_protection_report", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", resultText(t, result))
	}
	got := decodeProtectionReport(t, resultText(t, result))
	if len(got.Datasets) != 1 {
		t.Fatalf("%d rows, want 1: %+v", len(got.Datasets), got.Datasets)
	}
	return got.Datasets[0]
}

// TestProtectionReport_NewestSnapshot checks that the newest creation time wins
// whatever order the middleware sends.
func TestProtectionReport_NewestSnapshot(t *testing.T) {
	fixProtectionNow(t, protectionTestNow)

	oneHour, fiveHours, fiftyFourHours := time.Hour, 5*time.Hour, 54*time.Hour
	row := protectionOnlyRow(t, protectionOrderFixture(protectionTestNow, fiftyFourHours, oneHour, fiveHours))

	want := protectionAutoName(protectionTestNow.Add(-oneHour))
	if row.NewestSnapshot != want {
		t.Errorf("newest_snapshot = %q, want %q", row.NewestSnapshot, want)
	}
	if row.NewestAgeHours == nil || *row.NewestAgeHours != 1 {
		t.Errorf("newest_age_hours = %v, want 1", row.NewestAgeHours)
	}
	if len(row.Flags) != 0 {
		t.Errorf("flags = %v, want none", row.Flags)
	}

	// The same snapshots in another order give the same answer.
	again := protectionOnlyRow(t, protectionOrderFixture(protectionTestNow, fiveHours, fiftyFourHours, oneHour))
	if again.NewestSnapshot != want {
		t.Errorf("newest_snapshot = %q, want %q in any order", again.NewestSnapshot, want)
	}
}

// TestProtectionReport_MaxAgeRules checks the freshness threshold: the smallest
// covering interval wins, and a task without allow_empty sets no threshold.
func TestProtectionReport_MaxAgeRules(t *testing.T) {
	fixProtectionNow(t, protectionTestNow)

	t.Run("hourly and daily tasks", func(t *testing.T) {
		fixture := protectionOrderFixture(protectionTestNow, time.Hour) // fresh, age 1 h
		hourly := protectionTask(2, "tank", true, nil, "*", "0", true, "FINISHED")
		fixture.tasks = append(fixture.tasks, hourly)

		row := protectionOnlyRow(t, fixture)
		if !slices.Equal(row.SnapshotTasks, []int{1, 2}) {
			t.Fatalf("snapshot_tasks = %v, want [1 2]", row.SnapshotTasks)
		}
		// 2 x the smallest interval: 60 minutes -> 2 hours.
		if row.MaxAgeHours == nil || *row.MaxAgeHours != 2 {
			t.Errorf("max_age_hours = %v, want 2", row.MaxAgeHours)
		}
		if len(row.Flags) != 0 {
			t.Errorf("flags = %v, want none", row.Flags)
		}
	})

	t.Run("allow_empty false", func(t *testing.T) {
		fixture := protectionOrderFixture(protectionTestNow, 10*24*time.Hour) // very old
		fixture.tasks[0]["allow_empty"] = false

		row := protectionOnlyRow(t, fixture)
		if !slices.Equal(row.SnapshotTasks, []int{1}) {
			t.Fatalf("snapshot_tasks = %v, want [1]", row.SnapshotTasks)
		}
		if row.MaxAgeHours != nil {
			t.Errorf("max_age_hours = %v, want it omitted for a task without allow_empty", *row.MaxAgeHours)
		}
		if slices.Contains(row.Flags, "stale") {
			t.Errorf("flags = %v, want no stale without a threshold", row.Flags)
		}
	})
}

// TestProtectionReport_ReplicationRules checks which replicas and which copies
// the report counts.
func TestProtectionReport_ReplicationRules(t *testing.T) {
	fixProtectionNow(t, protectionTestNow)

	fresh := protectionTestNow.Add(-time.Hour)
	newFixture := func() protectionFixture {
		return protectionFixture{
			datasets: []map[string]any{
				protectionDatasetRowFixture("tank/data", "FILESYSTEM", 1024),       // local source
				protectionDatasetRowFixture("tank/copy", "FILESYSTEM", 1024),       // same name as a remote target
				protectionDatasetRowFixture("tank/restored", "FILESYSTEM", 1024),   // local PULL target
				protectionDatasetRowFixture("tank/remote-source", "FILESYSTEM", 0), // PULL source
				protectionDatasetRowFixture("tank/idle", "FILESYSTEM", 0),          // disabled replication target
			},
			tasks: []map[string]any{
				protectionTask(1, "tank/data", true, nil, "0", "0", true, "FINISHED"),
				// An hourly task that no dataset in this fixture sits under; a
				// replication uses it as its periodic snapshot task.
				protectionTask(3, "other/data", true, nil, "*", "0", true, "FINISHED"),
			},
			snapshots: []map[string]any{
				protectionSnapshotRow("tank/data", protectionAutoName(fresh), fresh),
				protectionSnapshotRow("tank/restored", protectionAutoName(fresh), fresh),
			},
		}
	}

	rows := func(t *testing.T, fixture protectionFixture) map[string]protectionDatasetJSON {
		t.Helper()
		result, err := callTool(t, fixture.mock(t), true, "truenas_protection_report", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("tool error: %s", resultText(t, result))
		}
		got := decodeProtectionReport(t, resultText(t, result))
		byName := map[string]protectionDatasetJSON{}
		for _, row := range got.Datasets {
			byName[row.Name] = row
		}
		return byName
	}

	t.Run("SSH PUSH copies but is not a local replica", func(t *testing.T) {
		fixture := newFixture()
		replication := protectionReplicationFixture(1, "to the remote host", []string{"tank/data"}, "tank/copy", true, nil, 1, "FINISHED")
		replication["transport"] = "SSH"
		fixture.replications = []map[string]any{replication}

		byName := rows(t, fixture)
		if got := byName["tank/data"].ReplicationTasks; !slices.Equal(got, []int{1}) {
			t.Errorf("tank/data replication_tasks = %v, want [1]", got)
		}
		if got := byName["tank/copy"].ReplicaOf; len(got) != 0 {
			t.Errorf("tank/copy replica_of = %v, want none for a remote target", got)
		}
		if got := byName["tank/copy"].Flags; !slices.Equal(got, []string{"uncovered"}) {
			t.Errorf("tank/copy flags = %v, want [uncovered]", got)
		}
	})

	t.Run("PULL makes the local target a replica", func(t *testing.T) {
		fixture := newFixture()
		replication := protectionReplicationFixture(2, "from the remote host",
			[]string{"tank/remote-source"}, "tank/restored", true, nil, 3, "FINISHED")
		replication["direction"] = "PULL"
		// SSH, not LOCAL: the PULL direction alone must make the target local.
		replication["transport"] = "SSH"
		fixture.replications = []map[string]any{replication}

		byName := rows(t, fixture)
		restored := byName["tank/restored"]
		if got := restored.ReplicaOf; !slices.Equal(got, []int{2}) {
			t.Errorf("tank/restored replica_of = %v, want [2]", got)
		}
		if got := restored.ReplicationTasks; len(got) != 0 {
			t.Errorf("tank/restored replication_tasks = %v, want none for a PULL target", got)
		}
		// The threshold of a replica comes from the replication's periodic task
		// (task 3, hourly), not from a covering snapshot task.
		if restored.MaxAgeHours == nil || *restored.MaxAgeHours != 2 {
			t.Errorf("tank/restored max_age_hours = %v, want 2", restored.MaxAgeHours)
		}
		if len(restored.Flags) != 0 {
			t.Errorf("tank/restored flags = %v, want none", restored.Flags)
		}
		// The source of a PULL is not copied by a local PUSH task.
		if got := byName["tank/remote-source"].ReplicationTasks; len(got) != 0 {
			t.Errorf("tank/remote-source replication_tasks = %v, want none", got)
		}
	})

	t.Run("disabled LOCAL PUSH has no effect", func(t *testing.T) {
		fixture := newFixture()
		replication := protectionReplicationFixture(4, "paused copy", []string{"tank/data"}, "tank/idle", true, nil, 1, "FINISHED")
		replication["enabled"] = false
		fixture.replications = []map[string]any{replication}

		byName := rows(t, fixture)
		if got := byName["tank/data"].ReplicationTasks; len(got) != 0 {
			t.Errorf("tank/data replication_tasks = %v, want none for a disabled task", got)
		}
		if got := byName["tank/idle"].ReplicaOf; len(got) != 0 {
			t.Errorf("tank/idle replica_of = %v, want none for a disabled task", got)
		}
	})
}

// protectionMixedSizeFixture builds count datasets under one recursive task.
// Every 10th dataset sits outside the task, so it is uncovered and flagged; the
// others are fresh and covered. It returns the sorted flagged names.
func protectionMixedSizeFixture(now time.Time, count int) (protectionFixture, []string) {
	fresh := now.Add(-time.Hour)
	datasets := make([]map[string]any, 0, count)
	snapshots := make([]map[string]any, 0, count)
	flagged := make([]string, 0, count/10)
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("tank/data/ds%04d", i)
		if i%10 == 0 {
			name = fmt.Sprintf("tank/out%04d", i)
			flagged = append(flagged, name)
		} else {
			snapshots = append(snapshots, protectionSnapshotRow(name, protectionAutoName(fresh), fresh))
		}
		datasets = append(datasets, map[string]any{"id": name, "name": name, "type": "FILESYSTEM", "pool": "tank"})
	}
	sort.Strings(flagged)
	return protectionFixture{
		datasets:  datasets,
		tasks:     []map[string]any{protectionTask(1, "tank/data", true, nil, "0", "0", true, "FINISHED")},
		snapshots: snapshots,
		appsPool:  "tank",
	}, flagged
}

// protectionAllFlaggedSizeFixture builds count long-named datasets outside the
// task, so every one of them is uncovered, and returns their sorted names.
func protectionAllFlaggedSizeFixture(now time.Time, count int) (protectionFixture, []string) {
	datasets := make([]map[string]any, 0, count)
	names := make([]string, 0, count)
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("tank/outside/dataset%04d", i)
		names = append(names, name)
		datasets = append(datasets, map[string]any{"id": name, "name": name, "type": "FILESYSTEM", "pool": "tank"})
	}
	sort.Strings(names)
	return protectionFixture{
		datasets: datasets,
		tasks:    []map[string]any{protectionTask(1, "tank/data", true, nil, "0", "0", true, "FINISHED")},
	}, names
}

// protectionManyTasksFixture builds 600 snapshot tasks and 600 replications with
// long dataset names, so the task lists alone are over the size limit.
func protectionManyTasksFixture(count int) protectionFixture {
	tasks := make([]map[string]any, 0, count)
	replications := make([]map[string]any, 0, count)
	for i := 0; i < count; i++ {
		dataset := fmt.Sprintf("tank/a-very-long-dataset-name-for-the-task-%04d", i)
		tasks = append(tasks, protectionTask(i+1, dataset, true, nil, "0", "0", true, "FINISHED"))
		replications = append(replications, protectionReplicationFixture(i+1,
			fmt.Sprintf("a-very-long-replication-name-number-%04d", i),
			[]string{dataset}, dataset+"-replica", true, nil, i+1, "FINISHED"))
	}
	datasets := make([]map[string]any, 0, 5)
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("tank/short%02d", i)
		datasets = append(datasets, map[string]any{"id": name, "name": name, "type": "FILESYSTEM", "pool": "tank"})
	}
	return protectionFixture{
		datasets:     datasets,
		tasks:        tasks,
		replications: replications,
		appsPool:     "tank",
	}
}

var (
	protectionUnflaggedLeftOut = regexp.MustCompile(`(\d+) datasets without flags are left out`)
	protectionFlaggedLeftOut   = regexp.MustCompile(`(\d+) flagged datasets are left out`)
)

// protectionTruncatedCounts reads the counts out of the truncated text; a count
// that the text does not mention is 0.
func protectionTruncatedCounts(t *testing.T, truncated string) (unflagged, flagged int) {
	t.Helper()
	read := func(re *regexp.Regexp) int {
		match := re.FindStringSubmatch(truncated)
		if match == nil {
			return 0
		}
		n, err := strconv.Atoi(match[1])
		if err != nil {
			t.Fatalf("count %q in %q is not a number: %v", match[1], truncated, err)
		}
		return n
	}
	return read(protectionUnflaggedLeftOut), read(protectionFlaggedLeftOut)
}

// protectionRowNames lists the dataset names of a report.
func protectionRowNames(got protectionReportJSON) []string {
	names := make([]string, 0, len(got.Datasets))
	for _, row := range got.Datasets {
		names = append(names, row.Name)
	}
	return names
}

// TestProtectionReport_SizeMixed checks the size steps against a mixed fixture:
// the rows without flags go first, the rows that stay are flagged and in name
// order, and the counts add up.
func TestProtectionReport_SizeMixed(t *testing.T) {
	fixProtectionNow(t, protectionTestNow)

	t.Run("every 10th dataset is flagged", func(t *testing.T) {
		const total = 2000
		fixture, flaggedNames := protectionMixedSizeFixture(protectionTestNow, total)

		result, err := callTool(t, fixture.mock(t), true, "truenas_protection_report", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		text := resultText(t, result)
		if len(text) > protectionReportMaxChars {
			t.Fatalf("result is %d characters, want at most %d", len(text), protectionReportMaxChars)
		}
		got := decodeProtectionReport(t, text)
		if got.Summary.Datasets != total {
			t.Errorf("summary.datasets = %d, want %d", got.Summary.Datasets, total)
		}

		for _, row := range got.Datasets {
			if len(row.Flags) == 0 {
				t.Errorf("%s is retained without flags", row.Name)
			}
		}
		kept := protectionRowNames(got)
		if len(kept) > len(flaggedNames) {
			t.Fatalf("%d rows are retained, but only %d are flagged", len(kept), len(flaggedNames))
		}
		if want := flaggedNames[:len(kept)]; !slices.Equal(kept, want) {
			t.Errorf("retained names are not the first %d flagged names in order", len(kept))
		}
		if got.Truncated == "" {
			t.Fatal("truncated is empty")
		}
		unflagged, flagged := protectionTruncatedCounts(t, got.Truncated)
		if want := total - len(flaggedNames); unflagged != want {
			t.Errorf("truncated says %d rows without flags are left out, want %d", unflagged, want)
		}
		if flagged != len(flaggedNames)-len(kept) {
			t.Errorf("truncated says %d flagged rows are left out, want %d", flagged, len(flaggedNames)-len(kept))
		}
		if keptCount := len(kept) + unflagged + flagged; keptCount != total {
			t.Errorf("kept %d + %d unflagged + %d flagged = %d, want %d",
				len(kept), unflagged, flagged, keptCount, total)
		}
	})

	t.Run("all 2000 datasets are flagged", func(t *testing.T) {
		const total = 2000
		fixture, names := protectionAllFlaggedSizeFixture(protectionTestNow, total)

		result, err := callTool(t, fixture.mock(t), true, "truenas_protection_report", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		text := resultText(t, result)
		if len(text) > protectionReportMaxChars {
			t.Fatalf("result is %d characters, want at most %d", len(text), protectionReportMaxChars)
		}
		got := decodeProtectionReport(t, text)
		kept := protectionRowNames(got)
		if len(kept) == 0 || len(kept) >= total {
			t.Fatalf("%d rows are retained, want some but fewer than %d", len(kept), total)
		}
		for _, row := range got.Datasets {
			if len(row.Flags) == 0 {
				t.Errorf("%s is retained without flags", row.Name)
			}
		}
		if want := names[:len(kept)]; !slices.Equal(kept, want) {
			t.Errorf("retained names are not the first %d names in order", len(kept))
		}
		unflagged, flagged := protectionTruncatedCounts(t, got.Truncated)
		if unflagged != 0 {
			t.Errorf("truncated says %d rows without flags are left out, want 0", unflagged)
		}
		if want := total - len(kept); flagged != want {
			t.Errorf("truncated says %d flagged rows are left out, want %d", flagged, want)
		}
		if keptCount := len(kept) + unflagged + flagged; keptCount != total {
			t.Errorf("kept %d + %d unflagged + %d flagged = %d, want %d",
				len(kept), unflagged, flagged, keptCount, total)
		}
	})

	t.Run("the task lists are removed", func(t *testing.T) {
		fixture := protectionManyTasksFixture(600)

		result, err := callTool(t, fixture.mock(t), true, "truenas_protection_report", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		text := resultText(t, result)
		if len(text) > protectionReportMaxChars {
			t.Fatalf("result is %d characters, want at most %d", len(text), protectionReportMaxChars)
		}
		got := decodeProtectionReport(t, text)
		if len(got.SnapshotTasks) != 0 {
			t.Errorf("%d snapshot task rows, want none", len(got.SnapshotTasks))
		}
		if len(got.ReplicationTasks) != 0 {
			t.Errorf("%d replication rows, want none", len(got.ReplicationTasks))
		}
		if !strings.Contains(got.Truncated, "the task lists are left out") {
			t.Errorf("truncated = %q, want it to name the task lists", got.Truncated)
		}
		unflagged, flagged := protectionTruncatedCounts(t, got.Truncated)
		if kept := len(got.Datasets) + unflagged + flagged; kept != got.Summary.Datasets {
			t.Errorf("kept %d + %d unflagged + %d flagged = %d, want %d",
				len(got.Datasets), unflagged, flagged, kept, got.Summary.Datasets)
		}
	})
}

// TestProtectionReport_OptionMaps pins the complete option maps the tool sends.
func TestProtectionReport_OptionMaps(t *testing.T) {
	fixProtectionNow(t, protectionTestNow)
	calls := &protectionCalls{}
	fixture := protectionBaseFixture(protectionTestNow)

	if _, err := callTool(t, fixture.mockRecording(t, calls), true, "truenas_protection_report",
		map[string]any{"pool": "big"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, tc := range []struct {
		method string
		want   map[string]any
	}{
		// Literal maps, not the production helpers, so a change in a helper
		// fails here.
		{"pool.dataset.query", map[string]any{
			"select": []string{"id", "name", "type", "pool", "usedbydataset"},
			"extra": map[string]any{
				"flat":                true,
				"properties":          []string{"usedbydataset"},
				"retrieve_user_props": false,
			},
		}},
		{"pool.snapshot.query", map[string]any{
			"extra": map[string]any{"properties": []string{"creation", "used", "referenced"}},
		}},
	} {
		params := calls.params[tc.method]
		if len(params) != 2 {
			t.Fatalf("%s params = %v, want filters and options", tc.method, params)
		}
		want, err := json.Marshal(tc.want)
		if err != nil {
			t.Fatalf("marshal want options: %v", err)
		}
		assertSameJSON(t, params[1], string(want))
	}

	// The snapshot options must ask for the creation time, or every age is
	// unknown.
	options := calls.params["pool.snapshot.query"][1].(map[string]any)
	extra, ok := options["extra"].(map[string]any)
	if !ok {
		t.Fatalf("snapshot options extra = %v, want an object", options["extra"])
	}
	properties, ok := extra["properties"].([]string)
	if !ok || !slices.Contains(properties, "creation") {
		t.Errorf("snapshot options properties = %v, want creation", extra["properties"])
	}
}

// TestProtectionReport_MalformedAnswers checks that a broken middleware answer
// gives a controlled error or an empty report, and never a panic.
func TestProtectionReport_MalformedAnswers(t *testing.T) {
	fixProtectionNow(t, protectionTestNow)

	t.Run("wrong top-level type", func(t *testing.T) {
		methods := []struct {
			method string
			label  string
		}{
			{"pool.dataset.query", "pool.dataset.query"},
			{"pool.snapshottask.query", "pool.snapshottask.query"},
			{"replication.query", "replication.query"},
			{"pool.snapshot.query", "snapshot.query"},
		}
		bodies := []struct {
			name string
			body string
		}{
			{"an object", `{"not": "a list"}`},
			{"a string", `"not a list"`},
		}
		for _, m := range methods {
			for _, b := range bodies {
				t.Run(m.method+" as "+b.name, func(t *testing.T) {
					fixture := protectionBaseFixture(protectionTestNow)
					inner := fixture.mock(t)
					mock := &mockCaller{CallFunc: func(called string, params ...interface{}) (json.RawMessage, error) {
						if called == m.method {
							return json.RawMessage(b.body), nil
						}
						return inner.Call(called, params...)
					}}
					result, err := callTool(t, mock, true, "truenas_protection_report", nil)
					text := toolError(t, result, err)
					if !strings.Contains(text, "parsing") || !strings.Contains(text, m.label) {
						t.Errorf("error text = %q, want a parse error naming %q", text, m.label)
					}
				})
			}
		}
	})

	t.Run("bad rows and field types", func(t *testing.T) {
		fixture := protectionFixture{
			datasets: []map[string]any{
				nil, // a null row
				{"name": "tank/data", "type": "FILESYSTEM", "children": 5, "usedbydataset": 7},
				{"id": "tank/other", "name": 5},
			},
			tasks: []map[string]any{
				nil,
				{"id": "abc", "dataset": "tank"},
				{"id": 1.0, "dataset": "tank", "recursive": "yes", "exclude": "x",
					"schedule": []any{}, "enabled": "true", "state": 5},
			},
			replications: []map[string]any{
				nil,
				{"id": 2.0, "source_datasets": "x", "periodic_snapshot_tasks": 5,
					"target_dataset": 7, "state": map[string]any{"state": 5}},
			},
			snapshots: []map[string]any{
				nil,
				{"name": "tank/data@auto-2026-11-20_11-00", "properties": 7},
				{"id": 5.0, "properties": map[string]any{"creation": map[string]any{"parsed": "x"}}},
			},
		}

		result, err := callTool(t, fixture.mock(t), true, "truenas_protection_report", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("tool error: %s", resultText(t, result))
		}
		got := decodeProtectionReport(t, resultText(t, result))
		if got.Summary.Datasets != 2 {
			t.Errorf("summary.datasets = %d, want 2 (tank/data and tank/other)", got.Summary.Datasets)
		}
		for _, row := range got.Datasets {
			if row.NewestAgeHours != nil {
				t.Errorf("%s has newest_age_hours %v, want none without a creation time", row.Name, *row.NewestAgeHours)
			}
		}
	})
}

func TestScheduleIntervalMinutes_LoadedFields(t *testing.T) {
	keys := []string{"minute", "hour", "dom", "month", "dow", "begin", "end"}
	for _, key := range keys {
		for _, value := range []any{nil, "", "  ", float64(5)} {
			name := fmt.Sprintf("%s=%v", key, value)
			t.Run(name, func(t *testing.T) {
				schedule := map[string]any{"minute": "0", "hour": "0", key: value}
				got, ok := scheduleIntervalMinutes(schedule)
				if got != 0 || ok {
					t.Errorf("scheduleIntervalMinutes(%v) = (%d, %v), want (0, false)", schedule, got, ok)
				}
			})
		}
	}

	for _, schedule := range []any{
		map[string]any{"hour": "0"},
		map[string]any{"minute": "0"},
		map[string]any{"minute": "x", "hour": "0"},
		map[string]any{"minute": "5-", "hour": "0"},
		map[string]any{"minute": "*/0", "hour": "0"},
		map[string]any{"minute": "99", "hour": "0"},
		map[string]any{"minute": "0", "hour": "x"},
		map[string]any{"minute": "0", "hour": "0", "dom": "x"},
		map[string]any{"minute": "0", "hour": "0", "month": "x"},
		map[string]any{"minute": "0", "hour": "0", "dow": "x"},
		map[string]any{"minute": "0", "hour": "0", "begin": "24:00"},
		map[string]any{"minute": "0", "hour": "0", "begin": "1"},
		map[string]any{"minute": "0", "hour": "0", "begin": "aa:bb"},
		map[string]any{"minute": "0", "hour": "0", "end": "24:00"},
		map[string]any{"minute": "0", "hour": "0", "end": "1"},
		map[string]any{"minute": "0", "hour": "0", "end": "aa:bb"},
		"0 0 * * *",
	} {
		got, ok := scheduleIntervalMinutes(schedule)
		if got != 0 || ok {
			t.Errorf("scheduleIntervalMinutes(%v) = (%d, %v), want (0, false)", schedule, got, ok)
		}
	}

	valid := []struct {
		name     string
		schedule any
		want     int
	}{
		{"every 15 minutes", map[string]any{"minute": "*/15", "hour": "*"}, 15},
		{"hourly overnight window gap", map[string]any{"minute": "0", "hour": "*", "begin": "08:00", "end": "17:00"}, 900},
		{"dow seven", map[string]any{"minute": "0", "hour": "0", "dow": "7"}, 10080},
		{"dow zero", map[string]any{"minute": "0", "hour": "0", "dow": "0"}, 10080},
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := scheduleIntervalMinutes(tc.schedule)
			if !ok || got != tc.want {
				t.Errorf("scheduleIntervalMinutes(%v) = (%d, %v), want (%d, true)", tc.schedule, got, ok, tc.want)
			}
		})
	}
}

func TestLoadProtectionInputsErrors(t *testing.T) {
	cases := []struct {
		name      string
		method    string
		wantError string
		wantCalls []string
	}{
		{"dataset transport", "pool.dataset.query", "pool.dataset.query: boom", []string{"pool.dataset.query"}},
		{"task transport", "pool.snapshottask.query", "pool.snapshottask.query: boom", []string{"pool.dataset.query", "pool.snapshottask.query"}},
		{"replication transport", "replication.query", "replication.query: boom", []string{"pool.dataset.query", "pool.snapshottask.query", "replication.query"}},
		{"snapshot transport", "pool.snapshot.query", "snapshot.query: boom", []string{"pool.dataset.query", "pool.snapshottask.query", "replication.query", "pool.snapshot.query"}},
		{"dataset invalid JSON", "pool.dataset.query", "parsing pool.dataset.query: unexpected end of JSON input", []string{"pool.dataset.query"}},
		{"task invalid JSON", "pool.snapshottask.query", "parsing pool.snapshottask.query: unexpected end of JSON input", []string{"pool.dataset.query", "pool.snapshottask.query"}},
		{"replication invalid JSON", "replication.query", "parsing replication.query: unexpected end of JSON input", []string{"pool.dataset.query", "pool.snapshottask.query", "replication.query"}},
		{"snapshot invalid JSON", "pool.snapshot.query", "parsing snapshot.query: unexpected end of JSON input", []string{"pool.dataset.query", "pool.snapshottask.query", "replication.query", "pool.snapshot.query"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := []string{}
			mock := &mockCaller{CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
				calls = append(calls, method)
				if method == tc.method {
					if strings.Contains(tc.name, "transport") {
						return nil, errors.New("boom")
					}
					return json.RawMessage(`{`), nil
				}
				return json.RawMessage(`[]`), nil
			}}

			_, err := loadProtectionInputs(mock, nil)
			if err == nil {
				t.Fatal("loadProtectionInputs returned no error")
			}
			if got := err.Error(); got != tc.wantError {
				t.Errorf("error = %q, want %q", got, tc.wantError)
			}
			if !reflect.DeepEqual(calls, tc.wantCalls) {
				t.Errorf("calls = %v, want %v", calls, tc.wantCalls)
			}
		})
	}
}
