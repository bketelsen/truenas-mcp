# Read-only by default

truenas-mcp has 2 modes. The default mode registers only the read-only tools.
The write mode also registers the tools that create, change or delete things
on the appliance. The decision and its reasons are in
[ADR-0004 — Read-only by default](https://github.com/bketelsen/truenas-mcp/blob/main/docs/adr/0004-read-only-by-default.md).

## The rule

- The server always registers the 22 read-only tools.
- It registers the 20 write tools only when you start it with
  `--enable-writes` or with `TRUENAS_ENABLE_WRITES` set to a true value.
- A write tool that is not registered does not exist for the client. It is
  absent from the tool list, and a call to it fails.

So a plain `truenas-mcp serve` cannot change the appliance, and an AI client
cannot reach a write tool until you restart the server in write mode.

## Which values turn writes on

`TRUENAS_ENABLE_WRITES` accepts these spellings, in lower case, upper case or
with a capital first letter:

| Value | Effect |
| --- | --- |
| `1`, `t`, `true`, `y`, `yes`, `on` | Writes on |
| `0`, `f`, `false`, `n`, `no`, `off` | Writes off |
| Empty, or any other value | The default: writes off |

`TRUENAS_TLS_INSECURE` reads its value by the same rule.

## What the mode does not do

Read-only mode limits scope. It is not a security boundary against a hostile
client on a server that runs in write mode. Inside write mode, each tool
carries MCP annotations that tell the client how careful to be. The
[enable write tools](../guides/enable-writes.md) guide explains them.

The roles of the API key's user are a second limit. TrueNAS refuses a call
that the roles do not allow, in either mode.

## Why it fails closed

The appliance holds live data, and some write tools destroy it:
`truenas_dataset_delete`, `truenas_snapshot_delete`, `truenas_app_delete`,
`truenas_app_compose_update`, the snapshot retention that
`truenas_snapshot_task_create` and `truenas_snapshot_task_run` start, and the
retention of the task that `truenas_replication_task_create` makes. On a
first connection you have not checked any tool result yet. A server that
registers every tool and relies on the client to ask first fails open: the
server cannot enforce a client policy.

See the [tool reference](../reference/tools.md) for the mode of every tool.
