# MCP Tools Reference

Complete catalog of MCP tools exposed by truenas-mcp. Tools marked with **[write]** are excluded by default and are registered only with `--enable-writes` or `TRUENAS_ENABLE_WRITES=true`.

Every tool carries MCP annotations (`Title`, `ReadOnlyHint`, `DestructiveHint`, `IdempotentHint`, `OpenWorldHint: false`) defined next to its `mcp.Tool` literal; `wantToolHints` in `server/annotations_test.go` is the authoritative classification table. All non-**[write]** tools are `readOnlyHint: true`. Destructive writes (`destructiveHint: true`): `truenas_dataset_delete`, `truenas_snapshot_delete`, `truenas_smb_delete`, `truenas_nfs_delete`, `truenas_app_configure`, `truenas_app_compose_update`, `truenas_app_delete`, `truenas_app_update`, `truenas_app_update_all`, `truenas_snapshot_task_create`. Every other **[write]** tool is `destructiveHint: false`.

## System Tools (`tools_system.go`)

### `truenas_health_report`
Return an aggregated read-only health report from system info, system state, pools, disks, and alerts.
- Parameters: none
- APIs: `system.info`, `system.state`, `pool.query`, `disk.query`, `alert.list`

### `truenas_system_info`
Get system hostname, version, uptime, and platform.
- Parameters: none
- API: `system.info`

### `truenas_disk_list`
List all physical disks with name, size, model, serial, and health status.
- Parameters: none
- API: `disk.query`

### `truenas_network_list`
List network interfaces with IP addresses and link status.
- Parameters: none
- API: `interface.query`

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

### `truenas_replication_task_list`
List replication tasks.
- Parameters: none
- API: `replication.query`, no parameters
- Task fields: `id`, `name`, `direction` (PUSH/PULL), `transport` (SSH/SSH+NETCAT/LOCAL), `source_datasets`, `target_dataset`, `recursive`, `exclude`, `enabled`, `auto`, `schedule`, `ssh_credentials` (`{"id", "name"}` or null), `periodic_snapshot_task_ids` (ids from `periodic_snapshot_tasks`), `naming_schema`, `also_include_naming_schema`, `readonly`, `encryption` (bool only), `retention` = `{"policy": retention_policy, "value", "unit"}`, `last_run`
- SECURITY: the entry holds `encryption_key`, `encryption_key_location`, and an `ssh_credentials` keychain credential whose `attributes` can contain a PRIVATE KEY. None of these are in the allow-list. `TestReplicationTaskList_NoSecretLeak` feeds a fixture with a private key and an encryption key and checks they are absent from the result text. When you add a field to this tool, add it to the allow-list deliberately and never copy a whole nested object.

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
List installed apps with name, version, status, and update availability.
- Parameters: none
- API: `app.query`

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
- Snapshots: TrueNAS stops the app and snapshots each host path's containing ZFS dataset as `<dataset>@ix-app-upgrade-<app>-<old version>` (non-recursive; non-ZFS paths skipped), plus the ix-volumes as `@<old version>`. `app.rollback` restores only the ix-volumes, so the tool stays `destructiveHint: true`. Custom apps get no snapshots. Details in `yeti/OVERVIEW.md` under "App Upgrade Snapshots".

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
- Validation of `compose`, in this order: parse with `go.yaml.in/yaml/v3`; exactly 1 YAML document (more is refused); the top level must be a mapping; every mapping key at every depth must be a string (an integer key such as `8080:` is refused); a top-level `services` key must exist and must be a mapping with at least 1 entry; every service value must be a mapping. The value is then normalized through `json.Marshal` + `json.Unmarshal` into `map[string]any`, and that object — not the YAML text — is what the diff, the hash and `app.update` use, so the appliance gets exactly what the caller saw.
- Canonical form: `canonicalYAML` in `tools_app_compose.go` renders both the stored compose and the proposal with a `yaml.v3` encoder, 2-space indent and sorted map keys. Equal content always gives equal text, so the identical check and the hash cover content only, not key order or YAML style.
- Flow: (1) validate; (2) read the stored compose and render both sides; (3) `current_sha256` = hex SHA-256 of the canonical current text; (4) diff with `unifiedDiff` from `server/linediff.go` (headers `--- current` and `+++ proposed`, 3 context lines per hunk, LCS-based, no diff dependency); (5) equal texts are refused as `identical`; (6) `dry_run` returns `{ name, dry_run: true, current_sha256, current_compose, proposed_compose, diff }`; (7) a write without `expected_sha256`, or with one that differs from `current_sha256`, is refused and names the current hash, so the caller must preview first; (8) `app.update` is called exactly once.
- No replay: a transport error from `app.update` returns the error plus a warning that the job state is unknown, that `truenas_jobs_list` must be checked, and that the tool must not be run again blindly. This follows the no-replay rule in `truenas/client.go` (the middleware keeps no prior compose).
- Job wait: `core.get_jobs` is polled every `jobPollInterval` (2 seconds; a package variable so tests use milliseconds) until `SUCCESS`, `FAILED` or `ABORTED`, the deadline, or context cancellation. A failed poll is a read and is retried at the next interval; its error text is kept for the final message. `composeJobWait`, a package variable, replaces `timeout_seconds` in tests, because the smallest allowed timeout is 30 seconds.
- Returns on `SUCCESS`: `{ "name", "job_id", "state": "SUCCESS", "previous_sha256", "previous_compose", "diff" }`. A `FAILED` or `ABORTED` job returns a tool error with the job ID, the state and the job's `error` text, and says that the previous compose is in the diff for a rollback. A timeout or cancellation returns a tool error with the job ID and the last state, and says the job can still run, so `truenas_jobs_list` must be checked and the tool must not be re-run.
- Secrets: like `truenas_app_config`, the result carries the compose itself, which can hold plaintext secrets such as environment values. The tool description says so.
- Annotations: `destructiveHint: true` (the stored compose is replaced whole and TrueNAS keeps no copy), `idempotentHint: false` (every accepted call starts a new job), `openWorldHint: false`.

## Job Tools (`tools_reports.go`)

### `truenas_app_configure` **[write]** (`tools_app_configure.go`)
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

### `truenas_jobs_list`
List recent TrueNAS jobs, optionally filtered by state or method.
- Parameters:
  - `state` (string, optional) — job state (`WAITING`, `RUNNING`, `SUCCESS`, `FAILED`, or `ABORTED`)
  - `method` (string, optional) — job method name
  - `limit` (number, optional) — maximum jobs to return, clamped to 200
- API: `core.get_jobs`
