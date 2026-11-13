# Spec: MCP tool catalog

This spec is the contract between the server and an MCP client: every tool it
registers, its parameters, the middleware methods it calls, and its safety
annotations. `server/tools_*.go` implements it; a change to a tool must change
this file in the same pull request.

Complete catalog of MCP tools exposed by truenas-mcp. Tools marked with **[write]** are excluded by default and are registered only with `--enable-writes` or `TRUENAS_ENABLE_WRITES=true`.

## Rules

- Every tool name starts with `truenas_`. Read-only tools are registered by the
  `register*ReadTools` and `register*Tools` functions that `server.New`
  (`server/server.go`) always calls; write tools only by the `*WriteTools`,
  `registerAppConfigureTools` and `registerAppComposeTools` functions it calls
  when `readOnly` is false. `registerDataProtectionWriteTools`
  (`server/tools_data_protection.go`) calls `registerSnapshotTaskRunTool` and
  `registerReplicationTaskCreateTool` in turn.
- Every tool sets `Annotations` with a non-empty `Title` and
  `OpenWorldHint: new(false)`.
- Every destructive write sets `DestructiveHint` explicitly; the MCP default is
  true, so a non-destructive write sets `new(false)`.

Every tool carries MCP annotations (`Title`, `ReadOnlyHint`, `DestructiveHint`, `IdempotentHint`, `OpenWorldHint: false`) defined next to its `mcp.Tool` literal; `wantToolHints` in `server/annotations_test.go` is the authoritative classification table. All non-**[write]** tools are `readOnlyHint: true`. Destructive writes (`destructiveHint: true`): `truenas_dataset_delete`, `truenas_snapshot_delete`, `truenas_smb_delete`, `truenas_nfs_delete`, `truenas_app_configure`, `truenas_app_compose_update`, `truenas_app_delete`, `truenas_app_update`, `truenas_app_update_all`, `truenas_snapshot_task_create`, `truenas_snapshot_task_run`, `truenas_replication_task_create`. Every other **[write]** tool is `destructiveHint: false`.

## System Tools (`tools_system.go`)

### `truenas_system_info`
Get system hostname, version, uptime, and platform.
- Parameters: none
- API: `system.info`

### `truenas_disk_list`
List all physical disks with name, size, model, serial, and health status.
- Parameters: none
- API: `disk.query` with no filters and `options.extra.pools = true`, so each disk reports the pool it belongs to

### `truenas_network_list`
List network interfaces with IP addresses and link status.
- Parameters: none
- API: `interface.query`

## Filesystem Tools (`tools_filesystem.go`)

### `truenas_filesystem_stat`
Return owner information (`uid`, `gid`, `user`, `group`), raw mode and formatted permissions, file type, resolved path and ACL type for 1 path below `/mnt`.
- Parameters:
  - `path` (string, required) — absolute path below `/mnt`, for example `/mnt/tank/data`
- Path rules:
  - The Server refuses a missing or non-string value, an empty string, NUL or control characters, a relative path and every `..` segment.
  - The Server cleans the path with Go's `path.Clean`, then requires the cleaned path to start with `/mnt/`.
  - `/mnt`, `/mnt/`, `/mntx/a`, `/mnt2/a` and paths outside `/mnt` are refused.
- API calls:
  - `filesystem.stat` with the cleaned path as 1 parameter.
  - `filesystem.getacl` with the cleaned `realpath` and `true` for simplified ACL output, only after the resolved path passes the `/mnt/` guard. A `realpath` that is missing or outside `/mnt/` (for example through a symlink) is an error, and the result holds no stat data.
- Output keys: `path`, `realpath`, `type`, `uid`, `gid`, `user`, `group`, `mode`, `permissions`, `acl_type`, `acl_trivial`.
  `user` and `group` are null when the stat response has null values. `permissions` is the mode masked with `07777` and formatted with 4 octal digits. The tool returns no ACL entries.

## Pool Tools (`tools_pool.go`)

### `truenas_pool_list`
List all ZFS pools with name, status, size, and health.
- Parameters: none
- API: `pool.query`

### `truenas_pool_get`
Get detailed pool info including topology (vdevs, disks).
- Parameters:
  - `name` (string, required) — pool name
- API: `pool.query` with filter `[["name", "=", name]]`

## Dataset Tools (`tools_dataset.go`)

### `truenas_dataset_list`
List datasets with usage, mountpoint, and compression info.
- Parameters:
  - `pool` (string, optional) — filter by pool name
- API: `pool.dataset.query`

### `truenas_dataset_get`
Get full properties for a specific dataset.
- Parameters:
  - `path` (string, required) — full dataset path (e.g., `tank/data`)
- API: `pool.dataset.query` with filter `[["id", "=", path]]`

### `truenas_dataset_create` **[write]**
Create a new ZFS dataset.
- Parameters:
  - `name` (string, required) — full dataset path (e.g., `tank/newdata`)
  - `comments` (string, optional) — description
  - `compression` (string, optional) — algorithm (e.g. `LZ4`, `ZSTD`, `OFF`; case-insensitive). The tool upper-cases the value before sending it, because `compression` is an upper-case enum on every release.
- API: `pool.dataset.create`
- See [api-audit.md](api-audit.md) (Gap 3).

### `truenas_dataset_delete` **[write]**
Delete a ZFS dataset. Destructive and irreversible.
- Parameters:
  - `path` (string, required) — full dataset path
- API: `pool.dataset.delete`

## Snapshot Tools (`tools_snapshot.go`)

### `truenas_snapshot_list`
List snapshots for a dataset with name, creation time, and referenced size.
- Parameters:
  - `dataset` (string, required) — dataset path
- API: `pool.snapshot.query` with filter `[["dataset", "=", dataset]]`

### `truenas_snapshot_get`
Get full details for a specific snapshot.
- Parameters:
  - `name` (string, required) — full snapshot name (e.g., `tank/data@snap1`)
- API: `pool.snapshot.query` with filter `[["id", "=", name]]`

### `truenas_snapshot_create` **[write]**
Create a ZFS snapshot. Auto-generates a timestamp name (`auto-YYYYMMDD-HHMMSS`) if name is omitted.
- Parameters:
  - `dataset` (string, required) — dataset path
  - `name` (string, optional) — snapshot name
- API: `pool.snapshot.create`

### `truenas_snapshot_delete` **[write]**
Delete a ZFS snapshot. Destructive.
- Parameters:
  - `name` (string, required) — full snapshot name (e.g., `tank/data@snap1`)
- API: `pool.snapshot.delete`

## Data Protection Tools (`tools_data_protection.go`)

