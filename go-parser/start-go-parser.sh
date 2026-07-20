#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"
export GOFLAGS=-buildvcs=false
go build -o /tmp/smap-go-parser .
exec > >(tee /tmp/goparser.log) 2>&1
exec env GO_PARSER_PORT="${GO_PARSER_PORT:-9000}" /tmp/smap-go-parser
