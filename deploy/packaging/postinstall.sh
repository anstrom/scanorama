#!/bin/sh
#
# nfpm postinstall: create the service account, fix config ownership, grant nmap
# the raw-socket capability, bootstrap the local database, and (on a fresh
# install) enable and start the service. Idempotent.
set -e

SVC_USER="scanorama"
SVC_GROUP="scanorama"
STATE_DIR="/var/lib/scanorama"
CONF_FILE="/etc/scanorama/config.yaml"
PG_SUPERUSER="postgres"

# bootstrap_database creates the scanorama role and database via `scanorama setup`,
# run as the postgres superuser so peer authentication over the local socket
# succeeds. Returns non-zero (without failing the install) when PostgreSQL is not
# present or not yet initialized — e.g. an RPM host where the cluster needs a
# manual `postgresql-setup --initdb` first.
bootstrap_database() {
  command -v runuser >/dev/null 2>&1 || return 1
  getent passwd "${PG_SUPERUSER}" >/dev/null 2>&1 || return 1
  runuser -u "${PG_SUPERUSER}" -- /usr/bin/scanorama setup
}

if ! getent group "${SVC_GROUP}" >/dev/null 2>&1; then
  groupadd --system "${SVC_GROUP}"
fi
if ! getent passwd "${SVC_USER}" >/dev/null 2>&1; then
  useradd --system --gid "${SVC_GROUP}" --home-dir "${STATE_DIR}" \
    --shell /usr/sbin/nologin --comment "Scanorama daemon" "${SVC_USER}"
fi

# /var/lib/scanorama is also created by the unit's StateDirectory, but create it
# here too so a one-shot CLI run before first start has a home.
install -d -o "${SVC_USER}" -g "${SVC_GROUP}" -m 0755 "${STATE_DIR}"

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

# Detect an upgrade vs a fresh install. nfpm passes the package manager's native
# args: rpm sends a numeric count ($1>=2 on upgrade), dpkg sends "configure" with
# the previously-installed version in $2 (empty on a fresh install).
is_upgrade=false
if [ "${1:-0}" -ge 2 ] 2>/dev/null || [ -n "${2:-}" ]; then
  is_upgrade=true
fi

if [ "${is_upgrade}" = true ]; then
  # Pick up the new binary without disturbing an already-configured service.
  if command -v systemctl >/dev/null 2>&1; then
    systemctl try-restart scanorama.service >/dev/null 2>&1 || true
    # If a prior install never finished bootstrapping (the unit is not enabled),
    # try-restart is a no-op — remind how to finish rather than stay silent.
    if ! systemctl is-enabled scanorama.service >/dev/null 2>&1; then
      echo "Scanorama upgraded but not yet running. Finish setup with:"
      echo "  sudo -u postgres scanorama setup"
      echo "  sudo systemctl enable --now scanorama"
    fi
  fi
  exit 0
fi

# Fresh install: bootstrap the database, then enable and start the service. If
# PostgreSQL isn't ready, leave the service stopped and print the manual steps.
if bootstrap_database; then
  if command -v systemctl >/dev/null 2>&1; then
    systemctl enable --now scanorama.service >/dev/null 2>&1 || true
  fi
  echo "Scanorama installed and started. Check it with: systemctl status scanorama"
else
  echo "Scanorama installed, but the database was not bootstrapped (PostgreSQL not"
  echo "found or not initialized). Once PostgreSQL is running, finish with:"
  echo "  sudo -u postgres scanorama setup"
  echo "  sudo systemctl enable --now scanorama"
fi

exit 0
