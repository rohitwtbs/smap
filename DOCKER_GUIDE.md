# sMAP — Docker Build & Run Guide

This is a complete, opinionated guide for building and running **sMAP**
(Simple Measurement and Actuation Profile) on your own machine using the
provided `Dockerfile` and `docker-compose.yml`. It is derived from a full
read of the project's documentation under `python/doc/en/2.0/` and the
configuration files under `python/conf/`.

> **Why Docker?** sMAP is a Python **2.7-only** codebase last actively
> maintained around 2014. Modern OSes no longer ship Python 2.7, and several
> of its dependencies (`avro==1.6.x`, old `Twisted`, `psycopg2` for old
> Postgres, etc.) are difficult to install natively in 2026. The provided
> `Dockerfile` (based on `python:2.7-slim` / Debian Buster) is the cleanest
> way to get a reproducible build.
>
> **Heads-up about Replit:** Docker cannot run inside the Replit workspace
> (no nested daemon). Run everything in this guide on your **own machine**
> (laptop, VM, or CI runner) that has a real Docker engine.

---

## 1. What is sMAP? (1-minute orientation)

From `python/doc/en/2.0/intro.rst`, sMAP has three logical pieces:

| Piece | Role | Process / Port |
|---|---|---|
| **sMAP Sources** | Drivers that talk to instruments (Modbus, BACnet, XML/HTTP, CSV, weather APIs, ISO price feeds…) and republish them as sMAP time series. | `twistd -n smap <driver.ini>` — defaults to **HTTP 8080** |
| **sMAP Archiver** | Historian: receives data from sources, stores metadata in **PostgreSQL** and raw time-series in **readingdb**, exposes the **ArchiverQuery** REST API. | `twistd -n smap-archiver /etc/smap/archiver.ini` — **HTTP 8079** |
| **powerdb2** | Django front-end for plotting / browsing streams (lives in a separate SVN repo — *not* in this repo). | Django dev server — typically **HTTP 8000** |

Data model (`intro.rst` "Key Concepts"):
- A **Timeseries** is a single stream of scalar readings, named by a 128-bit **UUID**.
- A **Collection** is a (nestable) set of Timeseries.
- Both are tagged with **Metadata** key/value pairs under
  `Metadata/Instrument/*`, `Metadata/Location/*`, `Metadata/Extra/*`
  (see `tags.rst`).

---

## 2. What's in this repository

```
.
├── Dockerfile               # python:2.7-slim + build-essential + libssl/libpq
├── docker-compose.yml       # single "smap" service mounting the repo at /opt/smap
├── python/
│   ├── setup.py             # distutils install for the smap package
│   ├── requirements.txt     # twisted, avro, configobj, pyOpenSSL, ply, autobahn, dateutil
│   ├── osx_requirements.txt # same set, OSX-tuned
│   ├── smap/                # the library: core.py, server.py, archiver/, drivers/, iface/ …
│   ├── bin/                 # CLI tools: smap-query, smap-tool, smap-load, smap-load-csv,
│   │                        # smap-run-driver, smap-run-conf, smap-monitize,
│   │                        # smap-reporting, smap-subscribe, jprint, uuid, bacnet-scan
│   ├── conf/                # ~25 example .ini files (archiver.ini, example.ini,
│   │                        # caiso_price.ini, dent meters, pjm/isone/nyiso ISOs, …)
│   ├── supervisor/          # archiver.conf for supervisord
│   ├── monit/               # archiver service file for monit
│   ├── debian/              # debian packaging metadata
│   └── doc/en/2.0/          # the full reStructuredText documentation
├── R/        JavaSmap …    # client bindings for R and Java
├── schema/   *.av           # Avro schemas (timeseries, collection, reporting, …)
├── xslt/                    # XSLT transforms (greenbutton, obvius, ted5000)
└── discovery/               # auxiliary device-discovery helpers
```

---

## 3. Prerequisites on your build machine

- **Docker Engine** ≥ 20.10 and **Docker Compose v2** (`docker compose …`)
  or v1 (`docker-compose …`). The commands below show both.
- ~2 GB free disk for the image + a few hundred MB for build cache.
- Outbound internet during the build (Debian archive, PyPI).
- Linux/macOS/Windows-WSL2 all work; pure Windows containers do **not**.

---

## 4. The provided Dockerfile — what it does and its gotchas

