package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
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
}

func TestProtectionReport_Registration(t *testing.T) {
	fixture := protectionBaseFixture(protectionTestNow)
	if names := listTools(t, fixture.mock(t), true); !slices.Contains(names, "truenas_protection_report") {
		t.Error("read-only server did not register truenas_protection_report")
	}
}
