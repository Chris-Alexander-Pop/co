#!/bin/sh
mkdir -p /var/lib/co /var/cache/co/src /var/cache/ccache /var/cache/pacman/pkg
chmod 1777 /var/cache/co/src /var/cache/ccache
exec /usr/local/bin/co-queue