```dockerfile
FROM python:2.7-slim
ENV PYTHONUNBUFFERED=1 PYTHONPATH=/opt/smap/python
WORKDIR /opt/smap

RUN sed -i 's|deb.debian.org|archive.debian.org|g' /etc/apt/sources.list && \
    sed -i 's|security.debian.org|archive.debian.org|g' /etc/apt/sources.list && \
    sed -i '/buster-updates/d' /etc/apt/sources.list && \
    apt-get update && apt-get install -y --no-install-recommends \
      build-essential python-dev libssl-dev libpq-dev && \
    rm -rf /var/lib/apt/lists/*

COPY . /opt/smap
RUN pip install --upgrade pip setuptools wheel && \
    pip install -r python/requirements.txt
RUN pip install 'bandit<1.7' 'safety<2.0'
CMD ["bash"]
```

What it does well:
- Pins to **`python:2.7-slim`** (Debian Buster) — the only realistic base.
- Rewrites apt sources to `archive.debian.org` because Buster is EOL,
  drops `buster-updates` which 404s.
- Installs the **build chain** needed for `pyOpenSSL`, `psycopg2`, `pycurl`,
  plus the Modbus C extension if you choose to enable it.
- Installs Python deps from `python/requirements.txt`.
- Lands you in a `bash` shell so you can run any of the sMAP CLIs by hand.

Gotchas / things that are intentionally **not** done by this Dockerfile:
1. It does **not** run `python setup.py install`. You import sMAP via
   `PYTHONPATH=/opt/smap/python`, and use the scripts in
   `/opt/smap/python/bin/` directly. This is fine for development; for a
   "production" image you may want to add `RUN cd /opt/smap/python && python setup.py install`.
2. The **Modbus** and **BACnet** C extensions in `setup.py` are commented
   out (see `ext_modules=[]`). If you need them you'll have to:
   - Modbus: uncomment `modbus_module` in `setup.py`, then `python setup.py build_ext --inplace`.
   - BACnet: download `bacnet-stack-0.6.0` from SourceForge, build it
     (must be GCC, not LLVM), then uncomment `bacnet_module`.
3. **readingdb is not in the image.** The archiver will not store
   time-series readings without it. See §7.
4. **PostgreSQL is not in the image.** The archiver needs it for metadata.
   See §7.
5. **powerdb2 is not in the image** and lives in a separate (now-defunct
   Google Code) SVN repo. See §8.

---

## 5. The provided docker-compose.yml — what it does and what's missing

```yaml
services:
  smap:
    build: { context: ., dockerfile: Dockerfile }
    image: smap:local
    volumes:
      - .:/opt/smap:cached      # live source mount (edits visible inside)
    working_dir: /opt/smap
    environment:
      PYTHONUNBUFFERED: '1'
      PYTHONPATH: /opt/smap/python
    tty: true
```

This compose file gives you one shell container with the source mounted.
It deliberately doesn't expose ports, doesn't add PostgreSQL, and doesn't
add readingdb. That's the right starting point for "smoke test the
library and run a single source"; for the **full archiver** you'll want
the extended compose file in §7.

---

## 6. Quick start — build the image and run a sMAP **source** (no archiver)

This is the "Task 1" path from `python/doc/en/2.0/tutorial.rst`.

```bash
# 1) Build
docker compose build          # or: docker-compose build

# 2) Open an interactive shell in the container (publish 8080 ad-hoc).
#    Note: do NOT combine --service-ports with -p; they are mutually exclusive.
docker compose run --rm -p 8080:8080 smap bash

# 3) Inside the container — sanity check that the library imports and
#    the twistd plugin is registered
root@xxxx:/opt/smap# python -c "import smap, smap.core; print(smap.core.__file__)"
root@xxxx:/opt/smap# twistd --help | grep -E 'smap|smap-archiver'
    smap             A sMAP server
    smap-archiver    A sMAP archiver

# 4) Run the bundled "example" source (counter driver + 3 file actuators)
root@xxxx:/opt/smap# cd python
root@xxxx:/opt/smap/python# touch ~/binary_actuator.txt ~/discrete_actuator.txt ~/continuous_actuator.txt
root@xxxx:/opt/smap/python# twistd -n smap conf/example.ini
# → "Site starting on 8080"

# 5) From your *host* machine, hit it:
$ curl http://localhost:8080/data/+ | python -m json.tool
$ curl http://localhost:8080/data/instrument0/sensor0 | python -m json.tool
```

Useful one-liner variants:

```bash
# Run the California ISO price-feed driver from the tutorial
twistd -n smap conf/caiso_price.ini

# Run with a custom port via [server] section in any .ini
# (default 8080; see internals.rst "Server Section")

# Generate a brand-new UUID for a new config file
python/bin/uuid
```

