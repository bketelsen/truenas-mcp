# truenas-mcp

An MCP server that exposes TrueNAS SCALE management capabilities to AI assistants. It bridges the MCP protocol (spoken to AI clients over stdio) and the TrueNAS WebSocket JSON-RPC API (spoken to a NAS appliance).

## Language

### Infrastructure

**Server**:
The MCP server process — the binary that speaks MCP to AI clients. Not the NAS hardware.
_Avoid_: host, daemon

**Appliance**:
The TrueNAS SCALE hardware or VM being managed. Identified by hostname or IP.
_Avoid_: server, host, NAS, box

**Tool**:
An MCP-protocol callable exposed by the server. All tool names are prefixed `truenas_`. Tools are either read-only or mutating; mutating tools are only registered when the server is started with `--enable-writes`.
_Avoid_: command, action, endpoint

**Report**:
A server-side aggregation tool that synthesizes data from multiple appliance queries into a single response. Unlike raw query tools, the server — not the appliance — does the assembly.
_Avoid_: summary, dashboard

### Storage

**Pool**:
A ZFS storage pool on the appliance — a collection of disks presenting a unified volume. TrueNAS term, rooted in ZFS.
_Avoid_: volume, disk group

**Dataset**:
A ZFS filesystem or volume created within a Pool. Identified by its full path (e.g. `tank/media/movies`), which encodes the pool hierarchy.
_Avoid_: share, folder, directory

**Snapshot**:
A point-in-time, read-only copy of a Dataset. Identified by dataset path and snapshot name.
_Avoid_: backup, clone

### Sharing

**Share**:
The umbrella concept for any network-exposed filesystem path on the appliance. Has two concrete subtypes: SMB Share and NFS Export.

**SMB Share**:
A Dataset mountpoint exposed over the SMB protocol. Has a name, path, and optional guest access flag.
_Avoid_: Windows share, CIFS share

**NFS Export**:
A Dataset mountpoint exposed over the NFS protocol. Identified by path and configured network access rules.
_Avoid_: NFS share, NFS mount

### Apps

**App**:
A containerized workload installed and running on the appliance. Scoped to installed apps only — catalog entries that are not installed are out of scope.
_Avoid_: container, service, pod

### Operations

**Alert**:
A TrueNAS system notification with a level (INFO, WARNING, CRITICAL) and a dismissed state. Covers all notifications regardless of dismissed state. "Active alert" is informal shorthand for an undismissed one.
_Avoid_: warning, notification, event

**Job**:
A TrueNAS-tracked asynchronous operation with a numeric ID and a lifecycle state (WAITING, RUNNING, SUCCESS, FAILED, ABORTED). Long-running operations (e.g. app updates) return a Job ID immediately. The AI client is responsible for polling status via `truenas_jobs_list` — the server does not wait or track job completion.
_Avoid_: task, operation, process

## Example dialogue

> **Dev:** I want to update all apps that have pending updates.
>
> **Domain expert:** Call `truenas_app_update_all`. It returns a list of Job IDs — one per App being updated. Then poll recent jobs with `truenas_jobs_list` and match those IDs client-side until each Job reaches SUCCESS or FAILED.
>
> **Dev:** What if I only want to update one App?
>
> **Domain expert:** Use `truenas_app_update` with the App name. Same pattern — you get back a single Job ID and poll from there.
>
> **Dev:** Can I check which Apps have updates before running it?
>
> **Domain expert:** Yes — `truenas_apps_update_report` is a Report that lists Apps with available updates. Read-only, no side effects, no Job IDs returned.
