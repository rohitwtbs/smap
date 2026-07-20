# sMAP Archiver

## Overview
The sMAP (Simple Measurement and Actuation Profile) archiver — a legacy Python 2 / Twisted time-series data platform from UC Berkeley — ported to Python 3.12 and running in Replit. It ingests sensor time-series data over HTTP and serves queries through the sMAP query language.

## Current State
- Codebase (under `python/`) converted from Python 2 to Python 3 (lib2to3 plus manual fixes for `zope.interface.implements`, str/bytes handling in the Twisted web layer, and SQL escaping).
- Storage uses Replit PostgreSQL via a SQL shim (`python/readingdb.py`) instead of the legacy readingdb time-series database.
- Schema loaded from `docker/postgres-init.sql` (tables: subscription, stream, permission, republish, data). API key `mykey` is seeded.
- Archiver runs via the `sMAP Archiver` workflow (`./start-archiver.sh`) on port 5000.

## How It Works
- `start-archiver.sh` generates `/tmp/archiver.ini` from the `PG*` environment variables and launches `twistd -n smap-archiver`.
- `python/smap/compat.py` monkeypatches Twisted `Request.write`/`Request.process` for str/bytes compatibility.
- Data ingestion: `POST /add/mykey` with sMAP JSON report objects.
- Queries: `POST /api/query` with sMAP query language (e.g. `select *`, `select data in (t1, t2) where uuid = '...'`).

## Dashboard
- The base page (`/`) serves a live dashboard (`python/smap/archiver/static/dashboard.html`): stream stats, a table of all streams with latest values, and click-to-plot SVG charts of recent readings. Self-contained (no external CDNs), auto-refreshes every 10s.
- Logs panel: Archiver/Driver tabs, refreshed every 5s from `GET /logs?name=archiver|driver&lines=N`. Start scripts tee output to `/tmp/archiver.log` / `/tmp/driver.log`; the endpoint (LogsResource in `python/smap/archiver/server.py`) tail-reads the file and redacts API keys (`/add/<key>`) and credential patterns before serving.

## Go Query Parser (strangler-fig phase 1)
- `go-parser/` holds a standalone Go HTTP service that parses sMAP QL and emits the same SQL as the legacy PLY parser. Runs via the `Go Query Parser` workflow (`./go-parser/start-go-parser.sh`, port 9000, `POST /parse`, `GET /healthz`).
- The archiver calls it when `SMAP_GO_PARSER=1` (set by default in `start-archiver.sh`; URL override `SMAP_GO_PARSER_URL`). Client: `python/smap/archiver/goparse.py`; hook: `QueryParser.try_go_parse` in `queryparse.py`, executed off the reactor thread via `deferToThread` with a 15s circuit breaker. Any failure or unsupported kind (apply execution, delete, set, help) silently falls back to PLY.
- Golden test harness: `python3 go-parser/golden/compare.py [--keys k1,k2] [--private]` compares PLY vs Go on the corpus in `go-parser/golden/queries.txt` (requires the Go service running and `/tmp/archiver.ini` present).

## Sample Driver
- `./start-example-driver.sh` runs `smap.drivers.example.Driver` (config in `example-driver.ini`), which publishes an incrementing counter once per second to the archiver at `/add/mykey`. Verify with: `curl -XPOST -d "select data before now limit 5 where Metadata/SourceName = 'Example Driver'" localhost:5000/api/query`

## Key Files
- `start-archiver.sh` — startup script
- `python/smap/archiver/` — archiver server, API, query parser
- `python/readingdb.py` — Postgres-backed readingdb shim
- `python/twisted/plugins/smap_archiver_plugin.py` — twistd plugin
- `docker/postgres-init.sql` — database schema

## User Preferences
(none recorded yet)
