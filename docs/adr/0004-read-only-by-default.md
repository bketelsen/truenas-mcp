# 0004 — Read-only by default

- **Status:** Accepted
- **Date:** 2026-05-19

## Context

The server exposes its tools to an AI client over stdio. An AI client can call
any tool it can see. The appliance holds live data, and some calls destroy it:
`truenas_dataset_delete`, `truenas_snapshot_delete`, `truenas_app_delete`,
`truenas_app_compose_update`, and the retention that `truenas_snapshot_task_create`
and `truenas_snapshot_task_run` start. First contact with a NAS appliance is when
the operator has validated no tool response yet.

## Decision

The server has 2 registration modes at `server.New(client, readOnly bool)` in
`server/server.go`. It always registers the 21 read-only tools. It registers the
19 write tools only when `readOnly` is false, in the `if !readOnly` block of the
same function.

`cmd/serve.go` sets the mode. `--enable-writes` (env `TRUENAS_ENABLE_WRITES`) is
off by default, and `NewServeCmd` calls `server.New(client, !cfg.EnableWrites)`.
`envBool` in `cmd/serve.go` reads the env value: the listed spellings of `1`,
`t`, `true`, `y`, `yes` and `on` (lower case, upper case, or capitalised) enable
writes; `0`, `f`, `false`, `n`, `no` and `off` keep them off; any other value
uses the flag default, which is off.

A write tool that is not registered does not exist for the client: it is absent
from `tools/list` and a `tools/call` for it fails. `server/server_test.go` checks
both registration modes.

## Consequences

- The safe path is the default path: a plain `truenas-mcp serve` cannot change
  the appliance.
- An operator must restart the server with `--enable-writes` or
  `TRUENAS_ENABLE_WRITES=true` to reach a write tool.
- Read-only mode is not a security boundary against a hostile client on a
  write-enabled server; it is a scope limit, and the tool annotations
  (`destructiveHint`) add the per-call warning on top of it.
- A new write tool must go into the `if !readOnly` block and into
  `wantToolHints` in `server/annotations_test.go`, or a test fails.

## Alternatives considered

- **Register every tool and rely on the client to confirm destructive calls:**
  rejected because a client policy is not enforceable by the server, and the
  MCP specification tells clients to treat annotations as untrusted.
- **A per-tool allow-list flag:** rejected because it multiplies the
  configurations an operator must validate, for no gain over 1 switch.
- **A `--read-only` flag that defaults to writes enabled:** rejected because it
  fails open.

## References

- Shapes: [design/overview.md](../design/overview.md) (Read-Only Mode),
  [specs/tools.md](../specs/tools.md) (the write tools),
  [`AGENTS.md`](../../AGENTS.md), [`README.md`](../../README.md)
- Code: `server/server.go`, `cmd/serve.go`, `server/annotations_test.go`
- Builds on: [ADR-0001](0001-record-architecture-decisions.md)
