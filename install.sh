#!/usr/bin/env bash
set -Eeuo pipefail

if [[ $EUID -ne 0 ]]; then echo "Run as root: sudo ./install.sh" >&2; exit 1; fi
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_USER="stepanel"; APP_DIR="/opt/stepanel"; DATA_DIR="/var/lib/ste-panel"; ENV_FILE="/etc/ste-panel.env"
if [[ ! -f "$ROOT_DIR/stepanel" || ! -x "$ROOT_DIR/stepanel" ]]; then echo "Build an executable stepanel binary before installing." >&2; exit 1; fi

ADMIN_USERNAME="${STEPANEL_ADMIN_USERNAME:-admin}"; ADMIN_PASSWORD="${STEPANEL_ADMIN_PASSWORD:-}"; SESSION_SECRET="${STEPANEL_SESSION_SECRET:-}"
DB_ENGINE="${STEPANEL_DB_ENGINE:-}"; DB_VERSION="${STEPANEL_DB_VERSION:-default}"
DB_HOST="${STEPANEL_DB_HOST:-localhost}"; DB_USER="${STEPANEL_DB_USER:-}"; DB_PASSWORD="${STEPANEL_DB_PASSWORD:-}"
INSTALL_FAIL2BAN="${STEPANEL_INSTALL_FAIL2BAN:-0}"; FAIL2BAN_JAILS="${STEPANEL_FAIL2BAN_JAILS:-auto}"; FAIL2BAN_IGNORE_IP="${STEPANEL_FAIL2BAN_IGNORE_IP:-}"
FPM_LENS_BINARY="${STEPANEL_FPM_LENS_BINARY:-}"
INSTALL_MODSEC="${STEPANEL_INSTALL_MODSEC:-0}"; MODSEC_MODE="${STEPANEL_MODSEC_MODE:-DetectionOnly}"
INSTALL_MAIL="${STEPANEL_INSTALL_MAIL:-0}"
ACTIVATE_MAIL="${STEPANEL_ACTIVATE_MAIL:-0}"
INSTALL_FTP="${STEPANEL_INSTALL_FTP:-0}"; ACTIVATE_FTP="${STEPANEL_ACTIVATE_FTP:-0}"
FTP_PASSIVE_MIN="${STEPANEL_FTP_PASSIVE_MIN:-40100}"
FTP_PASSIVE_MAX="${STEPANEL_FTP_PASSIVE_MAX:-40200}"
FTP_CERT_FILE="${STEPANEL_FTP_CERT_FILE:-}"; FTP_KEY_FILE="${STEPANEL_FTP_KEY_FILE:-}"
INSTALL_NODE="${STEPANEL_INSTALL_NODE:-0}"; NODE_VERSIONS="${STEPANEL_NODE_VERSIONS:-20.18.0}"
INSTALL_SECURITY="${STEPANEL_INSTALL_SECURITY:-0}"
INSTALL_TLS="${STEPANEL_INSTALL_TLS:-0}"
WPRESS_EXTRACT="${STEPANEL_WPRESS_EXTRACT:-wpress-extract}"
WPCLI="${STEPANEL_WPCLI:-wp}"
PANEL_HOSTNAME="${STEPANEL_PANEL_HOSTNAME:-}"
STAGE_RETENTION_HOURS="${STEPANEL_STAGE_RETENTION_HOURS:-168}"
MIN_FREE_BYTES="${STEPANEL_MIN_FREE_BYTES:-1073741824}"
unset STEPANEL_ADMIN_PASSWORD STEPANEL_SESSION_SECRET STEPANEL_DB_PASSWORD
if [[ -z "$ADMIN_PASSWORD" && -t 0 ]]; then read -r -s -p "StePanel admin password: " ADMIN_PASSWORD; echo; fi
if [[ -z "$ADMIN_PASSWORD" ]]; then echo "Set STEPANEL_ADMIN_PASSWORD or run the installer interactively." >&2; exit 1; fi
if [[ -z "$SESSION_SECRET" ]]; then SESSION_SECRET="$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')"; fi
if (( ${#ADMIN_PASSWORD} < 12 )); then echo "STEPANEL_ADMIN_PASSWORD must be at least 12 characters." >&2; exit 1; fi
if (( ${#SESSION_SECRET} < 32 )); then echo "STEPANEL_SESSION_SECRET must be at least 32 characters." >&2; exit 1; fi
if [[ ! "$ADMIN_USERNAME" =~ ^[a-zA-Z0-9._-]{1,64}$ || "$ADMIN_USERNAME" == *$'\n'* || "$ADMIN_USERNAME" == *$'\r'* ]]; then echo "Invalid admin username." >&2; exit 1; fi
if [[ ! "$PANEL_HOSTNAME" =~ ^([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$ ]]; then echo "Set STEPANEL_PANEL_HOSTNAME to the panel's fully qualified domain name." >&2; exit 1; fi
if [[ "$ADMIN_PASSWORD" == *$'\n'* || "$ADMIN_PASSWORD" == *$'\r'* || "$SESSION_SECRET" == *$'\n'* || "$SESSION_SECRET" == *$'\r'* || "$DB_PASSWORD" == *$'\n'* || "$DB_PASSWORD" == *$'\r'* ]]; then echo "Credentials may not contain newlines." >&2; exit 1; fi
if [[ "$DB_HOST" == *$'\n'* || "$DB_HOST" == *$'\r'* ]]; then echo "STEPANEL_DB_HOST may not contain newlines." >&2; exit 1; fi
if [[ -n "$DB_USER" && ! "$DB_USER" =~ ^[A-Za-z0-9_]{1,32}$ ]]; then echo "STEPANEL_DB_USER must contain only letters, numbers, and underscores." >&2; exit 1; fi
if [[ "$WPRESS_EXTRACT" == *$'\n'* || "$WPRESS_EXTRACT" == *$'\r'* || "$WPCLI" == *$'\n'* || "$WPCLI" == *$'\r'* ]]; then echo "WordPress executable paths may not contain newlines." >&2; exit 1; fi

if [[ "$INSTALL_FAIL2BAN" != "0" && "$INSTALL_FAIL2BAN" != "1" ]]; then echo "STEPANEL_INSTALL_FAIL2BAN must be 0 or 1." >&2; exit 1; fi
if [[ "$INSTALL_FAIL2BAN" == "1" && -z "$FAIL2BAN_IGNORE_IP" && -t 0 ]]; then read -r -p "Trusted management IPs/CIDRs for Fail2ban (required): " FAIL2BAN_IGNORE_IP; fi
if [[ "$INSTALL_FAIL2BAN" == "1" && -z "$FAIL2BAN_IGNORE_IP" ]]; then echo "Set STEPANEL_FAIL2BAN_IGNORE_IP before enabling Fail2ban; refusing an unattended lockout risk." >&2; exit 1; fi
if [[ "$FAIL2BAN_IGNORE_IP" == *$'\n'* || "$FAIL2BAN_IGNORE_IP" == *$'\r'* ]]; then echo "STEPANEL_FAIL2BAN_IGNORE_IP may not contain newlines." >&2; exit 1; fi
if [[ -n "$FPM_LENS_BINARY" && ! -x "$FPM_LENS_BINARY" ]]; then echo "STEPANEL_FPM_LENS_BINARY must point to an executable fpm-lens binary." >&2; exit 1; fi
if [[ "$INSTALL_MODSEC" != "0" && "$INSTALL_MODSEC" != "1" ]]; then echo "STEPANEL_INSTALL_MODSEC must be 0 or 1." >&2; exit 1; fi
if [[ "$MODSEC_MODE" != "Off" && "$MODSEC_MODE" != "DetectionOnly" && "$MODSEC_MODE" != "On" ]]; then echo "STEPANEL_MODSEC_MODE must be Off, DetectionOnly, or On." >&2; exit 1; fi
if [[ "$INSTALL_MAIL" != "0" && "$INSTALL_MAIL" != "1" ]]; then echo "STEPANEL_INSTALL_MAIL must be 0 or 1." >&2; exit 1; fi
if [[ "$ACTIVATE_MAIL" != "0" && "$ACTIVATE_MAIL" != "1" ]]; then echo "STEPANEL_ACTIVATE_MAIL must be 0 or 1." >&2; exit 1; fi
if [[ "$INSTALL_FTP" != "0" && "$INSTALL_FTP" != "1" ]]; then echo "STEPANEL_INSTALL_FTP must be 0 or 1." >&2; exit 1; fi
if [[ "$ACTIVATE_FTP" != "0" && "$ACTIVATE_FTP" != "1" ]]; then echo "STEPANEL_ACTIVATE_FTP must be 0 or 1." >&2; exit 1; fi
if [[ "$ACTIVATE_MAIL" == "1" && "$INSTALL_MAIL" != "1" ]]; then echo "STEPANEL_ACTIVATE_MAIL requires STEPANEL_INSTALL_MAIL=1." >&2; exit 1; fi
if [[ "$ACTIVATE_FTP" == "1" && "$INSTALL_FTP" != "1" ]]; then echo "STEPANEL_ACTIVATE_FTP requires STEPANEL_INSTALL_FTP=1." >&2; exit 1; fi
if [[ "$ACTIVATE_FTP" == "1" && ( ! -f "$FTP_CERT_FILE" || ! -r "$FTP_CERT_FILE" || ! -f "$FTP_KEY_FILE" || ! -r "$FTP_KEY_FILE" ) ]]; then echo "Activating FTP requires readable STEPANEL_FTP_CERT_FILE and STEPANEL_FTP_KEY_FILE for FTPS." >&2; exit 1; fi
if [[ "$ACTIVATE_FTP" == "1" && ( "$FTP_CERT_FILE" != /* || "$FTP_KEY_FILE" != /* || "$FTP_CERT_FILE" =~ [[:space:]] || "$FTP_KEY_FILE" =~ [[:space:]] ) ]]; then echo "FTPS certificate and key paths must be absolute and contain no whitespace." >&2; exit 1; fi
if [[ ! "$FTP_PASSIVE_MIN" =~ ^[0-9]+$ || ! "$FTP_PASSIVE_MAX" =~ ^[0-9]+$ || "$FTP_PASSIVE_MIN" -lt 1024 || "$FTP_PASSIVE_MAX" -gt 65535 || "$FTP_PASSIVE_MIN" -ge "$FTP_PASSIVE_MAX" ]]; then echo "STEPANEL_FTP_PASSIVE_MIN/MAX must define a valid increasing port range." >&2; exit 1; fi
if [[ "$INSTALL_NODE" != "0" && "$INSTALL_NODE" != "1" ]]; then echo "STEPANEL_INSTALL_NODE must be 0 or 1." >&2; exit 1; fi
if [[ "$INSTALL_SECURITY" != "0" && "$INSTALL_SECURITY" != "1" ]]; then echo "STEPANEL_INSTALL_SECURITY must be 0 or 1." >&2; exit 1; fi
if [[ "$INSTALL_TLS" != "0" && "$INSTALL_TLS" != "1" ]]; then echo "STEPANEL_INSTALL_TLS must be 0 or 1." >&2; exit 1; fi
if [[ ! "$STAGE_RETENTION_HOURS" =~ ^[1-9][0-9]*$ ]]; then echo "STEPANEL_STAGE_RETENTION_HOURS must be a positive integer." >&2; exit 1; fi
if [[ ! "$MIN_FREE_BYTES" =~ ^[1-9][0-9]*$ ]]; then echo "STEPANEL_MIN_FREE_BYTES must be a positive integer." >&2; exit 1; fi
[[ "$NODE_VERSIONS" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+(,v?[0-9]+\.[0-9]+\.[0-9]+)*$ ]] || { echo "Invalid STEPANEL_NODE_VERSIONS." >&2; exit 1; }

# shellcheck source=/etc/os-release
source /etc/os-release
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
if [[ "$PKG" == "apt" ]]; then DB_SERVICE="${DB_ENGINE/mysql/mysql}"; [[ "$DB_ENGINE" == "mariadb" ]] && DB_SERVICE="mariadb"; export DEBIAN_FRONTEND=noninteractive; apt-get update; apt-get install -y apache2 php php-cli php-mysql php-curl php-mbstring php-xml tar gzip ca-certificates curl sudo logrotate
else DB_SERVICE="$([[ "$DB_ENGINE" == "mysql" ]] && echo mysqld || echo mariadb)"; dnf install -y httpd php php-cli php-mysqlnd php-curl php-mbstring php-xml tar gzip ca-certificates curl sudo logrotate; fi

if [[ "$DB_VERSION" == "default" ]]; then
  if [[ "$PKG" == "apt" ]]; then apt-get install -y "$DB_PACKAGE"; else dnf install -y "$DB_PACKAGE"; fi
elif [[ "$PKG" == "apt" ]]; then
  if ! apt-cache madison "$DB_PACKAGE" | awk '{print $3}' | grep -Fxq "$DB_VERSION"; then echo "Requested $DB_PACKAGE version $DB_VERSION is not available. Configure the appropriate repository first." >&2; apt-cache madison "$DB_PACKAGE" || true; exit 1; fi
  apt-get install -y "$DB_PACKAGE=$DB_VERSION"
else
  if ! dnf --assumeno install "$DB_PACKAGE-$DB_VERSION" >/dev/null 2>&1; then echo "Requested $DB_PACKAGE version $DB_VERSION is not available. Enable the appropriate DNF repository/module first." >&2; dnf list --showduplicates "$DB_PACKAGE" || true; exit 1; fi
  dnf install -y "$DB_PACKAGE-$DB_VERSION"
fi

WEB_GROUP="$([[ "$PKG" == "apt" ]] && printf www-data || printf apache)"
getent group "$WEB_GROUP" >/dev/null || { echo "Apache group $WEB_GROUP was not created by the package installation." >&2; exit 1; }
if [[ "$PKG" == "apt" ]]; then PROXY_ROOT=/etc/apache2/stepanel-proxy; else PROXY_ROOT=/etc/httpd/conf.d/stepanel-proxy; fi

systemctl enable --now "$DB_SERVICE"
if [[ -z "$DB_USER" ]]; then
  [[ "$DB_HOST" == "localhost" ]] || { echo "Set STEPANEL_DB_USER and STEPANEL_DB_PASSWORD for a non-local database host." >&2; exit 1; }
  DB_USER=stepanel_admin
  DB_PASSWORD="$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')"
  DB_CLIENT=mysql
  command -v mariadb >/dev/null 2>&1 && DB_CLIENT=mariadb
  "$DB_CLIENT" --protocol=socket --batch <<SQL
CREATE USER IF NOT EXISTS '$DB_USER'@'localhost' IDENTIFIED BY '$DB_PASSWORD';
ALTER USER '$DB_USER'@'localhost' IDENTIFIED BY '$DB_PASSWORD';
GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, REFERENCES, INDEX, ALTER,
  CREATE TEMPORARY TABLES, LOCK TABLES, EXECUTE, CREATE VIEW, SHOW VIEW,
  CREATE ROUTINE, ALTER ROUTINE, CREATE USER, EVENT, TRIGGER
  ON *.* TO '$DB_USER'@'localhost' WITH GRANT OPTION;
FLUSH PRIVILEGES;
SQL
fi
DB_CLIENT=mysql
command -v mariadb >/dev/null 2>&1 && DB_CLIENT=mariadb
if ! env MYSQL_PWD="$DB_PASSWORD" "$DB_CLIENT" --host "$DB_HOST" --user "$DB_USER" --batch --skip-column-names --execute 'SELECT 1' | grep -Fxq 1; then
  echo "StePanel database credentials could not connect to $DB_HOST." >&2
  exit 1
fi

install_mail_stack() {
  if [[ "$PKG" == "apt" ]]; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get install -y exim4 dovecot-core dovecot-imapd spamassassin
    MAIL_SPAM_SERVICE="spamassassin"
  else
    dnf install -y exim dovecot spamassassin
    MAIL_SPAM_SERVICE="spamd"
  fi
  install -d -m 0750 "$DATA_DIR/mail"
  if [[ "$ACTIVATE_MAIL" == "0" ]]; then
    [[ "$MAIL_EXIM_PREEXISTING" == "1" ]] || systemctl disable --now "$([[ "$PKG" == "apt" ]] && echo exim4 || echo exim)" 2>/dev/null || true
    [[ "$MAIL_DOVECOT_PREEXISTING" == "1" ]] || systemctl disable --now dovecot 2>/dev/null || true
    [[ "$MAIL_SPAM_PREEXISTING" == "1" ]] || systemctl disable --now "$MAIL_SPAM_SERVICE" 2>/dev/null || true
  fi
}

install_ftp_stack() {
  local config backup
  if [[ "$FTP_PREEXISTING" == "1" && "$ACTIVATE_FTP" == "0" ]]; then
    echo "Existing vsftpd installation detected; preserving its configuration and service state."
    return
  fi
  if [[ "$PKG" == "apt" ]]; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get install -y vsftpd
    config="/etc/vsftpd.conf"
  else
    dnf install -y vsftpd
    config="/etc/vsftpd/vsftpd.conf"
  fi
  backup="${config}.bak.$(date -u +%Y%m%d%H%M%S)"
  [[ -e "$config" ]] && cp -p "$config" "$backup"
  install -d -m 0750 /etc/vsftpd
  cat > "$config" <<EOF
# Managed by StePanel. Review and enable FTPS before internet exposure.
listen=YES
listen_ipv6=NO
anonymous_enable=NO
local_enable=YES
write_enable=YES
local_umask=022
chroot_local_user=YES
allow_writeable_chroot=YES
user_sub_token=\$USER
local_root=/var/www/sites/\$USER
pasv_min_port=$FTP_PASSIVE_MIN
pasv_max_port=$FTP_PASSIVE_MAX
use_localtime=YES
xferlog_enable=YES
log_ftp_protocol=YES
EOF
  if [[ "$ACTIVATE_FTP" == "1" ]]; then
    printf 'ssl_enable=YES\nallow_anon_ssl=NO\nforce_local_data_ssl=YES\nforce_local_logins_ssl=YES\nssl_tlsv1=YES\nssl_sslv2=NO\nssl_sslv3=NO\nrsa_cert_file=%s\nrsa_private_key_file=%s\n' "$FTP_CERT_FILE" "$FTP_KEY_FILE" >> "$config"
  else
    printf 'ssl_enable=NO\n' >> "$config"
  fi
  if ! vsftpd -olisten=NO -olisten_ipv6=NO "$config" >/dev/null 2>&1; then
    [[ -e "$backup" ]] && mv -f "$backup" "$config"
    echo "vsftpd rejected its configuration; changes were rolled back." >&2
    return 1
  fi
  if [[ "$ACTIVATE_FTP" == "0" && "$FTP_PREEXISTING" == "0" ]]; then
    systemctl disable --now vsftpd 2>/dev/null || true
  fi
}

MAIL_EXIM_PREEXISTING=0
MAIL_DOVECOT_PREEXISTING=0
MAIL_SPAM_PREEXISTING=0
FTP_PREEXISTING=0
if command -v exim >/dev/null 2>&1 || command -v exim4 >/dev/null 2>&1; then MAIL_EXIM_PREEXISTING=1; fi
command -v dovecot >/dev/null 2>&1 && MAIL_DOVECOT_PREEXISTING=1
if command -v spamd >/dev/null 2>&1 || command -v spamassassin >/dev/null 2>&1; then MAIL_SPAM_PREEXISTING=1; fi
command -v vsftpd >/dev/null 2>&1 && FTP_PREEXISTING=1
if [[ "$INSTALL_MAIL" == "1" ]]; then install_mail_stack; fi
if [[ "$INSTALL_FTP" == "1" ]]; then install_ftp_stack; fi

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
  if [[ "$PKG" == "apt" ]]; then a2enconf stepanel-modsecurity >/dev/null 2>&1 || true; fi
  if ! apachectl -t >/dev/null 2>&1 && ! httpd -t >/dev/null 2>&1; then
    if [[ -e "$backup" ]]; then mv -f "$backup" "$modsec_conf"; else rm -f "$modsec_conf"; fi
    if [[ -e "$apache_backup" ]]; then mv -f "$apache_backup" "$apache_conf"; else rm -f "$apache_conf"; fi
    if [[ "$PKG" == "apt" ]]; then a2disconf stepanel-modsecurity >/dev/null 2>&1 || true; fi
    echo "Apache rejected the ModSecurity configuration; changes were rolled back." >&2
    return 1
  fi
  echo "ModSecurity configured in $MODSEC_MODE mode${crs_load:+ with OWASP CRS}."
}

if [[ "$INSTALL_MODSEC" == "1" ]]; then configure_modsecurity; fi

install -d -m 0750 "$APP_DIR" "$DATA_DIR/imports" "$DATA_DIR/mail" "$DATA_DIR/apps" /var/www/sites
install -d -m 0755 -o root -g root "$PROXY_ROOT"
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
ADMIN_PASSWORD_HASH="$(printf '%s' "$ADMIN_PASSWORD" | "$APP_DIR/stepanel" hash-password)"
unset ADMIN_PASSWORD
install -m 0755 "$ROOT_DIR/deploy/integrations/install-fail2ban.sh" "$APP_DIR/integrations/install-fail2ban.sh"
install -m 0755 "$ROOT_DIR/deploy/integrations/stepanel-appctl" /usr/local/sbin/stepanel-appctl
install -m 0755 "$ROOT_DIR/deploy/integrations/stepanel-proxyctl" /usr/local/sbin/stepanel-proxyctl
if [[ "$INSTALL_TLS" == "1" ]]; then install -m 0755 "$ROOT_DIR/deploy/integrations/stepanel-certbot" /usr/local/sbin/stepanel-certbot; fi
if [[ -n "$FPM_LENS_BINARY" ]]; then install -m 0755 "$FPM_LENS_BINARY" /usr/local/bin/fpm-lens; fi
install -m 0644 -D "$ROOT_DIR/web/index.html" "$APP_DIR/web/index.html"
install -m 0644 -D "$ROOT_DIR/web/static/app.css" "$APP_DIR/web/static/app.css"
install -m 0644 -D "$ROOT_DIR/web/static/import.css" "$APP_DIR/web/static/import.css"
install -m 0644 -D "$ROOT_DIR/web/static/cpmove.js" "$APP_DIR/web/static/cpmove.js"
install -m 0644 -D "$ROOT_DIR/web/static/deploy.js" "$APP_DIR/web/static/deploy.js"
install -m 0644 -D "$ROOT_DIR/web/static/certificates.js" "$APP_DIR/web/static/certificates.js"
install -m 0644 -D "$ROOT_DIR/web/static/wpress.js" "$APP_DIR/web/static/wpress.js"
install -m 0644 -D "$ROOT_DIR/web/static/favicon.svg" "$APP_DIR/web/static/favicon.svg"
if [[ "$PKG" == "apt" ]]; then
  install -m 0644 "$ROOT_DIR/deploy/apache/stepanel.conf" /etc/apache2/sites-available/stepanel.conf
  sed -i "s/panel\.example\.com/$PANEL_HOSTNAME/g" /etc/apache2/sites-available/stepanel.conf
  a2enmod proxy proxy_http headers >/dev/null
  if [[ -e /etc/apache2/conf-enabled/stepanel-proxy.conf ]]; then a2disconf stepanel-proxy >/dev/null; fi
  a2ensite stepanel >/dev/null
else
  install -m 0644 "$ROOT_DIR/deploy/apache/stepanel-rhel.conf" /etc/httpd/conf.d/stepanel.conf
  sed -i "s/panel\.example\.com/$PANEL_HOSTNAME/g" /etc/httpd/conf.d/stepanel.conf
  if [[ -f /etc/httpd/conf.d/stepanel-proxy.conf ]]; then printf '%s\n' '# Superseded by the root-owned stepanel-proxy directory.' > /etc/httpd/conf.d/stepanel-proxy.conf; fi
fi
chown -R root:root "$APP_DIR"
chmod 0755 "$APP_DIR"
if [[ -d "$APP_DIR/.nvm" ]]; then chown -R "$APP_USER:$APP_USER" "$APP_DIR/.nvm"; fi
chown -R "$APP_USER:$APP_USER" "$DATA_DIR"
chown "$APP_USER:$WEB_GROUP" /var/www/sites
chmod 2750 /var/www/sites
install -d -m 0700 -o "$APP_USER" -g "$APP_USER" /var/www/sites/.stepanel-recovery
write_env() {
  local key=$1 value=$2
  [[ "$value" != *$'\n'* && "$value" != *$'\r'* ]] || { echo "Invalid newline in $key." >&2; exit 1; }
  value=${value//\\/\\\\}
  value=${value//\"/\\\"}
  printf '%s="%s"\n' "$key" "$value"
}
env_tmp=$(mktemp /etc/ste-panel.env.XXXXXX)
trap 'rm -f "$env_tmp"' EXIT
{
  write_env STEPANEL_ENV production
  write_env STEPANEL_LISTEN 127.0.0.1:8090
  write_env STEPANEL_ADMIN_USERNAME "$ADMIN_USERNAME"
  write_env STEPANEL_ADMIN_PASSWORD_HASH "$ADMIN_PASSWORD_HASH"
  write_env STEPANEL_SESSION_SECRET "$SESSION_SECRET"
  write_env STEPANEL_DB_ENGINE "$DB_ENGINE"
  write_env STEPANEL_DB_VERSION "$DB_VERSION"
  write_env STEPANEL_DB_HOST "$DB_HOST"
  write_env STEPANEL_DB_USER "$DB_USER"
  write_env STEPANEL_DB_PASSWORD "$DB_PASSWORD"
  write_env STEPANEL_IMPORT_ROOT "$DATA_DIR/imports"
  write_env STEPANEL_WEB_ROOT /var/www
  write_env STEPANEL_MAIL_ROOT "$DATA_DIR/mail"
  write_env STEPANEL_NVM_DIR "$APP_DIR/.nvm"
  write_env STEPANEL_PROXY_ROOT "$PROXY_ROOT"
  write_env STEPANEL_APP_ROOT "$DATA_DIR/apps"
  write_env STEPANEL_MALWARE_ROOT "$DATA_DIR/quarantine"
  write_env STEPANEL_APPCTL /usr/local/sbin/stepanel-appctl
  write_env STEPANEL_PROXYCTL /usr/local/sbin/stepanel-proxyctl
  write_env STEPANEL_AUDIT_LOG "$DATA_DIR/audit.jsonl"
  write_env STEPANEL_JOB_STATE "$DATA_DIR/jobs.json"
  write_env STEPANEL_RECOVERY_ROOT /var/www/sites/.stepanel-recovery
  write_env STEPANEL_WPRESS_EXTRACT "$WPRESS_EXTRACT"
  write_env STEPANEL_WPCLI "$WPCLI"
  write_env STEPANEL_SUDO /usr/bin/sudo
  write_env STEPANEL_STAGE_RETENTION_HOURS "$STAGE_RETENTION_HOURS"
  write_env STEPANEL_MIN_FREE_BYTES "$MIN_FREE_BYTES"
  if [[ "$INSTALL_FTP" == "1" ]]; then write_env STEPANEL_FTP_PASSIVE_MIN "$FTP_PASSIVE_MIN"; write_env STEPANEL_FTP_PASSIVE_MAX "$FTP_PASSIVE_MAX"; fi
  if [[ "$INSTALL_TLS" == "1" ]]; then write_env STEPANEL_CERTBOT /usr/local/sbin/stepanel-certbot; fi
} > "$env_tmp"
chmod 0600 "$env_tmp"
mv -f "$env_tmp" "$ENV_FILE"
trap - EXIT
install -m 0644 "$ROOT_DIR/deploy/stepanel.service" /etc/systemd/system/stepanel.service
install -m 0644 "$ROOT_DIR/deploy/stepanel.logrotate" /etc/logrotate.d/stepanel
sudoers_tmp=$(mktemp)
trap 'rm -f "$sudoers_tmp"' EXIT
printf '%s ALL=(root) NOPASSWD: /usr/local/sbin/stepanel-appctl *\n%s ALL=(root) NOPASSWD: /usr/local/sbin/stepanel-proxyctl *\n' "$APP_USER" "$APP_USER" > "$sudoers_tmp"
if [[ "$INSTALL_TLS" == "1" ]]; then printf '%s ALL=(root) NOPASSWD: /usr/local/sbin/stepanel-certbot *\n' "$APP_USER" >> "$sudoers_tmp"; fi
visudo -cf "$sudoers_tmp" >/dev/null
install -m 0440 -o root -g root "$sudoers_tmp" /etc/sudoers.d/stepanel
if [[ "$INSTALL_SECURITY" == "1" ]]; then install -m 0755 "$ROOT_DIR/deploy/integrations/stepanel-malware-guard" /usr/local/sbin/stepanel-malware-guard; install -m 0644 "$ROOT_DIR/deploy/stepanel-malware-guard.service" /etc/systemd/system/stepanel-malware-guard.service; fi
if command -v selinuxenabled >/dev/null 2>&1 && selinuxenabled; then
  command -v restorecon >/dev/null 2>&1 && restorecon -RF /opt/stepanel /var/lib/ste-panel /var/www/sites /etc/httpd/conf.d/stepanel.conf "$PROXY_ROOT"
  if command -v setsebool >/dev/null 2>&1; then setsebool -P httpd_can_network_connect 1; fi
fi
systemctl daemon-reload; systemctl enable --now "$APACHE_SERVICE" "$DB_SERVICE" stepanel; systemctl reload "$APACHE_SERVICE" || true
if [[ "$INSTALL_SECURITY" == "1" ]]; then systemctl enable --now stepanel-malware-guard; fi
if [[ "$INSTALL_MAIL" == "1" && "$ACTIVATE_MAIL" == "1" ]]; then
  systemctl enable --now "$([[ "$PKG" == "apt" ]] && echo exim4 || echo exim)" dovecot "$MAIL_SPAM_SERVICE"
elif [[ "$INSTALL_MAIL" == "1" ]]; then
  [[ "$MAIL_EXIM_PREEXISTING" == "1" ]] || systemctl disable --now "$([[ "$PKG" == "apt" ]] && echo exim4 || echo exim)" 2>/dev/null || true
  [[ "$MAIL_DOVECOT_PREEXISTING" == "1" ]] || systemctl disable --now dovecot 2>/dev/null || true
  [[ "$MAIL_SPAM_PREEXISTING" == "1" ]] || systemctl disable --now "$MAIL_SPAM_SERVICE" 2>/dev/null || true
fi
if [[ "$INSTALL_FTP" == "1" && "$ACTIVATE_FTP" == "1" ]]; then
  systemctl enable --now vsftpd
elif [[ "$INSTALL_FTP" == "1" && "$FTP_PREEXISTING" == "0" ]]; then
  systemctl disable --now vsftpd 2>/dev/null || true
fi
if [[ "$INSTALL_TLS" == "1" ]]; then systemctl enable --now certbot.timer 2>/dev/null || true; fi
if [[ "$INSTALL_FAIL2BAN" == "1" ]]; then
  bash "$APP_DIR/integrations/install-fail2ban.sh" --yes --jails "$FAIL2BAN_JAILS" --ignore-ip "$FAIL2BAN_IGNORE_IP"
fi
echo "StePanel installed with $DB_ENGINE ($DB_VERSION). Verify the database service with: systemctl status $DB_SERVICE"
echo "Panel hostname: $PANEL_HOSTNAME. Complete TLS termination before signing in; Secure cookies are mandatory in production."
if [[ "$INSTALL_MAIL" == "1" ]]; then echo "Mail stack installed. Mailbox data is staged under $DATA_DIR/mail; services are active only when STEPANEL_ACTIVATE_MAIL=1."; fi
if [[ "$INSTALL_FTP" == "1" ]]; then echo "vsftpd installed with local-user chroot and passive ports $FTP_PASSIVE_MIN-$FTP_PASSIVE_MAX; it is active only when STEPANEL_ACTIVATE_FTP=1 with FTPS certificates."; fi
