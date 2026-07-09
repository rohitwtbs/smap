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
exec > >(tee /tmp/archiver.log) 2>&1
exec python3 -W ignore .pythonlibs/bin/twistd -n --pidfile= smap-archiver "$CONF"
