#!/usr/bin/env bash
set -Eeuo pipefail

[[ $EUID -eq 0 ]] || { echo 'install smoke must run as root' >&2; exit 1; }
cd /work

export STEPANEL_ENV=production
export STEPANEL_LISTEN=127.0.0.1:8090
export STEPANEL_TLS_TERMINATED=1
export STEPANEL_ADMIN_PASSWORD=ci-install-only-password
export STEPANEL_ADMIN_TOTP_SECRET=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP
export STEPANEL_SESSION_SECRET=ci-install-session-secret-012345678901234567890123
export STEPANEL_AUDIT_KEY=ci-install-audit-key-012345678901234567890123456789
export STEPANEL_DB_ENGINE=${STEPANEL_DB_ENGINE:-mariadb}
export STEPANEL_WEBSERVER=${STEPANEL_WEBSERVER:-caddy}
export STEPANEL_DB_VERSION=default
export STEPANEL_PANEL_HOSTNAME=panel.example.test
export STEPANEL_INSTALL_DB_ADMIN=0
export STEPANEL_INSTALL_MAIL=0
export STEPANEL_INSTALL_FTP=0
export STEPANEL_INSTALL_NODE=0
export STEPANEL_INSTALL_SECURITY=0
export STEPANEL_REQUIRE_OFFSITE_BACKUP=1
export STEPANEL_OFFSITE_TARGET=local:/tmp/stepanel-offsite

./install.sh
systemctl is-active --quiet stepanel.service
curl --fail --silent --max-time 5 http://127.0.0.1:8090/livez >/dev/null
systemctl restart stepanel.service
systemctl is-active --quiet stepanel.service
curl --fail --silent --max-time 5 http://127.0.0.1:8090/readyz >/dev/null

# Exercise the installed site helpers and selected webserver configuration,
# not just the panel daemon. This is intentionally a synthetic site.
site=ci-smoke
/usr/local/sbin/stepanel-sitectl prepare "$site"
mkdir -p "/var/www/sites/$site/public"
printf '%s\n' 'smoke' > "/var/www/sites/$site/public/index.html"
/usr/local/sbin/stepanel-sitectl seal "$site"
/usr/local/sbin/stepanel-vhostctl apply "$site" ci-smoke.example.test
if [[ $STEPANEL_WEBSERVER == apache ]]; then
  apachectl -t 2>/dev/null || httpd -t
else
  caddy validate --config /etc/caddy/Caddyfile
fi