`smap-tool` and `jprint` are also on the container's `$PATH` under
`python/bin/`:

```bash
python/bin/smap-tool -l http://localhost:8080
curl -s http://localhost:8080/data/+ | python/bin/jprint
```

---

## 7. Full archiver stack — extended docker-compose

The single-container compose file is enough to run **sources**, but the
**archiver** needs three more components per `archiver_manual.rst`:

| Component | Why | Default port |
|---|---|---|
| **PostgreSQL** with the `hstore` extension | Stores metadata / tags for the ArchiverQuery language | 5432 |
| **readingdb** | High-performance time-series store | 4242 |
| **smap-archiver** itself | Twisted service that ties them together | 8079 |

`python/conf/archiver.ini` shows the configurable settings:

```ini
[server]
  [[default]]
  port = 8079               ; archiver HTTP API

[database]
host = 127.0.0.1            ; postgres host (override per environment)
port = 5432
db   = archiver
user = archiver
password = password

[readingdb]
host = localhost            ; (override per environment)
port = 4242
```

**readingdb is not on PyPI and is not in the base image.** Per
`archiver_manual.rst`, it's built from <https://github.com/stevedh/readingdb>.
There is **no maintained Docker image** for it, so you have two options:

- **Easy path:** skip readingdb and use the archiver's MongoDB or
  republish-only mode. You'll be able to use the **query/tag** features
  of the archiver but storing/retrieving raw readings will fail.
- **Full path:** build a readingdb image yourself. A skeleton Dockerfile
  is in §7.3 below. Expect to spend an hour or two on this — it's the
  hard part of the install.

### 7.1 Extended `docker-compose.yml` (recommended starting point)

Save this as `docker-compose.archiver.yml` next to the existing
`docker-compose.yml`:

```yaml
services:
  postgres:
    image: postgres:11        # last release that matches the era of this code
    environment:
      POSTGRES_USER: archiver
      POSTGRES_PASSWORD: password
      POSTGRES_DB: archiver
    volumes:
      - pgdata:/var/lib/postgresql/data
      - ./docker/postgres-init.sql:/docker-entrypoint-initdb.d/00-hstore.sql:ro
    ports: ["5432:5432"]

  readingdb:
    build:
      context: ./docker/readingdb
    ports: ["4242:4242"]
    volumes:
      - readingdb_data:/var/lib/readingdb

  smap:
    build: { context: ., dockerfile: Dockerfile }
    image: smap:local
    depends_on: [postgres, readingdb]
    volumes:
      - .:/opt/smap:cached
      - ./docker/archiver.ini:/etc/smap/archiver.ini:ro
    working_dir: /opt/smap
    environment:
      PYTHONUNBUFFERED: "1"
      PYTHONPATH: /opt/smap/python
    ports: ["8079:8079", "8080:8080"]
    command: >
      bash -lc "
        until pg_isready -h postgres -U archiver; do sleep 1; done;
        twistd -n smap-archiver /etc/smap/archiver.ini
      "

volumes:
  pgdata:
  readingdb_data:
```

### 7.2 `docker/postgres-init.sql` (creates `hstore`)

```sql
CREATE EXTENSION IF NOT EXISTS hstore;
```

This replaces the manual `psql` steps in `archiver_manual.rst`.

### 7.3 `docker/readingdb/Dockerfile` (skeleton)

```dockerfile
FROM debian:buster-slim
RUN sed -i 's|deb.debian.org|archive.debian.org|g' /etc/apt/sources.list && \
    sed -i 's|security.debian.org|archive.debian.org|g' /etc/apt/sources.list && \
    sed -i '/buster-updates/d' /etc/apt/sources.list && \
    apt-get update && apt-get install -y --no-install-recommends \
      build-essential autoconf libtool pkg-config git swig check \
      libdb-dev libprotobuf-c-dev protobuf-c-compiler zlib1g-dev \
      python python-dev python-numpy && \
    rm -rf /var/lib/apt/lists/*
WORKDIR /src
RUN git clone https://github.com/stevedh/readingdb.git && \
    cd readingdb && autoreconf --install && ./configure --prefix=/usr && \
    make && make install && \
    cd iface_bin && make && make install
EXPOSE 4242
VOLUME /var/lib/readingdb
CMD ["readingdb-server", "-d", "/var/lib/readingdb"]
```

