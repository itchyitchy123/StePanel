#!/usr/bin/env bash
set -Eeuo pipefail
if [[ $EUID -ne 0 ]]; then echo "Run as root: sudo ./install.sh" >&2; exit 1; fi
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"; APP_USER="stepanel"; APP_DIR="/opt/stepanel"; DATA_DIR="/var/lib/ste-panel"; ENV_FILE="/etc/ste-panel.env"
if [[ ! -f "$ROOT_DIR/stepanel" || ! -x "$ROOT_DIR/stepanel" ]]; then echo "Build an executable stepanel binary before installing." >&2; exit 1; fi
ADMIN_USERNAME="${STEPANEL_ADMIN_USERNAME:-admin}"; ADMIN_PASSWORD="${STEPANEL_ADMIN_PASSWORD:-}"; SESSION_SECRET="${STEPANEL_SESSION_SECRET:-}"
if [[ -z "$ADMIN_PASSWORD" && -t 0 ]]; then read -r -s -p "StePanel admin password: " ADMIN_PASSWORD; echo; fi
if [[ -z "$ADMIN_PASSWORD" ]]; then echo "Set STEPANEL_ADMIN_PASSWORD or run the installer interactively." >&2; exit 1; fi
if [[ -z "$SESSION_SECRET" ]]; then SESSION_SECRET="$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')"; fi
if [[ ! "$ADMIN_USERNAME" =~ ^[a-zA-Z0-9._-]{1,64}$ || "$ADMIN_USERNAME" == *$'\n'* || "$ADMIN_USERNAME" == *$'\r'* ]]; then echo "Invalid admin username." >&2; exit 1; fi
if [[ "$ADMIN_PASSWORD" == *$'\n'* || "$ADMIN_PASSWORD" == *$'\r'* || "$SESSION_SECRET" == *$'\n'* || "$SESSION_SECRET" == *$'\r'* ]]; then echo "Credentials may not contain newlines." >&2; exit 1; fi
source /etc/os-release
WEB_GROUP="www-data"; if getent group apache >/dev/null 2>&1; then WEB_GROUP="apache"; fi
if command -v apt-get >/dev/null; then export DEBIAN_FRONTEND=noninteractive; apt-get update; apt-get install -y apache2 mysql-server php php-cli php-mysql php-curl php-mbstring php-xml tar gzip ca-certificates; APACHE_SERVICE=apache2; DB_SERVICE=mysql
elif command -v dnf >/dev/null; then dnf install -y httpd mariadb-server php php-cli php-mysqlnd php-curl php-mbstring php-xml tar gzip ca-certificates; APACHE_SERVICE=httpd; DB_SERVICE=mariadb
else echo "Unsupported operating system: $ID" >&2; exit 1; fi
install -d -m 0750 "$APP_DIR" "$DATA_DIR/imports" /var/www/sites; id "$APP_USER" >/dev/null 2>&1 || useradd --system --home-dir "$APP_DIR" --shell /usr/sbin/nologin "$APP_USER"; usermod -a -G "$WEB_GROUP" "$APP_USER"
install -m 0755 "$ROOT_DIR/stepanel" "$APP_DIR/stepanel"; install -m 0644 -D "$ROOT_DIR/web/index.html" "$APP_DIR/web/index.html"; install -m 0644 -D "$ROOT_DIR/web/static/app.css" "$APP_DIR/web/static/app.css"
install -m 0644 -D "$ROOT_DIR/web/static/import.css" "$APP_DIR/web/static/import.css"
if [[ "$APACHE_SERVICE" == "apache2" ]]; then install -m 0644 "$ROOT_DIR/deploy/apache/stepanel.conf" /etc/apache2/sites-available/stepanel.conf; a2enmod proxy proxy_http headers >/dev/null; a2ensite stepanel >/dev/null; fi
chown -R "$APP_USER:$APP_USER" "$APP_DIR" "$DATA_DIR"; chown "$APP_USER:$WEB_GROUP" /var/www/sites; chmod 2750 /var/www/sites; printf 'STEPANEL_ENV=production\nSTEPANEL_LISTEN=127.0.0.1:8090\nSTEPANEL_ADMIN_USERNAME=%s\nSTEPANEL_ADMIN_PASSWORD=%s\nSTEPANEL_SESSION_SECRET=%s\nSTEPANEL_IMPORT_ROOT=%s/imports\nSTEPANEL_WEB_ROOT=/var/www\nSTEPANEL_AUDIT_LOG=%s/audit.jsonl\n' "$ADMIN_USERNAME" "$ADMIN_PASSWORD" "$SESSION_SECRET" "$DATA_DIR" "$DATA_DIR" > "$ENV_FILE"; chmod 0600 "$ENV_FILE"
install -m 0644 "$ROOT_DIR/deploy/stepanel.service" /etc/systemd/system/stepanel.service; systemctl daemon-reload; systemctl enable --now "$APACHE_SERVICE" "$DB_SERVICE" stepanel; systemctl reload "$APACHE_SERVICE" || true
echo "StePanel installed. Put Apache in front of http://127.0.0.1:8090 and enable HTTPS before exposing it."
