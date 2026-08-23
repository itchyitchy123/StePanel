#!/usr/bin/env bash
set -Eeuo pipefail

if [[ $EUID -ne 0 ]]; then echo "Run as root: sudo ./install.sh" >&2; exit 1; fi
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_USER="stepanel"; APP_DIR="/opt/stepanel"; DATA_DIR="/var/lib/ste-panel"; ENV_FILE="/etc/ste-panel.env"
if [[ ! -f "$ROOT_DIR/stepanel" || ! -x "$ROOT_DIR/stepanel" ]]; then echo "Build an executable stepanel binary before installing." >&2; exit 1; fi

ADMIN_USERNAME="${STEPANEL_ADMIN_USERNAME:-admin}"; ADMIN_PASSWORD="${STEPANEL_ADMIN_PASSWORD:-}"; SESSION_SECRET="${STEPANEL_SESSION_SECRET:-}"
DB_ENGINE="${STEPANEL_DB_ENGINE:-}"; DB_VERSION="${STEPANEL_DB_VERSION:-default}"
INSTALL_FAIL2BAN="${STEPANEL_INSTALL_FAIL2BAN:-0}"; FAIL2BAN_JAILS="${STEPANEL_FAIL2BAN_JAILS:-auto}"; FAIL2BAN_IGNORE_IP="${STEPANEL_FAIL2BAN_IGNORE_IP:-}"
FPM_LENS_BINARY="${STEPANEL_FPM_LENS_BINARY:-}"
INSTALL_MODSEC="${STEPANEL_INSTALL_MODSEC:-0}"; MODSEC_MODE="${STEPANEL_MODSEC_MODE:-DetectionOnly}"
INSTALL_MAIL="${STEPANEL_INSTALL_MAIL:-0}"
INSTALL_NODE="${STEPANEL_INSTALL_NODE:-0}"; NODE_VERSIONS="${STEPANEL_NODE_VERSIONS:-20.18.0}"
INSTALL_SECURITY="${STEPANEL_INSTALL_SECURITY:-0}"
INSTALL_TLS="${STEPANEL_INSTALL_TLS:-0}"
if [[ -z "$ADMIN_PASSWORD" && -t 0 ]]; then read -r -s -p "StePanel admin password: " ADMIN_PASSWORD; echo; fi
if [[ -z "$ADMIN_PASSWORD" ]]; then echo "Set STEPANEL_ADMIN_PASSWORD or run the installer interactively." >&2; exit 1; fi
if [[ -z "$SESSION_SECRET" ]]; then SESSION_SECRET="$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')"; fi
if [[ ! "$ADMIN_USERNAME" =~ ^[a-zA-Z0-9._-]{1,64}$ || "$ADMIN_USERNAME" == *$'\n'* || "$ADMIN_USERNAME" == *$'\r'* ]]; then echo "Invalid admin username." >&2; exit 1; fi
if [[ "$ADMIN_PASSWORD" == *$'\n'* || "$ADMIN_PASSWORD" == *$'\r'* || "$SESSION_SECRET" == *$'\n'* || "$SESSION_SECRET" == *$'\r'* ]]; then echo "Credentials may not contain newlines." >&2; exit 1; fi

source /etc/os-release
WEB_GROUP="www-data"; if getent group apache >/dev/null 2>&1; then WEB_GROUP="apache"; fi
if command -v apt-get >/dev/null; then PKG="apt"; APACHE_SERVICE="apache2"; elif command -v dnf >/dev/null; then PKG="dnf"; APACHE_SERVICE="httpd"; else echo "Unsupported operating system: $ID" >&2; exit 1; fi

if [[ -z "$DB_ENGINE" && -t 0 ]]; then
  read -r -p "Database engine [mysql/mariadb] (mysql): " DB_ENGINE
fi
DB_ENGINE="${DB_ENGINE:-mysql}"
if [[ "$DB_ENGINE" != "mysql" && "$DB_ENGINE" != "mariadb" ]]; then echo "STEPANEL_DB_ENGINE must be mysql or mariadb." >&2; exit 1; fi
if [[ -z "${STEPANEL_DB_VERSION:-}" && -t 0 ]]; then
  read -r -p "${DB_ENGINE} version (default distro version): " DB_VERSION
