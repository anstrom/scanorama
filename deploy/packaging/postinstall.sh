#!/bin/sh
#
# nfpm postinstall: create the service account, fix config ownership, grant nmap
# the raw-socket capability, and reload systemd. Idempotent.
set -e

SVC_USER="scanorama"
SVC_GROUP="scanorama"
STATE_DIR="/var/lib/scanorama"
CONF_FILE="/etc/scanorama/config.yaml"

if ! getent group "${SVC_GROUP}" >/dev/null 2>&1; then
  groupadd --system "${SVC_GROUP}"
fi
if ! getent passwd "${SVC_USER}" >/dev/null 2>&1; then
  useradd --system --gid "${SVC_GROUP}" --home-dir "${STATE_DIR}" \
    --shell /usr/sbin/nologin --comment "Scanorama daemon" "${SVC_USER}"
fi

# /var/lib/scanorama is also created by the unit's StateDirectory, but create it
# here too so a one-shot CLI run before first start has a home.
install -d -o "${SVC_USER}" -g "${SVC_GROUP}" -m 0750 "${STATE_DIR}"

# The packaged config ships as root:root; hand read access to the service group.
if [ -f "${CONF_FILE}" ]; then
  chown root:"${SVC_GROUP}" "${CONF_FILE}"
  chmod 0640 "${CONF_FILE}"
fi

# Let nmap open raw sockets (SYN/ACK/UDP scans) while scanorama stays unprivileged.
if command -v setcap >/dev/null 2>&1 && command -v nmap >/dev/null 2>&1; then
  setcap cap_net_raw,cap_net_admin+eip "$(command -v nmap)" || true
fi

if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload || true
fi

echo "Scanorama installed. Edit /etc/scanorama/config.yaml (set database.password),"
echo "then: sudo systemctl enable --now scanorama"

exit 0
