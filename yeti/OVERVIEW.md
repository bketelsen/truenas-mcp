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
    ├── tools_system.go   System/disk/network query tools + shared schema helpers
    ├── tools_pool.go     ZFS pool tools
    ├── tools_dataset.go  Dataset list/get/create/delete tools
    ├── tools_snapshot.go Snapshot list/get/create/delete tools
    ├── tools_data_protection.go  Periodic snapshot task and replication task list tools, plus the periodic snapshot task create tool (allow-listed summaries; no secrets)
    ├── tools_share.go    SMB and NFS share tools
    ├── tools_alert.go    Alert list/dismiss tools
    ├── tools_reports.go  Aggregated health report and job-list tools
    ├── tools_app.go      App list/get/config/start/stop/restart/update/delete tools + update report
    ├── tools_app_configure.go  App configuration write tool (deep-merge over app.config, then app.update)
    ├── tools_app_compose.go  Custom (compose) app write tool (validate YAML, dry-run diff + hash guard, app.update once, wait for the job)
    ├── linediff.go       LCS-based unified line diff used by the compose tool (no external diff dependency)
    └── params.go         Typed MCP parameter accessors
```

### Data Flow

1. CLI parses flags/env vars and calls `truenas.Connect(host, apiKey, tlsInsecure)` to open a WebSocket
2. `server.New(client, readOnly)` creates the MCP server (`*mcp.Server`) and registers tools
3. `server.Run(ctx, s)` starts the MCP server on `StdioTransport`, blocking until disconnect
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
| `version` | Build metadata vars (`Version`, `Commit`, `Date`, `BuiltBy`) overwritten at link time via `-X`; defaults identify a plain `go build` as `dev`. Wired into fang via `WithVersion`/`WithCommit` in `main.go`. |

### Key Dependencies

| Dependency | Version | Purpose |
|-----------|---------|---------|
| Go | 1.26.1 | Language version (from `go.mod`) |
| `github.com/modelcontextprotocol/go-sdk` | v1.4.0 | MCP protocol implementation |
| `charm.land/fang/v2` | v2.0.1 | CLI framework (Cobra wrapper) |
| `github.com/spf13/cobra` | v1.10.2 | CLI command structure |
| `github.com/truenas/api_client_golang` | v0.0.0-20250820 | TrueNAS WebSocket JSON-RPC client |
| `go.yaml.in/yaml/v3` | v3.0.4 | Parse and canonically render a custom app's compose file |

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

Report-style read tools are registered alongside the domain tools: `tools_reports.go` provides the aggregated health report plus job listing, and `tools_app.go` includes the read-only app update report as well as the app update write tools.

Each tool is defined inline with `s.AddTool(&mcp.Tool{...}, handlerFunc)`. The handler uses typed accessors from `server/params.go` to read MCP arguments, calls the TrueNAS API, and returns pretty-printed JSON.

Every tool also sets `Annotations: &mcp.ToolAnnotations{...}` (go-sdk v1.4.0 field types: `ReadOnlyHint bool`, `DestructiveHint *bool`, `IdempotentHint bool`, `OpenWorldHint *bool`, `Title string`; the pointers use Go 1.26 `new(false)` / `new(true)`). Rules, enforced by `server/annotations_test.go`:
- `Title` is always set, and `OpenWorldHint` is always `new(false)` — the only world is the one appliance.
- Tools registered in read-only mode set `ReadOnlyHint: true`.
- Write tools set `ReadOnlyHint: false` and an explicit `DestructiveHint` (the MCP default when omitted is true). `true` means the call can lose data or settings or is hard to undo (deletes, `truenas_app_configure`, `truenas_app_compose_update`, `truenas_app_delete`, `truenas_app_update`, `truenas_app_update_all`, `truenas_snapshot_task_create`); `false` means additive or recoverable.
- `IdempotentHint: true` only when a repeat call with the same arguments has no further effect on the appliance.
- Non-obvious classifications carry a trailing one-line comment explaining why (e.g. app updates stay destructive even though `upgradeApp` sends `snapshot_hostpaths: true`, because `app.rollback` restores only the ix-volumes snapshot and host-path snapshots need a manual ZFS rollback; `truenas_app_start` is not idempotent because `app.start` always runs `compose up --force-recreate`).

Clients such as agent harnesses use these hints to decide what needs human confirmation (run read-only tools freely, always confirm destructive ones). A new tool must be added to `wantToolHints` in `server/annotations_test.go` and to the README's "Tool Annotations" section, or the tests fail.

### App Upgrade Snapshots

`truenas_app_update` and `truenas_app_update_all` both go through `upgradeApp` in `server/tools_app.go`, which calls `app.upgrade(name, {"app_version": "latest", "values": {}, "snapshot_hostpaths": true})` and returns the job ID. `tools_app_test.go` asserts `snapshot_hostpaths` is `true` on every `app.upgrade` call. The middleware default is `false` (`api/v25_10_0/app.py` `UpgradeOptions`). Behavior in the TrueNAS 25.10 middleware (checked at `TS-25.10.0`; the snapshot and rollback logic is unchanged through `TS-25.10.7`):

- `app.upgrade` (`plugins/apps/upgrade.py`) refuses stopped apps and apps with no upgrade before doing anything. Then `take_snapshot_of_hostpath_and_stop_app` lists host paths, stops the app, and, when `snapshot_hostpaths` is true, snapshots each host path's dataset as `<dataset>@ix-app-upgrade-<app>-<old version>` (`get_upgrade_snap_name` in `plugins/apps/utils.py`).
- Host paths come from `app.get_hostpaths_datasets` (`plugins/apps/resources.py`): every container mount source in `active_workloads.volumes` not under `/mnt/.ix-apps/`, resolved by `paths_to_datasets_impl` (`plugins/zfs_/utils.py`). That function statx()es the path and reads the mount's source, so a subdirectory resolves to the dataset that contains it. Paths not on ZFS, on the boot pool, or that fail to stat map to `None` and are skipped with a debug log. The upgrade continues, so `true` does not make upgrades fail for host paths that are not datasets.
- Snapshots are non-recursive (`pool.snapshot.create` default), so child datasets under a host path are not captured. If a snapshot with that name already exists it is kept rather than replaced. That dedupes several host paths in one dataset, but a retried upgrade from the same version reuses the older snapshot. A failed snapshot fails the job with the app already stopped.
- Custom apps (and `ix-app` image-only updates) take an early image-pull-and-redeploy branch with no snapshots at all.
- ix-volumes are snapshotted separately and unconditionally: `<ix-volumes ds>@<old version>`, recursive, replacing any existing one.
- `app.rollback` (`plugins/apps/rollback.py`) rolls back only `<ix-volumes ds>@<version>` (when `rollback_snapshot` is true). It never touches `ix-app-upgrade-*` snapshots, and nothing in the middleware deletes them.

That last point is why both update tools keep `DestructiveHint: true`. Host-path data is now recoverable, but only by a manual `zfs rollback` of the whole dataset, which also reverts other apps that share the dataset and destroys newer snapshots. The README's "App updates and recovery" section is the user-facing version of this. Do not mark the update tools non-destructive unless the server also gains a tool that restores the host-path snapshots, or TrueNAS's rollback starts doing it.

### Custom App Compose Updates

`truenas_app_compose_update` (`server/tools_app_compose.go`) is the only tool that writes a custom (compose) app. `truenas_app_configure` keeps refusing custom apps, because their definition is not a set of schema values but one compose object, `custom_compose_config`, which `app.update` replaces whole. The middleware role for that call is `APPS_WRITE`. The tool is registered only with `--enable-writes`, next to `registerAppConfigureTools` in `server/server.go`.

Why each part exists:

- YAML validation (`parseCompose`) runs before the stored compose is read. It refuses empty text, a parse error, more than 1 document, a non-mapping top level, a mapping key that is not a string at any depth, a cyclic alias, a missing or empty `services` mapping, and a service that is not a mapping. The key walk (`checkNodeKeys`) keeps the nodes on the current path, so an alias back to one of them is a `cyclic alias`, and it keeps the nodes already checked, so an anchor shared by many aliases costs one visit and a "billion laughs" document cannot make it exponential — the yaml decoder's own aliasing limit usually refuses such a document first. `resolveAlias` returns nil instead of looping on a cycle. The value is normalized through `json.Marshal` and a `json.Decoder` with `UseNumber()`, and the object — not the text — goes to the diff, the hash and `app.update`, so the appliance gets exactly what the caller saw.
- Numbers keep their exact form (`convertJSONNumbers`, `numberValue`): `int64` for an integer literal that fits, a refusal that names the value for a larger integer literal, and `float64` otherwise, except a float of 2^53 or more with no fraction, which is refused. A plain `float64` decode would turn `9007199254740993` into `9007199254740992` in the hash, the diff and the payload. The stored compose from `app.config` is decoded the same way; a number it cannot carry exactly is refused before any write.
- `canonicalYAML` renders both sides with a `yaml.v3` encoder at 2-space indent. `yaml.v3` sorts map keys (digit-aware), so equal content gives equal text whatever the key order or YAML style. The identical check and the hash therefore compare content only.
- `current_sha256` is the hex SHA-256 of the canonical stored compose. A write must pass it as `expected_sha256`; a missing or different value is refused with the current hash in the message, so a caller cannot apply a compose it never previewed, and a compose that changed after the preview cannot be overwritten silently.
- `unifiedDiff` (`server/linediff.go`) is a small LCS line diff: common prefix and suffix are copied, the middle comes from a dynamic-programming LCS table capped at 4M cells, and hunks carry 3 context lines with `--- current` / `+++ proposed` headers. There is no diff dependency.
- `app.update` is called once and never replayed. A transport error leaves the job state unknown, and the middleware keeps no previous compose, so the error tells the caller to check `truenas_jobs_list` instead of running the tool again.
- The job wait polls `core.get_jobs` every `jobPollInterval` (2 seconds; a package variable so tests use milliseconds) until `SUCCESS`, `FAILED`, `ABORTED`, the deadline or context cancellation. The context and the deadline are checked before each poll, and the wait between polls is the smaller of `jobPollInterval` and the time left, so no poll starts after the deadline. `truenas.Caller` takes no context, so a poll that is already running cannot be stopped; 1 such call is bounded by the client's 30-second call timeout. A failed poll is a read and is retried; its error is kept for the final message. `composeJobWait` (a package variable) replaces `timeout_seconds` in tests, because the smallest allowed timeout is 30 seconds.
- The full previous compose is `current_compose` of the dry run and `previous_compose` of a successful write; TrueNAS keeps no copy. The `FAILED`/`ABORTED` error says to send that complete compose back with this tool to roll back. A unified diff is not a full copy, so it is never offered as the rollback source.

### Read-Only Mode

Read-only mode is the default. Unless writes are explicitly enabled with `--enable-writes` or `TRUENAS_ENABLE_WRITES=true`, mutating tools (create, delete, start, stop, restart, dismiss, update, app delete) are never registered. AI clients cannot see or invoke them.

`TRUENAS_ENABLE_WRITES` is parsed with `envBool`, so common true values (`1`, `true`, `yes`, `on`) opt in and common false values (`0`, `false`, `no`, `off`) keep the default read-only behavior.

### Schema and Parameter Helpers

`tools_system.go` defines shared schema/result helpers used across tool files:
- `schema()`, `noArgs()` — build MCP input schema objects
- `stringProp()`, `numberProp()`, `boolProp()`, `arrayProp()` — property builders
- `jsonResult()` — wraps raw JSON as pretty-printed MCP text content

`server/params.go` defines typed accessors for MCP arguments:
- `requireString()`, `optionalString()`
- `requireFloat64()`, `optionalFloat64()`
- `optionalBool()`, `optionalSlice()`
- `strictBool()`, `strictStringList()` — for tools that must refuse a present argument of the wrong type

`optionalBool()` returns false for an absent key, and both `optionalBool()` and `optionalSlice()` drop a value of the wrong type silently. The SDK does not validate arguments against `InputSchema`, so a new write tool should use `strictBool()` and `strictStringList()` for any argument whose default would change the effect of the call (`truenas_snapshot_task_create` needs this for `dry_run`, `enabled`, `allow_empty`, `recursive`, and `exclude`). A JSON `null` counts as absent in both helpers.

Use these helpers instead of manually unpacking `req.Params.Arguments` in each tool.

### Lint Compliance: Intentionally Ignored Errors

The codebase uses explicit blank-identifier assignments to satisfy `errcheck` lint rules for errors that are intentionally not handled:
- `_, _ = fmt.Fprintf(...)` for stderr status messages where write failures are not actionable
- `_ = api.Close()` for cleanup in error paths and deferred closes where close errors cannot be meaningfully handled

### JSON Number Handling

MCP numeric arguments arrive through JSON unmarshalling as `float64`. Handlers should use `requireFloat64()` or `optionalFloat64()` from `server/params.go`, then cast to the API's expected type when needed (for example, converting an ID to `int` before passing it to TrueNAS).

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
| Flag | `--enable-writes` | Opt in to registering tools that create, delete, or modify TrueNAS resources |
| Env | `TRUENAS_ENABLE_WRITES` | Same as `--enable-writes` |
| Flag | `--tls-insecure` | Skip TLS certificate verification |
| Env | `TRUENAS_TLS_INSECURE` | Same as `--tls-insecure` |

Flags take precedence over defaults; env vars are used as default values for flags (via `envOrDefault` and `envBool` in `cmd/serve.go`). Writes are disabled by default.

### Connection Details

- `Client.Call` serializes access and probes with `core.ping` before each operation. A failed probe closes the stale connection and dials/authenticates again using the original connection settings.
- A transport failure during the operation invalidates the connection but does not replay the operation. This prevents duplicate writes when a response is lost. Explicit `Close` is terminal.
- `truenas/client_test.go` covers stale connections, failed reconnects, no write replay, terminal close, and concurrent callers; run with the race detector.
- WebSocket URL: `wss://<host>/api/current`
- TLS certificate verification is enabled by default; `--tls-insecure` / `TRUENAS_TLS_INSECURE` explicitly opts out (useful for self-signed NAS certificates)
- Authentication via API key (not username/password)
- API call timeout: 30 seconds

