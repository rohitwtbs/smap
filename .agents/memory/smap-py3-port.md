---
name: sMAP Python 3 port
description: Durable lessons from porting the Python 2 Twisted sMAP archiver to Python 3.12 on Replit
---

# sMAP Python 3 port — lessons

- **Twisted str/bytes boundary**: Twisted web on Py3 requires bytes for `putChild` paths, `request.write`, `request.prepath` segments, and websocket `sendMessage`. Instead of chasing every call site, `python/smap/compat.py` monkeypatches `Request.write` (encode str) and `Request.process` (decode args); remaining sites use `compat.to_str()`. **Why:** the codebase has hundreds of call sites; a boundary shim is far cheaper and centralizes the policy. **How to apply:** for any new resource/endpoint, keep putChild paths as bytes and decode `request.content.read()` / websocket payloads at entry.
- **psycopg2 `QuotedString(...).getquoted()` returns bytes on Py3.** Interpolating it into an SQL string with `%s` yields `b"'...'"` corruption (symptom: `syntax error at or near "b'='"`). `escape_string` in `smap/archiver/data.py` decodes to utf-8. Any new escaping must do the same.
- **`settings.metrics` only existed when statsd was configured** — a `_NullMetrics` no-op fallback in `smap/archiver/settings.py` prevents AttributeErrors. Don't remove it.
- **Autobahn websocket `onMessage` payload is bytes**; decode before feeding it to the query parser (fixed in republisher `onMessage`).
- **Testing recipe**: workflow `sMAP Archiver` runs `./start-archiver.sh` (port 5000, ini generated from PG env vars). Smoke test: `curl -XPOST -d 'select *' localhost:5000/api/query`; ingest via `POST /add/mykey` with sMAP JSON report; ws republish at `/wsrepublish` (text frames = where-clause topic updates).
- Nonfatal PLY warning at import ("rejected rule formula_where_clause") — pre-existing grammar quirk, safe to ignore.