fi
DB_VERSION="${DB_VERSION:-default}"
if [[ ! "$DB_VERSION" =~ ^(default|[0-9][0-9A-Za-z.+:~-]*)$ ]]; then echo "Invalid database version: $DB_VERSION" >&2; exit 1; fi

if [[ "$DB_ENGINE" == "mysql" ]]; then DB_PACKAGE="mysql-server"; else DB_PACKAGE="mariadb-server"; fi
if [[ "$PKG" == "apt" ]]; then DB_SERVICE="${DB_ENGINE/mysql/mysql}"; [[ "$DB_ENGINE" == "mariadb" ]] && DB_SERVICE="mariadb"; export DEBIAN_FRONTEND=noninteractive; apt-get update; apt-get install -y apache2 php php-cli php-mysql php-curl php-mbstring php-xml tar gzip ca-certificates curl
else DB_SERVICE="$([[ "$DB_ENGINE" == "mysql" ]] && echo mysqld || echo mariadb)"; dnf install -y httpd php php-cli php-mysqlnd php-curl php-mbstring php-xml tar gzip ca-certificates curl; fi

if [[ "$DB_VERSION" == "default" ]]; then
  [[ "$PKG" == "apt" ]] && apt-get install -y "$DB_PACKAGE" || dnf install -y "$DB_PACKAGE"
elif [[ "$PKG" == "apt" ]]; then
  if ! apt-cache madison "$DB_PACKAGE" | awk '{print $3}' | grep -Fxq "$DB_VERSION"; then echo "Requested $DB_PACKAGE version $DB_VERSION is not available. Configure the appropriate repository first." >&2; apt-cache madison "$DB_PACKAGE" || true; exit 1; fi
  apt-get install -y "$DB_PACKAGE=$DB_VERSION"
else
  if ! dnf --assumeno install "$DB_PACKAGE-$DB_VERSION" >/dev/null 2>&1; then echo "Requested $DB_PACKAGE version $DB_VERSION is not available. Enable the appropriate DNF repository/module first." >&2; dnf list --showduplicates "$DB_PACKAGE" || true; exit 1; fi
  dnf install -y "$DB_PACKAGE-$DB_VERSION"
fi

if [[ "$INSTALL_FAIL2BAN" != "0" && "$INSTALL_FAIL2BAN" != "1" ]]; then echo "STEPANEL_INSTALL_FAIL2BAN must be 0 or 1." >&2; exit 1; fi
if [[ "$INSTALL_FAIL2BAN" == "1" && -z "$FAIL2BAN_IGNORE_IP" && -t 0 ]]; then read -r -p "Trusted management IPs/CIDRs for Fail2ban (required): " FAIL2BAN_IGNORE_IP; fi
if [[ "$INSTALL_FAIL2BAN" == "1" && -z "$FAIL2BAN_IGNORE_IP" ]]; then echo "Set STEPANEL_FAIL2BAN_IGNORE_IP before enabling Fail2ban; refusing an unattended lockout risk." >&2; exit 1; fi
if [[ "$FAIL2BAN_IGNORE_IP" == *$'\n'* || "$FAIL2BAN_IGNORE_IP" == *$'\r'* ]]; then echo "STEPANEL_FAIL2BAN_IGNORE_IP may not contain newlines." >&2; exit 1; fi
if [[ -n "$FPM_LENS_BINARY" && ! -x "$FPM_LENS_BINARY" ]]; then echo "STEPANEL_FPM_LENS_BINARY must point to an executable fpm-lens binary." >&2; exit 1; fi
if [[ "$INSTALL_MODSEC" != "0" && "$INSTALL_MODSEC" != "1" ]]; then echo "STEPANEL_INSTALL_MODSEC must be 0 or 1." >&2; exit 1; fi
if [[ "$MODSEC_MODE" != "Off" && "$MODSEC_MODE" != "DetectionOnly" && "$MODSEC_MODE" != "On" ]]; then echo "STEPANEL_MODSEC_MODE must be Off, DetectionOnly, or On." >&2; exit 1; fi
if [[ "$INSTALL_MAIL" != "0" && "$INSTALL_MAIL" != "1" ]]; then echo "STEPANEL_INSTALL_MAIL must be 0 or 1." >&2; exit 1; fi
if [[ "$INSTALL_NODE" != "0" && "$INSTALL_NODE" != "1" ]]; then echo "STEPANEL_INSTALL_NODE must be 0 or 1." >&2; exit 1; fi
if [[ "$INSTALL_SECURITY" != "0" && "$INSTALL_SECURITY" != "1" ]]; then echo "STEPANEL_INSTALL_SECURITY must be 0 or 1." >&2; exit 1; fi
if [[ "$INSTALL_TLS" != "0" && "$INSTALL_TLS" != "1" ]]; then echo "STEPANEL_INSTALL_TLS must be 0 or 1." >&2; exit 1; fi
[[ "$NODE_VERSIONS" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+(,v?[0-9]+\.[0-9]+\.[0-9]+)*$ ]] || { echo "Invalid STEPANEL_NODE_VERSIONS." >&2; exit 1; }

