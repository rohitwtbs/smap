"""
Copyright (c) 2011, 2012, Regents of the University of California
All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions 
are met:

 - Redistributions of source code must retain the above copyright
   notice, this list of conditions and the following disclaimer.
 - Redistributions in binary form must reproduce the above copyright
   notice, this list of conditions and the following disclaimer in the
   documentation and/or other materials provided with the
   distribution.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS 
FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL 
THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, 
INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES 
(INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR 
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) 
HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, 
STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) 
ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED 
OF THE POSSIBILITY OF SUCH DAMAGE.
"""
"""
@author Stephen Dawson-Haggerty <stevedh@eecs.berkeley.edu>
"""

import copy
import json
import os
import re

from twisted.internet import reactor, defer
from twisted.web import resource, server, static
from twisted.web.resource import NoResource
from twisted.python import log

from smap import compat
from smap import subscriber
from smap import util
from smap.server import RootResource, setResponseCode
from smap.core import SmapException
from smap.archiver import settings, data, api, republisher, transfer
from smap.archiver.querygen import build_authcheck

class DataResource(resource.Resource):
    """This resource manages the functionality of the add/ resource,
    which is how data is inserted into the system.  Data is POSTed to
    /add/[api key]; this resource checks that it is a valid key and
    then inserts it into the postgres and readingdb databases.
    """
    def __init__(self, db, republisher):
        self.db = db
        self.republisher = republisher
        self.data = data.SmapData(db)
        resource.Resource.__init__(self)

    def _add_data(self, subid, obj):
        """Add the data using the data interface"""
        if len(obj) == 0: return True
        return self.data.add(subid, obj)

    def _check_subscriber(self, request, subid):
        """Check that there was an API key, and republish"""
        if len(subid) == 1:
            # send the object to the republisher
            public = subid[0][1]
            subid = subid[0][0]
            obj = transfer.read(request)

            # we want to republish the non-munged version of the data,
            # but then if that fails it may kill further processing.
            d = self.republisher(compat.to_str(request.prepath[-1]), public, obj)
            util.push_metadata(obj)
            d.addCallback(lambda _: (subid, obj))

            return d
        else:
            raise SmapException("Invalid key\n", 404)

    def getChild(self, name, request):
        return self

    def render_POST(self, request):
        """Handle new data"""
        # first check if the api key is valid
        d = self.db.runQuery("SELECT id, public FROM subscription WHERE key = %s", 
                             (compat.to_str(request.prepath[-1]),))
        d.addCallback(lambda x: self._check_subscriber(request, x))

        # if so, add the data
        d.addCallback(lambda subid_obj: self._add_data(subid_obj[0], subid_obj[1]))
        def add_success(x):
            if not x:
                print(x)
                request.setResponseCode(500)
            settings.metrics.increment("add_count")
            request.finish()
        def add_error(x):
            # return a 500 so the sMAP server can hold onto the data
            # until things can be fixed.
            settings.metrics.increment("add_error_count")
            try:
                setResponseCode(request, x.value, 500)
                request.write(str(x.value))
                request.finish()
            except:
                pass

        # and finish the request
        d.addCallback(add_success)
        d.addErrback(add_error)
        return server.NOT_DONE_YET

