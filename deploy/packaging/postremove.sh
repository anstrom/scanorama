#!/bin/sh
#
# nfpm postremove: reload systemd after unit removal. The scanorama user, its
# data in /var/lib/scanorama, and /etc/scanorama/config.yaml are intentionally
# left in place so an uninstall does not destroy scan history or configuration.
set -e

if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload >/dev/null 2>&1 || true
fi

exit 0