install_mail_stack() {
  if [[ "$PKG" == "apt" ]]; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get install -y exim4 dovecot-core dovecot-imapd dovecot-pop3d spamassassin
    MAIL_SPAM_SERVICE="spamassassin"
    systemctl enable exim4 dovecot
  else
    dnf install -y exim dovecot spamassassin
    MAIL_SPAM_SERVICE="spamd"
    systemctl enable exim dovecot
  fi
  systemctl enable "$MAIL_SPAM_SERVICE"
  install -d -m 0750 "$DATA_DIR/mail"
}

if [[ "$INSTALL_MAIL" == "1" ]]; then install_mail_stack; fi

configure_modsecurity() {
  local apache_conf modsec_conf audit_log crs_load backup apache_backup
  if [[ "$PKG" == "apt" ]]; then
    apt-get install -y libapache2-mod-security2 modsecurity-crs
    a2enmod security2 >/dev/null
    apache_conf="/etc/apache2/conf-available/stepanel-modsecurity.conf"
    modsec_conf="/etc/modsecurity/stepanel.conf"
    audit_log="/var/log/apache2/modsec_audit.log"
  else
    dnf install -y mod_security mod_security_crs
    apache_conf="/etc/httpd/conf.d/stepanel-modsecurity.conf"
    modsec_conf="/etc/modsecurity.d/stepanel.conf"
    audit_log="/var/log/httpd/modsec_audit.log"
  fi
  install -d -m 0750 "$(dirname "$modsec_conf")"
  backup="${modsec_conf}.bak.$(date -u +%Y%m%d%H%M%S)"
  apache_backup="${apache_conf}.bak.$(date -u +%Y%m%d%H%M%S)"
  if [[ -e "$apache_conf" ]]; then cp -p "$apache_conf" "$apache_backup"; fi
  if [[ -e "$modsec_conf" ]]; then cp -p "$modsec_conf" "$backup"; fi
  printf 'SecRuleEngine %s\nSecAuditEngine RelevantOnly\nSecAuditLogType Serial\nSecAuditLog %s\nSecRequestBodyAccess On\nSecResponseBodyAccess Off\n' "$MODSEC_MODE" "$audit_log" > "$modsec_conf"
  crs_load=""
  for candidate in /usr/share/modsecurity-crs/owasp-crs.load /etc/modsecurity/owasp-crs.load /etc/modsecurity.d/owasp-crs.load; do
    if [[ -f "$candidate" ]]; then crs_load="$candidate"; break; fi
  done
  if [[ -n "$crs_load" ]]; then printf 'IncludeOptional %s\n' "$crs_load" >> "$modsec_conf"; fi
  printf 'IncludeOptional %s\n' "$modsec_conf" > "$apache_conf"
  [[ "$PKG" == "apt" ]] && a2enconf stepanel-modsecurity >/dev/null 2>&1 || true
  if ! apachectl -t >/dev/null 2>&1 && ! httpd -t >/dev/null 2>&1; then
    [[ -e "$backup" ]] && mv -f "$backup" "$modsec_conf" || rm -f "$modsec_conf"
    [[ -e "$apache_backup" ]] && mv -f "$apache_backup" "$apache_conf" || rm -f "$apache_conf"
    [[ "$PKG" == "apt" ]] && a2disconf stepanel-modsecurity >/dev/null 2>&1 || true
    echo "Apache rejected the ModSecurity configuration; changes were rolled back." >&2
    return 1
  fi
  echo "ModSecurity configured in $MODSEC_MODE mode${crs_load:+ with OWASP CRS}."
}

