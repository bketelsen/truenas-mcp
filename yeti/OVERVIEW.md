# truenas-mcp Overview

## Purpose

truenas-mcp is an MCP (Model Context Protocol) server that exposes TrueNAS SCALE management capabilities to AI assistants. It connects to a TrueNAS SCALE instance (Goldeye/25.10+) via its WebSocket JSON-RPC API and presents storage, sharing, system, and app management as MCP tools over stdio transport.

## Architecture

```
main.go                  Entry point — uses Charm fang CLI framework
├── cmd/
│   ├── root.go          Root cobra command (truenas-mcp)
│   └── serve.go         `serve` subcommand — connects to TrueNAS, starts MCP server
├── truenas/
│   └── client.go        WebSocket JSON-RPC client wrapper
└── server/
    ├── server.go         MCP server setup and tool registration
    ├── tools_system.go   System/disk/network query tools + shared helpers
    ├── tools_pool.go     ZFS pool tools
    ├── tools_dataset.go  Dataset list/get/create/delete tools
    ├── tools_snapshot.go Snapshot list/get/create/delete tools
    ├── tools_share.go    SMB and NFS share tools
    ├── tools_alert.go    Alert list/dismiss tools
    └── tools_app.go      App list/get/start/stop/restart tools
```

### Data Flow

1. CLI parses flags/env vars and calls `truenas.Connect(host, apiKey)` to open a WebSocket
2. `server.New(client, readOnly)` creates the MCP server and registers tools
3. `server.Run()` starts the MCP server on stdio, blocking until disconnect
4. Each tool handler calls `client.Call(method, params...)` which:
   - Sends a JSON-RPC call over WebSocket with a 30-second timeout
   - Parses the response envelope, extracting `result` or `error`
   - Returns `json.RawMessage` which the tool handler pretty-prints as the MCP response

### Key Packages

| Package | Role |
|---------|------|
| `cmd` | CLI wiring via Cobra + Charm fang. Handles flag/env parsing. |
| `truenas` | Thin wrapper around `github.com/truenas/api_client_golang`. Defines the `Caller` interface and provides `Connect`, `Call`, `Close`. |
| `server` | MCP server construction and all tool definitions. Each `tools_*.go` file covers one domain. |
| `version` | Single `Version` constant (`0.1.0`), used by the CLI framework. |

## Key Patterns

### Caller Interface (Dependency Injection)

The `truenas` package defines a `Caller` interface:
```go
type Caller interface {
    Call(method string, params ...interface{}) (json.RawMessage, error)
}
```
`*truenas.Client` satisfies this interface. All server functions (`server.New()` and every `register*Tools` function) accept `truenas.Caller` rather than the concrete client. This enables testing with a `mockCaller` that injects canned responses without a real TrueNAS connection.

### Tool Registration Pattern

Tools are split into read and write registration functions per domain:
- `register<Domain>Tools` or `register<Domain>ReadTools` — always registered
- `register<Domain>WriteTools` — only registered when `readOnly` is false

Each tool is defined inline with `s.AddTool(&mcp.Tool{...}, handlerFunc)`. The handler extracts arguments from `req.Params.Arguments`, calls the TrueNAS API, and returns pretty-printed JSON.

### Read-Only Mode

When `--read-only` is set (or `TRUENAS_READ_ONLY` env var is non-empty), mutating tools (create, delete, start, stop, restart, dismiss) are never registered. AI clients cannot see or invoke them.

### Schema Helpers

`tools_system.go` defines shared helpers used across all tool files:
- `schema()`, `noArgs()` — build MCP input schema objects
- `stringProp()`, `numberProp()`, `boolProp()`, `arrayProp()` — property builders
- `args()` — extracts argument map from request
- `jsonResult()` — wraps raw JSON as pretty-printed MCP text content

### Lint Compliance: Intentionally Ignored Errors

The codebase uses explicit blank-identifier assignments to satisfy `errcheck` lint rules for errors that are intentionally not handled:
- `_, _ = fmt.Fprintf(...)` for stderr status messages where write failures are not actionable
- `_ = api.Close()` for cleanup in error paths and deferred closes where close errors cannot be meaningfully handled

### TrueNAS API Mapping

Tools map directly to TrueNAS JSON-RPC methods. The naming convention is:
- Tool: `truenas_<domain>_<action>` (e.g., `truenas_dataset_create`)
- API method: `<service>.<action>` (e.g., `pool.dataset.create`)

Query tools typically pass filter arrays like `[["field", "=", value]]` to the API.

### Error Handling

The `Client.Call` method checks for errors at two levels:
1. WebSocket/transport errors from the underlying client
2. JSON-RPC level errors in the response envelope (`error` field)

Tool handlers wrap API errors with context (e.g., `fmt.Errorf("pool.query: %w", err)`).

## Configuration

| Source | Variable/Flag | Description |
|--------|--------------|-------------|
| Flag | `--host` | TrueNAS host address (e.g., `truenas.local`) |
| Env | `TRUENAS_HOST` | Same as `--host` |
| Flag | `--api-key` | TrueNAS API key |
| Env | `TRUENAS_API_KEY` | Same as `--api-key` |
| Flag | `--read-only` | Restrict to read-only tools |
| Env | `TRUENAS_READ_ONLY` | Any non-empty value enables read-only mode |

Flags take precedence over defaults; env vars are used as default values for flags (via `envOrDefault` in `cmd/serve.go`).

### Connection Details

- WebSocket URL: `wss://<host>/api/current`
- SSL verification is disabled (self-signed certs common on NAS devices)
- Authentication via API key (not username/password)
- API call timeout: 30 seconds

## Testing

Tests use the `Caller` interface for dependency injection — no real TrueNAS server is needed.

### Test Infrastructure (`server/mock_test.go`)

- **`mockCaller`** — implements `truenas.Caller` with a `CallFunc` field for injecting per-test responses
- **`callTool()`** — spins up a full MCP server + client via the SDK's `InMemoryTransport`, then calls a tool by name. This tests the complete path: tool registration → argument parsing → API call → response formatting
- **`resultText()`** — extracts the text content from a `CallToolResult`

### Test Organization

Each `tools_*.go` file has a corresponding `tools_*_test.go` that tests both read and write tools. `helpers_test.go` covers the schema/arg parsing helpers. `server_test.go` tests `New()` for correct read-only vs read-write tool registration.

## CI

GitHub Actions workflow (`.github/workflows/ci.yml`) runs on push to `main` and on pull requests. Four parallel jobs:

| Job | What it does |
|-----|-------------|
| `lint` | `golangci-lint` via `golangci-lint-action@v8` |
| `fmt-check` | `gofmt -s -d .` — fails if unformatted code |
| `test` | `make test` (includes `-race -count=1`) |
| `build` | `make build` — verifies compilation |

Go version is read from `go.mod` via `go-version-file`, keeping CI in sync automatically.

## MCP Tools Reference

See [tools.md](tools.md) for the complete tool catalog with parameters.

## Build & Run

```bash
make              # fmt + vet + build
make run          # build and run `serve`
make test         # run tests with race detector
make lint         # golangci-lint
```

Binary name: `truenas-mcp`. Version: `0.1.0`.
