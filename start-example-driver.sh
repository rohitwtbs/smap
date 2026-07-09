#!/bin/bash
# Runs the sMAP example driver, which publishes an incrementing counter
# every second to the archiver at http://localhost:5000/add/mykey.
# Usage: ./start-example-driver.sh
cd "$(dirname "$0")/python"
exec > >(tee /tmp/driver.log) 2>&1
exec env PYTHONPATH=. ../.pythonlibs/bin/twistd -n --pidfile= smap ../example-driver.ini
