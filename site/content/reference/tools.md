# Tools

truenas-mcp registers 43 tools. Every name starts with `truenas_`. The 23
read-only tools are always registered. The 20 tools marked **Write tool**
exist only when the server runs with `--enable-writes` or
`TRUENAS_ENABLE_WRITES=true` (see
[read-only by default](../concepts/read-only.md)).

Parameters are optional unless marked *required*. Tools that return a job ID
do not wait for the job: follow it with `truenas_jobs_list`.
`truenas_app_compose_update` and `truenas_snapshot_task_run` wait and report
the result.

The full contract, with the TrueNAS middleware method behind each tool, is the
[tool catalog spec](https://github.com/bketelsen/truenas-mcp/blob/main/docs/specs/tools.md).

## Reports

### `truenas_health_report`

1 health report from system info, system state, pools, disks and alerts. No parameters.

### `truenas_protection_report`

How each dataset is protected: the snapshot tasks that cover it, the age of its newest periodic snapshot, and replication. Flags uncovered, stale and failing datasets. The result stays under 40,000 characters.

Parameters: `pool`, `dataset`, `flagged_only`.

### `truenas_apps_update_report`

The installed apps with an app update or a container image update. No parameters.

## System

### `truenas_system_info`

Host name, version, uptime and platform. No parameters.

### `truenas_disk_list`

Physical disks with size, model, serial, health and the pool of each disk. No parameters.

### `truenas_network_list`

Network interfaces with IP addresses and link state. No parameters.

## Filesystem

### `truenas_filesystem_stat`

Owner information (`uid`, `gid`, `user`, `group`), raw mode and formatted permissions, file type, resolved path and ACL type for 1 path below `/mnt`. The result also includes `realpath`, `acl_type` and `acl_trivial`; it does not return ACL entries.

Parameters: `path` *required*. It refuses paths outside `/mnt`, `..` segments and control characters. A path that resolves outside `/mnt` (for example through a symlink) is refused, and the result then holds no stat data.

## Pools and datasets

### `truenas_pool_list`

ZFS pools with status, size and health. No parameters.

### `truenas_pool_get`

1 pool with its topology (vdevs and disks).

Parameters: `name` *required*.

### `truenas_dataset_list`

Datasets with usage, mountpoint and compression.

Parameters: `pool`.

### `truenas_dataset_get`

All properties of 1 dataset, for example `tank/data`.

Parameters: `path` *required*.

### `truenas_dataset_create`

**Write tool.** Registered only with `--enable-writes`.

Create a dataset. `compression` is an algorithm such as `LZ4`, `ZSTD` or `OFF`, in any case.

Parameters: `name` *required*, `comments`, `compression`.

### `truenas_dataset_delete`

**Write tool.** Registered only with `--enable-writes`.

Delete a dataset and the data in it.

Parameters: `path` *required*.

## Snapshots

### `truenas_snapshot_list`

The snapshots of a dataset, with creation time and referenced size.

Parameters: `dataset` *required*.

### `truenas_snapshot_get`

1 snapshot, for example `tank/data@snap1`.

Parameters: `name` *required*.

### `truenas_snapshot_create`

**Write tool.** Registered only with `--enable-writes`.

Take a snapshot. Without `name`, the name is `auto-YYYYMMDD-HHMMSS`.

Parameters: `dataset` *required*, `name`.

### `truenas_snapshot_delete`

**Write tool.** Registered only with `--enable-writes`.

Delete a snapshot.

Parameters: `name` *required*.

## Data protection

### `truenas_snapshot_task_list`

Periodic snapshot tasks with schedule, retention and last run.

Parameters: `dataset`.

### `truenas_scrub_task_list`

Scrub tasks with schedules, thresholds and enabled state, joined to each
Pool's last scan. The summary lists Pools without an enabled task, Pools past
threshold, Pools without a finished scrub, and task ids with an unknown Pool.
A resilver hides the last scrub date. For a Pool without a finished scrub,
TrueNAS decides from the pool history, which this tool cannot see. Schedules
run in the Appliance's local time.

Parameters: `pool` (exact Pool name).

### `truenas_snapshot_task_create`

**Write tool.** Registered only with `--enable-writes`.

Create a periodic snapshot task. Refuses a 2nd task with the same dataset and naming schema. `dry_run` returns the payload.

Parameters: `dataset`, `lifetime_value`, `lifetime_unit`, `schedule` *required*; `recursive`, `exclude`, `naming_schema`, `allow_empty`, `enabled`, `dry_run`.

`lifetime_value` is 1 to 10000, and `lifetime_unit` is `HOUR`, `DAY`, `WEEK`,
`MONTH` or `YEAR`. `schedule` needs `minute` and `hour`; `dom`, `month` and
`dow` default to `*`, and `begin` and `end` (`HH:MM`) default to `00:00` and
`23:59`. The default naming schema is `auto-%Y-%m-%d_%H-%M`. `exclude` needs
`recursive: true`.

### `truenas_snapshot_task_run`

**Write tool.** Registered only with `--enable-writes`.

Run 1 periodic snapshot task now. Dry run by default; `confirm: true` runs it once, waits for the run and verifies the snapshot.

Parameters: `id` *required*; `confirm`, `timeout_seconds`.

The tool waits 120 seconds by default; `timeout_seconds` takes 10 to 600.

The task's own retention does not delete the snapshot of a run off the task's
schedule. zettarepl deletes only the snapshots whose time matches the schedule
of a periodic snapshot task on that dataset with that naming schema. So a
snapshot from a run at 09:49 of a daily 00:00 task is pruned only by another
task on that dataset with the same naming schema and a schedule that matches
09:49; with no such task, it stays until someone deletes it. To get a snapshot
that the task's retention prunes, wait for the scheduled run.

### `truenas_replication_task_list`

Replication tasks with source, target, schedule, retention and last run. Never returns encryption keys or SSH credential secrets. No parameters.

### `truenas_replication_task_create`

**Write tool.** Registered only with `--enable-writes`.

Create a replication task that copies datasets into a new target dataset on the same appliance. The task is a `PUSH` with the `LOCAL` transport; any other transport is refused.

Parameters: `source_datasets`, `target_dataset`, `periodic_snapshot_task_ids` *required*; `name`, `recursive`, `exclude`, `retention_policy`, `lifetime_value`, `lifetime_unit`, `readonly`, `enabled`, `transport`, `dry_run`, `expected_sha256`.

The task runs after each run of the periodic snapshot tasks it is bound to, so every source dataset must be covered by 1 of those tasks, and an enabled task needs enabled snapshot tasks. The target dataset must not exist yet and must lie outside the tree of every other snapshot and replication task: zettarepl creates the target on the first run. A path that TrueNAS hides from its dataset query (a boot-pool dataset, or 1 whose path holds `/.system`, `/ix-applications/` or `/ix-apps`) is refused as a target, a source or an exclude, because the tool cannot prove that such a dataset is new.

`dry_run: true` returns the payload, the bound snapshot tasks and `plan_sha256`. A write needs `expected_sha256` set to that hash; it is refused when the arguments or the appliance changed after the dry run. The dry run applies the same checks as the write.

`retention_policy` is `SOURCE` (default), `CUSTOM` or `NONE`; `lifetime_value` (1 to 10000) and `lifetime_unit` are required with `CUSTOM`. `readonly` is `SET` (default), `REQUIRE` or `IGNORE`. `exclude` needs `recursive: true`, and each entry must be below a source dataset. Needs the `REPLICATION_TASK_WRITE` role.

## Shares

### `truenas_smb_list`

SMB shares with name, path and enabled state. No parameters.

### `truenas_smb_create`

**Write tool.** Registered only with `--enable-writes`.

Create an SMB share. `guest_ok` allows guest access.

Parameters: `name`, `path` *required*; `comment`, `guest_ok`.

### `truenas_smb_delete`

**Write tool.** Registered only with `--enable-writes`.

Delete an SMB share. The data stays.

Parameters: `id` *required*.

### `truenas_nfs_list`

NFS exports with path, networks and enabled state. No parameters.

### `truenas_nfs_create`

**Write tool.** Registered only with `--enable-writes`.

Create an NFS export.

Parameters: `path` *required*; `networks`, `hosts`.

### `truenas_nfs_delete`

**Write tool.** Registered only with `--enable-writes`.

Delete an NFS export. The data stays.

Parameters: `id` *required*.

## Alerts and jobs

### `truenas_alert_list`

Alerts with level, message, time and dismissed state. `level` is `INFO`, `WARNING` or `CRITICAL`.

Parameters: `level`.

### `truenas_alert_dismiss`

**Write tool.** Registered only with `--enable-writes`.

Dismiss an alert.

Parameters: `id` *required*.

### `truenas_jobs_list`

Recent TrueNAS jobs, newest first. `limit` defaults to 50, at most 200.

Parameters: `state`, `method`, `limit`.

## Apps

### `truenas_app_list`

Every installed app as a compact summary: name, state, version, updates, portals and train. No parameters.

### `truenas_app_get`

1 app in full: containers, images, mounts and ports.

Parameters: `name` *required*.

### `truenas_app_config`

The configuration values of an app. Can return plaintext secrets.

Parameters: `name` *required*.

### `truenas_app_start`

**Write tool.** Registered only with `--enable-writes`.

Start an app.

Parameters: `name` *required*.

### `truenas_app_stop`

**Write tool.** Registered only with `--enable-writes`.

Stop an app.

Parameters: `name` *required*.

### `truenas_app_restart`

**Write tool.** Registered only with `--enable-writes`.

Redeploy the containers of an app. Returns a job ID.

Parameters: `name` *required*.

### `truenas_app_update`

**Write tool.** Registered only with `--enable-writes`.

Upgrade an app to the latest version. Returns a job ID. Refuses an app whose `upgrade_available` is false, also when it has a container image update.

Parameters: `name` *required*.

### `truenas_app_update_all`

**Write tool.** Registered only with `--enable-writes`.

Upgrade every app whose `upgrade_available` is true. Returns 1 job ID for each app. An app with only a container image update is not upgraded. No parameters.

### `truenas_app_configure`

**Write tool.** Registered only with `--enable-writes`.

Change configuration values of a catalog app. `values` is a partial configuration in the shape of `truenas_app_config`; the tool merges it into the current values, so only the named values change. The app's containers restart unless the app is stopped.

Parameters: `name`, `values` *required*; `dry_run`.

### `truenas_app_compose_update`

**Write tool.** Registered only with `--enable-writes`.

Replace the whole compose file of a custom app. Run it with `dry_run: true` first: the write needs the `current_sha256` that the dry run returns. Waits for the job, 600 seconds by default (30 to 1800).

Parameters: `name`, `compose` *required*; `dry_run`, `expected_sha256`, `timeout_seconds`.

### `truenas_app_delete`

**Write tool.** Registered only with `--enable-writes`.

Delete an app. `confirm` must repeat the app name exactly. Returns a job ID.

Parameters: `name`, `confirm` *required*; `remove_ix_volumes`, `dry_run`.

## Destructive tools

These write tools carry `destructiveHint: true`. They can lose data or
settings, or are hard to undo.

| Tool | Why it is destructive |
| --- | --- |
| `truenas_dataset_delete` | Deletes the dataset and the data in it. |
| `truenas_snapshot_delete` | Deletes the snapshot. |
| `truenas_smb_delete` | Removes the share's settings and cuts off clients. The data stays. |
| `truenas_nfs_delete` | Removes the export's settings and cuts off clients. The data stays. |
| `truenas_app_configure` | Overwrites stored configuration values. TrueNAS keeps no earlier copy. |
| `truenas_app_compose_update` | Replaces the stored compose whole. TrueNAS keeps no earlier copy; the dry run's `current_compose` and the write's `previous_compose` hold it. Needs the `APPS_WRITE` role. |
| `truenas_app_delete` | Deletes the app, its containers, docker volumes and images; with `remove_ix_volumes` also its ix-volumes and their snapshots. Host paths stay. |
| `truenas_app_update` | TrueNAS snapshots the host-path datasets, but an app rollback restores only the ix-volumes. See [app updates and recovery](../guides/app-updates.md). |
| `truenas_app_update_all` | The same as `truenas_app_update`, for every app with an app update at once. |
| `truenas_snapshot_task_create` | Retention selects snapshots by dataset and naming schema, not by the task that made them, so the new task can delete older snapshots whose names match. |
| `truenas_snapshot_task_run` | A run does what a scheduled run does: retention can delete expired snapshots, and linked replication tasks start. The dry run lists those replication tasks. The snapshot of a run off the task's schedule is never pruned by the task's own retention; only another task whose schedule matches that time can prune it. Needs the `SNAPSHOT_TASK_WRITE` role. |
| `truenas_replication_task_create` | Every run of the new task writes into the target dataset, and retention can delete snapshots stored there: policy `SOURCE` deletes the target snapshots that the source does not have, and `CUSTOM` deletes them by age. Needs the `REPLICATION_TASK_WRITE` role. |

`idempotentHint: true` marks the write tools where a repeat of the same call
has no further effect: the 4 deletes of datasets, snapshots and shares,
`truenas_dataset_create`, `truenas_smb_create`, `truenas_alert_dismiss`,
`truenas_app_stop`, `truenas_app_delete`, `truenas_app_update` and
`truenas_app_update_all`.

## Tools that can return secrets

- `truenas_app_config` returns an app's configuration values, which can hold
  database passwords, tokens and API keys.
- `truenas_app_compose_update` returns the compose file, which can hold
  environment values in plain text.

Call them only when the configuration itself is the question.