> Note: readingdb's upstream build can be brittle. If `autoreconf` or the
> protobuf-c step fails you may need to pin to a specific commit. This is
> the single biggest source of friction in the whole stack — and the
> reason most users only use the **library** part of sMAP today.

### 7.4 `docker/archiver.ini` (mounted into `/etc/smap/`)

Copy `python/conf/archiver.ini` and override hosts so they resolve to
the compose service names:

```ini
[server]
  [[default]]
  port = 8079
  interface = 0.0.0.0

[database]
host = postgres
port = 5432
db = archiver
user = archiver
password = password

[readingdb]
host = readingdb
port = 4242
```

### 7.5 Bring the stack up

```bash
docker compose -f docker-compose.archiver.yml up --build
# wait until you see "Site starting on 8079" from the smap container

# from the host:
curl http://localhost:8079/                     # → {"Contents": ["add","api","republish"]}
curl http://localhost:8079/api                  # → {"Contents": ["streams","query","data",…]}
curl http://localhost:8079/api/query            # → ["Path","Properties/UnitofMeasure",…]
```

### 7.6 Point a source at the archiver

In any source config (e.g. a copy of `python/conf/example.ini`), add a
report section pointing at the archiver. You'll need an **API key**:
without `powerdb2` (see §8) the simplest way is to insert one directly
via SQL into the `subscription` table once the archiver has created its
schema on first start, or to enable the open-add path in `archiver.ini`.

```ini
[report 0]
ReportDeliveryLocation = http://archiver-host:8079/add/<APIKEY>
ReportResource = /+
```

Then run the source in the same container (or a sibling one):

```bash
docker compose -f docker-compose.archiver.yml exec smap \
    twistd -n smap python/conf/example.ini
```

Verify ingestion with `smap-query`:

```bash
docker compose -f docker-compose.archiver.yml exec smap \
    python/bin/smap-query -u http://localhost:8079/api/query
query> select *
query> select distinct Metadata/SourceName
query> select data before now where has Path
```

(Query language full reference: `archiver.rst` "Query Language" section,
which covers `select / delete / set`, the `where` operators `=`, `like`,
`~`, `has`, `and`, `or`, `not`, plus relative time expressions like
`now -5minutes`.)

---

## 8. The powerdb2 front-end (optional)

