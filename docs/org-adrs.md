# Org-wide decisions (frostyard/core ADRs)

Conventions this repository follows that are decided at the org level are
recorded as ADRs in
[frostyard/core](https://github.com/frostyard/core/tree/main/docs/adr).
The ones that bind truenas-mcp:

- [ADR-0002 — Agent-portable instruction surface](https://github.com/frostyard/core/blob/main/docs/adr/0002-agent-portable-instruction-surface.md) — one canonical [`AGENTS.md`](../AGENTS.md); `CLAUDE.md`, `GEMINI.md`, `.cursorrules` and `.github/copilot-instructions.md` are symlinks to it, and `.claude/skills` is a symlink to [`.agents/skills/`](../.agents/skills/)
- [ADR-0018 — Org-wide agent instruction and knowledge surfaces](https://github.com/frostyard/core/blob/main/docs/adr/0018-org-wide-agent-instruction-and-knowledge-surfaces.md) — `AGENTS.md` is canonical, the per-tool files are the symlinks above, and durable corrections live in the append-only [`.memory/corrections.jsonl`](../.memory/README.md) (5 fields: `date`, `scope`, `correction`, `evidence`, `promoted_to`)
- [ADR-0021 — SHA-pinned actions and least-privilege CI workflows](https://github.com/frostyard/core/blob/main/docs/adr/0021-sha-pinned-actions-and-least-privilege-ci.md) — binds [`.github/workflows/`](../.github/workflows/): every action is pinned to a commit SHA with a `# vX.Y.Z` comment, `permissions: {}` at the top and `contents: read` per job, and checkout sets `persist-credentials: false`
- [ADR-0025 — One docs/ tree per repository, in core's four-category shape](https://github.com/frostyard/core/blob/main/docs/adr/0025-consolidate-repository-docs-into-docs.md) — this [`docs/`](README.md) tree in `adr/`, `design/`, `specs/` and `plans/`, formerly the `yeti/` tree, indexed in [docs/README.md](README.md)
- [ADR-0043 — Pin repository tools in mise and name the verify gate](https://github.com/frostyard/core/blob/main/docs/adr/0043-pin-repository-tools-in-mise-and-name-the-verify-gate.md) — [`mise.toml`](../mise.toml) pins `golangci-lint` and `node` with a committed [`mise.lock`](../mise.lock), and the Makefile reads the lint pin so `make lint` and CI cannot drift
- [ADR-0044 — Expose the make gate triad in every repository](https://github.com/frostyard/core/blob/main/docs/adr/0044-expose-the-make-gate-triad-in-every-repository.md) — the [`Makefile`](../Makefile) exposes `make verify` (credential-free, non-mutating), `make check` (developer, may format) and `make ci` (the gate CI runs)
- [ADR-0051 — ADRs state facts only, not the process that produced them](https://github.com/frostyard/core/blob/main/docs/adr/0051-adrs-state-facts-not-process.md) — every ADR here ([0004](adr/0004-read-only-by-default.md), [0005](adr/0005-use-the-official-mcp-go-sdk.md), [0006](adr/0006-use-the-official-truenas-websocket-client.md)) gives context, decision, consequences and alternatives only
- [ADR-0068 — Label pull requests that touch high-risk paths](https://github.com/frostyard/core/blob/main/docs/adr/0068-label-pull-requests-that-touch-high-risk-paths.md) — [`.github/risk-paths.json`](../.github/risk-paths.json) lists this repository's high-risk paths and the caller [`.github/workflows/risk-paths.yml`](../.github/workflows/risk-paths.yml) applies the `risk:high` label

When changing behavior covered by one of these, update or supersede the ADR
in frostyard/core first, then change this repository in the same effort.
