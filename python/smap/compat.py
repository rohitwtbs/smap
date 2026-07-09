"""Python 3 compatibility shims for twisted.web request handling.

Under Python 3, twisted.web delivers URL path segments, query
arguments, and request bodies as bytes, and Request.write() requires
bytes.  The sMAP codebase was written against the Python 2 behavior
where these were all (byte)strings.  Importing this module patches
twisted.web so the legacy string-based call sites keep working:

 - Request.write() transparently encodes str to utf-8 bytes.
 - Request.args keys/values are decoded to str before rendering.
"""
from twisted.web import http, server

_orig_write = http.Request.write

def _write(self, data):
    if isinstance(data, str):
        data = data.encode('utf-8')
    return _orig_write(self, data)

if not getattr(http.Request.write, '_smap_compat', False):
    _write._smap_compat = True
    http.Request.write = _write

_orig_process = server.Request.process

def _process(self):
    if self.args:
        self.args = {
            (k.decode('utf-8') if isinstance(k, bytes) else k):
            [(v.decode('utf-8') if isinstance(v, bytes) else v)
             for v in vs]
            for k, vs in self.args.items()}
    return _orig_process(self)

if not getattr(server.Request.process, '_smap_compat', False):
    _process._smap_compat = True
    server.Request.process = _process


def to_str(s):
    """Decode bytes to str (utf-8); pass anything else through."""
    if isinstance(s, bytes):
        return s.decode('utf-8')
    return s
