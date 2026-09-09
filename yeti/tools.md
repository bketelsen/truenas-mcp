# MCP Tools Reference

Complete catalog of MCP tools exposed by truenas-mcp. Tools marked with **[write]** are excluded by default and are registered only with `--enable-writes` or `TRUENAS_ENABLE_WRITES=true`.

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
  - `compression` (string, optional) — algorithm (lz4, zstd, off)
- API: `pool.dataset.create`

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
- API: `zfs.snapshot.query` with filter `[["dataset", "=", dataset]]`

### `truenas_snapshot_get`
Get full details for a specific snapshot.
- Parameters:
  - `name` (string, required) — full snapshot name (e.g., `tank/data@snap1`)
- API: `zfs.snapshot.query` with filter `[["id", "=", name]]`

### `truenas_snapshot_create` **[write]**
Create a ZFS snapshot. Auto-generates a timestamp name (`auto-YYYYMMDD-HHMMSS`) if name is omitted.
- Parameters:
  - `dataset` (string, required) — dataset path
  - `name` (string, optional) — snapshot name
- API: `zfs.snapshot.create`

### `truenas_snapshot_delete` **[write]**
Delete a ZFS snapshot. Destructive.
- Parameters:
  - `name` (string, required) — full snapshot name (e.g., `tank/data@snap1`)
- API: `zfs.snapshot.delete`

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
  - `guest_ok` (boolean, optional) — allow guest access (default false)
- API: `sharing.smb.create`

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
- API: `alert.list`

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
- API: `app.query` with filter `[["name", "=", name]]`, then `app.upgrade`

### `truenas_app_update_all` **[write]**
Upgrade all apps with updates available and return the job IDs.
- Parameters: none
- API: `app.query` with filter `[["upgrade_available", "=", true]]`, then `app.upgrade` for each app

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