## Testing

Tests use the `Caller` interface for dependency injection — no real TrueNAS server is needed.

### Test Infrastructure (`server/mock_test.go`)

- **`mockCaller`** — implements `truenas.Caller` with a `CallFunc` field for injecting per-test responses
- **`callTool()`** — spins up a full MCP server + client via the SDK's `InMemoryTransport`, then calls a tool by name. This tests the complete path: tool registration → argument parsing → API call → response formatting
- **`listTools()` / `listToolDefs()`** — spin up the same in-memory server + client and return the registered tool names, or the full `*mcp.Tool` definitions (with annotations) as a client receives them
- **`resultText()`** — extracts the text content from a `CallToolResult`

### Test Organization

Each `tools_*.go` file has corresponding `tools_*_test.go` coverage for read and write tools, plus report/update coverage such as `tools_reports_test.go` and `tools_app_update_report_test.go`. `helpers_test.go` covers the schema/result helpers, and `params_test.go` covers the typed parameter accessors. `server_test.go` tests `New()` for correct read-only vs read-write tool registration. `annotations_test.go` checks every tool's annotations in both registration modes and pins each tool's classification to the `wantToolHints` table. `cmd/serve_test.go` tests environment variable handling and command validation (e.g., missing host/API key errors).

## CI

GitHub Actions workflow `.github/workflows/ci.yml` (named `CI`) runs on push to `main`, on pull requests, and on `merge_group`. Top-level `permissions: {}`; each job grants only `contents: read`. All actions are pinned to commit SHAs. Jobs:

