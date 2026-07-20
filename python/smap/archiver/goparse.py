"""Client for the Go query-parser sidecar service.

When the ``SMAP_GO_PARSER`` environment variable is truthy, the
archiver first sends each query to the Go parser service (default
``http://127.0.0.1:8081/parse``).  Any failure -- service down,
unsupported query kind, bad response -- causes a silent fallback to
the PLY parser, so enabling the flag never changes behavior.
"""
import os
import json
import logging
import urllib.request
import urllib.error

log = logging.getLogger('goparse')


def enabled():
    return os.environ.get('SMAP_GO_PARSER', '').lower() in ('1', 'true', 'yes', 'on')


def service_url():
    return os.environ.get('SMAP_GO_PARSER_URL', 'http://127.0.0.1:8081/parse')


class GoParseUnavailable(Exception):
    """Raised when the Go parser could not handle the query."""


# Circuit breaker: after a connection failure, skip the Go parser for
# BACKOFF seconds instead of re-incurring the connection wait on every
# query.
import time as _time
BACKOFF = 15.0
_down_until = 0.0


def available():
    return _time.time() >= _down_until


def _mark_down():
    global _down_until
    _down_until = _time.time() + BACKOFF


def parse_remote(query, keys, private, timeout=2.0):
    """Send a query to the Go parser.  Returns the decoded response
    dict, or raises GoParseUnavailable on any error.

    This performs blocking I/O and must not be called on the Twisted
    reactor thread; run it via deferToThread."""
    if not available():
        raise GoParseUnavailable('go parser in backoff after recent failure')
    body = json.dumps({
        'query': query,
        'key': list(keys),
        'private': bool(private),
    }).encode('utf-8')
    req = urllib.request.Request(service_url(), data=body,
                                 headers={'Content-Type': 'application/json'})
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return json.loads(resp.read().decode('utf-8'))
    except urllib.error.HTTPError as e:
        # 400 = syntax error according to the Go parser; the service
        # is healthy, so don't trip the circuit breaker.  Let the PLY
        # parser produce the canonical error message.
        raise GoParseUnavailable('go parser returned %d' % e.code)
    except Exception as e:
        _mark_down()
        raise GoParseUnavailable(str(e))
