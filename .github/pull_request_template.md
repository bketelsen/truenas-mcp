<!-- The repository owner reviews risk:high pull requests. Branch from the
latest base branch. Reviews apply the repository's docs and PR rules. -->

## Summary

<!-- What changes and why, in a few sentences. Link the issue(s) this closes. -->

## Risk tier

Declare the highest applicable tier, never lower. A `level:L1` or `level:L2`
label, or a `risk:high` label (a path in `.github/risk-paths.json`), means
tier 3. A project can replace this scale in an ADR.

Scale: 1 low = docs, tests, comments; 2 medium = behaviour change in code; 3 high = security, data loss, releases or publishing, CI and permissions, or any `level:L1`, `level:L2` or `risk:high` label.

Risk tier: 1 — <!-- justification -->

## Docs housekeeping

<!-- Delete rows that do not apply. -->

- [ ] New docs started from their category's `TEMPLATE.md`
- [ ] Every new doc indexed in `docs/README.md`
- [ ] Cross-links added in both directions (ADR ↔ design ↔ spec ↔ plan)
- [ ] New significant decision recorded as an ADR first, in this PR
- [ ] New or edited ADRs are fact-only: context, decision and consequences
- [ ] Conformance aliases and canonical symlinks are unchanged; canonical targets are edited instead

## Verification

<!-- Paste evidence for every applicable gate. -->

- [ ] `make verify` green
- [ ] Revert test for a fix: the new test fails with the fix reverted; command: <!-- name it -->
- [ ] Workflows changed: every action is SHA-pinned with `# vX.Y.Z`, `permissions: {}`, and `persist-credentials: false`
- [ ] Blocking review point fixed: guard added and 1 file added under `.memory/corrections/`

## Safety notes

- [ ] This change keeps read-only mode fail-closed by default.
- [ ] This change does not introduce write-capable behavior.
- [ ] This change introduces or modifies write-capable behavior and documents the opt-in safety model.
- [ ] I removed API keys, private hostnames, and NAS-private details from logs, fixtures, and screenshots.
