---
type: memory
category: decisions
id: DECISIONS-LOG
tags:
  - decisions
  - governance
created_at: YYYY-MM-DD
updated_at: YYYY-MM-DD
---

# Decision Log

> Append-only. Never delete or overwrite decisions.
> Only the Discovery Agent may add entries (or Bootstrap Agent during initialization).
> Format: one entry per decision, newest at top.
> For large projects, split by domain: `decisions/auth.md`, `decisions/api.md`, etc.

---

## Entry Template

```markdown
### DEC-YYYY-MM-DD-NN — [Decision Title]

**Context:** Why a decision was needed.
**Decision:** What was chosen.
**Rationale:** Why this option.
**Impact:** What it affects.
**Date:** YYYY-MM-DD
```

Next available ID: 3

---

### DEC-2 — Use TrueNAS official Go WebSocket JSON-RPC client
File: `decisions/DEC-2.md` | Domain: architecture | Level: architectural
Date: 2026-03-12

### DEC-1 — Use official modelcontextprotocol/go-sdk for MCP server
File: `decisions/DEC-1.md` | Domain: architecture | Level: architectural
Date: 2026-03-12

<!-- Add decisions above this line, newest first -->
