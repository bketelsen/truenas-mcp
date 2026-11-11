package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"truenas-mcp/truenas"
)

// protectionNow is the clock the protection report reads. Tests replace it so
// snapshot ages are fixed.
var protectionNow = time.Now

// protectionReportMaxChars is the largest result text the tool returns. Pi
// Pocket keeps at most 40,000 characters of an MCP result; the margin leaves
// room for the wrapper a client adds.
const protectionReportMaxChars = 36000

// protectionReportDescription is the tool description. It states the flags, the
// hidden-dataset limit, the arguments, and the size rule, because a client sees
// only this text before it calls the tool.
const protectionReportDescription = "Report how each dataset is protected: the enabled periodic snapshot tasks that cover it " +
	"(recursive and exclude rules applied), its newest periodic snapshot (a name that matches a task naming schema, " +
	"auto-* by default) with its age, and the replication tasks that copy it or that it is a local replica of. " +
	"Flags: uncovered (no enabled snapshot task and not a replica), stale (newest snapshot older than 2 task intervals), " +
	"no_snapshot, task_failing, replication_failing. TrueNAS hides internal datasets such as the apps pool's ix-apps " +
	"from its queries; limits names them. pool or dataset narrows the scope; flagged_only returns only flagged datasets. " +
	"The result stays under 40,000 characters: datasets without flags are left out first, and truncated says so."

// registerProtectionReportTools registers the single read-only call that answers
// "is this data protected?" for every dataset of a pool.
func registerProtectionReportTools(s *mcp.Server, client truenas.Caller) {
	s.AddTool(&mcp.Tool{
		Name:        "truenas_protection_report",
		Description: protectionReportDescription,
		InputSchema: schema(map[string]any{
			"pool":         stringProp("optional pool name; only datasets of this pool are reported"),
			"dataset":      stringProp("optional dataset path; only this dataset and its descendants are reported"),
			"flagged_only": boolProp("when true, return only datasets that have at least one flag"),
		}),
		Annotations: &mcp.ToolAnnotations{
			Title:         "Protection Report",
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		report, err := buildProtectionReport(client, req)
		if err != nil {
			return nil, err
		}
		text, err := renderProtectionReport(report)
		if err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: text}},
		}, nil
	})
}

// protectionReport is the whole result. Field order is the order a reader sees,
// and every list is built with make so it comes out as [] and never null.
type protectionReport struct {
	Summary          protectionSummary           `json:"summary"`
	Datasets         []protectionDatasetRow      `json:"datasets"`
	SnapshotTasks    []protectionSnapshotTaskRow `json:"snapshot_tasks"`
	ReplicationTasks []protectionReplicationRow  `json:"replication_tasks"`
	Limits           []string                    `json:"limits"`
	Truncated        string                      `json:"truncated,omitempty"`
}