Per `archiver_manual.rst`, powerdb2 is a Django app that originally
lived at `http://smap-data.googlecode.com/svn/branches/powerdb2`.
Google Code is dead, so you'll need a mirror; the
[SoftwareDefinedBuildings org on GitHub](https://github.com/SoftwareDefinedBuildings)
has historical forks. Once obtained:

```bash
pip install django==1.4 django-piston avro python-dateutil
# edit settings.py: database host/user/pass
python manage.py syncdb         # creates admin user
python manage.py runserver 0.0.0.0:8000
```

Visit `http://localhost:8000`, log in, and go to
`/admin/smap/subscription/add` to mint an API key. That key is what you
plug into `ReportDeliveryLocation = http://archiver:8079/add/<key>` in
your source configs.

> Realistically powerdb2 is the most decayed piece of sMAP. Most modern
> users either skip it or build a custom Grafana / dashboard front-end
> directly against the `api/query` and `api/data` REST endpoints
> documented in `archiver.rst`.

---

## 9. Useful CLI tools (all present in `python/bin/`)

| Command | Doc | Purpose |
|---|---|---|
| `twistd -n smap <conf.ini>` | `tutorial.rst` | Run a sMAP **source** in the foreground |
| `twistd -n smap-archiver /etc/smap/archiver.ini` | `archiver_manual.rst` | Run the **archiver** in the foreground |
| `smap-query -u <url>` | `tools.rst` | Interactive ArchiverQuery REPL |
| `smap-tool -l <url>` | `tools.rst` | Inspect a running source: list latest readings |
| `smap-tool -c <dest> <src>` | `tools.rst` | Add a report destination dynamically (POST `/reports`) |
| `smap-tool -d <reportid> <src>` | `tools.rst` | Remove a report destination |
| `smap-load -s <start> -e <end> <conf.ini>` | `tools.rst` | Backfill historical data through a driver's `load()` method |
| `smap-load-csv --report-dest=… file.csv` | `tools.rst` | Bulk-import a CSV into the archiver |
| `smap-monitize <conf.ini>` | `tools.rst` | Generate `monit` service files for production deploys |
| `smap-run-driver`, `smap-run-conf` | (in `bin/`) | Lower-level launchers used by `smap-monitize` |
| `smap-reporting`, `smap-subscribe` | (in `bin/`) | Manage report subscriptions on running servers |
| `jprint` | `tutorial.rst` | Pretty-print JSON (used in many docs examples) |
| `uuid` | `internals.rst` | Generate a new UUID for a source root |
| `bacnet-scan` | `driver_index.rst` | Scan a BACnet network (requires BACnet ext module) |

Run any of them inside the container, e.g.:

```bash
docker compose exec smap python/bin/smap-query -u http://localhost:8079/api/query
```

---

## 10. Writing your own source (cheat sheet from `tutorial.rst` / `drivers.rst`)

Minimal config (`internals.rst` "Root Section"):

```ini
[/]
uuid = 75503ac2-abf0-11e0-b7d6-0026bb56ec92    # mandatory; generate with python/bin/uuid
Metadata/SourceName = My First Source

[server]
port = 8080
DataDir = /var/run/smap         # where on-disk report buffers live

[/instrument0]
type = smap.driver.BaseDriver   # the test driver: a 1 Hz counter
StartVal = 10
Metadata/Instrument/Manufacturer = sMAP Implementer Forum

[report 0]
ReportDeliveryLocation = http://archiver:8079/add/<APIKEY>
ReportResource = /+
```

Minimal driver subclass (`tutorial.rst` "Task 2"):

```python
import time
from smap import driver, util

class MyDriver(driver.SmapDriver):
    def setup(self, opts):
        self.add_timeseries('/sensor0', 'V')
        self.set_metadata('/sensor0',
            {'Instrument/ModelName': 'ExampleInstrument'})
        self.counter = int(opts.get('StartVal', 0))

    def start(self):
        util.periodicSequentialCall(self.read).start(1)

    def read(self):
        self.add('/sensor0', time.time(), self.counter)
        self.counter += 1
```

For periodic HTTP scraping use `smap.driver.FetchDriver` and override
`process(data)` (`drivers.rst` "Periodic scraping").

For XML sources use `smap.drivers.xslt.XMLDriver` with an XSLT stylesheet
(examples in `xslt/`: `greenbutton.xsl`, `obvius.xsl`, `ted5000.xsl`).

For actuation, subclass `smap.actuate.BinaryActuator`,
`NStateActuator`, `IntegerActuator`, or `ContinuousActuator` and
implement `get_state()` / `set_state()` — see `drivers.rst` "Actuation"
and `python/smap/drivers/file.py` for a complete example.

---

## 11. Talking to the archiver (clients)

### REST (any language) — `archiver.rst`

```bash
# Discover tag names
curl http://localhost:8079/api/query

# Get full metadata for one stream
curl http://localhost:8079/api/tags/uuid/<UUID>

# Last reading from streams matching a clause
curl -XPOST -d 'Metadata/Extra/Type = "oat"' \
     http://localhost:8079/api/query
# (or use the higher-level `select … where …` language via POST to /api/query)

# Live republish stream (long-lived HTTP)
curl -XPOST -d 'has Path' http://localhost:8079/republish

# Manual data publish (JSON Edition — archiver.rst §"Manual data publication")
curl -XPOST -H "Content-Type: application/json" -d @data.json \
     http://localhost:8079/add/<APIKEY>
```

### Python — `python_access.rst`

```python
from smap.archiver.client import SmapClient
from smap.contrib import dtutil
c = SmapClient("http://localhost:8079")
start = dtutil.dt2ts(dtutil.strptime_tz("1-1-2013", "%m-%d-%Y"))
end   = dtutil.dt2ts(dtutil.strptime_tz("1-2-2013", "%m-%d-%Y"))
uuids, data = c.data("Metadata/Extra/Type = 'oat'", start, end)
```

`SmapClient` also exposes `latest`, `prev`, `next`, `data_uuid`, `tags`.

### R — `R_access.rst`

```r
library(RSmap)
RSmap("http://localhost:8079")
data <- RSmap.data("Metadata/Extra/Type = 'oat'",
                   as.numeric(strptime("3-29-2013","%m-%d-%Y"))*1000,
                   as.numeric(strptime("3-31-2013","%m-%d-%Y"))*1000)
```

The R package source is in `R/RSmap_1.0.tar.gz`; install with
`R CMD install R/RSmap_1.0.tar.gz` (needs `RCurl`, `bitops`, `RJSONIO`).

### Java

A pre-built jar is in `java/JavaSmap_0.1.jar`; source under `java/JavaSmap`.

---

## 12. Operating the source/archiver in production (from `tools.rst`)

The repo ships two service-management options:

- **monit** (`python/monit/archiver`): pings the HTTP endpoint and
  restarts on failure. Wire it up with `smap-monitize <conf>`.
- **supervisord** (`python/supervisor/archiver.conf`):
  ```ini
  [program:archiver]
  command = /usr/bin/twistd -n smap-archiver --pidfile=/var/run/archiver.pid /etc/smap/archiver.ini
  autorestart = true
  user = smap
  stdout_logfile = /var/log/archiver.stdout.log
  stderr_logfile = /var/log/archiver.stderr.log
  ```
  In Docker you generally don't need either — let the container be the
  unit of supervision and rely on `restart: unless-stopped` in
  docker-compose. If you want multiple sources in one container, mount
  the supervisor config and run `supervisord -n` as `CMD`.

Report buffering (`tutorial.rst` "Buffering"): up to 10 000 points per
stream per destination are stored on disk under `[server] DataDir` so
they survive restarts. Buffers drain only on HTTP `200/201/204` from the
destination, and `ReportDeliveryLocation0..9` give you round-robin
backup destinations.

SSL (`internals.rst` "SSL Support"): both source servers and the
archiver accept `sslport`, `cert`, `key`, `ca`, and `verify` directives
in their `[server]` block.

---

## 13. Common build problems & fixes

| Symptom | Cause / Fix |
|---|---|
| `apt-get update` fails with 404 on `buster-updates` | Already fixed in the Dockerfile (line that removes `buster-updates`). If you forked an older version, apply the same sed. |
| `pip install pyOpenSSL` fails with `cryptography` build errors | Buster's OpenSSL is too new for the very old `cryptography` wheel. Pin: `pip install 'cryptography<3.4' 'pyOpenSSL<22'` before `pip install -r requirements.txt`. |
| `psycopg2` build fails | Make sure `libpq-dev` is installed (it is, in the Dockerfile). For a binary wheel: `pip install psycopg2-binary==2.8.6`. |
| `twistd` doesn't list `smap` / `smap-archiver` plugins | `PYTHONPATH` isn't pointing at `/opt/smap/python`, or `twisted/plugins/dropin.cache` is stale. Run `python -c "from twisted.plugin import IPlugin, getPlugins; list(getPlugins(IPlugin))"` once to regenerate. |
| `ImportError: No module named avro.schema` | `avro` 1.9+ dropped Python 2 — pin `pip install 'avro==1.8.2'`. |
| Archiver crashes "relation X does not exist" | Schema not initialized. The archiver auto-creates tables on first start; make sure the `hstore` extension is enabled **before** it starts (see `docker/postgres-init.sql`). |
| Modbus or BACnet drivers can't be imported | Their C extensions are commented out in `setup.py`. Re-enable them and rebuild (§4 gotchas). |

---

## 14. Useful reading order in `python/doc/en/2.0/`

If you want to go deeper than this guide:

1. `intro.rst` — high-level architecture (read first).
2. `install.rst` — historical install instructions (mostly superseded by Docker).
3. `tutorial.rst` — write & run your first source, configure reports.
4. `drivers.rst` — driver patterns: periodic scraping, XML/XSLT, actuation.
5. `internals.rst` — config file syntax, programmatic API.
6. `tags.rst` — the standard metadata tag namespace.
7. `archiver_install.rst` + `archiver_manual.rst` — the install dance this
   guide automates with Docker.
8. `archiver.rst` — the **ArchiverQuery REST API** and query language.
9. `python_access.rst`, `R_access.rst` — client bindings.
10. `tools.rst` — every command-line tool, with all flags.
11. `driver_index.rst` — catalog of every bundled driver (Dent, Veris,
    Modbus generic, BACnet generic, CalISO/ERCOT/NYISO/PJM/ISONE price
    feeds, Wunderground, NOAA forecast, EnLighted, Hue, Raritan, …).
12. `additional.rst` — papers, talks, mailing list.
13. `v2.tex` (root of repo) — the design document, if you want the
    full theoretical background.

---

## 15. TL;DR

```bash
# Source only:
docker compose build
docker compose run --rm --service-ports -p 8080:8080 smap \
    twistd -n smap python/conf/example.ini
curl localhost:8080/data/+ | python -m json.tool

# Full archiver (after writing the §7 extra files):
docker compose -f docker-compose.archiver.yml up --build
curl localhost:8079/api/query
```

You now have a working sMAP stack — happy archiving.