if [[ "$INSTALL_MODSEC" == "1" ]]; then configure_modsecurity; fi

install -d -m 0750 "$APP_DIR" "$DATA_DIR/imports" "$DATA_DIR/mail" "$DATA_DIR/apps" /var/www/sites
install -d -m 0755 "$DATA_DIR/proxy"
install -d -m 0755 "$APP_DIR/integrations"
id "$APP_USER" >/dev/null 2>&1 || useradd --system --home-dir "$APP_DIR" --shell /usr/sbin/nologin "$APP_USER"
usermod -a -G "$WEB_GROUP" "$APP_USER"
if [[ "$INSTALL_NODE" == "1" ]]; then bash "$ROOT_DIR/deploy/integrations/install-node-nvm.sh" "$APP_USER" "$APP_DIR/.nvm" "$NODE_VERSIONS"; fi
if [[ "$INSTALL_SECURITY" == "1" ]]; then
  if [[ "$PKG" == "apt" ]]; then apt-get install -y clamav clamav-daemon inotify-tools; else dnf install -y clamav clamav-update inotify-tools; fi
  install -d -m 0700 "$DATA_DIR/quarantine"
fi
if [[ "$INSTALL_TLS" == "1" ]]; then
  if [[ "$PKG" == "apt" ]]; then apt-get install -y certbot python3-certbot-apache; else dnf install -y certbot python3-certbot-apache; fi
