#!/usr/bin/env bash
set -Eeuo pipefail
if [[ $EUID -ne 0 ]]; then echo "Run as root: sudo ./install.sh" >&2; exit 1; fi
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"; APP_USER="stepanel"; APP_DIR="/opt/stepanel"; DATA_DIR="/var/lib/ste-panel"; ENV_FILE="/etc/ste-panel.env"
source /etc/os-release
if command -v apt-get >/dev/null; then export DEBIAN_FRONTEND=noninteractive; apt-get update; apt-get install -y apache2 mysql-server php php-cli php-mysql php-curl php-mbstring php-xml tar gzip ca-certificates; APACHE_SERVICE=apache2; DB_SERVICE=mysql
elif command -v dnf >/dev/null; then dnf install -y httpd mariadb-server php php-cli php-mysqlnd php-curl php-mbstring php-xml tar gzip ca-certificates; APACHE_SERVICE=httpd; DB_SERVICE=mariadb
else echo "Unsupported operating system: $ID" >&2; exit 1; fi
install -d -m 0750 "$APP_DIR" "$DATA_DIR/imports" /var/www/sites; id "$APP_USER" >/dev/null 2>&1 || useradd --system --home-dir "$APP_DIR" --shell /usr/sbin/nologin "$APP_USER"
install -m 0755 "$ROOT_DIR/stepanel" "$APP_DIR/stepanel"; install -m 0644 -D "$ROOT_DIR/web/index.html" "$APP_DIR/web/index.html"; install -m 0644 -D "$ROOT_DIR/web/static/app.css" "$APP_DIR/web/static/app.css"
install -m 0644 -D "$ROOT_DIR/web/static/import.css" "$APP_DIR/web/static/import.css"
if [[ "$APACHE_SERVICE" == "apache2" ]]; then install -m 0644 "$ROOT_DIR/deploy/apache/stepanel.conf" /etc/apache2/sites-available/stepanel.conf; a2enmod proxy proxy_http headers >/dev/null; a2ensite stepanel >/dev/null; fi
chown -R "$APP_USER:$APP_USER" "$APP_DIR" "$DATA_DIR" /var/www/sites; printf 'STEPANEL_LISTEN=127.0.0.1:8090\nSTEPANEL_IMPORT_ROOT=%s/imports\nSTEPANEL_WEB_ROOT=/var/www\nSTEPANEL_AUDIT_LOG=%s/audit.jsonl\n' "$DATA_DIR" "$DATA_DIR" > "$ENV_FILE"; chmod 0600 "$ENV_FILE"
install -m 0644 "$ROOT_DIR/deploy/stepanel.service" /etc/systemd/system/stepanel.service; systemctl daemon-reload; systemctl enable --now "$APACHE_SERVICE" "$DB_SERVICE" stepanel
echo "StePanel installed. Put Apache in front of http://127.0.0.1:8090 and enable HTTPS before exposing it."
