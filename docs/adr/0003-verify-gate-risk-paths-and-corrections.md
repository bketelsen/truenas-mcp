# 0003 — Verify gate, risk paths and corrections

- **Status:** Accepted
- **Date:** 2026-11-05

## Context

A template repository needs one local gate for canonical instruction surfaces,
documentation links, policy files, an append-only knowledge inbox and copied
implementation code. Pull requests that touch governance or CI files need an
explicit risk label. Blocking reviews need one durable route into a future
guard.

## Decision

The repository exposes the `make verify`/`check`/`ci` triad. The CI job `verify`
is the required status check. `make verify` checks the template with Node.js
and `make`; `make check` runs the verify gate; `make ci` runs verify and, with
`BASE` set, the corrections append-only check.

The repository stores a strict `.github/risk-paths.json` list and runs the
pinned frostyard/core caller workflow. A matched pull request receives the
`risk:high` label. The list and caller are themselves high-risk paths.

The repository stores durable corrections in `.memory/corrections.jsonl`.
A blocker-to-guard rule requires a targeted test or an `AGENTS.md` rule in the
fixing pull request. The PR that fixes the blocker appends 1 corrections line.

The repository copies the risk-path library and corrections checker from
frostyard/core. `make verify` records and checks a SHA-256 pin for each copied
file. A core update is a copy from the pinned core commit, not a manual edit.

## Consequences

- Node.js 22 or later and `make` are required for local verification.
- `make verify` has no package install, `package.json` or third-party runtime dependency.
- A repository made from this template needs 2 manual steps: create the
  `risk:high` label and add a branch ruleset that requires `verify`. The template
  does not copy rulesets or labels.
- The vendored core files stay independent of project dependencies. Updates
  require a copy from core and a hash-pin update.
- A risk:high pull request declares tier 3 and waits for the repository's owner
  review policy.
- A blocking review produces a guard and a durable correction record.

## Alternatives considered

- **Unpinned shell or package-manager setup:** rejected because the template
  must be language-neutral and must need only Node.js 22 and `make`.
- **No risk-path list:** rejected because CI and policy files are not protected
  without an explicit high-risk path gate.
- **Local corrections without append-only validation:** rejected because
  corrections could disappear or change without review.
- **Copied and edited core implementation:** rejected because vendored files
  need byte-identical copies and verifiable updates.

## References

- Builds on: [ADR-0001](0001-record-architecture-decisions.md), [ADR-0002](0002-agent-portable-instruction-surface.md)
- Shapes: [`AGENTS.md`](../../AGENTS.md), [`Makefile`](../../Makefile), [`.github/risk-paths.json`](../../.github/risk-paths.json)
- Risk-path contract: [frostyard/core docs/specs/risk-paths.md](https://github.com/frostyard/core/blob/main/docs/specs/risk-paths.md)
- Risk label rationale: [frostyard/core docs/adr/0068-label-pull-requests-that-touch-high-risk-paths.md](https://github.com/frostyard/core/blob/main/docs/adr/0068-label-pull-requests-that-touch-high-risk-paths.md)
- Corrections surface rationale: [frostyard/core docs/adr/0018-org-wide-agent-instruction-and-knowledge-surfaces.md](https://github.com/frostyard/core/blob/main/docs/adr/0018-org-wide-agent-instruction-and-knowledge-surfaces.md)
- Verify triad rationale: [frostyard/core docs/adr/0044-expose-the-make-gate-triad-in-every-repository.md](https://github.com/frostyard/core/blob/main/docs/adr/0044-expose-the-make-gate-triad-in-every-repository.md)
