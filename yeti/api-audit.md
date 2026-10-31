# Middleware method audit — TrueNAS 27.0

Audit date: 2026-10-22. Target: live NAS `TrueNAS-27.0.0-RC.1` (837 public methods).

## How the audit was done

- **Live, read-only.** A throwaway client (`truenas.Connect`, the repo's own client) called only
  `system.version`, `core.get_methods`, and read-only query methods with the exact parameters the
  tools send. No write method was called; nothing on the NAS changed.
- **Presence:** every method name the tools call was looked up in `core.get_methods`.
- **Parameters:** each method's `accepts` JSON schema was compared with the payload the tool builds.
  Write methods were checked by schema only (never called). Read methods were also called live.
- **Older releases:** where a fix changes a payload, middleware source for `v25_10_5` and `v27_0_0`
  (`src/middlewared/middlewared/api/`) was read to keep older releases working.

Re-run the check after a TrueNAS major upgrade: list every `client.Call("...")` and
`callSnapshot(client, "...")` in `server/`, plus `core.ping` in `truenas/client.go`, and look each
one up in `core.get_methods` (keys are method names; each value has `accepts`, `returns`, `job`).

## Method table

| Method | Tool(s) | 27.0 | Parameters sent | Result |
|---|---|---|---|---|
| `core.ping` | client probe before every call | present | none | OK |
| `system.info` | `truenas_system_info`, `truenas_health_report` | present | none | OK (live) |
| `system.state` | `truenas_health_report` | present | none | OK (live) |
| `pool.query` | `truenas_pool_list`, `truenas_pool_get`, `truenas_health_report` | present | filters `name =` | OK (live) |
| `disk.query` | `truenas_disk_list`, `truenas_health_report` | present | `options.extra.pools=true` | OK (live, PR #42) |
| `interface.query` | `truenas_network_list` | present | none | OK (live) |
| `pool.dataset.query` | `truenas_dataset_list`, `truenas_dataset_get` | present | filters `pool =` / `id =` | OK (live) |
| `pool.dataset.create` | `truenas_dataset_create` | present | `name`, `comments`, `compression` | **Gap 3** (fixed): `compression` is an upper-case enum |
| `pool.dataset.delete` | `truenas_dataset_delete` | present | `id` string | OK (schema) |
| `pool.snapshot.query` | `truenas_snapshot_list`, `truenas_snapshot_get` | present | filters, `options.extra.properties` | OK (live, PRs #41, #42) |
| `pool.snapshot.create` | `truenas_snapshot_create` | present | `dataset`, `name` | OK (schema) |
| `pool.snapshot.delete` | `truenas_snapshot_delete` | present | `id` string | OK (schema) |
| `zfs.snapshot.query/create/delete` | legacy fallback in `callSnapshot` | **removed** | same as above | Handled by PR #41 (pre-25.10 fallback only) |
| `sharing.smb.query` | `truenas_smb_list` | present | none | OK (live) |
| `sharing.smb.create` | `truenas_smb_create` | present | `name`, `path`, `comment`, `guest_ok` | **Gap 2** (fixed): top-level `guestok` rejected |
| `sharing.smb.delete` | `truenas_smb_delete` | present | `id` integer | OK (schema) |
| `sharing.nfs.query` | `truenas_nfs_list` | present | none | OK (live) |
| `sharing.nfs.create` | `truenas_nfs_create` | present | `path`, `hosts`, `networks` | OK (schema) |
| `sharing.nfs.delete` | `truenas_nfs_delete` | present | `id` integer | OK (schema) |
| `alert.list` | `truenas_alert_list`, `truenas_health_report` | present | none, or a `level` filter | **Gap 1** (fixed): accepts no arguments |
| `alert.dismiss` | `truenas_alert_dismiss` | present | `uuid` string | OK (schema) |
| `app.query` | `truenas_app_list`, `truenas_app_get`, `truenas_apps_update_report`, `truenas_app_update`, `truenas_app_update_all`, `truenas_app_configure`, `truenas_app_compose_update` | present | filters `name =` / `upgrade_available =` | OK (live); returned fields used by the report all present |
| `app.config` | `truenas_app_config`, `truenas_app_configure`, `truenas_app_compose_update` | present | `app_name` | OK (schema) |
| `app.start` | `truenas_app_start` | present (job) | `app_name` | OK (schema) |
| `app.stop` | `truenas_app_stop` | present (job) | `app_name` | OK (schema) |
| `app.redeploy` | `truenas_app_restart` | present (job) | `app_name` | OK (schema) |
| `app.update` | `truenas_app_configure` (with `{values}`), `truenas_app_compose_update` (with `{custom_compose_config}`) | present (job) | `app_name`, `{values}` or `{custom_compose_config}` | OK (schema) |
| `app.upgrade` | `truenas_app_update`, `truenas_app_update_all` | present (job) | `app_version`, `values`, `snapshot_hostpaths` | OK (schema) |
| `core.get_jobs` | `truenas_jobs_list`, `truenas_app_compose_update` (poll loop) | present | filters, `order_by`, `limit`; the poll uses the filter `id =` | OK (live) |
| `pool.snapshottask.query` | `truenas_snapshot_task_list`, `truenas_snapshot_task_create` | present | none, or filter `dataset =` | OK (schema + middleware source, not called live) |
| `pool.snapshottask.create` | `truenas_snapshot_task_create` | present | 1 object: `dataset`, `recursive`, `exclude`, `lifetime_value`, `lifetime_unit` (upper-case enum), `naming_schema`, `schedule` (minute, hour, dom, month, dow, begin, end), `allow_empty`, `enabled` | OK (schema + middleware source, not called live) |
| `replication.query` | `truenas_replication_task_list` | present | none | OK (schema + middleware source, not called live) |
| `app.delete` | `truenas_app_delete` | present (job) | `app_name`, `{remove_images, remove_ix_volumes, force_remove_custom_app}` | OK (schema + middleware source, not called live) |

The last 4 rows (`pool.snapshottask.query`, `pool.snapshottask.create`, `replication.query`, `app.delete`) were added after the audit. They were checked against the `v25_10_0` and `v27_0_0` API models in truenas/middleware (`PeriodicSnapshotTaskEntry`, `PeriodicSnapshotTaskCreate`, `ReplicationEntry`, `AppDelete`), not against a live NAS. `pool.snapshottask.create` was never called, because it writes.

## Gaps and fixes

### Gap 1 — `alert.list` level filter

- `alert.list` accepts **no** arguments (`AlertListArgs` is empty in `v25_10_5` and `v27_0_0`; it is
  not a filterable query method). Live on 27.0.0-RC.1, `alert.list([["level","=","CRITICAL"]])`
  fails with `[EINVAL] : Too many arguments (expected 0, found 1)`.
- So `truenas_alert_list` with `level` failed on every current release; without `level` it worked.
- Fix: always call `alert.list` with no arguments and filter by `level` in the tool,
  case-insensitively. Works on all releases; no fallback call is needed.
- Live check (read-only build of the fix, no `--enable-writes`, 27.0.0-RC.1): no `level` gave 15
  alerts; `level: info` gave 14 (`INFO`), `level: NOTICE` gave 1, `warning` and `CRITICAL` gave 0.

### Gap 2 — `sharing.smb.create` with `guest_ok`

- Since 25.10 the share schema has `purpose` plus a per-purpose `options` object, with
  `additionalProperties: false` at the top level. `guestok` exists only in the `LEGACY_SHARE`
  options (`LegacyOpt`). A top-level `guestok` is rejected on 25.10 and 27.0.
- Fix: with `guest_ok`, send `purpose: "LEGACY_SHARE"` and `options: {"guestok": true}`. If the
  server rejects that payload in validation (an `EINVAL` error that names `purpose` or `options`,
  as releases before 25.10 do), retry once with the legacy top-level `guestok`. A validation error
  means the create never ran, so the retry cannot make a duplicate share.
- Without `guest_ok` the payload (`name`, `path`, `comment`) is unchanged and valid on all releases;
  27.0 gives it the `DEFAULT_SHARE` purpose.

### Gap 3 — `pool.dataset.create` compression case

- `compression` is an enum of upper-case values (`LZ4`, `ZSTD`, `OFF`, `INHERIT`, ...) in both
  `v25_10_5` and `v27_0_0`. The tool documented `lz4, zstd, off`, which the server rejects.
- Fix: upper-case the value before sending. Works on all releases.
- `type` is not sent; 27.0 defaults it to `FILESYSTEM` (`set_default_type` validator), so no change.
