# MCP Tools Reference

Complete catalog of MCP tools exposed by truenas-mcp. Tools marked with **[write]** are excluded by default and are registered only with `--enable-writes` or `TRUENAS_ENABLE_WRITES=true`.

Every tool carries MCP annotations (`Title`, `ReadOnlyHint`, `DestructiveHint`, `IdempotentHint`, `OpenWorldHint: false`) defined next to its `mcp.Tool` literal; `wantToolHints` in `server/annotations_test.go` is the authoritative classification table. All non-**[write]** tools are `readOnlyHint: true`. Destructive writes (`destructiveHint: true`): `truenas_dataset_delete`, `truenas_snapshot_delete`, `truenas_smb_delete`, `truenas_nfs_delete`, `truenas_app_configure`, `truenas_app_delete`, `truenas_app_update`, `truenas_app_update_all`. Every other **[write]** tool is `destructiveHint: false`.

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

Both tools are read-only and registered by `registerDataProtectionReadTools`. They return a summary, not the raw middleware entry: each task is built from an allow-list of fields, so nothing unlisted can leak. Field names were checked against `PeriodicSnapshotTaskEntry` and `ReplicationEntry` in the `v25_10_0` and `v27_0_0` API models of truenas/middleware (not live).

Output shape for both: `{ "summary": {"total", "enabled", "failing"}, "tasks": [...] }`, tasks sorted by `id`. `failing` counts tasks whose `last_run.state` is `ERROR`. List fields are never null; `schedule` and `ssh_credentials` stay null when the middleware sends null.

`last_run` is built by `lastRun` from the zettarepl `state` of the entry: `{"state", "datetime", "last_snapshot", "error"}`, absent keys omitted. `state` can be an object or a plain string in some releases; both work. A missing `state` gives `{}`. Datetimes arrive as `{"$date": <unix millis>}` and `wireDatetime` converts them to an RFC 3339 UTC string; any other form passes through unchanged and never fails the tool. For replication, `last_run.job` is `{"id", "state"}` taken from the top-level `job` (or `state.job` when that is null); the rest of the job (arguments, logs) is dropped.

### `truenas_snapshot_task_list`
List periodic snapshot tasks.
- Parameters:
  - `dataset` (string, optional) — only tasks for exactly this dataset
- API: `pool.snapshottask.query`; with `dataset` the filter `[["dataset", "=", dataset]]`, otherwise no parameters
- Task fields: `id`, `dataset`, `recursive`, `exclude`, `enabled`, `naming_schema`, `allow_empty`, `schedule` (object passed through), `retention` = `{"value": lifetime_value, "unit": lifetime_unit}` (unit is HOUR/DAY/WEEK/MONTH/YEAR), `last_run`

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
