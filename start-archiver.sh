#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"

CONF=/tmp/archiver.ini
cat > "$CONF" <<EOF
[server]
  [[default]]
  port = 5000
  interface = 0.0.0.0

[database]
host = ${PGHOST}
port = ${PGPORT}
db = ${PGDATABASE}
user = ${PGUSER}
password = ${PGPASSWORD}

[readingdb]
module = readingdb
EOF

export PYTHONPATH="$PWD/python"
# Go query-parser sidecar: enabled by default; the archiver silently
# falls back to the PLY parser if the service is off or unreachable.
export SMAP_GO_PARSER="${SMAP_GO_PARSER:-1}"
export SMAP_GO_PARSER_URL="${SMAP_GO_PARSER_URL:-http://127.0.0.1:9000/parse}"
exec > >(tee /tmp/archiver.log) 2>&1
exec python3 -W ignore .pythonlibs/bin/twistd -n --pidfile= smap-archiver "$CONF"