class LogsResource(resource.Resource):
    """Serve the tail of the archiver/driver log files as JSON, for the
    dashboard log viewer.  GET /logs?name=archiver|driver&lines=N
    """
    isLeaf = True
    LOG_FILES = {
        'archiver': '/tmp/archiver.log',
        'driver': '/tmp/driver.log',
        'goparser': '/tmp/goparser.log',
        }
    MAX_READ = 256 * 1024

    # patterns whose matches are redacted before lines leave the server,
    # so the public dashboard never exposes API keys or credentials.
    REDACTIONS = [
        # API-key-bearing URL paths: /add/<key>
        (re.compile(r'(/add/)[^\s"\'/?#]+'), r'\1<redacted>'),
        # key=value style credentials
        (re.compile(r'((?:password|passwd|secret|token|apikey|api_key|key)'
                    r'\s*[=:]\s*)\S+', re.IGNORECASE), r'\1<redacted>'),
        ]

    @classmethod
    def redact(cls, line):
        for pat, repl in cls.REDACTIONS:
            line = pat.sub(repl, line)
        return line

    def render_GET(self, request):
        request.setHeader(b'Content-Type', b'application/json')
        # smap.compat decodes request.args keys/values to str
        name = request.args.get('name', ['archiver'])[0]
        try:
            nlines = int(request.args.get('lines', ['200'])[0])
        except (ValueError, TypeError):
            nlines = 200
        nlines = max(1, min(nlines, 1000))

        path = self.LOG_FILES.get(name)
        if path is None:
            request.setResponseCode(404)
            return json.dumps({'error': 'unknown log: ' + name}).encode('utf-8')
        if not os.path.exists(path):
            return json.dumps({'name': name, 'lines': [],
                               'note': 'log file not found (has the '
                                       'process been started?)'}).encode('utf-8')
        try:
            with open(path, 'rb') as fp:
                fp.seek(0, os.SEEK_END)
                size = fp.tell()
                fp.seek(max(0, size - self.MAX_READ))
                chunk = fp.read()
        except IOError as e:
            request.setResponseCode(500)
            return json.dumps({'error': str(e)}).encode('utf-8')
        text = chunk.decode('utf-8', 'replace')
        lines = text.splitlines()
        if size > self.MAX_READ and lines:
            lines = lines[1:]        # first line probably truncated
        lines = [self.redact(l) for l in lines[-nlines:]]
        return json.dumps({'name': name, 'lines': lines}).encode('utf-8')

def getSite(db, 
            resources=['add', 'api', 'republish', 'wsrepublish', 'static'],
            http_repub=None, websocket_repub=None, mongo_repub=None, pg_repub=None):
    """Get the twisted site for smap-archiver"""
    root = RootResource(value={'Contents': resources})
    if not http_repub:
        http_repub = republisher.ReResource(db)
    if not websocket_repub:
        websocket_repub = republisher.WebSocketRepublishResource(db)

    def repub_fn(*args):
        dl = []
        if 'republish' in resources:
            http_repub.republish(*copy.deepcopy(args))
        if 'wsrepublish' in resources:
            websocket_repub.republish(*copy.deepcopy(args))
        if mongo_repub:
            dl.append(mongo_repub.republish(*copy.deepcopy(args)))
        if pg_repub:
            dl.append(pg_repub.republish(*copy.deepcopy(args)))
        return defer.DeferredList(dl, fireOnOneErrback=True)

    if 'republish' in resources:
        root.putChild(b'republish', http_repub)
    if 'wsrepublish' in resources:
        root.putChild(b'wsrepublish', websocket_repub)
    if 'add' in resources:
        root.putChild(b'add', DataResource(db, repub_fn))
    if 'api' in resources:
        root.putChild(b'api', api.Api(db))
    if 'static' in resources:
        root.putChild(b'static', static.File('static'))

    # log tail endpoint for the dashboard
    root.putChild(b'logs', LogsResource())

    # serve the dashboard on the base page (no-cache so browsers always
    # pick up dashboard changes on refresh)
    dashboard_path = os.path.join(os.path.dirname(__file__),
                                  'static', 'dashboard.html')
    if os.path.exists(dashboard_path):
        class DashboardFile(static.File):
            def render_GET(self, request):
                request.setHeader(b'Cache-Control',
                                  b'no-cache, no-store, must-revalidate')
                request.setHeader(b'Pragma', b'no-cache')
                request.setHeader(b'Expires', b'0')
                return static.File.render_GET(self, request)
        root.putChild(b'', DashboardFile(dashboard_path,
                                         defaultType='text/html'))
    return server.Site(root)

