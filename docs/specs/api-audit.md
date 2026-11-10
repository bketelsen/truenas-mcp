# Spec: Middleware method audit — TrueNAS 27.0

This spec records, for every middleware method the server calls, whether the
audited TrueNAS release serves it and whether the payload the tool builds is
accepted. `server/tools_*.go` and `truenas/client.go` implement it; re-run the
audit after a TrueNAS major upgrade and after a tool changes the method or the
payload it sends.

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
| `pool.query` | `truenas_pool_list`, `truenas_pool_get`, `truenas_health_report`, `truenas_scrub_task_list` | present | none, or filters `name =`; the scrub task list uses none | OK (live) |
| `pool.scrub.query` | `truenas_scrub_task_list` | present | none | OK (middleware source TS-27.0.0-RC.1 `api/v27_0_0/pool_scrub.py` `PoolScrubEntry`, role `POOL_SCRUB_READ`; not called live) |
| `disk.query` | `truenas_disk_list`, `truenas_health_report` | present | `options.extra.pools=true` | OK (live, PR #42) |
| `interface.query` | `truenas_network_list` | present | none | OK (live) |
| `filesystem.stat` | `truenas_filesystem_stat` | present | cleaned absolute path below `/mnt` | OK (live, 2026-11-06, READONLY_ADMIN) |
| `filesystem.getacl` | `truenas_filesystem_stat` | present | cleaned `realpath`, `true` (simplified) | OK (live, 2026-11-06, READONLY_ADMIN) |
| `pool.dataset.query` | `truenas_dataset_list`, `truenas_dataset_get`, `truenas_replication_task_create` (existence of the sources, the target and the target's parent) | present | filters `pool =` / `id =`; the replication task create tool uses filter `id in` | OK (live) |
| `pool.dataset.create` | `truenas_dataset_create` | present | `name`, `comments`, `compression` | **Gap 3** (fixed): `compression` is an upper-case enum |
| `pool.dataset.delete` | `truenas_dataset_delete` | present | `id` string | OK (schema) |
| `pool.snapshot.query` | `truenas_snapshot_list`, `truenas_snapshot_get`, `truenas_snapshot_task_run` (verify) | present | filters, `options.extra.properties` | OK (live, PRs #41, #42) |
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
| `core.get_jobs` | `truenas_jobs_list`, `truenas_app_compose_update` and `truenas_snapshot_task_run` (poll loop `waitForJob`) | present | filters, `order_by`, `limit`; the poll uses the filter `id =` | OK (live) |
| `pool.snapshottask.query` | `truenas_snapshot_task_list`, `truenas_snapshot_task_create`, `truenas_snapshot_task_run`, `truenas_replication_task_create` (every task, on a dry run too) | present | none, or filter `dataset =`; the run tool uses filter `id =` (before the run and in its state poll); the replication task create tool uses no parameters | OK (schema + middleware source, not called live) |
| `pool.snapshottask.create` | `truenas_snapshot_task_create` | present | 1 object: `dataset`, `recursive`, `exclude`, `lifetime_value`, `lifetime_unit` (upper-case enum), `naming_schema`, `schedule` (minute, hour, dom, month, dow, begin, end), `allow_empty`, `enabled` | OK (schema + middleware source, not called live) |
| `pool.snapshottask.run` | `truenas_snapshot_task_run` | present (not a job in 25.10/27.0: returns null; a job in master: returns the job id) | `id` integer | OK (checked against middleware source TS-25.10.8, TS-27.0.0-RC.1 and master, not called live (it writes)); role `SNAPSHOT_TASK_WRITE` |
| `replication.query` | `truenas_replication_task_list`, `truenas_snapshot_task_run` (linked replication tasks for the dry run and the result), `truenas_replication_task_create` (every task, on a dry run too) | present | none | OK (schema + middleware source, not called live) |
| `app.delete` | `truenas_app_delete` | present (job) | `app_name`, `{remove_images, remove_ix_volumes, force_remove_custom_app}` | OK (schema + middleware source, not called live) |
| `replication.create` | `truenas_replication_task_create` | present | 1 object: `name`, `direction` (`PUSH`), `transport` (`LOCAL`), `source_datasets`, `target_dataset`, `recursive`, `exclude`, `periodic_snapshot_tasks` (integers), `auto`, `retention_policy`, `lifetime_value` and `lifetime_unit` (only with `CUSTOM`), `readonly`, `enabled`, `allow_from_scratch`, `replicate`, `encryption` | OK (checked against the middleware source TS-27.0.0-RC.1 `api/v27_0_0/replication.py` `ReplicationCreate` and `plugins/replication.py`, and the `v25_10_0` model, not called live because it writes); role `REPLICATION_TASK_WRITE` |
| `pool.dataset.query` (options) | `truenas_protection_report`, `truenas_replication_task_create` (lookup of the datasets the plan names) | present | `truenas_protection_report`: filters `[]` or `pool =`; options `select` [`id`,`name`,`type`,`pool`,`usedbydataset`], `extra` {`flat`: true, `properties`: [`usedbydataset`], `retrieve_user_props`: false}; `truenas_replication_task_create`: filter `id in`, options `select` [`id`,`name`], `extra` {`flat`: true, `retrieve_children`: false, `properties`: [], `retrieve_user_props`: false} | OK (live, read-only build, 2026-11-02; the `id in` lookup was checked against the middleware source, not called live) |
| `docker.config` | `truenas_protection_report` (apps pool for the limits text; an error is ignored) | present | none | OK (live, read-only build, 2026-11-02) |

The `pool.scrub.query` row was checked against middleware source TS-27.0.0-RC.1 (`api/v27_0_0/pool_scrub.py`, `plugins/pool_/scrub.py`, `plugins/zpool/scrub_impl.py`). The `pool.query` `scan` shape is `PoolScan`; its `{"$date": <unix millis>}` times were seen in live `pool.query` output. `pool.scrub.query` has not been called live; check it live before a release. The [tool contract](tools.md#truenas_scrub_task_list) records the TrueNAS 27.0 threshold rule.

The 2 `filesystem.*` rows were added on 2026-11-06 for `truenas_filesystem_stat`, called live, read-only, on 27.0.0-RC.1 with a `READONLY_ADMIN` key, on `/mnt`. `filesystem.stat` accepts `..` paths (`/mnt/../etc` returned data), so `truenas_filesystem_stat` validates the path in the Server before it calls middleware, and refuses a `realpath` outside `/mnt`. `filesystem.stat` reports only `acl` (a boolean), not the ACL type, so the tool reads `acltype` and `trivial` from `filesystem.getacl`.

The 4 rows from `pool.snapshottask.query` to `app.delete` (`pool.snapshottask.query`, `pool.snapshottask.create`, `replication.query`, `app.delete`) were added after the audit. They were checked against the `v25_10_0` and `v27_0_0` API models in truenas/middleware (`PeriodicSnapshotTaskEntry`, `PeriodicSnapshotTaskCreate`, `ReplicationEntry`, `AppDelete`), not against a live NAS. `pool.snapshottask.create` was never called, because it writes.

The last 2 rows were added for `truenas_protection_report` (2026-11-02). They were called live, read-only, on 27.0.0-RC.1 by a build of the tool without `--enable-writes`, together with `pool.snapshottask.query`, `replication.query` and `pool.snapshot.query` (no filter, `extra.properties` [`creation`]): 114 datasets, about 33,000 characters, in about 1 second. The dataset options follow `pool.dataset.query` in `plugins/pool_/dataset.py` (master); the tool walks nested `children` too, so a release that ignores `flat` or `select` still works.

The `replication.create` row, and the `id in` dataset lookup of `truenas_replication_task_create`, were added after the audit, for the local replication task create tool. They were checked against the middleware source TS-27.0.0-RC.1 (`api/v27_0_0/replication.py` `ReplicationCreate`, `plugins/replication.py` for the role and its `_validate_source_datasets`) and the `v25_10_0` model, not against a live NAS: `replication.create` writes, so it was never called.

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

## References

- Rationale: [ADR-0006 — Use the official TrueNAS WebSocket client](../adr/0006-use-the-official-truenas-websocket-client.md)
- Context: [design/overview.md](../design/overview.md) (Data Flow, Connection
  Details)
- Tool contract: [tools.md](tools.md) (which tool calls which method)