fi
install -m 0755 "$ROOT_DIR/stepanel" "$APP_DIR/stepanel"
install -m 0755 "$ROOT_DIR/deploy/integrations/install-fail2ban.sh" "$APP_DIR/integrations/install-fail2ban.sh"
install -m 0755 "$ROOT_DIR/deploy/integrations/stepanel-appctl" /usr/local/sbin/stepanel-appctl
if [[ "$INSTALL_TLS" == "1" ]]; then install -m 0755 "$ROOT_DIR/deploy/integrations/stepanel-certbot" /usr/local/sbin/stepanel-certbot; fi
if [[ -n "$FPM_LENS_BINARY" ]]; then install -m 0755 "$FPM_LENS_BINARY" /usr/local/bin/fpm-lens; fi
install -m 0644 -D "$ROOT_DIR/web/index.html" "$APP_DIR/web/index.html"
install -m 0644 -D "$ROOT_DIR/web/static/app.css" "$APP_DIR/web/static/app.css"
install -m 0644 -D "$ROOT_DIR/web/static/import.css" "$APP_DIR/web/static/import.css"
install -m 0644 -D "$ROOT_DIR/web/static/deploy.js" "$APP_DIR/web/static/deploy.js"
install -m 0644 -D "$ROOT_DIR/web/static/certificates.js" "$APP_DIR/web/static/certificates.js"
install -m 0644 -D "$ROOT_DIR/web/static/favicon.svg" "$APP_DIR/web/static/favicon.svg"
if [[ "$PKG" == "apt" ]]; then install -m 0644 "$ROOT_DIR/deploy/apache/stepanel.conf" /etc/apache2/sites-available/stepanel.conf; a2enmod proxy proxy_http headers >/dev/null; a2ensite stepanel >/dev/null; fi
chown -R "$APP_USER:$APP_USER" "$APP_DIR" "$DATA_DIR"; chown "$APP_USER:$WEB_GROUP" /var/www/sites; chmod 2750 /var/www/sites
printf 'STEPANEL_ENV=production\nSTEPANEL_LISTEN=127.0.0.1:8090\nSTEPANEL_ADMIN_USERNAME=%s\nSTEPANEL_ADMIN_PASSWORD=%s\nSTEPANEL_SESSION_SECRET=%s\nSTEPANEL_DB_ENGINE=%s\nSTEPANEL_DB_VERSION=%s\nSTEPANEL_IMPORT_ROOT=%s/imports\nSTEPANEL_WEB_ROOT=/var/www\nSTEPANEL_MAIL_ROOT=%s/mail\nSTEPANEL_NVM_DIR=%s/.nvm\nSTEPANEL_PROXY_ROOT=%s/proxy\nSTEPANEL_APP_ROOT=%s/apps\nSTEPANEL_APPCTL=/usr/local/sbin/stepanel-appctl\nSTEPANEL_APACHE_RELOAD=/usr/local/sbin/stepanel-apache-reload\nSTEPANEL_AUDIT_LOG=%s/audit.jsonl\n' "$ADMIN_USERNAME" "$ADMIN_PASSWORD" "$SESSION_SECRET" "$DB_ENGINE" "$DB_VERSION" "$DATA_DIR" "$DATA_DIR" "$APP_DIR" "$DATA_DIR" "$DATA_DIR" "$DATA_DIR" > "$ENV_FILE"
chmod 0600 "$ENV_FILE"
printf 'STEPANEL_MALWARE_ROOT=%s/quarantine\n' "$DATA_DIR" >> "$ENV_FILE"
if [[ "$INSTALL_TLS" == "1" ]]; then printf 'STEPANEL_CERTBOT=/usr/local/sbin/stepanel-certbot\n' >> "$ENV_FILE"; fi
install -m 0644 "$ROOT_DIR/deploy/stepanel.service" /etc/systemd/system/stepanel.service
install -m 0755 "$ROOT_DIR/deploy/integrations/stepanel-apache-reload" /usr/local/sbin/stepanel-apache-reload
if [[ "$INSTALL_SECURITY" == "1" ]]; then install -m 0755 "$ROOT_DIR/deploy/integrations/stepanel-malware-guard" /usr/local/sbin/stepanel-malware-guard; install -m 0644 "$ROOT_DIR/deploy/stepanel-malware-guard.service" /etc/systemd/system/stepanel-malware-guard.service; fi
if [[ "$PKG" == "apt" ]]; then printf 'IncludeOptional %s/proxy/*.conf\n' "$DATA_DIR" > /etc/apache2/conf-available/stepanel-proxy.conf; a2enconf stepanel-proxy >/dev/null; else printf 'IncludeOptional %s/proxy/*.conf\n' "$DATA_DIR" > /etc/httpd/conf.d/stepanel-proxy.conf; fi
systemctl daemon-reload; systemctl enable --now "$APACHE_SERVICE" "$DB_SERVICE" stepanel; systemctl reload "$APACHE_SERVICE" || true
if [[ "$INSTALL_SECURITY" == "1" ]]; then systemctl enable --now stepanel-malware-guard; fi
if [[ "$INSTALL_MAIL" == "1" ]]; then systemctl enable --now "$([[ "$PKG" == "apt" ]] && echo exim4 || echo exim)" dovecot "$MAIL_SPAM_SERVICE"; fi
if [[ "$INSTALL_TLS" == "1" ]]; then systemctl enable --now certbot.timer 2>/dev/null || true; fi
if [[ "$INSTALL_FAIL2BAN" == "1" ]]; then
  bash "$APP_DIR/integrations/install-fail2ban.sh" --yes --jails "$FAIL2BAN_JAILS" --ignore-ip "$FAIL2BAN_IGNORE_IP"
fi
echo "StePanel installed with $DB_ENGINE ($DB_VERSION). Verify the database service with: systemctl status $DB_SERVICE"
if [[ "$INSTALL_MAIL" == "1" ]]; then echo "Mail stack installed. Mailbox data is staged under $DATA_DIR/mail and requires domain mapping before activation."; fi