The two list tools are read-only and registered by `registerDataProtectionReadTools`. They return a summary, not the raw middleware entry: each task is built from an allow-list of fields, so nothing unlisted can leak. Field names were checked against `PeriodicSnapshotTaskEntry` and `ReplicationEntry` in the `v25_10_0` and `v27_0_0` API models of truenas/middleware (not live).

Output shape for both: `{ "summary": {"total", "enabled", "failing"}, "tasks": [...] }`, tasks sorted by `id`. `failing` counts tasks whose `last_run.state` is `ERROR`. List fields are never null; `schedule` and `ssh_credentials` stay null when the middleware sends null.

`last_run` is built by `lastRun` from the zettarepl `state` of the entry: `{"state", "datetime", "last_snapshot", "error"}`, absent keys omitted. `state` can be an object or a plain string in some releases; both work. A missing `state` gives `{}`. Datetimes arrive as `{"$date": <unix millis>}` and `wireDatetime` converts them to an RFC 3339 UTC string; any other form passes through unchanged and never fails the tool. For replication, `last_run.job` is `{"id", "state"}` taken from the top-level `job` (or `state.job` when that is null); the rest of the job (arguments, logs) is dropped.

### `truenas_snapshot_task_list`
List periodic snapshot tasks.
- Parameters:
  - `dataset` (string, optional) — only tasks for exactly this dataset
- API: `pool.snapshottask.query`; with `dataset` the filter `[["dataset", "=", dataset]]`, otherwise no parameters
- Task fields: `id`, `dataset`, `recursive`, `exclude`, `enabled`, `naming_schema`, `allow_empty`, `schedule` (object passed through), `retention` = `{"value": lifetime_value, "unit": lifetime_unit}` (unit is HOUR/DAY/WEEK/MONTH/YEAR), `last_run`
- The per-task projection is the `snapshotTaskSummary` helper in `tools_data_protection.go`. `truenas_snapshot_task_create` reports its created task through the same helper, so both tools always show the same fields.

### `truenas_scrub_task_list`
List scrub tasks and each Pool's last scan. Code: `server/tools_scrub.go`, registered by `registerScrubReadTools` in both modes.
- Parameters:
  - `pool` (string, optional) — exact Pool name; the Server filters tasks by `pool_name` and Pools by `name`. When neither matches, error `pool "<name>" not found`.
