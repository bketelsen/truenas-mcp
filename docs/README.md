# Documentation

Docs are split by the question they answer:

| Directory | Question | Contents |
| --- | --- | --- |
| [adr/](adr/) | **Why** did we choose this? | Architecture Decision Records — immutable once accepted; superseded, never edited. Org-wide decisions in [org-adrs.md](org-adrs.md) |
| [design/](design/) | **How** does it fit together? | Living documents describing the current architecture |
| [specs/](specs/) | **What exactly** is the contract? | Precise, testable interface definitions |
| [plans/](plans/) | **When/in what order** do we build? | Roadmaps and phase plans; updated as work lands |
| [`first-contact.md`](first-contact.md) | **How** do I try it safely? | Guide: the first connection to a real appliance |

## Index

### Decisions (ADRs)

- [0001 — Record architecture decisions](adr/0001-record-architecture-decisions.md)
- [0002 — Agent-portable instruction surface](adr/0002-agent-portable-instruction-surface.md)
- [0003 — Verify gate, risk paths and corrections](adr/0003-verify-gate-risk-paths-and-corrections.md)
- [0004 — Read-only by default](adr/0004-read-only-by-default.md)
- [0005 — Use the official MCP Go SDK](adr/0005-use-the-official-mcp-go-sdk.md)
- [0006 — Use the official TrueNAS WebSocket client](adr/0006-use-the-official-truenas-websocket-client.md)
- [Org-wide decisions (frostyard/core ADRs)](org-adrs.md)

### Design

- [overview.md](design/overview.md) — purpose, architecture, data flow, tool
  registration, annotations, configuration, testing, CI, release pipeline
- [glossary.md](design/glossary.md) — Server, Appliance, Tool, Report, Pool,
  Dataset, Snapshot, Share, App, Alert, Job

### Specs

- [tools.md](specs/tools.md) — every MCP tool: parameters, middleware methods,
  annotations, and the registration rules
- [api-audit.md](specs/api-audit.md) — every middleware method against TrueNAS
  27.0, with the gaps found and the fallbacks

### Plans

- [2026-11-02-protection-report.md](plans/2026-11-02-protection-report.md) —
  the per-dataset protection report (shipped)

### Guides

- [first-contact.md](first-contact.md) — the safe first connection to a real
  TrueNAS system, and `scripts/first-contact.sh`

## Conventions

- **New docs start from their category's `TEMPLATE.md`** (in each directory).
- New decision → new ADR with the next number; if it reverses an old one, mark
  the old one `Superseded by NNNN` rather than editing it.
- Design docs are updated in place to always reflect reality.
- Specs change only alongside the code that implements them.
- Cross-links between categories are mandatory in both directions — see the
  documentation rules in [AGENTS.md](../AGENTS.md) (CLAUDE.md, GEMINI.md,
  .cursorrules and .github/copilot-instructions.md are symlinks to it,
  ADR-0002).
- Adding a doc means adding it to the index above.
