#!/bin/bash
# Clear twisted plugin cache (in case of old broken cache)
rm -f python/twisted/plugins/dropin.cache

# Start readingdb daemon in the background
reading-server &
# Wait for readingdb to start
sleep 2
# Start smap archiver
exec twistd -n smap-archiver python/conf/archiver.ini