- APIs, in order: `pool.scrub.query`, then `pool.query`, both with no parameters. Calls are never retried. Call and parse errors name the method.
- Output: `{ "summary": {...}, "tasks": [...], "pools": [...], "notes": [...] }`. Tasks and Pools are sorted by numeric `id`; list fields are always arrays, never null.
- Summary fields: `generated_at` (RFC 3339 UTC, from the injected `scrubNow` clock), `tasks_total`, `tasks_enabled`, `pools_total`, `pools_without_enabled_task`, `pools_past_threshold`, `pools_without_finished_scrub` (Pool name lists in Pool id order), `tasks_unknown_pool` (task ids in task id order). Counts and lists describe the filtered results.
- Task fields (allow-list): `id`, `pool` (Pool id), `pool_name`, `threshold_days` (middleware `threshold`), `description`, `schedule` (`minute`, `hour`, `dom`, `month`, `dow` only), `enabled`, `pool_found`. `pool_found` checks the task's Pool id against all ids from `pool.query`; unknown ids enter `tasks_unknown_pool`.
- Pool fields (allow-list): `id`, `name`, `scrub_task_ids` (all matching task ids, including disabled tasks), `last_scan` (null or `function`, `state`, `start_time`, `end_time`, `errors`), `days_since_last_scrub` (number or null), `past_threshold` (boolean). Datetimes use `wireDatetime`: `{"$date": <unix millis>}` becomes RFC 3339 UTC; null remains null.
- `days_since_last_scrub` is the age in days, rounded to 1 decimal, only for a `SCRUB` scan in `FINISHED` state with a parseable `end_time`. Otherwise it is null and the Pool enters `pools_without_finished_scrub`.
- `past_threshold` uses the unrounded age: it is true only with an enabled task and a finished scrub older than `(threshold - 1)` days. The smallest enabled threshold applies when multiple tasks refer to a Pool. Equality is not past threshold; missing finished scrub dates and disabled tasks cannot make it true. For a Pool without a finished scrub, TrueNAS 27.0 reads the pool history instead (`validate_pool` in `plugins/zpool/scrub_impl.py`) and can still start a scrub at the next run; the Tool cannot see that history, so `past_threshold` is a sufficient condition, not a complete list of Pools due for a scrub.
- Fixed notes explain that `last_scan` is the most recent scrub or resilver (a resilver hides the last scrub date), that the TrueNAS 27.0 threshold rule applies at the next scheduled run, that a Pool without a finished scrub depends on the pool history the Tool cannot see, and that schedules run in the Appliance's local time.
- Annotations: title `List Scrub Tasks`, `readOnlyHint: true`, `openWorldHint: false`. Source verification and its live-call limits: [api-audit.md](api-audit.md). Context: [overview.md](../design/overview.md#tool-registration-pattern).

### `truenas_snapshot_task_create` **[write]**
Create a periodic snapshot task: TrueNAS snapshots the dataset on the given cron schedule and deletes expired snapshots of the dataset by the retention lifetime. Retention selects snapshots by dataset and naming schema, not by the task that made them, so the tool is destructive.
- Parameters:
  - `dataset` (string, required) — dataset path (e.g. `tank/data`)
  - `lifetime_value` (number, required) — retention lifetime, an integer from 1 to 10000
  - `lifetime_unit` (string, required) — `HOUR`, `DAY`, `WEEK`, `MONTH`, or `YEAR`; any case is accepted and the tool sends upper case
  - `schedule` (object, required) — `minute` and `hour` (required strings); `dom`, `month`, `dow` (optional, default `*`); `begin` (default `00:00`) and `end` (default `23:59`), both `HH:MM`. Unknown keys are refused, so a typo cannot fall back to a default silently
  - `recursive` (boolean, optional, default false) — snapshot child datasets too
  - `exclude` (string[], optional, default empty) — datasets to skip; refused when `recursive` is not true
  - `naming_schema` (string, optional) — default `auto-%Y-%m-%d_%H-%M`
  - `allow_empty` (boolean, optional, default true) — take a snapshot even when the dataset holds no data
  - `enabled` (boolean, optional, default true) — create the task enabled
  - `dry_run` (boolean, optional, default false) — return the payload and do not create
- Argument types are checked with `strictBool` and `strictStringList` in `server/params.go`: a present value of the wrong type (for example the string `"false"` for `enabled`) is an error before any middleware call. The SDK does not validate arguments against `InputSchema`, so a lenient reader would turn such a value into the default and create a task the caller did not ask for.
- APIs: `pool.snapshottask.query` with filter `[["dataset", "=", dataset]]` (duplicate guard, also on a dry run), then `pool.snapshottask.create` with 1 object: `dataset`, `recursive`, `exclude`, `lifetime_value`, `lifetime_unit`, `naming_schema`, `schedule` (all 7 keys), `allow_empty`, `enabled`. No other keys are sent
- Duplicate guard: the client never replays a call after a transport error, and an AI client can retry a call whose result it did not see. When a task with the same `dataset` and the same `naming_schema` exists, the tool returns an error that names the existing task id and creates nothing. The same dataset with another naming schema is allowed
- Returns: the created task, projected by `snapshotTaskSummary` (the field set of `truenas_snapshot_task_list`); with `dry_run`: `{ "dry_run": true, "method": "pool.snapshottask.create", "payload": {...} }`
- Retention risk: zettarepl retention selects snapshots by dataset and naming schema, not by the task that made them. A new task can therefore delete older snapshots whose names match its naming schema, from a removed task or made by hand. So the tool is `destructiveHint: true`. It is not idempotent: the duplicate guard is a read-then-write check, so a repeat can add a task.
- `lifetime_value` is limited to 1 to 10000; the bound keeps the value inside the middleware Integer field and inside an `int`, which the payload conversion needs. `begin` and `end` must be the canonical `HH:MM` form: `time.Parse("15:04", v)` must format back to `v`, so `1:30` is refused and `01:30` is accepted.

### `truenas_snapshot_task_run` **[write]**
Run 1 existing periodic snapshot task now (TrueNAS "Run Now"). Code: `server/tools_snapshot_task_run.go`, registered by `registerDataProtectionWriteTools`. Middleware role: `SNAPSHOT_TASK_WRITE` (it includes `SNAPSHOT_TASK_READ`; `replication.query` and the snapshot query need `READONLY_ADMIN` or the matching `*_READ` roles).
- Parameters:
  - `id` (number, required) — task id, an integer of at least 1; `0`, a negative value, `1.5` or a string is refused
  - `confirm` (boolean, optional, default false; `strictBool`) — only `true` runs the task; anything else is a dry run
  - `timeout_seconds` (number, optional, default 120, allowed 10 to 600) — the whole wait for the run (job wait plus state poll). `snapshotRunWait` (a package variable) replaces it in tests, after the range check
- APIs, in order:
  1. `pool.snapshottask.query` with `[["id", "=", id]]`. Empty: error `no periodic snapshot task with id N; nothing was run`.
  2. A disabled task is refused, also on a dry run, because the middleware refuses it (`CallError "Task is not enabled"`).
  3. `replication.query` (no parameters). `starts_replication` lists the tasks zettarepl starts after this task: `enabled` true, `auto` true, `schedule` null, and this id in `periodic_snapshot_tasks`. Allow-list output: `id`, `name`, `direction`, `transport`, `target_dataset`; credentials and keys never reach the result. An error here is a tool error and nothing was run.
  4. Dry run returns `{ "dry_run": true, "method": "pool.snapshottask.run", "params": [id], "task": snapshotTaskSummary, "starts_replication": [...], "note" }`.
  5. Just before the write: a `ctx.Err()` check (a cancel gives `cancelled before the run of task N; nothing was run`), then `pool.snapshottask.query` `[["id", "=", id]]` again for a fresh baseline (`state.datetime`, `state.last_snapshot`). Gone or disabled now: error, nothing was run. `state.state` `RUNNING` now: `task N is running now (a scheduled or earlier run); nothing was run; try again after it finishes`. A 2nd `ctx.Err()` check follows. The baseline is read just before the call, so a run that finished between the preflight and the call never counts as this run.
  6. `pool.snapshottask.run` with `id`, exactly once. It is never called again in any path (no retry, no replay; the no-replay rule in `truenas/client.go`). An error says the state of the run is unknown and to check `truenas_snapshot_task_list` before a new try.
  7. When the result is a JSON integer (a job id), `waitForJob` (`server/jobs.go`) polls `core.get_jobs`. `FAILED`/`ABORTED` is an error with the job text; the error says "nothing new was made" only when the text says "already existed". Timeout or cancel: the state is unknown, check `truenas_snapshot_task_list`. `SUCCESS`, `null` and any other result go on to the state poll.
  8. `pool.snapshottask.query` `[["id", "=", id]]` every `jobPollInterval` until the deadline (`waitForSnapshotTaskState`, same goroutine-per-poll pattern as `waitForJob`). The run is seen when `state.datetime` differs from the baseline datetime read just before the call (raw value compare with `reflect.DeepEqual`; a missing value before means the task never ran) AND `state.state` is `FINISHED` or `ERROR`. `RUNNING` and a failed poll keep the wait going; the last poll error goes into the final message. Any FINISHED state with a datetime after that baseline is reported: a scheduled run that finishes after the call is accepted, because its snapshot is newer than the call.
  9. Verify with `callSnapshot(client, "query", [["id", "=", snapshot]], snapshotQueryOptions())` (`pool.snapshot.query`, falling back to `zfs.snapshot.query` on "method does not exist"). `findSnapshotBounded` runs it in a goroutine with a 1-slot channel and waits for the result, `ctx.Done()`, or the time left to the deadline with a floor of `snapshotVerifyFloor` (5 seconds; tests shrink it), so a run seen just before the deadline can still be verified. Cancel or timeout: `the snapshot S was not verified ... check truenas_snapshot_list`. A last `ctx.Err()` check runs before the success result.
- 2 middleware forms of `pool.snapshottask.run`:
  - TS-25.10.x and TS-27.0.0-RC.1: not a job. It checks the task, queues the run in zettarepl, and returns `null` at once. The snapshot is made later.
  - middleware master (a future release): a `@job`. Over the websocket it returns the job id. The job waits for zettarepl and fails with `EEXIST` "Snapshot X@Y already existed. This probably happened because the task already ran on schedule." when 2 runs fall in the same minute.
  - In both forms zettarepl writes the task state: `RUNNING` with a new datetime, then `last_snapshot` and `FINISHED` with a new datetime, or `ERROR` with `error`. In 25.10/27.0 an "already existed" run still sets `FINISHED` with a new datetime and the same `last_snapshot`.
- Returns on success (allow-list): `{ "ran": true, "task_id", "dataset", "recursive", "snapshot": "dataset@name", "already_existed": bool, "verified": true, "created": <unix seconds, omitted when unknown>, "last_run": lastRun(state), "starts_replication": [...], "note"? }`. `already_existed` is true when the finished state names the same `last_snapshot` as before: the name existed (the task probably ran on schedule in this minute) and no new snapshot was made. `note` says so, and for a recursive task it says child datasets got the same snapshot name.
- Errors after the call: state `ERROR` (with the state error text; no verified snapshot exists from this run); `FINISHED` with no `last_snapshot`; the snapshot not found or the query failed (names the snapshot, "not verified"; with `allow_empty` false the message says TrueNAS can skip or remove the snapshot of an unchanged dataset); deadline with no finished run ("the run was accepted ... do not run this tool again: check truenas_snapshot_task_list"); cancel.
- Off-schedule snapshot (description and dry-run note): the retention of this task never deletes the snapshot of a run whose time is off the task's schedule; another task can. zettarepl retention deletes only a snapshot that a periodic snapshot task owns, and a task owns a snapshot only when the snapshot time matches its schedule (`PeriodicSnapshotTaskSnapshotOwner.owns_snapshot` returns `schedule.should_run(...)`, which checks the cron fields and the `begin`/`end` window; `calculate_dataset_snapshots_to_remove` keeps a snapshot with no owner; zettarepl `release/25.10.3` and `master`). So the snapshot stays until someone deletes it, unless another task on the same dataset with the same naming schema has a schedule that matches its time. Observed on TrueNAS 25.10: the snapshot of a Run Now at 09:49 of a daily 00:00 task with a lifetime of 7 days was still present after 8 days, and the next scheduled snapshot was pruned on time ([issue #70](https://github.com/bketelsen/truenas-mcp/issues/70)).
- Annotations: `destructiveHint: true` — a run does what a scheduled run does, so retention can delete expired snapshots and linked replication tasks start. `idempotentHint: false` — each run makes a new snapshot or fails on the name of this minute.
- The handler body is `runSnapshotTask(ctx, client, req)`, so tests can pass a context they cancel.
- Tests: `server/tools_snapshot_task_run_test.go` (fake middleware with a state sequence, a per-query task before the run, and a hook that cancels or blocks; asserts `pool.snapshottask.run` is called exactly once, or never on a dry run or a refused argument).

### `truenas_replication_task_list`
List replication tasks.
- Parameters: none
- API: `replication.query`, no parameters
- Task fields: `id`, `name`, `direction` (PUSH/PULL), `transport` (SSH/SSH+NETCAT/LOCAL), `source_datasets`, `target_dataset`, `recursive`, `exclude`, `enabled`, `auto`, `schedule`, `ssh_credentials` (`{"id", "name"}` or null), `periodic_snapshot_task_ids` (ids from `periodic_snapshot_tasks`), `naming_schema`, `also_include_naming_schema`, `readonly`, `encryption` (bool only), `retention` = `{"policy": retention_policy, "value", "unit"}`, `last_run`
- SECURITY: the entry holds `encryption_key`, `encryption_key_location`, and an `ssh_credentials` keychain credential whose `attributes` can contain a PRIVATE KEY. None of these are in the allow-list. `TestReplicationTaskList_NoSecretLeak` feeds a fixture with a private key and an encryption key and checks they are absent from the result text. When you add a field to this tool, add it to the allow-list deliberately and never copy a whole nested object.
- The per-task projection is the `replicationTaskSummary` helper in `tools_data_protection.go`. `truenas_replication_task_create` reports its created task through the same helper, so both tools always show the same fields.

### `truenas_replication_task_create` **[write]**
Create a replication task that copies datasets into a new target dataset on this appliance. Code: `server/tools_replication_task.go`, registered by `registerDataProtectionWriteTools` (next to `registerSnapshotTaskRunTool`). Middleware method: `replication.create`, role `REPLICATION_TASK_WRITE` (the `role_prefix` of `plugins/replication.py` is `REPLICATION_TASK`; TS-27.0.0-RC.1). The 3 reads need `READONLY_ADMIN` or the matching `*_READ` roles.
- Local only: the task is always `direction` `PUSH` with `transport` `LOCAL`, so both sides are datasets of the same appliance. A remote transport would need an SSH credential and an encryption key; the tool has no argument for either and refuses every `transport` but `LOCAL` (any case), also on a dry run, with an error that says it creates local replication only. Nothing is called then.
- Parameters:
  - `source_datasets` (string[], required) — 1 or more dataset paths to copy
  - `target_dataset` (string, required) — the new dataset to copy into; at least 1 `/`, so a pool root is refused
  - `periodic_snapshot_task_ids` (number[], required) — 1 or more ids of the periodic snapshot tasks the replication runs after; an id must be an integer of at least 1, and `0`, a negative value, `1.5`, a string or a duplicate is refused
  - `name` (string, optional) — default: the sources joined with `, `, then ` - `, then the target (for example `fast/a - slow/aback`); longer than 120 characters is refused
  - `recursive` (boolean, optional, default false; `strictBool`)
  - `exclude` (string[], optional, default empty) — each entry must be a strict child of a source (`<source>/...`); refused when `recursive` is not true
  - `retention_policy` (string, optional, default `SOURCE`) — `SOURCE`, `CUSTOM` or `NONE`, any case, sent upper case
  - `lifetime_value` (number) and `lifetime_unit` (string) — both required with `CUSTOM`, both refused with `SOURCE` or `NONE`; `lifetime_value` is an integer from 1 to 10000, `lifetime_unit` is `HOUR`, `DAY`, `WEEK`, `MONTH` or `YEAR` (any case, sent upper case)
  - `readonly` (string, optional, default `SET`) — `SET`, `REQUIRE` or `IGNORE`, any case, sent upper case
  - `enabled` (boolean, optional, default true)
  - `transport` (string, optional) — only `LOCAL`
  - `dry_run` (boolean, optional, default false; `strictBool`)
  - `expected_sha256` (string) — the `plan_sha256` a dry run returned; required when `dry_run` is false
- Argument names are an allow-list: any other key (for example `ssh_credentials`, `direction`, `allow_from_scratch`, `encryption`) is refused with `unknown parameter "<key>"` before a middleware call, so a remote-replication argument cannot be dropped silently. Values are checked with `strictBool`, `strictStringList` and the helpers in `server/params.go`; the SDK does not validate arguments against `InputSchema`.
- Dataset path rule for the sources, the target and the excludes: not empty, no leading or trailing `/`, no `//`, no `@`, no `#`, no whitespace.
- Hidden paths are refused with the arguments, before any middleware call: a target, a source or an exclude entry that `pool.dataset.query` does not return. The middleware hides internal datasets from that query (`internal_datasets_filters`), so a hidden path would always look absent, and the tool would accept an existing internal dataset as its new target. `hiddenDatasetPath` applies the same 4 rules to the path: the pool part (the text before the first `/`) is `boot-pool` or `freenas-boot`; or the path contains `/.system`; or the path padded with a trailing `/` contains `/ix-applications/`; or the path padded with a trailing `/` contains `/ix-apps`. `/.system` has no trailing slash in the rule, because the filter matches any id that holds it, so `slow/.system`, `slow/.system-backup` and `slow/x/.systemd` are all hidden; the padding makes `slow/ix-applications` and `slow/ix-apps` match too, where the hidden name ends the path. The error says the query does not show internal datasets, so the tool cannot prove that the target is new (or that a source exists); nothing was created.
- APIs, in order, on a dry run and on a write:
  1. `pool.snapshottask.query` with no parameters (every periodic snapshot task).
  2. `replication.query` with no parameters (every replication task).
  3. `pool.dataset.query` with filters `[["id", "in", ids]]`, where `ids` are the sources, the target and the target's parent, sorted and without duplicates, and options `select` [`id`, `name`] and `extra` {`flat`: true, `retrieve_children`: false, `properties`: [], `retrieve_user_props`: false}. A row's `id` (or `name`) is a found dataset. A flat query reports a nested child once, with its full name.
  - An error or a parse error in any of them is a tool error that ends with `; nothing was created`.
- Checks, all of them on a dry run too, and every refusal says that nothing was created. `within(path, root)` is `path == root` or `path` starts with `root + "/"`. A task "covers" a path `P` when `P == task.dataset`, or the task is `recursive`, `within(P, task.dataset)` and `P` is not within 1 of the task's `exclude` entries:
  - a. Each source exists. The target does not exist (zettarepl creates it on the first run, and the tool replicates into a new dataset only). The target's parent exists.
  - b. The target is not within a source, and no source is within the target.
  - c. No existing replication task has the same `name`.
  - d. For each existing replication task `T`: `within(target, T.target_dataset)` or `within(T.target_dataset, target)` is a refusal, and so is a target covered by 1 of `T`'s sources (same covering rule, with `T.recursive` and `T.exclude`). The error names the task id and name.
  - e. No existing periodic snapshot task covers the target: the target must be outside every task's tree. The error names the task id and dataset.
  - f. Each id in `periodic_snapshot_task_ids` exists. When `enabled` is true, each bound task is enabled. Each source is covered by at least 1 bound task. For each source `S` and each bound task `B` with `within(S, B.dataset)`: when `recursive` is true, every entry `X` of `B.exclude` with `within(X, S)` must be in `exclude`; when it is false, `S` must not be in `B.exclude`. This mirrors `_validate_source_datasets` in the middleware.
- `plan_sha256` is the hex SHA-256 of the `json.Marshal` of a map with these keys (`json.Marshal` sorts map keys, so the text of a plan is stable):
  - `payload`: the full payload below.
  - `snapshot_tasks`: every periodic snapshot task, sorted by id, projected to `id`, `dataset`, `recursive`, `exclude`, `enabled`, `naming_schema`, `lifetime_value`, `lifetime_unit`, `schedule`, `allow_empty`. Every configuration field the dry run shows is covered, so a change to a bound task's schedule or `allow_empty` is refused; only `state` is left out, so a run of a task does not change the hash.
  - `replication_tasks`: every replication task, sorted by id, projected to `id`, `name`, `direction`, `transport`, `source_datasets`, `target_dataset`, `recursive`, `exclude`, `enabled`, `auto`, `periodic_snapshot_task_ids`.
  - `datasets_found`: the sorted dataset ids the 3rd query found.
- Payload of `replication.create`, exactly these keys: `name`, `direction` (`PUSH`), `transport` (`LOCAL`), `source_datasets`, `target_dataset`, `recursive`, `exclude` (never null, `[]` when empty), `periodic_snapshot_tasks` (the ids as integers), `auto` (`true`), `retention_policy`, `lifetime_value` and `lifetime_unit` (only with `CUSTOM`), `readonly`, `enabled`, `allow_from_scratch` (`false`), `replicate` (`false`), `encryption` (`false`).
- A dry run returns `{ "dry_run": true, "method": "replication.create", "payload": {...}, "plan_sha256": "<64 hex>", "bound_snapshot_tasks": [snapshotTaskSummary of each bound task, in the order the ids were given], "note": "..." }`. The note says that zettarepl runs the task after each run of the bound tasks and creates the target on the first run, that retention `SOURCE` deletes target snapshots absent on the source, that `readonly` `SET` sets `readonly=on` on the target datasets, and to call again with `expected_sha256` set to `plan_sha256`.
- A write refuses an empty `expected_sha256` (the error names the current hash and says to run a dry run first) and a hash that does not match (the error says the arguments or the appliance changed since the dry run and names the current hash). `replication.create` is not called in either case. Then the tool checks `ctx.Err()` (a cancel gives an error with `nothing was created`) and calls `replication.create` exactly once with the payload as its only parameter. It never retries and never replays: on an error the tool says `replication.create: <err>; the result is unknown: check truenas_replication_task_list before you try again, and do not run this tool again blindly`. A validation error from the middleware arrives the same way, which is correct: the create did not happen.
- Returns on success: `{ "created": true, "task": replicationTaskSummary(created), "note" }`. The note says the task runs after the next run of the bound snapshot tasks, and that `truenas_snapshot_task_run` on a bound task runs it now. The summary is an allow-list, so `encryption_key`, `encryption_key_location` and the ssh credential attributes never reach the result.
- Annotations: `destructiveHint: true` — every run writes into the target dataset, and retention deletes target snapshots. `idempotentHint: false` — every accepted call creates a task, even when an identical one exists under another name.
- Tests: `server/tools_replication_task_test.go` (fake middleware that answers `pool.dataset.query` by the `in` filter it gets and counts every call; the payload is compared with `reflect.DeepEqual` after a JSON round trip; checks a to f each have a refusing case, and the accepting case is a target that the covering task excludes).

## Protection Report (`tools_protection.go`)

### `truenas_protection_report`
Report how each dataset is protected, in 1 read-only call. Design and rules: [plans/2026-11-02-protection-report.md](../plans/2026-11-02-protection-report.md).
- Parameters (all optional):
  - `pool` (string) — only datasets of this pool
  - `dataset` (string) — only this dataset and its descendants; its first path component is the pool. A `pool` that names another pool is an error
  - `flagged_only` (boolean, `strictBool`) — only datasets with at least 1 flag; the summary still counts the whole scope
- APIs, in this order: `pool.dataset.query` (filters `[]` or `[["pool","=",P]]`, options `select` + `extra` {`flat`, `properties: ["usedbydataset"]`, `retrieve_user_props: false`}), `pool.snapshottask.query`, `replication.query`, `callSnapshot("query", filters, snapshotQueryOptions())`, `docker.config`. An error in the first 4 is a tool error. A `docker.config` error is ignored: the limits text then says "the apps pool's ix-apps dataset".
- Rules (zettarepl semantics):
  - Covers: an enabled snapshot task covers its own dataset, and with `recursive` each descendant that is not an `exclude` entry or below one (`taskCovers`). An excluded dataset lists the task in `excluded_by`.
  - `replication_tasks`: enabled PUSH replications whose `source_datasets` cover the dataset by the same rule.
  - `replica_of`: enabled replications with a target on this NAS (`transport` LOCAL, or direction PULL) whose `target_dataset` is the dataset or an ancestor.
  - Periodic snapshot: a snapshot name that matches the naming schema of any snapshot task, or a replication `naming_schema` / `also_include_naming_schema` (`namingSchemaRegexp`, strftime to an anchored regexp). With no schema anywhere, `auto-%Y-%m-%d_%H-%M`.
  - Interval: the longest gap between 2 runs of the cron schedule, `begin`/`end` window applied, enumerated over 2027-2028 (`scheduleIntervalMinutes`). Day-of-month and day-of-week are OR when both are restricted (croniter). This is an estimate: the NAS runs schedules in its local time, and the tool enumerates in UTC. A daylight-saving change can make 1 real gap up to 1 hour longer or shorter; the 2-interval threshold absorbs that for schedules of 1 hour or more.
  - `max_age_hours` = 2 x the smallest interval of the covering enabled tasks with `allow_empty` true (for a replica: the replication's periodic snapshot tasks). A task with `allow_empty` false skips unchanged datasets, so it sets no threshold.
- Flags: `uncovered` (no enabled snapshot task and not a replica), `no_snapshot` (covered or a replica, no periodic snapshot), `stale` (the unrounded age in minutes is greater than 2 x the selected interval in minutes; `newest_age_hours` and `max_age_hours` are rounded to 1 decimal for display only, so they can look equal on a stale row), `task_failing` (a covering task's last run is `ERROR`), `replication_failing`.
- Output: `{ "summary": {generated_at, status ok|warning, datasets, covered, uncovered, stale, no_snapshot, replicated, replicas, flagged, failing_tasks}, "datasets": [{name, type (VOLUME only), used_by_dataset (IEC text), snapshot_tasks, excluded_by, newest_snapshot (the part after @), newest_age_hours, max_age_hours, replication_tasks, replica_of, flags}], "snapshot_tasks": [...], "replication_tasks": [...], "limits": [3 texts], "truncated"? }`. Empty fields are omitted. Replication rows are an allow-list: no keys and no credentials (`TestProtectionReport_NoSecretLeak`).
- Size: the text is at most `protectionReportMaxChars` = 36,000 characters (Pi Pocket keeps 40,000). Steps: indented JSON, compact JSON, drop datasets without flags, drop flagged datasets from the end (binary search), drop the task lists. `truncated` says what was left out. `TestProtectionReport_SizeLimit` covers 3,000 datasets.
- Limit named in the result: `pool.dataset.query` and `pool.snapshot.query` hide internal datasets (`<apps pool>/ix-apps`, `<pool>/.system`), so their protection is not reported. Datasets such as `fast/.ix-virt` are visible and are reported.

## Share Tools (`tools_share.go`)

### `truenas_smb_list`
List all SMB shares with name, path, and enabled status.
- Parameters: none
- API: `sharing.smb.query`

### `truenas_smb_create` **[write]**
Create an SMB share.
- Parameters:
  - `name` (string, required) — share name
  - `path` (string, required) — filesystem path (e.g., `/mnt/tank/data`)
  - `comment` (string, optional) — description
  - `guest_ok` (boolean, optional) — allow guest access (default false); on TrueNAS 25.10 and later this creates a `LEGACY_SHARE` share, the only purpose that allows guests
- API: `sharing.smb.create` with `name`, `path`, and optional `comment`. Without `guest_ok` this payload is sent unchanged.
  With `guest_ok` the tool adds `purpose: "LEGACY_SHARE"` and `options: {"guestok": true}`. If the server
  rejects that payload in validation (`EINVAL` naming `sharing_smb_create.purpose` or
  `sharing_smb_create.options`, as releases before 25.10 do), the tool retries once with the legacy
  top-level `guestok: true` and no `purpose` or `options`. A validation error
  means the create never ran, so the retry cannot create a duplicate share. If the retry also fails,
  the error keeps both messages.
- See [api-audit.md](api-audit.md) (Gap 2).

### `truenas_smb_delete` **[write]**
Delete an SMB share.
- Parameters:
  - `id` (number, required) — share ID
- API: `sharing.smb.delete`

### `truenas_nfs_list`
List all NFS exports with path, networks, and enabled status.
- Parameters: none
- API: `sharing.nfs.query`

### `truenas_nfs_create` **[write]**
Create an NFS export.
- Parameters:
  - `path` (string, required) — filesystem path
  - `networks` (string[], optional) — allowed networks (e.g., `192.168.1.0/24`)
  - `hosts` (string[], optional) — allowed hosts
- API: `sharing.nfs.create`

### `truenas_nfs_delete` **[write]**
Delete an NFS export.
- Parameters:
  - `id` (number, required) — export ID
- API: `sharing.nfs.delete`

## Alert Tools (`tools_alert.go`)

### `truenas_alert_list`
List active alerts with level, message, datetime, and dismissed status.
- Parameters:
  - `level` (string, optional) — filter: `INFO`, `WARNING`, `CRITICAL`, or empty for all
- API: `alert.list` with no parameters. `alert.list` takes no arguments on TrueNAS 27.0 (a query filter is
  rejected with `EINVAL`), so with `level` set the tool filters the returned array itself, ignoring case.
- See [api-audit.md](api-audit.md) (Gap 1).

### `truenas_alert_dismiss` **[write]**
Dismiss an alert.
- Parameters:
  - `id` (string, required) — alert ID
- API: `alert.dismiss`

## App Tools (`tools_app.go`)

### `truenas_app_list`
List every installed app as a compact summary: `name`, `id`, `state`, `version`, `human_version`,
`latest_version`, `latest_app_version`, `upgrade_available`, `image_updates_available`,
`custom_app`, `migrated`, `action_required`, `portals`, `train` (from `metadata.train`), and
`containers` (the container count, from `active_workloads.containers`).
- Parameters: none
- API: `app.query` with no parameters
- Returns: `{ "summary": { "apps_total": <n> }, "apps": [ <entry>, ... ] }`, sorted by `name` and always an array (`apps: []` when no app is installed). Each entry carries only the fields above; a field the app does not have is `null`.
- Why compact: the raw `app.query` payload was about 104,700 characters for 13 apps (each app carries `notes` Markdown, `metadata`, and `active_workloads` with Docker network detail). The Pi Pocket MCP client cuts a tool result at 40,000 characters, keeping the first and last 20,000, so on 2026-11-02 only 7 of 13 apps reached the caller. `notes`, `metadata`, `active_workloads` and `config` are therefore dropped here. Projection lives in `summarizeApp` in `server/tools_app.go`; the regression test builds 13 apps of that size and asserts the result stays under 40,000 characters.
- For one app's full detail (containers, images, mounts, ports, notes, metadata), call `truenas_app_get`; for the raw config object, `truenas_app_config`.

### `truenas_app_get`
Get detailed info for a specific app.
- Parameters:
  - `name` (string, required) — app name
- API: `app.query` with filter `[["name", "=", name]]`

### `truenas_app_config`
Get the configuration values an app was installed or last edited with (environment, storage, network, resources, run-as IDs). Separate from `truenas_app_get` on purpose: `app.query` without `retrieve_config` omits this object, and it may contain plaintext secrets (database passwords, claim tokens, API keys). Read-only, but callers should request it only when the configuration itself is the question.
- Parameters:
  - `name` (string, required) — app name
- API: `app.config` with the app name
- Returns: `{ "name": <name>, "config": <raw config object> }`

### `truenas_apps_update_report`
Report installed apps with TrueNAS app or container image updates available.
- Parameters: none
- API: `app.query`

### `truenas_app_start` **[write]**
Start a stopped app.
- Parameters:
  - `name` (string, required) — app name
- API: `app.start`

### `truenas_app_stop` **[write]**
Stop a running app.
- Parameters:
  - `name` (string, required) — app name
- API: `app.stop`

### `truenas_app_restart` **[write]**
Restart an app by redeploying its containers.
- Parameters:
  - `name` (string, required) — app name
- API: `app.redeploy` (a job). TrueNAS SCALE has no `app.restart` method — verified against 25.10.4's `core.get_methods`; the tool called it until 2026-09-08 and every invocation failed with "Method does not exist".
- Returns: `{ "name", "job_id" }`; poll `truenas_jobs_list` for completion.

### `truenas_app_update` **[write]**
Upgrade an app to the latest available version and return the job ID.
- Parameters:
  - `name` (string, required) — app name
- API: `app.query` with filter `[["name", "=", name]]`, then `app.upgrade` with `{"app_version": "latest", "values": {}, "snapshot_hostpaths": true}` (via `upgradeApp`)
- Returns: `{ "name", "job_id" }`; poll `truenas_jobs_list` for completion.
- Snapshots: TrueNAS stops the app and snapshots each host path's containing ZFS dataset as `<dataset>@ix-app-upgrade-<app>-<old version>` (non-recursive; non-ZFS paths skipped), plus the ix-volumes as `@<old version>`. `app.rollback` restores only the ix-volumes, so the tool stays `destructiveHint: true`. Custom apps get no snapshots. Details in [design/overview.md](../design/overview.md) under "App Upgrade Snapshots".

### `truenas_app_update_all` **[write]**
Upgrade all apps with updates available and return the job IDs.
- Parameters: none
- API: `app.query` with filter `[["upgrade_available", "=", true]]`, then `app.upgrade` for each app, with the same options and snapshots as `truenas_app_update`
- Returns: `{ "summary", "jobs": [{ "name", "job_id" }], "failures": [{ "name", "error" }] }`

### `truenas_app_delete` **[write]**
Delete an installed app and return the job ID. Destructive and irreversible.
- Parameters:
  - `name` (string, required) — app name to delete
  - `confirm` (string, required) — type the app name again to confirm deletion; must equal `name` exactly (case-sensitive, no trimming)
  - `remove_ix_volumes` (boolean, optional, default false) — also destroy the app's ix-volumes dataset (TrueNAS-managed storage) recursively, with its snapshots. Host paths are never removed
  - `dry_run` (boolean, optional, default false) — check and describe; do not delete
- API: `app.query` with filter `[["name", "=", name]]` (existence check, also on dry runs), then `app.delete(name, options)`. This is a TrueNAS job; the result is the job ID.
- Steps: (1) `confirm != name` is an error and makes NO middleware call; (2) no app match is the error `app "<name>" not found`; (3) options are built explicitly with all 3 keys: `{"remove_images": true, "remove_ix_volumes": <flag>, "force_remove_custom_app": false}` (`AppDelete` model; the middleware defaults are true/false/false, but the tool never relies on them); (4) with `dry_run`, `app.delete` is NOT called.
- Returns: `{ "name", "job_id", "remove_ix_volumes" }`; with `dry_run`: `{ "dry_run": true, "name", "method": "app.delete", "options", "app": {"state", "version", "custom_app"}, "effects": [...] }`
- Annotations: destructive, and idempotent because a second call finds no app and changes nothing (like `truenas_dataset_delete`).
- `force_remove_custom_app` is always sent as `false`; the tool has no parameter to change it.

## Custom App Compose Tool (`tools_app_compose.go`)

### `truenas_app_compose_update` **[write]**
Replace the whole compose file of a custom (compose) app and wait for the TrueNAS job. Custom apps only: `truenas_app_configure` still refuses them, and this tool refuses a catalog app with an error that names `truenas_app_configure`.
- Parameters:
  - `name` (string, required) — custom app name
  - `compose` (string, required) — the complete new compose file as YAML text; empty or whitespace-only text is refused
  - `dry_run` (boolean, optional, default false) — return the current compose, the proposed compose, the diff and `current_sha256`; do not call `app.update`
  - `expected_sha256` (string) — required when `dry_run` is false; must equal the `current_sha256` the dry run returned
  - `timeout_seconds` (number, optional, default 600) — how long to wait for the job; only 30 to 1800 is accepted, anything else is refused before any middleware call
- APIs: `app.query` with filter `[ ["name", "=", name] ]` (existence and `custom_app` check), `app.config` (current compose), then `app.update(name, {"custom_compose_config": <object>})`, then `core.get_jobs` with filter `[ ["id", "=", jobID] ]` in a poll loop
- Role: `app.update` needs `APPS_WRITE`.
- Validation of `compose`, in this order: parse with `go.yaml.in/yaml/v3`; exactly 1 YAML document (more is refused); the top level must be a mapping; every mapping key at every depth must be a string (an integer key such as `8080:` is refused; `<<` merge keys are allowed and their anchor is checked once); the compose must not contain a cyclic alias (`&x [*x]` is refused as `cyclic alias`, and an anchor shared by many aliases is checked once, so the walk stays linear); a top-level `services` key must exist and must be a mapping with at least 1 entry; every service value must be a mapping. The value is then normalized through `json.Marshal` and a `json.Decoder` with `UseNumber()` into `map[string]any`, and that object — not the YAML text — is what the diff, the hash and `app.update` use, so the appliance gets exactly what the caller saw.
- Numbers keep their exact form. `checkScalar`, which runs on the YAML node tree before any value exists, refuses a `!!float` that is infinite or NaN (JSON has neither) or that has no fraction and is 2^53 or more in absolute value, and a `!!int` that does not decode to `int` or `int64` (yaml.v3 yields `uint64` or `float64` when the value does not fit `int64`); each error names the line and the value. This check must read the node, because the decoder has already rounded `9007199254740993.0` to `9007199254740992`. After that, `convertJSONNumbers` replaces every `json.Number`: an integer literal that fits `int64` becomes `int64`, an integer literal outside `int64` is refused and the error names the value, and any other number becomes `float64`. `5.0` becomes the integer 5; `1.5` stays a float. The stored compose from `app.config` is decoded the same way, and a number it cannot carry exactly is refused with an error that says so, before any write.
- Canonical form: `canonicalYAML` in `tools_app_compose.go` renders both the stored compose and the proposal with a `yaml.v3` encoder, 2-space indent and sorted map keys. Equal content always gives equal text, so the identical check and the hash cover content only, not key order or YAML style.
- Flow: (1) validate; (2) read the stored compose and render both sides; (3) `current_sha256` = hex SHA-256 of the canonical current text; (4) diff with `unifiedDiff` from `server/linediff.go` (headers `--- current` and `+++ proposed`, 3 context lines per hunk, LCS-based, no diff dependency); (5) equal texts are refused as `identical`; (6) `dry_run` returns `{ name, dry_run: true, current_sha256, current_compose, proposed_compose, diff }`; (7) a write without `expected_sha256`, or with one that differs from `current_sha256`, is refused and names the current hash, so the caller must preview first; (8) `app.update` is called exactly once.
- No replay: a transport error from `app.update` returns the error plus a warning that the job state is unknown, that `truenas_jobs_list` must be checked, and that the tool must not be run again blindly. This follows the no-replay rule in `truenas/client.go` (the middleware keeps no prior compose).
- Job wait: `core.get_jobs` is polled every `jobPollInterval` (2 seconds; a package variable so tests use milliseconds) until `SUCCESS`, `FAILED` or `ABORTED`, the deadline, or context cancellation. The context and the deadline are checked before each poll, and the wait between polls is the smaller of `jobPollInterval` and the time left, so no poll starts after the deadline and a cancel ends the wait at once. A failed poll is a read and is retried at the next interval; its error text is kept for the final message. `composeJobWait`, a package variable, replaces `timeout_seconds` in tests, because the smallest allowed timeout is 30 seconds. `truenas.Caller` takes no context, so a poll call cannot be stopped: each poll runs in its own goroutine and reports to a channel with room for 1 result, so the wait returns at the deadline or on a cancel without waiting for the call. An abandoned poll is a read; it finishes in the background and its result is dropped, and `app.update` is never called again (the no-replay rule).
- Returns on `SUCCESS`: `{ "name", "job_id", "state": "SUCCESS", "previous_sha256", "previous_compose", "diff" }`, where `previous_compose` is the complete compose as it was before the write. A `FAILED` or `ABORTED` job returns a tool error with the job ID, the state and the job's `error` text, and says that the complete previous compose is `current_compose` of the dry run (and `previous_compose` of a successful write), which can be sent back with this tool to roll back. A timeout or cancellation returns a tool error with the job ID and the last state, and says the job can still run, so `truenas_jobs_list` must be checked and the tool must not be re-run.
- Secrets: like `truenas_app_config`, the result carries the compose itself, which can hold plaintext secrets such as environment values. The tool description says so.
- Annotations: `destructiveHint: true` (the stored compose is replaced whole and TrueNAS keeps no copy), `idempotentHint: false` (every accepted call starts a new job), `openWorldHint: false`.

## App Configuration Tool (`tools_app_configure.go`)

### `truenas_app_configure` **[write]**
Change configuration values of an installed catalog app and return the job ID.
- Parameters:
  - `name` (string, required) — app name
  - `values` (object, required) — PARTIAL nested config in the shape `truenas_app_config` returns
  - `dry_run` (boolean, optional) — return the merged config and changed paths without applying
- API: `app.query` (existence + `custom_app` check), `app.config` (current values), then `app.update(name, {"values": <merged>})`
- Returns: `{ "name", "job_id", "changed_paths" }`; with `dry_run`: `{ "name", "dry_run": true, "changed_paths", "values" }`
- Why the client-side merge: TrueNAS `app.update` performs a SHALLOW top-level `dict.update` of the stored config with the caller's `values` (middleware `plugins/apps/crud.py`, `update_internal`), then re-populates schema defaults for anything now missing. A partial nested payload therefore replaces an entire section and resets its siblings — proven live on 25.10.4: after setting `resources.limits.memory=512`, a raw `app.update` carrying only `{"resources": {"limits": {"cpus": 2}}}` reset memory to its 4096 default. The tool deep-merges into the current config (objects recurse; lists, scalars, and null replace whole) and sends the complete result, so only the named leaves change.
- Guards: top-level sections must already exist (typo protection); `ix_*` sections are refused as input and stripped from the payload and change report because the middleware regenerates them (`RESERVED_NAMES`); an identical payload is refused as a no-op; custom compose apps are refused (their config is `custom_compose_config`, a different contract).
- Side effect: unless the app is STOPPED, the middleware runs `compose up --force-recreate`, so applying a change restarts the app's containers.

## Job and Report Tools (`tools_reports.go`)

### `truenas_jobs_list`
List recent TrueNAS jobs, optionally filtered by state or method.
- Parameters:
  - `state` (string, optional) — job state (`WAITING`, `RUNNING`, `SUCCESS`, `FAILED`, or `ABORTED`)
  - `method` (string, optional) — job method name
  - `limit` (number, optional) — maximum jobs to return, default 50, clamped to 200
- API: `core.get_jobs` with the filters built from `state` (upper-cased) and `method`, and `options` `{"order_by": ["-time_started"], "limit": <n>}`

### `truenas_health_report`
Return an aggregated read-only health report from system info, system state, pools, disks, and alerts.
- Parameters: none
- APIs: `system.info`, `system.state`, `pool.query`, `disk.query`, `alert.list`

## References

- Rationale: [ADR-0004 — Read-only by default](../adr/0004-read-only-by-default.md)
  (how write tools are gated), [ADR-0005 — Use the official MCP Go SDK](../adr/0005-use-the-official-mcp-go-sdk.md)
  (annotations and schema), [ADR-0006 — Use the official TrueNAS WebSocket client](../adr/0006-use-the-official-truenas-websocket-client.md)
  (every API method here)
- Context: [design/overview.md](../design/overview.md) (tool registration
  pattern, annotations), [design/glossary.md](../design/glossary.md) (Tool,
  Report, Job)
- Method audit: [api-audit.md](api-audit.md)
