# `truenas_protection_report` — design

**Status:** Done (2026-11-03, recorded 2026-11-13). [#49](https://github.com/bketelsen/truenas-mcp/pull/49)
(merge commit [`326049e`](https://github.com/bketelsen/truenas-mcp/commit/326049e42bcc9f568bdaaa0a448f9eae60660584))
added `truenas_protection_report` in `server/tools_protection.go`, registered in `server/server.go`, with
tests in `server/tools_protection_test.go`.

Goal: 1 read-only call answers "is this data protected?" for every dataset. Today an agent joins
`truenas_dataset_list`, `truenas_snapshot_task_list`, `truenas_replication_task_list` and
`truenas_snapshot_list` by hand (homewiki `docs/backups.md`, "Coverage").

Source: Moneo's muster proposal (desk T712, 2026-11-02), night-shift line 22.

## Tool

- Name: `truenas_protection_report`. Read-only, registered in both modes (`registerProtectionReportTools`
  in `server/tools_protection.go`). Annotations: `ReadOnlyHint: true`, `OpenWorldHint: false`.
- Arguments (all optional):
  - `pool` (string): only datasets of this pool.
  - `dataset` (string): only this dataset and its descendants. The pool is its first path component.
  - `flagged_only` (bool, `strictBool`): only datasets that have at least 1 flag.

## Middleware calls (read only)

| Order | Method | Parameters | On error |
|---|---|---|---|
| 1 | `pool.dataset.query` | filters `[]` or `[["pool","=",P]]`; options `{"select":["id","name","type","pool","usedbydataset"],"extra":{"flat":true,"properties":["usedbydataset"],"retrieve_user_props":false}}` | tool error |
| 2 | `pool.snapshottask.query` | none | tool error |
| 3 | `replication.query` | none | tool error |
| 4 | `callSnapshot(client, "query", filters, snapshotQueryOptions())` | filters `[]` or `[["pool","=",P]]` | tool error |
| 5 | `docker.config` | none | ignored (apps pool unknown) |

The dataset parser walks every row and every nested `children` array and keeps each `name` once, so it
works whether or not a release honours `flat` and `select`.

## Rules

- **Covers** (snapshot task T on dataset D): D == T.dataset, or T.recursive and D is a descendant of
  T.dataset (`strings.HasPrefix(D, T.dataset+"/")`) and D is not an exclude entry or a descendant of one
  (zettarepl `is_child`). Only enabled tasks count. A dataset under a recursive task but excluded lists
  the task id in `excluded_by`.
- **Replicated** (PUSH replication R copies D): the same rule with R.source_datasets, R.recursive,
  R.exclude. Only enabled PUSH tasks count.
- **Replica**: D == R.target_dataset or a descendant of it, for an enabled R whose target is on this NAS
  (`transport == "LOCAL"` or `direction == "PULL"`).
- **Periodic snapshot**: a snapshot whose name (the part after `@`) matches the naming schema of any
  snapshot task, or a replication's `naming_schema` / `also_include_naming_schema` (strftime → anchored
  regexp: `%Y` 4 digits, `%m %d %H %M %S %y` 2 digits, `%j` 3 digits, `%z` sign and 4 digits, `%s`
  digits, `%%` a literal `%`, other `%x` `.+?`; literals quoted). The newest by creation time is
  `newest_snapshot`, its age `newest_age_hours` (1 decimal).
- **Interval** of a task: the longest gap between 2 runs of its cron schedule (`minute hour dom month dow`,
  `begin`/`end` window, names `jan`..`dec` and `sun`..`sat`, dow 7 = 0, dom/dow OR when both are not `*`),
  enumerated minute-exact over 2027-01-01..2028-12-31 UTC. Unknown or unparsable: no interval.
- **Threshold** `max_age_hours` = 2 × the smallest interval among the dataset's covering enabled tasks with
  `allow_empty` true (a task with `allow_empty` false skips unchanged datasets). For a replica, among the
  replication's `periodic_snapshot_tasks` with `allow_empty` true.

## Flags (per dataset)

| Flag | When |
|---|---|
| `uncovered` | no enabled snapshot task covers it, and it is not a replica |
| `no_snapshot` | covered or a replica, and no periodic snapshot exists |
| `stale` | the unrounded age in minutes > 2 x the selected interval in minutes (the displayed hours are rounded) |
| `task_failing` | a covering enabled snapshot task has last run state `ERROR` |
| `replication_failing` | an enabled replication in `replication_tasks` or `replica_of` has last run state `ERROR` |

## Output

```json
{
  "summary": {"generated_at", "status": "ok|warning", "datasets", "covered", "uncovered", "stale",
              "no_snapshot", "replicated", "replicas", "flagged", "failing_tasks"},
  "datasets": [{"name", "type" (VOLUME only), "used_by_dataset", "snapshot_tasks", "excluded_by",
                "newest_snapshot", "newest_age_hours", "max_age_hours", "replication_tasks",
                "replica_of", "flags"}],
  "snapshot_tasks": [{"id", "dataset", "recursive", "exclude", "enabled", "naming_schema",
                      "allow_empty", "interval_hours", "last_run_state"}],
  "replication_tasks": [{"id", "name", "direction", "transport", "source_datasets", "target_dataset",
                         "recursive", "exclude", "enabled", "periodic_snapshot_task_ids", "last_run_state"}],
  "limits": ["..."],
  "truncated": "..."
}
```

Empty fields are omitted. Replication rows are an allow-list: no keys, no credentials.

## Size limit

Pi Pocket keeps at most 40,000 characters of an MCP result. The tool keeps its text at or under
`protectionReportMaxChars` = 36,000: indented JSON; if too long, compact JSON; then datasets without
flags are dropped; then flagged datasets are dropped from the end; then the task lists. `truncated`
says what was left out and how to narrow the scope. Summary counts always cover the whole scope.

## Limits named in the result

1. Internal datasets are hidden: `pool.dataset.query` and `pool.snapshot.query` leave out
   `<apps pool>/ix-apps` (app ix-volumes, app configuration and container images) and system datasets
   such as `<pool>/.system`. The report cannot see them or their snapshots. The apps pool comes from
   `docker.config`; when that call fails the text says "the apps pool".
2. Schedules run in the NAS's local time. The interval is the longest gap between 2 scheduled runs, counted in UTC,
   so a daylight-saving change can shift 1 gap by up to 1 hour.
3. Replication to another host is listed, but the remote copy is not checked.

## References

- Implements: the `truenas_protection_report` entry in [specs/tools.md](../specs/tools.md)
- Context: [design/overview.md](../design/overview.md) (Key Patterns)
