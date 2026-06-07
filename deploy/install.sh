#!/usr/bin/env bash
#
# Scanorama bare-metal installer: binary + systemd unit + production layout.
#
# Use this when installing from a release tarball (no .deb/.rpm). It is
# idempotent — safe to re-run to upgrade the binary or unit in place.
#
# Usage:
#   sudo ./deploy/install.sh [/path/to/scanorama]
#
# If the binary path is omitted, the script looks next to itself and in the
# current directory.
set -euo pipefail

BIN_DST="/usr/bin/scanorama"
UNIT_DST="/etc/systemd/system/scanorama.service"
CONF_DIR="/etc/scanorama"
CONF_FILE="${CONF_DIR}/config.yaml"
STATE_DIR="/var/lib/scanorama"
SVC_USER="scanorama"
SVC_GROUP="scanorama"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BINARY_SRC="${1:-}"

die() { echo "error: $*" >&2; exit 1; }
info() { echo "==> $*"; }

[ "$(id -u)" -eq 0 ] || die "must run as root (try: sudo $0)"

# Locate the binary: explicit arg, alongside the script, or in the cwd.
if [ -z "${BINARY_SRC}" ]; then
  for candidate in "${SCRIPT_DIR}/scanorama" "${SCRIPT_DIR}/../scanorama" "./scanorama"; do
    if [ -x "${candidate}" ]; then BINARY_SRC="${candidate}"; break; fi
  done
fi
[ -n "${BINARY_SRC}" ] && [ -x "${BINARY_SRC}" ] || \
  die "scanorama binary not found; pass its path: $0 /path/to/scanorama"

command -v nmap >/dev/null 2>&1 || \
  echo "warning: nmap not found on PATH — install it before scanning (e.g. apt-get install nmap)" >&2

info "Creating ${SVC_USER} system user and group"
getent group "${SVC_GROUP}" >/dev/null || groupadd --system "${SVC_GROUP}"
getent passwd "${SVC_USER}" >/dev/null || useradd --system --gid "${SVC_GROUP}" \
  --home-dir "${STATE_DIR}" --shell /usr/sbin/nologin --comment "Scanorama daemon" "${SVC_USER}"

info "Creating directories"
install -d -o "${SVC_USER}" -g "${SVC_GROUP}" -m 0755 "${STATE_DIR}"
install -d -o root -g root -m 0755 "${CONF_DIR}"

info "Installing binary -> ${BIN_DST}"
install -o root -g root -m 0755 "${BINARY_SRC}" "${BIN_DST}"

info "Installing systemd unit -> ${UNIT_DST}"
install -o root -g root -m 0644 "${SCRIPT_DIR}/scanorama.service" "${UNIT_DST}"

if [ ! -f "${CONF_FILE}" ]; then
  info "Installing default config -> ${CONF_FILE}"
  install -o root -g "${SVC_GROUP}" -m 0640 "${SCRIPT_DIR}/config.example.yaml" "${CONF_FILE}"
else
  info "Keeping existing config ${CONF_FILE}"
fi

if command -v setcap >/dev/null 2>&1 && command -v nmap >/dev/null 2>&1; then
  info "Granting raw-socket capability to the nmap binary"
  setcap cap_net_raw,cap_net_admin+eip "$(command -v nmap)"
else
  echo "warning: setcap or nmap missing — SYN/ACK/UDP scans stay unavailable until you run:" >&2
  echo "         setcap cap_net_raw,cap_net_admin+eip \"\$(command -v nmap)\"" >&2
fi

info "Reloading systemd"
systemctl daemon-reload

# Bootstrap the database via peer auth (the scanorama role authenticates as the
# scanorama OS user over the local socket — no password). Needs a running local
# PostgreSQL; if absent, print the manual steps instead of failing.
if command -v runuser >/dev/null 2>&1 && getent passwd postgres >/dev/null 2>&1 \
   && runuser -u postgres -- "${BIN_DST}" setup; then
  info "Enabling and starting the service"
  systemctl enable --now scanorama
  cat <<EOF

Scanorama installed and started. Check it with:
  systemctl status scanorama
EOF
else
  cat <<EOF

Scanorama installed, but the database was not bootstrapped (PostgreSQL not found
or not running). Once a local PostgreSQL is running, finish with:
  sudo -u postgres ${BIN_DST} setup
  sudo systemctl enable --now scanorama
EOF
fi
