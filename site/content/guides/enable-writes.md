# Enable write tools

This guide turns on the write tools, and sets up the client so that it asks
before a destructive call. Do it only after a read-only
[first connection](../getting-started/first-connection.md) gave the results you
expect.

## Step 1: check the roles of the API key

TrueNAS checks each call against the roles of the user that owns the API key.
A write tool fails when the roles do not allow its call. Give the user only the
roles for the writes you plan. 3 tools name their role:

| Tool | Role |
| --- | --- |
| `truenas_snapshot_task_run` | `SNAPSHOT_TASK_WRITE` |
| `truenas_replication_task_create` | `REPLICATION_TASK_WRITE` |
| `truenas_app_compose_update` | `APPS_WRITE` |

## Step 2: start the server in write mode

=== "Flag"

    ```sh
    truenas-mcp serve --enable-writes
    ```

=== "Environment"

    ```sh
    TRUENAS_ENABLE_WRITES=true truenas-mcp serve
    ```

=== "Client entry"

    Add the flag to the `args` of the client entry from
    [configuration](../reference/configuration.md#connect-an-mcp-client):

    ```json
    "args": ["serve", "--enable-writes"]
    ```

The server confirms the mode on stderr:

```text
Starting MCP server on stdio (writes enabled)...
```

The client now lists 42 tools: the 22 read-only tools and the 20 write tools.

## Step 3: set a client policy from the annotations

Every tool carries [MCP tool annotations](https://modelcontextprotocol.io/specification/2025-06-18/schema#toolannotations):
a `title`, `readOnlyHint`, `destructiveHint`, `idempotentHint` and
`openWorldHint`. `openWorldHint` is `false` on every tool, because the server
talks only to the 1 appliance it is configured for.

| Class | Tools |
| --- | --- |
| Read-only (`readOnlyHint: true`) | Every tool that the read-only mode registers. |
| Destructive (`destructiveHint: true`) | 12 write tools that can lose data or settings, or are hard to undo. The [tool reference](../reference/tools.md#destructive-tools) lists each one and why. |
| Non-destructive write (`destructiveHint: false`) | `truenas_dataset_create`, `truenas_snapshot_create`, `truenas_smb_create`, `truenas_nfs_create`, `truenas_alert_dismiss`, `truenas_app_start`, `truenas_app_stop` and `truenas_app_restart`. |

A reasonable client policy is:

- run read-only tools without asking;
- always ask a person before a destructive tool;
- apply the client's normal approval policy to the other write tools.

Starting or restarting an app still interrupts it for a short time, and
`truenas_smb_create` with `guest_ok` widens access.

!!! warning "The hints are advisory"

    The MCP specification tells clients to treat annotations as untrusted
    unless they come from a trusted server. The read-only mode is the limit
    the server enforces. Use the hints to add confirmation on top of
    `--enable-writes`, not instead of it.

## Step 4: use the dry runs

Several write tools take a `dry_run` parameter, or run dry unless you confirm.
A dry run calls no write method on the appliance and shows what the write
would do:

| Tool | Dry run |
| --- | --- |
| `truenas_snapshot_task_create` | `dry_run: true` returns the payload. |
| `truenas_snapshot_task_run` | Dry run by default; `confirm: true` runs the task. |
| `truenas_replication_task_create` | `dry_run: true` returns the payload, the bound snapshot tasks and the plan hash that the write needs. |
| `truenas_app_configure` | `dry_run: true` returns the merged configuration and the changed paths. |
| `truenas_app_compose_update` | `dry_run: true` returns the current compose, the diff and the hash that the write needs. |
| `truenas_app_delete` | `dry_run: true` describes the delete. |

## Undo it

Restart the server without `--enable-writes`, and unset
`TRUENAS_ENABLE_WRITES`. The write tools disappear from the client. A write
that already ran stays; [app updates and recovery](app-updates.md) explains how
to undo an app upgrade.
