# truenas-mcp

An MCP server over stdio for TrueNAS SCALE: it exposes storage, sharing, system
and app management to an AI client through the appliance's WebSocket JSON-RPC
API. Written in Go. Start at [docs/README.md](docs/README.md) for the design,
the tool catalog, the API audit and the decisions.

This file (`AGENTS.md`) is the CANONICAL agent instructions — `CLAUDE.md`,
`GEMINI.md`, `.cursorrules`, and `.github/copilot-instructions.md` are symlinks
to it, and `.claude/skills` symlinks to `.agents/skills/`
([ADR-0002](docs/adr/0002-agent-portable-instruction-surface.md)). Edit only
the canonical paths; keep content tool-agnostic.

## Skills (follow these for common tasks)

Step-by-step procedures live in [.agents/skills/](.agents/skills/); follow
them rather than improvising, whichever agent you are:

- *(none yet — add the first one when a procedure repeats)*

## Code conventions (live — the code exists)

- **Read-only by default.** `server.New(client, readOnly bool)` in
  `server/server.go` always registers the 23 read-only tools and registers the
  20 write tools only inside `if !readOnly`. `NewServeCmd` in `cmd/serve.go`
  passes `!cfg.EnableWrites`, so write tools exist only with `--enable-writes`
  or `TRUENAS_ENABLE_WRITES=true` ([ADR-0004](docs/adr/0004-read-only-by-default.md)).
  A new write tool goes into that block, into `wantToolHints` in
  `server/annotations_test.go`, and into the README's "Tool Annotations".
- **Every tool name starts with `truenas_`** (`truenas_<domain>_<action>`).
- **Use the glossary terms** from [docs/design/glossary.md](docs/design/glossary.md)
  (Server, Appliance, Tool, Report, Pool, Dataset, Snapshot, Share, App, Alert,
  Job), and not the words each entry marks as avoided.
- **Connection recovery and no replay.** `truenas/client.go` probes with
  `core.ping` before each operation, reconnects and authenticates again on a
  failed probe, and serializes calls. It never replays an operation after a
  transport error: a write may already have taken effect. Keep this rule in
  every new call path, and change it only with a test in
  `truenas/client_test.go`.
- **Version comes from the build.** `make build` injects `version.Version`,
  `Commit`, `Date` and `BuiltBy` with `-X` ldflags from `git describe`; never
  hardcode a version or add a version constant.
- **The test mock is `server/mock_test.go`.** `mockCaller` implements
  `truenas.Caller` with a `CallFunc`; `callTool`, `listTools`, `listToolDefs`
  and `resultText` run a real in-memory MCP client and server. No test talks to
  a real appliance, and the server code takes `truenas.Caller`, not `*Client`.
- **Conventional Commits.** `svu` derives the next version from the messages
  since the last tag and GoReleaser groups the changelog by prefix, so a
  non-conforming subject breaks the release.
- Run `make verify` before you call any change done: it runs `go mod tidy
  -diff`, `go vet`, `scripts/check-gofmt.sh` and its self-test, the pinned
  `golangci-lint`, the template gate and the unit tests. CI runs the same
  checks in job `verify` (the unit tests run in job `test`, with the race
  detector). `make ci` adds the race tests and the cross-builds, and with
  `BASE` set the append-only check of `.memory/corrections.jsonl` against the
  PR base; run `make ci BASE=origin/main` locally for the same check. The gate
  needs Node.js 22 or later, and `mise install` gets the pinned Node and
  `golangci-lint`.

**Test reliability.** No sleeps for timing (wait on a condition or inject a
clock). No network and no wall clock in unit tests (inject them). A flaky test
gets an issue, then a fix or a quarantine (skip with the issue link) in the
same PR. Never retry a job or a test until it is green.

**Blocker to guard.** When a review (agent cross-review, owner review, or a
human) finds a blocking point, the PR that fixes it also adds a guard: a
targeted test that fails without the fix (preferred), or an `AGENTS.md` rule
when no test can catch it. The same PR appends 1 line to
`.memory/corrections.jsonl` (`evidence`: the PR and the review;
`promoted_to`: the test or the rule). See `.memory/README.md`.

**Risk paths.** `.github/risk-paths.json` lists the high-risk paths. The list is
schema version 2 with levels L1 to L4 (core ADR-0078, see
[docs/org-adrs.md](docs/org-adrs.md)). The caller
`.github/workflows/risk-paths.yml` calls core's `pr-triage.yml`. That workflow
adds `level:L1` to `level:L4` and, for L1 and L2, `risk:high`. It also removes
`ready-for-human` when the head moves after the label (core ADR-0077). A PR at
L1 or L2, or with `risk:high`, declares tier 3. Add a path when you add a file
of that kind (signing, auth, publish, secrets, destructive operations, CI).
Every destructive tool file must be at L1:
`test/risk-paths-coverage.test.mjs` fails otherwise.

