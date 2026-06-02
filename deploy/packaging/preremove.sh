#!/bin/sh
#
# nfpm preremove: stop and disable the service before the files go away.
set -e

if command -v systemctl >/dev/null 2>&1; then
  systemctl stop scanorama.service >/dev/null 2>&1 || true
  systemctl disable scanorama.service >/dev/null 2>&1 || true
fi

exit 0
