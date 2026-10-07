# truenas-mcp

MCP server that exposes TrueNAS SCALE management capabilities to AI assistants like Claude.

Connects to TrueNAS SCALE (Goldeye+) via the WebSocket JSON-RPC API and exposes storage, sharing, system, and app management as MCP tools over stdio.

## Requirements

- Go 1.26+
- TrueNAS SCALE Goldeye (25.10) or later
- A TrueNAS API key (create in TrueNAS UI → Settings → API Keys)

## Install

Prebuilt binaries for Linux, macOS, and Windows are attached to every
[GitHub release](https://github.com/bketelsen/truenas-mcp/releases), along with
`.deb`, `.rpm`, and `.apk` packages that also install shell completions and a
man page. A `dev` pre-release tracks the latest `main` commit.

Every release asset is signed with GitHub build provenance. Verify a download with:

```bash
gh attestation verify truenas-mcp_*_linux_amd64.tar.gz --repo bketelsen/truenas-mcp
```

Or install from source (the module path is `truenas-mcp`, so `go install <url>@latest` does not apply):

```bash
git clone https://github.com/bketelsen/truenas-mcp.git
cd truenas-mcp
make install   # -> $GOPATH/bin/truenas-mcp
```

## Build

```bash
make            # fmt + vet + build -> build/truenas-mcp
make help       # list all targets
```

`make build` stamps the binary with the git tag, commit, and build time; `truenas-mcp --version` prints them.

## Usage

```bash
# safe default: read-only tools only
truenas-mcp serve --host truenas.local --api-key YOUR_API_KEY

# with environment variables
export TRUENAS_HOST=truenas.local
export TRUENAS_API_KEY=YOUR_API_KEY
truenas-mcp serve

# opt into mutating tools only when you intentionally want writes
truenas-mcp serve --host truenas.local --api-key YOUR_API_KEY --enable-writes

# opt into mutating tools via environment variable
TRUENAS_ENABLE_WRITES=true truenas-mcp serve

# allow self-signed certificates only when explicitly needed
truenas-mcp serve --host truenas.local --api-key YOUR_API_KEY --tls-insecure
```

### Read-Only by Default

The MCP server starts in read-only mode unless you explicitly pass `--enable-writes` or set `TRUENAS_ENABLE_WRITES=true`. In the default mode, tools that create, delete, or modify resources are not registered — AI clients cannot see or invoke them.

This fail-closed default is intended to make first contact with a TrueNAS system safe. Keep the server read-only until you have tested the tool responses against your NAS.

TLS certificate verification is enabled by default. If your TrueNAS appliance uses a self-signed certificate, pass `--tls-insecure` or set `TRUENAS_TLS_INSECURE=true` after you understand the tradeoff.

The client checks the appliance connection before each operation and reconnects and authenticates again if that check fails. Operations are serialized. If an operation itself fails at the transport layer, it is reported without automatic replay, since a write may already have taken effect; the next request opens a fresh connection.

For a step-by-step safe first connection checklist, see [First Contact with TrueNAS](docs/first-contact.md).

You can also use the guided runbook script:

```bash
export TRUENAS_HOST=truenas.local
read -rs TRUENAS_API_KEY
export TRUENAS_API_KEY
scripts/first-contact.sh
```

## MCP Configuration

Add to your Claude Code MCP settings (`~/.claude/settings.json`):

```json
{
  "mcpServers": {
    "truenas": {
      "command": "/path/to/truenas-mcp",
      "args": ["serve", "--host", "truenas.local", "--api-key", "YOUR_API_KEY"]
    }
  }
}
```

For write-enabled mode:

```json
{
  "mcpServers": {
    "truenas": {
      "command": "/path/to/truenas-mcp",
      "args": ["serve", "--host", "truenas.local", "--api-key", "YOUR_API_KEY", "--enable-writes"]
    }
  }
}
```

## Available Tools

Tools marked with `*` are excluded by default and are registered only with `--enable-writes` or `TRUENAS_ENABLE_WRITES=true`.

| Tool | Description |
|------|-------------|
| `truenas_health_report` | Aggregated system, pool, disk, and alert health report |
| `truenas_system_info` | System hostname, version, uptime, platform |
| `truenas_disk_list` | Physical disks with health status |
| `truenas_network_list` | Network interfaces and IPs |
| `truenas_pool_list` | ZFS pools with status and health |
| `truenas_pool_get` | Detailed pool info including topology |
| `truenas_dataset_list` | Datasets with usage and compression |
| `truenas_dataset_get` | Full dataset properties |
| `truenas_dataset_create` | Create a new dataset `*` |
| `truenas_dataset_delete` | Delete a dataset `*` |
| `truenas_snapshot_list` | Snapshots for a dataset |
| `truenas_snapshot_get` | Snapshot details |
| `truenas_snapshot_create` | Create a snapshot `*` |
| `truenas_snapshot_delete` | Delete a snapshot `*` |
| `truenas_smb_list` | SMB shares |
| `truenas_smb_create` | Create an SMB share `*` |
| `truenas_smb_delete` | Delete an SMB share `*` |
| `truenas_nfs_list` | NFS exports |
| `truenas_nfs_create` | Create an NFS export `*` |
| `truenas_nfs_delete` | Delete an NFS export `*` |
| `truenas_alert_list` | Active alerts (filterable by level) |
| `truenas_alert_dismiss` | Dismiss an alert `*` |
| `truenas_app_list` | Installed apps with status |
| `truenas_app_get` | App details: running containers, images, mounts, ports |
| `truenas_app_config` | Installed app configuration values (may contain plaintext secrets) |
| `truenas_apps_update_report` | Apps with app or container image updates available |
| `truenas_app_start` | Start an app `*` |
| `truenas_app_stop` | Stop an app `*` |
| `truenas_app_restart` | Restart an app (redeploys its containers, returns a job ID) `*` |
| `truenas_app_update` | Upgrade an app to the latest available version `*` |
| `truenas_app_update_all` | Upgrade all apps with updates available `*` |
| `truenas_app_configure` | Change an app's configuration values (deep-merged, redeploys the app) `*` |
| `truenas_jobs_list` | Recent TrueNAS jobs, optionally filtered by state or method |

## Tool Annotations

Every tool carries [MCP tool annotations](https://modelcontextprotocol.io/specification/2025-06-18/schema#toolannotations): a `title` plus `readOnlyHint`, `destructiveHint`, `idempotentHint`, and `openWorldHint`. Clients can use them to decide how much confirmation a call needs. `openWorldHint` is `false` on every tool, because the server only talks to the one appliance it is configured for.

**Read-only** (`readOnlyHint: true`): every tool registered without `--enable-writes`. They query the appliance and change nothing. `truenas_app_config` is read-only but can return plaintext secrets.

**Destructive** (`destructiveHint: true`): these can lose data or settings, or are hard to undo.

| Tool | Why it is destructive |
|------|-----------------------|
| `truenas_dataset_delete` | Deletes the dataset and the data in it |
| `truenas_snapshot_delete` | Deletes the snapshot |
| `truenas_smb_delete` | Removes the share's settings and cuts off clients (the data stays) |
| `truenas_nfs_delete` | Removes the export's settings and cuts off clients (the data stays) |
| `truenas_app_configure` | Overwrites stored configuration values; TrueNAS keeps no prior copy |
| `truenas_app_update` | The upgrade snapshots the app's host-path datasets as well as its ix-volumes, but TrueNAS's app rollback restores only the ix-volumes. Undoing changes on host paths means rolling a dataset back by hand, which also reverts anything else stored in it (see [App updates and recovery](#app-updates-and-recovery)) |
| `truenas_app_update_all` | The same as `truenas_app_update`, for every app with an update at once |

**Non-destructive writes** (`destructiveHint: false`): these are additive or recoverable. They are `truenas_dataset_create`, `truenas_snapshot_create`, `truenas_smb_create`, `truenas_nfs_create`, `truenas_alert_dismiss`, `truenas_app_start`, `truenas_app_stop`, and `truenas_app_restart`. Starting or restarting an app still interrupts it briefly, and `truenas_smb_create` with `guest_ok` widens access.

`idempotentHint: true` marks the write tools where repeating a call with the same arguments has no further effect: the four deletes, `truenas_dataset_create`, `truenas_smb_create`, `truenas_alert_dismiss`, `truenas_app_stop`, `truenas_app_update`, and `truenas_app_update_all`.

### App updates and recovery

`truenas_app_update` and `truenas_app_update_all` call TrueNAS `app.upgrade` with `snapshot_hostpaths: true`. Before the new version runs, TrueNAS (25.10) stops the app and snapshots:

- **Host paths**: every bind-mounted path outside `/mnt/.ix-apps` is mapped to the ZFS dataset it lives on, and that whole dataset is snapshotted (not recursively) as `<dataset>@ix-app-upgrade-<app>-<previous version>`. A host path that is a subdirectory snapshots the dataset that contains it. Paths that are not on a ZFS pool are skipped, and the upgrade goes ahead without them.
- **ix-volumes**: the app's ix-volumes dataset is snapshotted recursively as `@<previous version>`. This is the snapshot an app rollback restores.

Custom (compose) apps are the exception: their update only pulls new images, and TrueNAS takes no snapshots for them.

To undo a bad upgrade, first roll the app back in the TrueNAS UI (TrueNAS only rolls back a running app); that restores the previous version and its ix-volumes. Then stop the app (`truenas_app_stop`), roll each host-path dataset back to its `ix-app-upgrade-…` snapshot from the dataset's **Manage Snapshots** list, and start the app again. That rollback reverts everything in the dataset, including files other apps keep there, and ZFS has to destroy any newer snapshots of the dataset (for example from a periodic snapshot task) to do it. TrueNAS never deletes these snapshots itself. They take little space at first but hold on to data that is later changed or deleted, so remove old ones with `truenas_snapshot_delete` once you trust the new version.

### Using the hints with `--enable-writes`

`--enable-writes` decides whether write tools exist at all. The annotations help a client decide what to do with the ones that do. A reasonable client policy is:

- run read-only tools without asking;
- always ask a person before a destructive tool;
- apply the client's normal approval policy to the other write tools.

The hints are advisory: the MCP specification tells clients to treat annotations as untrusted unless they come from a trusted server. Read-only mode is the safety boundary: keep the server read-only unless you need writes, and use the hints to add confirmation on top of `--enable-writes`, not instead of it.

## License

MIT. See [LICENSE](LICENSE).