// protectionSummary counts the whole scope, even when the size limit leaves
// some rows out of the body.
type protectionSummary struct {
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

// protectionDatasetRow is the per-dataset verdict.
type protectionDatasetRow struct {
	Name             string   `json:"name"`
	Type             string   `json:"type,omitempty"` // only a VOLUME is named; a FILESYSTEM row adds nothing
	UsedByDataset    string   `json:"used_by_dataset,omitempty"`
	SnapshotTasks    []int    `json:"snapshot_tasks,omitempty"`
	ExcludedBy       []int    `json:"excluded_by,omitempty"`
	NewestSnapshot   string   `json:"newest_snapshot,omitempty"`
	NewestAgeHours   *float64 `json:"newest_age_hours,omitempty"`
	MaxAgeHours      *float64 `json:"max_age_hours,omitempty"`
	ReplicationTasks []int    `json:"replication_tasks,omitempty"`
	ReplicaOf        []int    `json:"replica_of,omitempty"`
	Flags            []string `json:"flags,omitempty"`
}

// protectionSnapshotTaskRow is an allow-list view of a periodic snapshot task.
type protectionSnapshotTaskRow struct {
	ID            int      `json:"id"`
	Dataset       string   `json:"dataset,omitempty"`
	Recursive     bool     `json:"recursive"`
	Exclude       []string `json:"exclude,omitempty"`
	Enabled       bool     `json:"enabled"`
	NamingSchema  string   `json:"naming_schema,omitempty"`
	AllowEmpty    bool     `json:"allow_empty"`
	IntervalHours *float64 `json:"interval_hours,omitempty"`
	LastRunState  string   `json:"last_run_state,omitempty"`
}

// protectionReplicationRow is an allow-list view of a replication task. A
// replication entry carries encryption_key, encryption_key_location, and an
// ssh_credentials keychain credential whose attributes can hold a private key;
// none of that is copied here.
type protectionReplicationRow struct {
	ID                      int      `json:"id"`
	Name                    string   `json:"name,omitempty"`
	Direction               string   `json:"direction,omitempty"`
	Transport               string   `json:"transport,omitempty"`
	SourceDatasets          []string `json:"source_datasets,omitempty"`
	TargetDataset           string   `json:"target_dataset,omitempty"`
	Recursive               bool     `json:"recursive"`
	Exclude                 []string `json:"exclude,omitempty"`
	Enabled                 bool     `json:"enabled"`
	PeriodicSnapshotTaskIDs []int    `json:"periodic_snapshot_task_ids,omitempty"`
	LastRunState            string   `json:"last_run_state,omitempty"`
}

// protectionSnapshotTask is the internal view of a periodic snapshot task.
type protectionSnapshotTask struct {
	ID              int
	Dataset         string
	Recursive       bool
	Exclude         []string
	Enabled         bool
	NamingSchema    string
	AllowEmpty      bool
	IntervalMinutes int
	HasInterval     bool
	LastRunState    string
	Failed          bool
}

// protectionReplication is the internal view of a replication task.
type protectionReplication struct {
	ID              int
	Name            string
	Direction       string
	Transport       string
	SourceDatasets  []string
	TargetDataset   string
	Recursive       bool
	Exclude         []string
	Enabled         bool
	PeriodicTaskIDs []int
	NamingSchemas   []string
	LastRunState    string
	Failed          bool
}

// protectionSnapshot is one snapshot entry.
type protectionSnapshot struct {
	Dataset     string
	Name        string // the part after "@"
	Creation    int64
	HasCreation bool
}

// protectionDataset is one dataset of the scope.
type protectionDataset struct {
	Name    string
	Type    string
	Used    float64
	HasUsed bool
}

// protectionInputs is everything the middleware answered.
type protectionInputs struct {
	datasets      []protectionDataset
	snapshotTasks []protectionSnapshotTask
	replications  []protectionReplication
	snapshots     []protectionSnapshot
	appsPool      string
}

// buildProtectionReport runs the 5 read-only calls and assembles the report.
func buildProtectionReport(client truenas.Caller, req *mcp.CallToolRequest) (protectionReport, error) {
	pool, dataset, flaggedOnly, err := protectionScope(req)
	if err != nil {
		return protectionReport{}, err
	}

	filters := [][]any{}
	if pool != "" {
		filters = [][]any{{"pool", "=", pool}}
	}

	in, err := loadProtectionInputs(client, filters)
	if err != nil {
		return protectionReport{}, err
	}

	in.datasets = protectionNarrowDatasets(in.datasets, pool, dataset)

	now := protectionNow().UTC()
	rows := protectionAnalyze(in, now)
	summary := protectionSummarize(rows, in, now)

	if flaggedOnly {
		rows = protectionFlaggedRows(rows)
	}

	return protectionReport{
		Summary:          summary,
		Datasets:         rows,
		SnapshotTasks:    protectionSnapshotTaskRows(in.snapshotTasks),
		ReplicationTasks: protectionReplicationRows(in.replications),
		Limits:           protectionLimitTexts(in.appsPool),
	}, nil
}

// protectionScope reads the 3 arguments and resolves the pool the query filters
// on. A dataset argument implies its pool, so a pool that names another pool is
// refused instead of silently reported as empty.
func protectionScope(req *mcp.CallToolRequest) (pool, dataset string, flaggedOnly bool, err error) {
	pool = optionalString(req, "pool")
	dataset = optionalString(req, "dataset")
	flaggedOnly, err = strictBool(req, "flagged_only", false)
	if err != nil {
		return "", "", false, err
	}
	if dataset != "" {
		dsPool := protectionPoolOf(dataset)
		if pool != "" && pool != dsPool {
			return "", "", false, fmt.Errorf("parameter %q names pool %q and parameter %q names dataset %q in pool %q; "+
				"they must agree", "pool", pool, "dataset", dataset, dsPool)
		}
		pool = dsPool
	}
	return pool, dataset, flaggedOnly, nil
}

// protectionPoolOf returns the first path component of a dataset path.
func protectionPoolOf(dataset string) string {
	pool, _, _ := strings.Cut(dataset, "/")
	return pool
}

// protectionDatasetQueryOptions asks for exactly the fields the report reads.
// TrueNAS 27.0 answers a flat list only when asked.
func protectionDatasetQueryOptions() map[string]any {
	return map[string]any{
		"select": []string{"id", "name", "type", "pool", "usedbydataset"},
		"extra": map[string]any{
			"flat":                true,
			"properties":          []string{"usedbydataset"},
			"retrieve_user_props": false,
		},
	}
}

// loadProtectionInputs makes the calls in the designed order. The first 4 are
// required; docker.config only feeds the limits text, so its error is ignored.
func loadProtectionInputs(client truenas.Caller, filters [][]any) (protectionInputs, error) {
	in := protectionInputs{}

	raw, callErr := client.Call("pool.dataset.query", filters, protectionDatasetQueryOptions())
	datasetRows, err := queryRows("pool.dataset.query", raw, callErr)
	if err != nil {
		return in, err
	}

	raw, callErr = client.Call("pool.snapshottask.query")
	taskRows, err := queryRows("pool.snapshottask.query", raw, callErr)
	if err != nil {
		return in, err
	}

	raw, callErr = client.Call("replication.query")
	replicationRows, err := queryRows("replication.query", raw, callErr)
	if err != nil {
		return in, err
	}

	raw, callErr = callSnapshot(client, "query", filters, snapshotQueryOptions())
	snapshotRows, err := queryRows("snapshot.query", raw, callErr)
	if err != nil {
		return in, err
	}

	// The apps pool only sharpens the limits text. A release without
	// docker.config, or a failure, leaves the generic wording in place. A name
	// that is not a plausible pool name is ignored, so a hostile or broken
	// answer cannot grow the text.
	if raw, callErr := client.Call("docker.config"); callErr == nil {
		var cfg map[string]any
		if jsonErr := json.Unmarshal(raw, &cfg); jsonErr == nil {
			if pool, ok := cfg["pool"].(string); ok && protectionPoolNamePattern.MatchString(pool) {
				in.appsPool = pool
			}
		}
	}

	in.datasets = protectionDatasetsFromRows(datasetRows)
	in.snapshotTasks = protectionSnapshotTasksFromRows(taskRows)
	in.replications = protectionReplicationsFromRows(replicationRows)
	in.snapshots = protectionSnapshotsFromRows(snapshotRows)
	return in, nil
}

// protectionPoolNamePattern is the shape of a pool name the limits text may
// repeat: 1 to 255 characters, starting with a letter or a digit.
var protectionPoolNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,254}$`)

// protectionNarrowDatasets applies the scope to the dataset rows. The middleware
// already filters by pool; repeating it here keeps a caller that answers with
// more rows from widening the report.
func protectionNarrowDatasets(datasets []protectionDataset, pool, dataset string) []protectionDataset {
	if pool == "" && dataset == "" {
		return datasets
	}
	out := make([]protectionDataset, 0, len(datasets))
	for _, d := range datasets {
		if pool != "" && protectionPoolOf(d.Name) != pool {
			continue
		}
		if dataset != "" && d.Name != dataset && !strings.HasPrefix(d.Name, dataset+"/") {
			continue
		}
		out = append(out, d)
	}
	return out
}

// protectionAnalyze builds one row per dataset.
func protectionAnalyze(in protectionInputs, now time.Time) []protectionDatasetRow {
	matchers := protectionPeriodicMatchers(in.snapshotTasks, in.replications)

	newest := map[string]protectionSnapshot{}
	for _, s := range in.snapshots {
		if !protectionMatchesAny(matchers, s.Name) {
			continue
		}
		if cur, ok := newest[s.Dataset]; ok && !protectionSnapshotIsNewer(s, cur) {
			continue
		}
		newest[s.Dataset] = s
	}

	taskByID := make(map[int]protectionSnapshotTask, len(in.snapshotTasks))
	for _, t := range in.snapshotTasks {
		taskByID[t.ID] = t
	}

	rows := make([]protectionDatasetRow, 0, len(in.datasets))
	for _, d := range in.datasets {
		rows = append(rows, protectionDatasetRowFor(d, in, taskByID, newest, now))
	}
	return rows
}

// protectionDatasetRowFor applies the cover, replica, age, and flag rules to one
// dataset.
func protectionDatasetRowFor(d protectionDataset, in protectionInputs, taskByID map[int]protectionSnapshotTask,
	newest map[string]protectionSnapshot, now time.Time) protectionDatasetRow {
	row := protectionDatasetRow{Name: d.Name}
	if d.Type == "VOLUME" {
		row.Type = d.Type
	}
	if d.HasUsed {
		row.UsedByDataset = formatBytesIEC(d.Used)
	}

	var intervals []int
	taskFailing := false

	for _, t := range in.snapshotTasks {
		if !t.Enabled {
			continue
		}
		covered, excluded := taskCovers(t.Dataset, t.Recursive, t.Exclude, d.Name)
		if covered {
			row.SnapshotTasks = append(row.SnapshotTasks, t.ID)
			if t.AllowEmpty && t.HasInterval {
				intervals = append(intervals, t.IntervalMinutes)
			}
			if t.Failed {
				taskFailing = true
			}
		}
		if excluded {
			row.ExcludedBy = append(row.ExcludedBy, t.ID)
		}
	}

	replicationFailing := false
	for _, r := range in.replications {
		if !r.Enabled || !strings.EqualFold(r.Direction, "PUSH") {
			continue
		}
		for _, source := range r.SourceDatasets {
			if covered, _ := taskCovers(source, r.Recursive, r.Exclude, d.Name); covered {
				row.ReplicationTasks = append(row.ReplicationTasks, r.ID)
				if r.Failed {
					replicationFailing = true
				}
				break
			}
		}
	}

	for _, r := range in.replications {
		if !r.Enabled || r.TargetDataset == "" {
			continue
		}
		// A target on this NAS: LOCAL transport, or a PULL task whose target is local.
		if !strings.EqualFold(r.Transport, "LOCAL") && !strings.EqualFold(r.Direction, "PULL") {
			continue
		}
		if d.Name != r.TargetDataset && !strings.HasPrefix(d.Name, r.TargetDataset+"/") {
			continue
		}
		row.ReplicaOf = append(row.ReplicaOf, r.ID)
		if r.Failed {
			replicationFailing = true
		}
		// The replica's freshness comes from the task that feeds the replication.
		for _, id := range r.PeriodicTaskIDs {
			if t, ok := taskByID[id]; ok && t.AllowEmpty && t.HasInterval {
				intervals = append(intervals, t.IntervalMinutes)
			}
		}
	}

	// The threshold stays exact in minutes. Only the displayed value is rounded,
	// so 2 task intervals of 2 minutes still catch an 8-minute-old snapshot.
	var maxAgeMinutes float64
	hasMaxAge := false
	if len(intervals) > 0 {
		smallest := slices.Min(intervals)
		maxAgeMinutes = 2 * float64(smallest)
		hasMaxAge = true
		maxAge := protectionRound1(maxAgeMinutes / 60)
		row.MaxAgeHours = &maxAge
	}

	var ageMinutes float64
	hasAge := false
	if s, ok := newest[d.Name]; ok {
		row.NewestSnapshot = s.Name
		if s.HasCreation {
			ageMinutes = now.Sub(time.Unix(s.Creation, 0)).Minutes()
			hasAge = true
			age := protectionRound1(ageMinutes / 60)
			row.NewestAgeHours = &age
		}
	}

	uncovered := len(row.SnapshotTasks) == 0 && len(row.ReplicaOf) == 0
	switch {
	case uncovered:
		row.Flags = append(row.Flags, "uncovered")
	case row.NewestSnapshot == "":
		row.Flags = append(row.Flags, "no_snapshot")
	}
	if hasAge && hasMaxAge && ageMinutes > maxAgeMinutes {
		row.Flags = append(row.Flags, "stale")
	}
	if taskFailing {
		row.Flags = append(row.Flags, "task_failing")
	}
	if replicationFailing {
		row.Flags = append(row.Flags, "replication_failing")
	}
	return row
}

// protectionSummarize counts the whole scope.
func protectionSummarize(rows []protectionDatasetRow, in protectionInputs, now time.Time) protectionSummary {
	summary := protectionSummary{
		GeneratedAt: now.UTC().Format(time.RFC3339),
		Status:      "ok",
		Datasets:    len(rows),
	}
	for _, row := range rows {
		if len(row.SnapshotTasks) > 0 {
			summary.Covered++
		}
		if len(row.ReplicationTasks) > 0 {
			summary.Replicated++
		}
		if len(row.ReplicaOf) > 0 {
			summary.Replicas++
		}
		if len(row.Flags) > 0 {
			summary.Flagged++
		}
		for _, flag := range row.Flags {
			switch flag {
			case "uncovered":
				summary.Uncovered++
			case "stale":
				summary.Stale++
			case "no_snapshot":
				summary.NoSnapshot++
			}
		}
	}
	for _, t := range in.snapshotTasks {
		if t.Enabled && t.Failed {
			summary.FailingTasks++
		}
	}
	for _, r := range in.replications {
		if r.Enabled && r.Failed {
			summary.FailingTasks++
		}
	}
	if summary.Flagged > 0 {
		summary.Status = "warning"
	}
	return summary
}

// protectionFlaggedRows keeps only datasets with at least one flag, in order.
func protectionFlaggedRows(rows []protectionDatasetRow) []protectionDatasetRow {
	out := make([]protectionDatasetRow, 0, len(rows))
	for _, row := range rows {
		if len(row.Flags) > 0 {
			out = append(out, row)
		}
	}
	return out
}

// protectionSnapshotTaskRows projects the tasks for the result.
func protectionSnapshotTaskRows(tasks []protectionSnapshotTask) []protectionSnapshotTaskRow {
	out := make([]protectionSnapshotTaskRow, 0, len(tasks))
	for _, t := range tasks {
		row := protectionSnapshotTaskRow{
			ID:           t.ID,
			Dataset:      t.Dataset,
			Recursive:    t.Recursive,
			Exclude:      t.Exclude,
			Enabled:      t.Enabled,
			NamingSchema: t.NamingSchema,
			AllowEmpty:   t.AllowEmpty,
			LastRunState: t.LastRunState,
		}
		if t.HasInterval {
			hours := protectionRound1(float64(t.IntervalMinutes) / 60)
			row.IntervalHours = &hours
		}
		out = append(out, row)
	}
	return out
}

// protectionReplicationRows projects the replications for the result.
func protectionReplicationRows(replications []protectionReplication) []protectionReplicationRow {
	out := make([]protectionReplicationRow, 0, len(replications))
	for _, r := range replications {
		out = append(out, protectionReplicationRow{
			ID:                      r.ID,
			Name:                    r.Name,
			Direction:               r.Direction,
			Transport:               r.Transport,
			SourceDatasets:          r.SourceDatasets,
			TargetDataset:           r.TargetDataset,
			Recursive:               r.Recursive,
			Exclude:                 r.Exclude,
			Enabled:                 r.Enabled,
			PeriodicSnapshotTaskIDs: r.PeriodicTaskIDs,
			LastRunState:            r.LastRunState,
		})
	}
	return out
}

// protectionLimitTexts is the fixed list of things the report cannot see.
func protectionLimitTexts(appsPool string) []string {
	apps := "the apps pool's ix-apps dataset"
	if appsPool != "" {
		apps = appsPool + "/ix-apps"
	}
	return []string{
		"Internal datasets are hidden: pool.dataset.query and pool.snapshot.query leave out " + apps +
			" (app ix-volumes, app configuration and container images) and system datasets such as <pool>/.system" +
			". This report cannot see them or their snapshots.",
		"Schedules run in the NAS's local time. The interval is the longest gap between 2 scheduled runs, counted in UTC, so a daylight-saving change can shift 1 gap by up to 1 hour.",
		"Replication to another host is listed, but the remote copy is not checked.",
	}
}

// renderProtectionReport applies the size steps of the design: indented JSON,
// then compact JSON, then the datasets without flags, then the flagged datasets
// from the end, then the task lists. The returned text is at most
// protectionReportMaxChars characters.
func renderProtectionReport(report protectionReport) (string, error) {
	if text, ok := protectionReportText(report); ok {
		return text, nil
	}

	total := len(report.Datasets)
	flagged := protectionFlaggedRows(report.Datasets)

	build := func(kept int, withTasks bool) (string, bool) {
		candidate := report
		candidate.Datasets = append([]protectionDatasetRow{}, flagged[:kept]...)
		if !withTasks {
			candidate.SnapshotTasks = []protectionSnapshotTaskRow{}
			candidate.ReplicationTasks = []protectionReplicationRow{}
		}
		candidate.Truncated = protectionTruncatedText(total-len(flagged), len(flagged)-kept, total, !withTasks)
		return protectionReportText(candidate)
	}

	if text, ok := build(len(flagged), true); ok {
		return text, nil
	}

	// The text grows with the number of rows kept, so a binary search finds the
	// largest prefix of the flagged rows that still fits.
	lo, hi := 0, len(flagged)
	if _, ok := build(0, true); !ok {
		lo, hi = -1, 0
	}
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if _, ok := build(mid, true); ok {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if lo >= 0 {
		if text, ok := build(lo, true); ok {
			return text, nil
		}
	}

	if text, ok := build(0, false); ok {
		return text, nil
	}

	// Nothing else fit: keep the summary, the generic limits, and the counts.
	// The unflagged count is what the scope holds beyond the flagged rows; the
	// total is not that count, or it would be counted twice.
	truncated := protectionTruncatedText(total-len(flagged), len(flagged), total, true)
	tiny := protectionMinimalReport{
		Summary:   report.Summary,
		Limits:    protectionLimitTexts(""),
		Truncated: truncated,
	}
	text, err := json.Marshal(tiny)
	if err != nil {
		return "", fmt.Errorf("formatting result: %w", err)
	}
	if len(text) <= protectionReportMaxChars {
		return string(text), nil
	}

	// The answer must never exceed the limit, so the counts give way to one
	// fixed sentence when even the smallest report is too long.
	tiny.Truncated = "Size limit: the report does not fit; use the pool or dataset argument, or flagged_only"
	text, err = json.Marshal(tiny)
	if err != nil {
		return "", fmt.Errorf("formatting result: %w", err)
	}
	return string(text), nil
}

// protectionMinimalReport is the smallest valid answer: the summary, the
// generic limits, and what the size limit left out.
type protectionMinimalReport struct {
	Summary   protectionSummary `json:"summary"`
	Limits    []string          `json:"limits"`
	Truncated string            `json:"truncated,omitempty"`
}

// protectionReportText marshals the report indented, then compact, and reports
// whether either form fits the limit.
func protectionReportText(report protectionReport) (string, bool) {
	if text, err := json.MarshalIndent(report, "", "  "); err == nil && len(text) <= protectionReportMaxChars {
		return string(text), true
	}
	if text, err := json.Marshal(report); err == nil && len(text) <= protectionReportMaxChars {
		return string(text), true
	}
	return "", false
}

// protectionTruncatedText says what the size limit left out and how to narrow
// the scope.
func protectionTruncatedText(droppedUnflagged, droppedFlagged, total int, droppedTasks bool) string {
	parts := []string{}
	if droppedUnflagged > 0 {
		parts = append(parts, fmt.Sprintf("%d datasets without flags are left out", droppedUnflagged))
	}
	if droppedFlagged > 0 {
		parts = append(parts, fmt.Sprintf("%d flagged datasets are left out", droppedFlagged))
	}
	if droppedTasks {
		parts = append(parts, "the task lists are left out")
	}
	what := "nothing is left out"
	if len(parts) > 0 {
		what = strings.Join(parts, ", ")
	}
	return fmt.Sprintf("Size limit: %s of %d datasets; use the pool or dataset argument, or flagged_only to narrow the scope",
		what, total)
}

// protectionRound1 rounds to 1 decimal place.
func protectionRound1(v float64) float64 {
	if v >= 0 {
		return math.Round(v*10) / 10
	}
	return -math.Round(-v*10) / 10
}

// taskCovers reports whether a periodic snapshot task or replication with the
// given root, recursive flag, and exclude list covers ds, and whether an exclude
// entry hides ds from it.
func taskCovers(root string, recursive bool, exclude []string, ds string) (covered, excluded bool) {
	if ds == root {
		return true, false
	}
	if !recursive || !strings.HasPrefix(ds, root+"/") {
		return false, false
	}
	for _, entry := range exclude {
		if entry == "" {
			continue
		}
		if ds == entry || strings.HasPrefix(ds, entry+"/") {
			return false, true
		}
	}
	return true, false
}

// namingSchemaRegexp turns a strftime naming schema into an anchored regular
// expression that matches a whole snapshot name. Every literal is quoted, so a
// dot or a bracket in a schema matches only itself.
func namingSchemaRegexp(schema string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(schema); i++ {
		if schema[i] != '%' {
			b.WriteString(regexp.QuoteMeta(string(schema[i])))
			continue
		}
		if i+1 >= len(schema) {
			b.WriteString(regexp.QuoteMeta("%"))
			break
		}
		i++
		switch schema[i] {
		case 'Y':
			b.WriteString(`\d{4}`)
		case 'm', 'd', 'H', 'M', 'S', 'y':
			b.WriteString(`\d{2}`)
		case 'j':
			b.WriteString(`\d{3}`)
		case 'z':
			b.WriteString(`[+-]\d{4}`)
		case 's':
			b.WriteString(`\d+`)
		case '%':
			b.WriteString(regexp.QuoteMeta("%"))
		default:
			b.WriteString(`.+?`)
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

// protectionPeriodicMatchers builds one pattern per naming schema of a snapshot
// task or a replication. A schema that does not compile is skipped.
func protectionPeriodicMatchers(tasks []protectionSnapshotTask, replications []protectionReplication) []*regexp.Regexp {
	var matchers []*regexp.Regexp
	add := func(schema string) {
		if schema == "" {
			return
		}
		if re, err := namingSchemaRegexp(schema); err == nil {
			matchers = append(matchers, re)
		}
	}
	for _, t := range tasks {
		add(t.NamingSchema)
	}
	for _, r := range replications {
		for _, schema := range r.NamingSchemas {
			add(schema)
		}
	}
	if len(matchers) == 0 {
		// With no schema anywhere, the TrueNAS default names a periodic snapshot:
		// auto-<timestamp>.
		add(defaultSnapshotNamingSchema)
	}
	return matchers
}

// protectionMatchesAny reports whether a snapshot name matches any schema.
func protectionMatchesAny(matchers []*regexp.Regexp, name string) bool {
	for _, re := range matchers {
		if re.MatchString(name) {
			return true
		}
	}
	return false
}

// protectionSnapshotIsNewer reports whether a is newer than b. An entry with a
// known creation time wins over one without.
func protectionSnapshotIsNewer(a, b protectionSnapshot) bool {
	if a.HasCreation != b.HasCreation {
		return a.HasCreation
	}
	return a.Creation > b.Creation
}

// protectionMonthNames and protectionWeekdayNames are the cron names the
// middleware accepts, lower-cased.
var protectionMonthNames = map[string]int{
	"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
	"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
}

var protectionWeekdayNames = map[string]int{
	"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6,
}

// scheduleIntervalMinutes returns the longest gap in minutes between 2 runs of a
// periodic snapshot task's cron schedule, with its time window applied. It
// enumerates the fire times of 2027-01-01 00:00 to 2028-12-31 23:59 UTC. Fewer
// than 2 fire times, or a field it cannot parse, gives false.
func scheduleIntervalMinutes(schedule any) (int, bool) {
	obj, ok := schedule.(map[string]any)
	if !ok {
		return 0, false
	}

	// A default applies only when the key is absent. A key that is present with
	// null, a blank string, or another type fails closed: the interval is then
	// unknown and the report does not claim a freshness threshold.
	field := func(key, def string) (string, bool) {
		v, present := obj[key]
		if !present {
			return def, true
		}
		s, ok := v.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return "", false
		}
		return s, true
	}

	fields := map[string]string{}
	for _, spec := range []struct{ key, def string }{
		{"minute", ""}, {"hour", ""}, {"dom", "*"}, {"month", "*"},
		{"dow", "*"}, {"begin", "00:00"}, {"end", "23:59"},
	} {
		value, ok := field(spec.key, spec.def)
		if !ok {
			return 0, false
		}
		fields[spec.key] = value
	}
	minuteField, hourField := fields["minute"], fields["hour"]
	domField, monthField, dowField := fields["dom"], fields["month"], fields["dow"]
	beginField, endField := fields["begin"], fields["end"]

	cronFields := []struct {
		value    string
		min, max int
		names    map[string]int
	}{
		{minuteField, 0, 59, nil},
		{hourField, 0, 23, nil},
		{domField, 1, 31, nil},
		{monthField, 1, 12, protectionMonthNames},
		{dowField, 0, 7, protectionWeekdayNames},
	}
	cronValues := make([]map[int]bool, len(cronFields))
	for i, spec := range cronFields {
		values, ok := protectionCronValues(spec.value, spec.min, spec.max, spec.names)
		if !ok {
			return 0, false
		}
		cronValues[i] = values
	}
	minutes, hours, doms, months, dows := cronValues[0], cronValues[1], cronValues[2], cronValues[3], cronValues[4]
	if dows[7] { // 7 and 0 both name Sunday
		delete(dows, 7)
		dows[0] = true
	}

	beginMinute, ok := protectionClockMinute(beginField)
	if !ok {
		return 0, false
	}
	endMinute, ok := protectionClockMinute(endField)
	if !ok {
		return 0, false
	}

	hourList := protectionSortedKeys(hours)
	minuteList := protectionSortedKeys(minutes)
	// A restricted day-of-month and a restricted day-of-week are an OR in cron.
	domOrDow := strings.TrimSpace(domField) != "*" && strings.TrimSpace(dowField) != "*"

	first := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(2028, 12, 31, 0, 0, 0, 0, time.UTC)

	fireCount, maxGap := 0, 0
	var previous int64
	for day := first; !day.After(last); day = day.AddDate(0, 0, 1) {
		if !months[int(day.Month())] {
			continue
		}
		domMatch := doms[day.Day()]
		dowMatch := dows[int(day.Weekday())]
		var dayMatch bool
		if domOrDow {
			dayMatch = domMatch || dowMatch
		} else {
			dayMatch = domMatch && dowMatch
		}
		if !dayMatch {
			continue
		}
		for _, hour := range hourList {
			for _, minute := range minuteList {
				if !protectionInWindow(hour*60+minute, beginMinute, endMinute) {
					continue
				}
				at := day.Add(time.Duration(hour)*time.Hour+time.Duration(minute)*time.Minute).Unix() / 60
				if fireCount > 0 {
					if gap := int(at - previous); gap > maxGap {
						maxGap = gap
					}
				}
				previous = at
				fireCount++
			}
		}
	}
	if fireCount < 2 {
		return 0, false
	}
	return maxGap, true
}

// protectionInWindow reports whether a minute of the day is inside a begin/end
// window. A window whose begin is later than its end wraps past midnight.
func protectionInWindow(minuteOfDay, begin, end int) bool {
	if begin <= end {
		return minuteOfDay >= begin && minuteOfDay <= end
	}
	return minuteOfDay >= begin || minuteOfDay <= end
}

// protectionClockMinute parses HH:MM into a minute of the day.
func protectionClockMinute(value string) (int, bool) {
	hour, minute, ok := strings.Cut(strings.TrimSpace(value), ":")
	if !ok {
		return 0, false
	}
	h, err := strconv.Atoi(strings.TrimSpace(hour))
	if err != nil || h < 0 || h > 23 {
		return 0, false
	}
	m, err := strconv.Atoi(strings.TrimSpace(minute))
	if err != nil || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// protectionCronValues expands one cron field into the set of values it fires
// on. It accepts *, */n, a, a-b, a-b/n, and a/n, with or without names.
func protectionCronValues(field string, min, max int, names map[string]int) (map[int]bool, bool) {
	values := map[int]bool{}
	for _, item := range strings.Split(field, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			return nil, false
		}
		base := item
		step, hasStep := 1, false
		if idx := strings.Index(item, "/"); idx >= 0 {
			base = strings.TrimSpace(item[:idx])
			n, err := strconv.Atoi(strings.TrimSpace(item[idx+1:]))
			if err != nil || n <= 0 {
				return nil, false
			}
			step, hasStep = n, true
		}

		var low, high int
		switch {
		case base == "*":
			low, high = min, max
		case strings.Contains(base, "-"):
			lowPart, highPart, _ := strings.Cut(base, "-")
			a, ok := protectionCronValue(lowPart, names)
			if !ok {
				return nil, false
			}
			b, ok := protectionCronValue(highPart, names)
			if !ok {
				return nil, false
			}
			low, high = a, b
		default:
			a, ok := protectionCronValue(base, names)
			if !ok {
				return nil, false
			}
			low, high = a, a
			if hasStep {
				high = max
			}
		}
		if low < min || high > max || low > high {
			return nil, false
		}
		// Add only while the next value stays at or below high: a huge step such
		// as 59/9223372036854775807 must not overflow.
		for v := low; ; {
			values[v] = true
			if step > high-v {
				break
			}
			v += step
		}
	}
	if len(values) == 0 {
		return nil, false
	}
	return values, true
}

// protectionCronValue reads one cron number or name.
func protectionCronValue(value string, names map[string]int) (int, bool) {
	value = strings.TrimSpace(value)
	if names != nil {
		if v, ok := names[strings.ToLower(value)]; ok {
			return v, true
		}
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return n, true
}

// protectionSortedKeys returns the keys of a set in ascending order.
func protectionSortedKeys(values map[int]bool) []int {
	keys := make([]int, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

// protectionByteUnits are the IEC units above bytes.
var protectionByteUnits = []string{"KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}

// formatBytesIEC formats a byte count with IEC units and 1 decimal place.
func formatBytesIEC(n float64) string {
	if n < 1024 {
		return fmt.Sprintf("%.0f B", n)
	}
	value := n
	for i, unit := range protectionByteUnits {
		value /= 1024
		if value < 1024 || i == len(protectionByteUnits)-1 {
			return fmt.Sprintf("%.1f %s", value, unit)
		}
	}
	return fmt.Sprintf("%.1f EiB", value)
}

// protectionDatasetsFromRows walks a pool.dataset.query answer and keeps each
// dataset name once. It follows every nested children array, so it works whether
// or not the release honours the flat option.
func protectionDatasetsFromRows(rows []map[string]any) []protectionDataset {
	out := []protectionDataset{}
	seen := map[string]bool{}

	var walk func(rows []map[string]any)
	walk = func(rows []map[string]any) {
		for _, row := range rows {
			name := protectionString(row["name"])
			if name == "" {
				name = protectionString(row["id"])
			}
			if name != "" && !seen[name] {
				seen[name] = true
				dataset := protectionDataset{Name: name, Type: protectionString(row["type"])}
				dataset.Used, dataset.HasUsed = protectionParsedNumber(row["usedbydataset"])
				out = append(out, dataset)
			}
			children, ok := row["children"].([]any)
			if !ok {
				continue
			}
			nested := make([]map[string]any, 0, len(children))
			for _, child := range children {
				if m, ok := child.(map[string]any); ok {
					nested = append(nested, m)
				}
			}
			walk(nested)
		}
	}
	walk(rows)

	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// protectionParsedNumber reads a middleware property object such as
// usedbydataset: {"parsed": 1234, "rawvalue": "1234"}.
func protectionParsedNumber(v any) (float64, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return 0, false
	}
	if n, ok := m["parsed"].(float64); ok {
		return n, true
	}
	if s, ok := m["rawvalue"].(string); ok {
		if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// protectionSnapshotTasksFromRows reads the periodic snapshot tasks, sorted by id.
func protectionSnapshotTasksFromRows(rows []map[string]any) []protectionSnapshotTask {
	out := make([]protectionSnapshotTask, 0, len(rows))
	for _, row := range rows {
		id, ok := protectionInt(row["id"])
		if !ok {
			continue
		}
		task := protectionSnapshotTask{
			ID:           id,
			Dataset:      protectionString(row["dataset"]),
			Recursive:    protectionBool(row["recursive"]),
			Exclude:      protectionStringList(row["exclude"]),
			Enabled:      protectionBool(row["enabled"]),
			NamingSchema: protectionString(row["naming_schema"]),
			AllowEmpty:   protectionBool(row["allow_empty"]),
		}
		if minutes, ok := scheduleIntervalMinutes(row["schedule"]); ok {
			task.IntervalMinutes, task.HasInterval = minutes, true
		}
		task.LastRunState, _ = lastRun(row["state"], nil, false)["state"].(string)
		task.Failed = task.LastRunState == "ERROR"
		out = append(out, task)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// protectionReplicationsFromRows reads the replication tasks, sorted by id, onto
// an allow-list of fields.
func protectionReplicationsFromRows(rows []map[string]any) []protectionReplication {
	out := make([]protectionReplication, 0, len(rows))
	for _, row := range rows {
		id, ok := protectionInt(row["id"])
		if !ok {
			continue
		}
		replication := protectionReplication{
			ID:              id,
			Name:            protectionString(row["name"]),
			Direction:       protectionString(row["direction"]),
			Transport:       protectionString(row["transport"]),
			SourceDatasets:  protectionStringList(row["source_datasets"]),
			TargetDataset:   protectionString(row["target_dataset"]),
			Recursive:       protectionBool(row["recursive"]),
			Exclude:         protectionStringList(row["exclude"]),
			Enabled:         protectionBool(row["enabled"]),
			PeriodicTaskIDs: protectionIntList(periodicSnapshotTaskIDs(row["periodic_snapshot_tasks"])),
		}
		replication.NamingSchemas = append(replication.NamingSchemas, protectionStringOrList(row["naming_schema"])...)
		replication.NamingSchemas = append(replication.NamingSchemas, protectionStringOrList(row["also_include_naming_schema"])...)
		replication.LastRunState, _ = lastRun(row["state"], row["job"], true)["state"].(string)
		replication.Failed = replication.LastRunState == "ERROR"
		out = append(out, replication)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// protectionSnapshotsFromRows reads the snapshot entries. The dataset comes from
// the dataset field or from the part of name before "@"; the snapshot name from
// the snapshot_name field or from the part after "@".
func protectionSnapshotsFromRows(rows []map[string]any) []protectionSnapshot {
	out := make([]protectionSnapshot, 0, len(rows))
	for _, row := range rows {
		dataset := protectionString(row["dataset"])
		full := protectionString(row["name"])
		if full == "" {
			full = protectionString(row["id"])
		}
		snapName := protectionString(row["snapshot_name"])
		if before, after, ok := strings.Cut(full, "@"); ok {
			if dataset == "" {
				dataset = before
			}
			if snapName == "" {
				snapName = after
			}
		}
		if dataset == "" || snapName == "" {
			continue
		}
		snapshot := protectionSnapshot{Dataset: dataset, Name: snapName}
		snapshot.Creation, snapshot.HasCreation = snapshotCreation(row)
		out = append(out, snapshot)
	}
	return out
}

// protectionString reads a string field, or "" for any other type.
func protectionString(v any) string {
	s, _ := v.(string)
	return s
}

// protectionBool reads a boolean field, or false for any other type.
func protectionBool(v any) bool {
	b, _ := v.(bool)
	return b
}

// protectionStringList reads a list of strings.
func protectionStringList(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// protectionStringOrList reads a field that one release sends as a string and
// another as a list of strings.
func protectionStringOrList(v any) []string {
	if s, ok := v.(string); ok {
		if s == "" {
			return nil
		}
		return []string{s}
	}
	return protectionStringList(v)
}

// protectionInt reads a JSON number as an int.
func protectionInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i), true
		}
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
			return i, true
		}
	}
	return 0, false
}

// protectionIntList reads a list of JSON numbers as ints.
func protectionIntList(values []any) []int {
	out := make([]int, 0, len(values))
	for _, value := range values {
		if id, ok := protectionInt(value); ok {
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
