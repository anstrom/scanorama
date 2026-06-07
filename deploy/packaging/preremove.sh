#!/bin/sh
#
# nfpm preremove: stop and disable the service on a real removal only.
#
# During an upgrade the old package's preremove runs before the new files unpack;
# stopping/disabling there would turn off a service the user had enabled. nfpm
# passes the package manager's native args: rpm sends "0" on final removal ("1"
# on upgrade), dpkg sends "remove" on removal ("upgrade" on upgrade).
set -e

if [ "${1:-}" = remove ] || [ "${1:-}" = 0 ]; then
  if command -v systemctl >/dev/null 2>&1; then
    systemctl stop scanorama.service    >/dev/null 2>&1 || true
    systemctl disable scanorama.service >/dev/null 2>&1 || true
  fi
fi

exit 0
