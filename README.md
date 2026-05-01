# truenas-mcp

MCP server that exposes TrueNAS SCALE management capabilities to AI assistants like Claude.

Connects to TrueNAS SCALE (Goldeye+) via the WebSocket JSON-RPC API and exposes storage, sharing, system, and app management as MCP tools over stdio.

## Requirements

- Go 1.26+
- TrueNAS SCALE Goldeye (25.10) or later
- A TrueNAS API key (create in TrueNAS UI → Settings → API Keys)

## Build

```bash
make
```

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
| `truenas_app_get` | App details |
| `truenas_app_start` | Start an app `*` |
| `truenas_app_stop` | Stop an app `*` |
| `truenas_app_restart` | Restart an app `*` |
