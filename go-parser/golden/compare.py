#!/usr/bin/env python3
"""Golden comparison harness: parse each corpus query with the legacy
PLY parser and with the Go parser service, and compare the generated
SQL (and data specs).

Usage: python3 go-parser/golden/compare.py [--keys k1,k2] [--private]

Notes on normalization:
- Multi-tag SELECT column order comes from Python set iteration order,
  which varies with the hash seed, so select columns are compared as
  sorted sets.
- OR-groups inside apply formula restrictions likewise come from set
  iteration and are compared order-insensitively.
- Queries relative to 'now' embed timestamps in the data spec (never
  in SQL); those are compared with a 2-second tolerance.
"""
import json
import os
import re
import sys
import urllib.request

sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', 'python'))

from smap.archiver import settings
settings.conf = settings.load(os.environ.get('ARCHIVER_INI', '/tmp/archiver.ini'))

from smap.archiver import queryparse

GO_URL = os.environ.get('SMAP_GO_PARSER_URL', 'http://127.0.0.1:8081/parse')


class FakeRequest(object):
    def __init__(self, args):
        self.args = args

    def write(self, data): pass
    def finish(self): pass
    def registerProducer(self, a1, a2): pass
    def unregisterProducer(self): pass
    def setHeader(self, k, v): pass
    def getHeader(self, k): return None


def ply_parse(query, keys, private):
    args = {}
    if keys: args['key'] = list(keys)
    if private: args['private'] = []
    request = FakeRequest(args)
    qp = queryparse.QueryParser(request)
    ext, q = qp.parse(query)
    return q, request.args


def go_parse(query, keys, private):
    body = json.dumps({'query': query, 'key': list(keys),
                       'private': private}).encode()
    req = urllib.request.Request(GO_URL, data=body,
                                 headers={'Content-Type': 'application/json'})
    with urllib.request.urlopen(req, timeout=5) as resp:
        return json.loads(resp.read().decode())


def normalize_select_cols(sql):
    """Sort the column list between SELECT and FROM."""
    m = re.match(r'^SELECT (.*?) FROM (.*)$', sql, re.S)
    if not m:
        return sql
    cols = sorted(c.strip() for c in m.group(1).split(', '))
    return 'SELECT %s FROM %s' % ('; '.join(cols), m.group(2))


def normalize_or_groups(sql):
    """Sort clauses inside 'AND ((..) OR (..))' formula-restriction groups."""
    def fix(m):
        parts = sorted(m.group(1).split(' OR '))
        return ' AND (%s)' % ' OR '.join(parts)
    return re.sub(r" AND \((\(\(s\.metadata[^)]*\) ~ [^)]*\)\)(?: OR \(\(s\.metadata[^)]*\) ~ [^)]*\)\))*)\)", fix, sql)


def sql_equal(a, b):
    if a == b:
        return True
    if normalize_select_cols(a) == normalize_select_cols(b):
        return True
    return normalize_or_groups(a) == normalize_or_groups(b)


def times_close(a, b, tol=2000):
    return abs(float(a) - float(b)) <= tol


def compare(query, keys, private):
    try:
        ply_q, ply_args = ply_parse(query, keys, private)
    except Exception as e:
        return False, 'PLY failed: %s' % e
    try:
        go = go_parse(query, keys, private)
    except urllib.error.HTTPError as e:
        return False, 'Go returned %d: %s' % (e.code, e.read().decode())
    except Exception as e:
        return False, 'Go request failed: %s' % e

    kind = go.get('kind')
    if kind in ('select', 'data'):
        ply_sql = ply_q if not isinstance(ply_q, list) else ply_q[1]
        if not sql_equal(go['sql'], ply_sql):
            return False, 'SQL mismatch:\n  PLY: %r\n  Go:  %r' % (ply_sql, go['sql'])
        if kind == 'data':
            ds = go['dataSpec']
            for pk, gk, tol in (('starttime', 'start', 2000),
                                ('endtime', 'end', 2000),
                                ('limit', 'limit', 0),
                                ('streamlimit', 'streamlimit', 0)):
                pv = ply_args[pk][0]
                gv = ds[gk]
                if tol:
                    if not times_close(pv, gv, tol):
                        return False, '%s mismatch: PLY %r vs Go %r' % (pk, pv, gv)
                elif float(pv) != float(gv):
                    return False, '%s mismatch: PLY %r vs Go %r' % (pk, pv, gv)
        return True, None
    if kind == 'apply':
        # PLY apply result: q == [None, tag_query, data_query]
        if not (isinstance(ply_q, list) and len(ply_q) == 3):
            return False, 'PLY apply result has unexpected shape: %r' % (ply_q,)
        if not sql_equal(go['tagSql'], ply_q[1]):
            return False, 'tag SQL mismatch:\n  PLY: %r\n  Go:  %r' % (ply_q[1], go['tagSql'])
        if not sql_equal(go['dataSql'], ply_q[2]):
            return False, 'data SQL mismatch:\n  PLY: %r\n  Go:  %r' % (ply_q[2], go['dataSql'])
        return True, None
    return False, 'unexpected Go kind %r' % kind


def main():
    keys = []
    private = False
    if '--keys' in sys.argv:
        keys = sys.argv[sys.argv.index('--keys') + 1].split(',')
    if '--private' in sys.argv:
        private = True

    corpus = os.path.join(os.path.dirname(__file__), 'queries.txt')
    queries = []
    with open(corpus) as fp:
        for line in fp:
            line = line.strip()
            if line and not line.startswith('#'):
                queries.append(line)

    failures = 0
    for q in queries:
        ok, msg = compare(q, keys, private)
        status = 'PASS' if ok else 'FAIL'
        print('%s  %s' % (status, q))
        if not ok:
            failures += 1
            print('      %s' % msg)
    print()
    print('%d/%d passed' % (len(queries) - failures, len(queries)))
    return 1 if failures else 0


if __name__ == '__main__':
    sys.exit(main())
