---
name: sMAP QL parser parity quirks
description: Non-obvious PLY grammar/lexer behaviors that any reimplementation of the sMAP query language must replicate exactly
---

Quirks verified while golden-testing the Go parser against the legacy PLY parser:

- **Precedence is inverted**: in the PLY table, OR binds *tighter* than AND (`a and b or c` → `a AND (b or c)`), and NOT binds tightest. Any reimplementation must copy this, not conventional precedence.
- **NUMBER lexing**: leading `+`/`-` is part of the number only if digits follow immediately (`limit -1` → NUMBER(-1); `5 - 3` → literal `-`). Token match order is CMP, QSTRING, LVALUE, NUMBER, literals.
- **Timestamps floor to seconds**: `dt2ts` uses `utctimetuple()`, so all timerefs floor to whole seconds before ×1000 (ms input 1783371600500 becomes 1783371600000).
- **escape_string** (psycopg2 QuotedString, unconnected): doubles `'` and `\`, no `E` prefix.
- **Set-order nondeterminism**: multi-tag select column order and apply formula-restriction OR-groups come from Python set iteration (varies with hash seed). Golden comparisons must sort select columns / OR clauses; single-tag queries compare exactly.
- **Data limits**: `in` defaults limit 10000, `before`/`after` default 1, `-1` → 1e7 (float); streamlimit default 1000. `select data` without WHERE is a syntax error.
- `select <lvalue>` with an unknown tag is *valid* (returns nulls) — not an error.
- Golden harness lives at `go-parser/golden/compare.py`; FakeRequest needs write/finish/registerProducer/unregisterProducer/setHeader stubs for apply queries.
- Background processes started in a bash tool call die when the call ends — sidecars must run as workflows. Workflow port checks require binding 0.0.0.0, and port 8080 is taken by the example driver.

**Why:** these behaviors silently change generated SQL/results if not replicated; they cost several golden-test iterations to discover.
**How to apply:** consult before extending the Go parser (delete/set/apply execution) or porting more of the query path.
