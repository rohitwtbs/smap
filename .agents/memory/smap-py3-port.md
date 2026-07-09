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
- **avro 1.12 rejects unknown record keys** that legacy avro silently ignored — driver metadata must use exact schema field names (e.g. `Instrument/Model`, not `ModelName`), or `schema.validate` fails with SmapSchemaException. Schemas live in `python/smap/schema/*.av`.
- **Driver reporting path (Twisted HTTP client) needs bytes**: `Agent.request(b'POST', url_bytes, ...)` and body producers (`AsyncJSON`, formatters) must write bytes to the consumer.
- **Twisted `render_*` must return bytes** — returning `str` gives "Request did not return bytes" 500 pages that mask the real error (e.g. query syntax errors surfaced as 500 instead of 400).
- **sMAP QL data queries require a where clause**: `select data before now limit 1` alone is a syntax error; use `... where has uuid` to get latest reading per stream. PLY `p_error` receives `t=None` at EOF — must be guarded.
- **Dashboard**: served at `/` from `python/smap/archiver/static/dashboard.html` via `root.putChild(b'', static.File(...))` in archiver getSite; self-contained vanilla JS + SVG (no CDNs, works in Replit preview), polls `/api/query` every 10s.
- **Sample driver recipe**: `./start-example-driver.sh` runs `smap.drivers.example.Driver` via `twistd -n smap example-driver.ini`, publishing a 1 Hz counter to `/add/mykey`; verify with `select data before now limit 5 where Metadata/SourceName = 'Example Driver'`. Drivers write report-buffer state dirs under CWD (`python/<uuid>*`), gitignored.