**Vendored files.** `scripts/lib/risk-paths.mjs` and
`scripts/check-corrections.mjs` are copies from frostyard/core; never edit them
by hand (`make verify` checks their hashes).

## Repository boundary

What does not belong here: API keys, NAS hostnames and any NAS-private data
(dataset names with client data, serial numbers, screenshots of the appliance)
— not in code, tests, fixtures, docs or commit messages.

Releases and dependencies:

- **Tag format:** `v1.2.3`, the stable series is `v0.x`.
  `dev` is the rolling pre-release tag from `snapshot.yml`; skip it in release
  checks.
- **How a release starts:** a tag push runs `.github/workflows/release.yml`
  (workflow `goreleaser`, GoReleaser Pro with `.goreleaser.yaml`, then the
  build-provenance attestation). `make bump` makes and pushes the tag.
- **Who tags:** the repository owner. Agents never run `make bump`, never tag,
  push a tag or publish a release.
- **Version rule:** `svu next` (Conventional Commits since the last `v*` tag);
  there is no `.svu.yml`.
- **Internal dependencies:** none. `go.mod` has no `github.com/frostyard/...` or
  `github.com/bketelsen/...` module. Tool pins live in `mise.toml` and
  `mise.lock` (`golangci-lint`, `node`) and in `go.mod` (Go). The Go version is
  read from `go.mod`; the Makefile reads the `golangci-lint` pin from
  `mise.toml`.
- **Who uses this repository:** no frostyard or bketelsen repository imports it
  (module path `truenas-mcp`). It ships as the `truenas-mcp` binary.

## Documentation rules (enforced)

Docs live in `docs/` in four categories. **Every new doc starts from its
category's `TEMPLATE.md`** and follows its structure:

- `docs/adr/` — why we decided. Immutable once Accepted; reversals are new
  ADRs that mark the old one Superseded.
- `docs/design/` — how it fits together. Living; updated in place to match
  reality.
- `docs/specs/` — exact contracts. Change only alongside implementing code.
- `docs/plans/` — order of work. Phases with "Done when" outcomes.

After a source change, update [docs/design/overview.md](docs/design/overview.md),
[docs/specs/tools.md](docs/specs/tools.md), [README.md](README.md) and the
affected user pages in `site/content/` (the docs site, `make site`), plus
[docs/specs/api-audit.md](docs/specs/api-audit.md) when a middleware method or
the payload it receives changes.

**Site pages state checked facts only.** Each claim on a `site/content/`
page comes from the code, a workflow or a doc at the PR head, with its scope
kept: a claim about release assets names the workflow that makes them
(`release.yml` attests tagged releases; the `dev` pre-release from
`snapshot.yml` has no attestation), a claim about a tool names the field it
acts on (`truenas_app_update*` act on `upgrade_available`, not on
`image_updates_available`), and an audit date covers only the methods the
audit checked on that date.

A decision local to this repository gets an ADR in `docs/adr/`. A decision that
binds more than this repository goes to
[frostyard/core](https://github.com/frostyard/core/tree/main/docs/adr) and gets
a line in [docs/org-adrs.md](docs/org-adrs.md).

### Cross-linking is mandatory

A doc without its required links is incomplete — do not finish a docs change
until they exist, in both directions:

- **ADR** → links every design doc/spec it shapes, and prior ADRs it builds on.
- **Design doc** → links the ADR(s) providing its rationale, the spec(s)
  pinning its contracts, and the roadmap phase that builds it.
- **Spec** → links its motivating ADR(s) and the design doc showing where it
  fits.
- **Plan** → every phase links the design docs/specs it implements; resolved
  open questions become ADRs.

When you touch a doc, verify its links still hold (targets exist, section
anchors valid) and add the back-links on the targets. Use relative paths.

### Housekeeping

- New doc ⇒ add a line to the index in [docs/README.md](docs/README.md).
- New significant decision ⇒ new ADR *first*, then update the affected design
  docs/specs in the same change.
- Convert relative dates ("next weekend") to absolute dates in all docs.

## Issues

Bugs and feature requests live in GitHub Issues; a fix arrives as a pull
request that closes its issue. Labels in use: `ready-for-human` (a PR that is
reviewed, has green CI and is ready for the owner), `sweep` (a finding of the
conformance sweep), `level:L1` to `level:L4` (applied by the PR-triage
workflow), `risk:high` (applied by the PR-triage workflow for L1 and L2).
