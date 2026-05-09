#!/bin/bash
# Start readingdb daemon in the background
readingdb &
# Wait for readingdb to start
sleep 2
# Start smap archiver
exec twistd -n smap-archiver python/conf/archiver.ini