| Job | What it does |
|-----|-------------|
| `lint` | Installs the `golangci-lint` pinned in `mise.toml`/`mise.lock` via `jdx/mise-action`, runs `make lint-version-check`, then `golangci-lint run` |
| `verify` | `go mod tidy -diff`, `go vet`, `gofmt -s -l` on tracked Go files |
| `test` | `make test` (`-race -count=1`) |
| `build` | `make build` matrix: linux/amd64, linux/arm64, darwin/arm64, windows/amd64 |
| `release-config` | `goreleaser check` with the Pro distribution; fails on trusted runs if `GORELEASER_KEY` is missing, warns on fork PRs |

Go version is read from `go.mod` via `go-version-file`. The only other tool pin is `golangci-lint` in `mise.toml`; the Makefile reads that pin so `make lint` and CI cannot drift.

## Release Pipeline

Modeled on `frostyard/updex`. Three pieces:

- **`Makefile`** — `make bump` runs build/test/fmt/lint, requires a clean tree, tags `$(svu next)` (semver derived from Conventional Commit messages since the last tag) and pushes the tag. `make snapshot` builds into `dist/` without publishing; `make release-check` validates the config. Both need `goreleaser-pro` on PATH.
- **`.goreleaser.yaml`** — GoReleaser Pro (`pro: true`). Builds `CGO_ENABLED=0 -trimpath` binaries for linux/darwin/windows × amd64/arm64 with `-X truenas-mcp/version.*` ldflags. `before` hooks run `go mod tidy`, `scripts/completions.sh` (bash/zsh/fish via fang's `completion` command) and `scripts/manpages.sh` (fang's `man` command, gzipped). Produces tar.gz (zip on Windows) archives, `checksums.txt`, and deb/rpm/apk packages that install the binary, completions, and man page. Changelog is grouped by conventional-commit prefix. `nightly` publishes a single rolling `dev` pre-release (`{{ incmajor .Version }}-dev`).
- **Workflows** — `release.yml` (`goreleaser`) runs on any tag push: GoReleaser Pro release, then `actions/attest-build-provenance` signs `checksums.txt` and every archive/package. `snapshot.yml` runs on `workflow_run` after a successful `CI` on `main` and does `goreleaser release --nightly --clean`; a `concurrency` group cancels stale nightlies. Both need the `GORELEASER_KEY` repository secret.

Generated directories `build/`, `dist/`, `completions/`, `manpages/` are gitignored. `scripts/first-contact.sh` executes `build/truenas-mcp`.

## MCP Tools Reference

See [tools.md](tools.md) for the complete tool catalog with parameters.

See [api-audit.md](api-audit.md) for the audit of every middleware method against TrueNAS 27.0, with the method table and the fallbacks.

## Build & Run

```bash
mise install      # pinned golangci-lint
make              # fmt + vet + build -> build/truenas-mcp
make run          # build and run `serve`
make test         # run tests with race detector
make lint         # pinned golangci-lint (refuses a mismatched version)
make verify       # static checks + tests (credential-free gate)
make ci           # verify-static + race tests + cross-builds
make help         # list all targets
```

Binary name: `truenas-mcp`, built into `build/`. Version comes from `git describe --tags` at build time (`dev` for plain `go build`); there is no version constant to bump. Tags are created by `make bump`.
