# 0006 — Use the official TrueNAS WebSocket client

- **Status:** Accepted
- **Date:** 2026-03-12

## Context

TrueNAS SCALE Goldeye (25.10) deprecates the REST API in favour of the WebSocket
JSON-RPC 2.0 API. Each middleware method is a JSON-RPC method name (`pool.query`,
`app.update`), and a query takes a filter array such as
`[["name", "=", "tank"]]`.

## Decision

All appliance calls go through `github.com/truenas/api_client_golang`
(`go.mod`, pseudo-version `v0.0.0-20250820184128-fc6edc0b6ebe`), wrapped by
`truenas/client.go`. The wrapper is the `truenas.Caller` interface, which every
tool handler takes instead of the concrete client.

- The URL is `wss://%s/api/current` (`connectAPI` in `truenas/client.go`). The
  host and port come from `--host` / `TRUENAS_HOST`.
- Authentication is an API key: `api.Login("", "", apiKey)`, which the library
  sends as `auth.login_with_api_key`.
- TLS certificate verification is on unless the operator passes
  `--tls-insecure` / `TRUENAS_TLS_INSECURE` (`verifySSL := !tlsInsecure`).
- `Client.Call` probes with `core.ping` before each operation, reconnects and
  authenticates again when the probe fails, and serializes calls with a mutex.
  It never replays an operation after a transport error.
- Each call passes a 30-second timeout to the library, and the result is the
  envelope's `result` member as `json.RawMessage`.

## Consequences

- The server tracks the API that current TrueNAS releases serve, so a REST
  fallback is never needed for a current appliance.
- The library is LGPL-3.0, and the MIT-licensed binary includes it as a
  statically linked Go module, so a redistribution of the binary carries the
  LGPL-3.0 obligations for that module.
- Transport errors are ambiguous for writes: the response may be lost after the
  appliance applied the change. Callers of a write tool must not retry blindly;
  `truenas_app_compose_update` and `truenas_snapshot_task_run` say so in their
  error text.
- The library has no context-aware call, so a wait that must stop on a
  cancellation runs each poll in its own goroutine (`server/jobs.go`).

## Alternatives considered

- **The REST API:** rejected because TrueNAS deprecated it in favour of the
  WebSocket API.
- **A hand-written WebSocket JSON-RPC client:** rejected because the library
  already handles the envelope, the login and the ping, and a private copy would
  need its own tests for the same behaviour.

## References

- Shapes: [design/overview.md](../design/overview.md) (Data Flow, Connection
  Details), [specs/api-audit.md](../specs/api-audit.md),
  [specs/tools.md](../specs/tools.md), [`AGENTS.md`](../../AGENTS.md)
- Code: `go.mod`, `truenas/client.go`, `truenas/client_test.go`
- Builds on: [ADR-0001](0001-record-architecture-decisions.md), and pairs with
  [ADR-0005](0005-use-the-official-mcp-go-sdk.md)
