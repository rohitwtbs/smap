# sMAP Modernized (Docker + TimescaleDB)

This repository contains a modernized version of **sMAP 2.0**, optimized for Docker and high-performance time-series storage using **TimescaleDB**.

## Features

- **One-Command Setup:** Run the entire sMAP stack (Postgres, Archiver, and Example Source) with `docker compose`.
- **Modern Backend:** Replaced the legacy C++ `readingdb` with a **TimescaleDB** (Postgres) hypertable via a SQL shim.
- **Optimized Build:** Uses pre-compiled Debian packages for Python 2.7 to ensure fast and reliable builds.
- **Out-of-the-Box Ingestion:** Pre-configured to start collecting data from an example driver immediately.

---

## Quick Start

### 1. Build and Start the Stack
```bash
docker compose up --build --remove-orphans
```

### 2. Verify Components
- **Archiver API:** [http://localhost:8079/api/query](http://localhost:8079/api/query)
- **Example Source:** [http://localhost:8081/data/+](http://localhost:8081/data/+)

### 3. Query Archived Data
You can query the data being stored in TimescaleDB using the sMAP API:
```bash
curl -XPOST -d 'select *' http://localhost:8079/api/query
```

Or check the database directly:
```bash
docker compose exec postgres psql -U archiver -d archiver -c "SELECT * FROM data LIMIT 10;"
```

---

## Architecture

| Service | Port | Description |
| :--- | :--- | :--- |
| `postgres` | `5432` | TimescaleDB for metadata and time-series readings. |
| `archiver` | `8079` | sMAP Archiver service with SQL-based readingdb shim. |
| `source` | `8081` | Example sMAP driver reporting to the archiver. |

### Key Files
- `Dockerfile`: Optimized Python 2.7 environment.
- `docker-compose.yml`: Service orchestration.
- `python/readingdb.py`: The SQL shim that enables Postgres as a time-series backend.
- `docker/postgres-init.sql`: Automated schema and TimescaleDB initialization.

---

## Configuration

- **API Key:** The default API key is `mykey` (seeded in Postgres).
- **Archiver Settings:** Located in `docker/archiver.ini`.
- **Source Settings:** Located in `docker/example.ini`.

For full sMAP documentation, see [python/doc/en/2.0/](python/doc/en/2.0/).
