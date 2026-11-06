# App updates and recovery

`truenas_app_update` and `truenas_app_update_all` upgrade apps. Both are write
tools and both are destructive, because TrueNAS can roll back only part of what
an upgrade changes. This guide says what TrueNAS saves before an upgrade and
how to undo a bad one.

## What TrueNAS snapshots before an upgrade

The tools call TrueNAS `app.upgrade` with `snapshot_hostpaths: true`. Before
the new version runs, TrueNAS (25.10) stops the app and takes these
snapshots:

- **Host paths.** TrueNAS maps every bind-mounted path outside
  `/mnt/.ix-apps` to the ZFS dataset it lives on, and snapshots that whole
  dataset, not recursively, as
  `<dataset>@ix-app-upgrade-<app>-<previous version>`. A host path that is a
  subdirectory snapshots the dataset that contains it. A path that is not on
  a ZFS pool is skipped, and the upgrade goes on without it.
- **ix-volumes.** TrueNAS snapshots the app's ix-volumes dataset recursively
  as `@<previous version>`. An app rollback restores this snapshot.

Custom (compose) apps are the exception. Their update only pulls new images,
and TrueNAS takes no snapshot for them.

## Step 1: check for updates

`truenas_apps_update_report` lists the apps with an app update or a container
image update. It is read-only.

## Step 2: upgrade

`truenas_app_update` upgrades 1 app to the latest version and returns the job
ID. `truenas_app_update_all` upgrades every app with an update and returns 1
job ID for each. Follow the jobs with `truenas_jobs_list` until each one is
`SUCCESS` or `FAILED`.

## Step 3: undo a bad upgrade

1. Roll the app back in the TrueNAS web interface. TrueNAS rolls back only a
   running app. The rollback restores the previous version and its
   ix-volumes.
2. Stop the app with `truenas_app_stop`.
3. Roll each host-path dataset back to its `ix-app-upgrade-…` snapshot, from
   the dataset's **Manage Snapshots** list in the TrueNAS web interface.
4. Start the app again with `truenas_app_start`.

!!! warning "A dataset rollback reverts everything in the dataset"

    The rollback also reverts files that other apps keep in the same dataset.
    ZFS must destroy every newer snapshot of the dataset to do it, for example
    the snapshots of a periodic snapshot task.

## Step 4: remove old upgrade snapshots

TrueNAS never deletes the `ix-app-upgrade-…` snapshots. They take little space
at first, but they hold on to data that later changes or is deleted. When you
trust the new version, remove them with `truenas_snapshot_delete`.
