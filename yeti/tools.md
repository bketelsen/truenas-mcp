# MCP Tools Reference

Complete catalog of MCP tools exposed by truenas-mcp. Tools marked with **[write]** are excluded in `--read-only` mode.

## System Tools (`tools_system.go`)

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
Restart an app.
- Parameters:
  - `name` (string, required) — app name
- API: `app.restart`
