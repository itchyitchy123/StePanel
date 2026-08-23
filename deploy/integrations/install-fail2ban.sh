#!/usr/bin/env bash
set -Eeuo pipefail

# Install Fail2ban and create a conservative set of local jails.
# Supported package managers: apt, dnf, and yum.

readonly PROGRAM_NAME=${0##*/}
readonly VERSION=1.1.0
DRY_RUN=0
ASSUME_YES=0
RESTORE_BACKUP=0
SSH_PORT=${SSH_PORT:-ssh}
BAN_TIME=${BAN_TIME:-1h}
FIND_TIME=${FIND_TIME:-10m}
MAX_RETRY=${MAX_RETRY:-5}
DEST_EMAIL=${DEST_EMAIL:-root@localhost}
IGNORE_IP=${IGNORE_IP:-"127.0.0.1/8 ::1"}
CONFIG_FILE=${CONFIG_FILE:-/etc/fail2ban/jail.d/99-local-hardening.conf}
FILTER_DIR=${FILTER_DIR:-/etc/fail2ban/filter.d}
JAILS=${JAILS:-auto}
CPANEL_ROOT=${CPANEL_ROOT:-/usr/local/cpanel}
ACCESS_LOGS=${ACCESS_LOGS:-}
PACKAGE_MANAGER=${PACKAGE_MANAGER:-auto}

usage() {
  cat <<EOF
Usage: $PROGRAM_NAME [options]

Install and configure Fail2ban with jails for SSH and detected web/mail services.

Options:
  --ssh-port PORT       SSH port or service name (default: $SSH_PORT)
  --ban-time TIME       Initial ban duration (default: $BAN_TIME)
  --find-time TIME      Failure counting window (default: $FIND_TIME)
  --max-retry COUNT     Failures allowed in the window (default: $MAX_RETRY)
  --ignore-ip LIST      Space-separated trusted IPs/CIDRs
  --dest-email ADDRESS  Address used by mail actions (default: $DEST_EMAIL)
  --config-file PATH    Configuration destination (default: $CONFIG_FILE)
  --filter-dir PATH     Custom filter destination (default: $FILTER_DIR)
  --jails LIST          Comma-separated jails, "auto", or "all"
  --access-logs LIST    Space-separated custom web access-log paths
  --restore-backup      Restore the newest backup and restart Fail2ban
  --dry-run             Print changes without applying them
  -y, --yes             Skip the interactive confirmation
  --version             Show the program version
  -h, --help            Show this help

Available jails: sshd, recidive, nginx-http-auth, nginx-botsearch,
nginx-bad-request, apache-auth, apache-badbots, apache-noscript,
apache-botsearch, apache-shellshock, postfix, postfix-sasl, dovecot,
web-scanners, ai-scrapers.

Environment variables with the uppercase option names, including CONFIG_FILE,
FILTER_DIR, JAILS, ACCESS_LOGS, and PACKAGE_MANAGER, are also supported.
EOF
}

log() { printf '[%s] %s\n' "$PROGRAM_NAME" "$*"; }
die() { printf '[%s] ERROR: %s\n' "$PROGRAM_NAME" "$*" >&2; exit 1; }

run() {
  if (( DRY_RUN )); then
    printf 'DRY RUN:'
    printf ' %q' "$@"
    printf '\n'
  else
    "$@"
  fi
}

while (( $# )); do
  case "$1" in
    --ssh-port|--ban-time|--find-time|--max-retry|--ignore-ip|--dest-email|--config-file|--filter-dir|--jails|--access-logs)
      (( $# >= 2 )) || die "$1 requires a value"
      case "$1" in
        --ssh-port) SSH_PORT=$2 ;;
        --ban-time) BAN_TIME=$2 ;;
        --find-time) FIND_TIME=$2 ;;
        --max-retry) MAX_RETRY=$2 ;;
        --ignore-ip) IGNORE_IP=$2 ;;
        --dest-email) DEST_EMAIL=$2 ;;
        --config-file) CONFIG_FILE=$2 ;;
        --filter-dir) FILTER_DIR=$2 ;;
        --jails) JAILS=$2 ;;
        --access-logs) ACCESS_LOGS=$2 ;;
      esac
      shift 2
      ;;
    --dry-run) DRY_RUN=1; shift ;;
    --restore-backup) RESTORE_BACKUP=1; shift ;;
    -y|--yes) ASSUME_YES=1; shift ;;
    --version) printf '%s %s\n' "$PROGRAM_NAME" "$VERSION"; exit 0 ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown option: $1" ;;
  esac
done

[[ $SSH_PORT =~ ^[A-Za-z0-9_-]+$ ]] || die "invalid SSH port/service: $SSH_PORT"
[[ $MAX_RETRY =~ ^[1-9][0-9]*$ ]] || die "--max-retry must be a positive integer"
[[ $BAN_TIME =~ ^[1-9][0-9]*[smhdw]?$ ]] || die "invalid --ban-time value: $BAN_TIME"
[[ $FIND_TIME =~ ^[1-9][0-9]*[smhdw]?$ ]] || die "invalid --find-time value: $FIND_TIME"
[[ $DEST_EMAIL != *$'\n'* && $IGNORE_IP != *$'\n'* ]] || die "newline characters are not allowed"
[[ $DEST_EMAIL =~ ^[^[:space:]@]+@[^[:space:]@]+$ ]] || die "invalid --dest-email value: $DEST_EMAIL"
[[ $IGNORE_IP =~ ^[A-Za-z0-9._:/%[:space:]-]+$ ]] || die "invalid --ignore-ip value"
[[ $CONFIG_FILE == /* && $CONFIG_FILE != */ ]] || die "--config-file must be an absolute file path"
[[ $FILTER_DIR == /* && $FILTER_DIR != */ ]] || die "--filter-dir must be an absolute directory path"
[[ $ACCESS_LOGS != *$'\n'* && $ACCESS_LOGS != *$'\r'* ]] || die "invalid --access-logs value"
[[ $PACKAGE_MANAGER == auto || $PACKAGE_MANAGER == apt || $PACKAGE_MANAGER == dnf || $PACKAGE_MANAGER == yum ]] || die "PACKAGE_MANAGER must be auto, apt, dnf, or yum"

readonly AVAILABLE_JAILS='sshd recidive nginx-http-auth nginx-botsearch nginx-bad-request apache-auth apache-badbots apache-noscript apache-botsearch apache-shellshock postfix postfix-sasl dovecot web-scanners ai-scrapers'

is_cpanel() { [[ -d $CPANEL_ROOT || -f $CPANEL_ROOT/cpanel ]]; }
has_csf() { command -v csf >/dev/null 2>&1 || [[ -x /usr/local/cpanel/3rdparty/bin/csf ]]; }

validate_jails() {
  local jail available found
  [[ $JAILS == auto || $JAILS == all ]] && return
  JAILS=${JAILS//,/ }
  for jail in $JAILS; do
    found=0
    for available in $AVAILABLE_JAILS; do
      [[ $jail == "$available" ]] && found=1
    done
    (( found )) || die "unknown jail: $jail"
  done
}

selected() {
  local wanted=$1 item
  [[ $JAILS == all ]] && return 0
  for item in $JAILS; do [[ $item == "$wanted" ]] && return 0; done
  return 1
}

ask_jail() {
  local jail=$1 recommendation=$2 reply
  read -r -p "Enable $jail? [$recommendation] " reply
  reply=${reply:-$recommendation}
  [[ $reply == [Yy] || $reply == [Yy][Ee][Ss] ]] && JAILS+=" $jail"
}

validate_jails

restart_fail2ban() {
  if command -v systemctl >/dev/null 2>&1; then
    systemctl enable --now fail2ban
    systemctl restart fail2ban
  else
    service fail2ban restart
  fi
}

if (( EUID != 0 && ! DRY_RUN )); then
  die "run this script as root (for example: sudo ./$PROGRAM_NAME)"
fi

if (( RESTORE_BACKUP )); then
  [[ -d ${CONFIG_FILE%/*} ]] || die "configuration directory does not exist: ${CONFIG_FILE%/*}"
  backup=$(find "${CONFIG_FILE%/*}" -maxdepth 1 -type f \
    -name "${CONFIG_FILE##*/}.bak.*" -print 2>/dev/null | sort | tail -n 1)
  [[ -n $backup ]] || die "no backup found for $CONFIG_FILE"
  if (( DRY_RUN )); then
    log "would restore $backup to $CONFIG_FILE"
    exit 0
  fi
  current=
  if [[ -f $CONFIG_FILE ]]; then
    current=$(mktemp)
    cp -a "$CONFIG_FILE" "$current"
  fi
  cp -a "$backup" "$CONFIG_FILE"
  if ! fail2ban-client -t; then
    if [[ -n $current ]]; then
      cp -a "$current" "$CONFIG_FILE"
      rm -f -- "$current"
    else
      rm -f -- "$CONFIG_FILE"
    fi
    die "backup validation failed; the current configuration was preserved"
  fi
  [[ -z $current ]] || rm -f -- "$current"
  restart_fail2ban
  log "restored $backup to $CONFIG_FILE"
  exit 0
fi

if (( ! DRY_RUN )); then
  log "planned changes: install Fail2ban, write $CONFIG_FILE, and restart the service"
  if (( ! ASSUME_YES )) && [[ -t 0 ]]; then
    read -r -p "Continue? [y/N] " reply
    [[ $reply == [Yy] || $reply == [Yy][Ee][Ss] ]] || die "cancelled"
  fi
fi

if [[ $PACKAGE_MANAGER == auto ]]; then
  if command -v apt-get >/dev/null 2>&1; then PACKAGE_MANAGER=apt
  elif command -v dnf >/dev/null 2>&1; then PACKAGE_MANAGER=dnf
  elif command -v yum >/dev/null 2>&1; then PACKAGE_MANAGER=yum
  else die "no supported package manager found (apt, dnf, or yum required)"
  fi
fi

if [[ $PACKAGE_MANAGER == apt ]]; then
  run apt-get update
  run env DEBIAN_FRONTEND=noninteractive apt-get install -y fail2ban
elif [[ $PACKAGE_MANAGER == dnf ]]; then
  if is_cpanel || has_csf; then
    run dnf install -y --setopt=install_weak_deps=False --exclude=firewalld fail2ban
  else
    run dnf install -y fail2ban
  fi
elif [[ $PACKAGE_MANAGER == yum ]]; then
  if is_cpanel || has_csf; then
    run yum install -y --exclude=firewalld fail2ban
  else
    run yum install -y fail2ban
  fi
fi

service_exists() {
  local name=$1
  command -v systemctl >/dev/null 2>&1 && \
    systemctl list-unit-files "${name}.service" --no-legend 2>/dev/null | grep -q "^${name}\.service"
}

has_any_file() {
  local pattern
  for pattern in "$@"; do
    compgen -G "$pattern" >/dev/null && return 0
  done
  return 1
}

enabled_if_detected() {
  local service=$1
  shift
  if service_exists "$service" || has_any_file "$@"; then
    printf 'true'
  else
    printf 'false'
  fi
}

NGINX_DETECTED=$(enabled_if_detected nginx '/var/log/nginx/*access*.log' '/var/log/nginx/*error*.log')
if service_exists apache2 || service_exists httpd || \
   has_any_file '/var/log/apache2/*access*.log' '/var/log/httpd/*access_log' \
     '/etc/apache2/logs/access_log' '/etc/apache2/logs/domlogs/*'; then
  APACHE_DETECTED=true
else
  APACHE_DETECTED=false
fi
[[ -z $ACCESS_LOGS ]] || APACHE_DETECTED=true
POSTFIX_DETECTED=$(enabled_if_detected postfix '/var/log/mail.log' '/var/log/maillog')
DOVECOT_DETECTED=$(enabled_if_detected dovecot '/var/log/mail.log' '/var/log/maillog')

if [[ $JAILS == auto && -t 0 && $ASSUME_YES -eq 0 ]]; then
  JAILS=
  ask_jail sshd Y
  ask_jail recidive Y
  ask_jail nginx-http-auth "$([[ $NGINX_DETECTED == true ]] && printf Y || printf N)"
  ask_jail nginx-botsearch "$([[ $NGINX_DETECTED == true ]] && printf Y || printf N)"
  ask_jail nginx-bad-request "$([[ $NGINX_DETECTED == true ]] && printf Y || printf N)"
  ask_jail apache-auth "$([[ $APACHE_DETECTED == true ]] && printf Y || printf N)"
  ask_jail apache-badbots "$([[ $APACHE_DETECTED == true ]] && printf Y || printf N)"
  ask_jail apache-noscript "$([[ $APACHE_DETECTED == true ]] && printf Y || printf N)"
  ask_jail apache-botsearch "$([[ $APACHE_DETECTED == true ]] && printf Y || printf N)"
  ask_jail apache-shellshock "$([[ $APACHE_DETECTED == true ]] && printf Y || printf N)"
  ask_jail postfix "$([[ $POSTFIX_DETECTED == true ]] && printf Y || printf N)"
  ask_jail postfix-sasl "$([[ $POSTFIX_DETECTED == true ]] && printf Y || printf N)"
  ask_jail dovecot "$([[ $DOVECOT_DETECTED == true ]] && printf Y || printf N)"
  if [[ $NGINX_DETECTED == true || $APACHE_DETECTED == true ]]; then
    ask_jail web-scanners Y
    ask_jail ai-scrapers N
  fi
elif [[ $JAILS == auto ]]; then
  JAILS='sshd recidive'
  [[ $NGINX_DETECTED == true ]] && JAILS+=' nginx-http-auth nginx-botsearch nginx-bad-request web-scanners'
  [[ $APACHE_DETECTED == true ]] && JAILS+=' apache-auth apache-badbots apache-noscript apache-botsearch apache-shellshock web-scanners'
  [[ $POSTFIX_DETECTED == true ]] && JAILS+=' postfix postfix-sasl'
  [[ $DOVECOT_DETECTED == true ]] && JAILS+=' dovecot'
fi

if (selected web-scanners || selected ai-scrapers) && \
   [[ $NGINX_DETECTED != true && $APACHE_DETECTED != true ]]; then
  die "web-scanners and ai-scrapers require a detected Apache or Nginx access log"
fi

jail_state() { selected "$1" && printf true || printf false; }
SSHD_ENABLED=$(jail_state sshd)
RECIDIVE_ENABLED=$(jail_state recidive)
NGINX_HTTP_AUTH_ENABLED=$(jail_state nginx-http-auth)
NGINX_BOTSEARCH_ENABLED=$(jail_state nginx-botsearch)
NGINX_BAD_REQUEST_ENABLED=$(jail_state nginx-bad-request)
APACHE_AUTH_ENABLED=$(jail_state apache-auth)
APACHE_BADBOTS_ENABLED=$(jail_state apache-badbots)
APACHE_NOSCRIPT_ENABLED=$(jail_state apache-noscript)
APACHE_BOTSEARCH_ENABLED=$(jail_state apache-botsearch)
APACHE_SHELLSHOCK_ENABLED=$(jail_state apache-shellshock)
POSTFIX_JAIL_ENABLED=$(jail_state postfix)
POSTFIX_SASL_ENABLED=$(jail_state postfix-sasl)
DOVECOT_JAIL_ENABLED=$(jail_state dovecot)
WEB_SCANNERS_ENABLED=$(jail_state web-scanners)
AI_SCRAPERS_ENABLED=$(jail_state ai-scrapers)

FIREWALL_SETTINGS=
if is_cpanel || has_csf; then
  FIREWALL_SETTINGS=$'banaction = iptables-multiport\nbanaction_allports = iptables-allports'
  log "detected cPanel/CSF; firewalld installation is disabled and iptables actions will be used"
fi

CONFIG_DIR=${CONFIG_FILE%/*}
readonly CONFIG_DIR
TMP_FILE=$(mktemp)
readonly TMP_FILE
TMP_WEB_FILTER=$(mktemp)
readonly TMP_WEB_FILTER
TMP_AI_FILTER=$(mktemp)
readonly TMP_AI_FILTER
trap 'rm -f "$TMP_FILE" "$TMP_WEB_FILTER" "$TMP_AI_FILTER"' EXIT

cat >"$TMP_WEB_FILTER" <<'EOF'
# Managed by install-fail2ban.sh.
[Definition]
failregex = ^(?:\S+ )?<HOST> \S+ \S+ \[\] "(?:GET|POST|HEAD|OPTIONS) [^"?]*(?:/\.env(?=[./? ])|/\.git(?=[/? ])|/\.svn(?=[/? ])|/wp-config(?:\.php)?(?=[./? ])|/vendor/phpunit/|/phpunit/|/cgi-bin/[^"?]*(?:shell|cmd)|/(?:shell|wso|c99|r57|b374k|alfa|priv8|mini|cmd)\.php(?=[? ])|/xmlrpc\.php(?=[? ])|/wp-content/(?:uploads|plugins|themes)/[^"?]*\.php(?=[? ])|/(?:actuator|server-status|phpinfo\.php)(?=[/? ]))[^" ]* HTTP/\d(?:\.\d)?" \d{3} \S+(?: "[^"]*" "[^"]*")?\s*$
ignoreregex =
datepattern = ^[^\[]*\[({DATE})
              {^LN-BEG}
EOF

cat >"$TMP_AI_FILTER" <<'EOF'
# Managed by install-fail2ban.sh. Enable only when these crawlers are unwanted.
[Definition]
failregex = ^(?:\S+ )?<HOST> \S+ \S+ \[\] "(?:GET|POST|HEAD) [^"]* HTTP/\d(?:\.\d)?" \d{3} \S+ "[^"]*" "[^"]*(?:GPTBot|ChatGPT-User|OAI-SearchBot|ClaudeBot|Claude-Web|anthropic-ai|CCBot|Bytespider|PerplexityBot|YouBot|Amazonbot|Meta-ExternalAgent|cohere-ai)[^"]*"\s*$
ignoreregex =
datepattern = ^[^\[]*\[({DATE})
              {^LN-BEG}
EOF

WEB_LOGPATH=
for access_log in $ACCESS_LOGS; do
  [[ $access_log == /* ]] || die "--access-logs entries must be absolute paths"
  WEB_LOGPATH+=$'\n        '"$access_log"
done
if [[ $NGINX_DETECTED == true ]]; then
  WEB_LOGPATH+=$'\n        /var/log/nginx/*access*.log'
fi
if [[ $APACHE_DETECTED == true ]]; then
  WEB_LOGPATH+=$'\n        /var/log/apache2/*access*.log\n        /var/log/httpd/*access_log'
  if is_cpanel; then
    WEB_LOGPATH+=$'\n        /etc/apache2/logs/access_log\n        /etc/apache2/logs/domlogs/*'
  fi
fi

cat >"$TMP_FILE" <<EOF
# Managed by $PROGRAM_NAME. Re-run the installer to update this file.
[DEFAULT]
ignoreip = $IGNORE_IP
bantime = $BAN_TIME
findtime = $FIND_TIME
maxretry = $MAX_RETRY
destemail = $DEST_EMAIL
$FIREWALL_SETTINGS
backend = auto

# Increment repeat-offender bans up to one week.
bantime.increment = true
bantime.factor = 2
bantime.maxtime = 1w

[sshd]
enabled = $SSHD_ENABLED
port = $SSH_PORT
mode = aggressive

[nginx-http-auth]
enabled = $NGINX_HTTP_AUTH_ENABLED

[nginx-botsearch]
enabled = $NGINX_BOTSEARCH_ENABLED

[nginx-bad-request]
enabled = $NGINX_BAD_REQUEST_ENABLED

[apache-auth]
enabled = $APACHE_AUTH_ENABLED

[apache-badbots]
enabled = $APACHE_BADBOTS_ENABLED

[apache-noscript]
enabled = $APACHE_NOSCRIPT_ENABLED

[apache-botsearch]
enabled = $APACHE_BOTSEARCH_ENABLED

[apache-shellshock]
enabled = $APACHE_SHELLSHOCK_ENABLED

[postfix]
enabled = $POSTFIX_JAIL_ENABLED
mode = aggressive

[postfix-sasl]
enabled = $POSTFIX_SASL_ENABLED

[dovecot]
enabled = $DOVECOT_JAIL_ENABLED

[web-scanners]
enabled = $WEB_SCANNERS_ENABLED
filter = local-web-scanners
port = http,https
logpath =$WEB_LOGPATH
maxretry = 2

[ai-scrapers]
enabled = $AI_SCRAPERS_ENABLED
filter = local-ai-scrapers
port = http,https
logpath =$WEB_LOGPATH
maxretry = 1
bantime = 1d

# Ban addresses repeatedly caught by other jails.
[recidive]
enabled = $RECIDIVE_ENABLED
logpath = /var/log/fail2ban.log
bantime = 1w
findtime = 1d
maxretry = 5
EOF

if (( DRY_RUN )); then
  log "would write $CONFIG_FILE with selected jails:$JAILS"
  cat "$TMP_FILE"
  log "would write $FILTER_DIR/local-web-scanners.conf"
  cat "$TMP_WEB_FILTER"
  log "would write $FILTER_DIR/local-ai-scrapers.conf"
  cat "$TMP_AI_FILTER"
  exit 0
fi

install -d -m 0755 "$CONFIG_DIR"
install -d -m 0755 "$FILTER_DIR"
WEB_FILTER_FILE=$FILTER_DIR/local-web-scanners.conf
AI_FILTER_FILE=$FILTER_DIR/local-ai-scrapers.conf
backup=
web_backup=
ai_backup=
backup_stamp=$(date +%Y%m%d%H%M%S)
if [[ -f $CONFIG_FILE ]]; then
  backup="${CONFIG_FILE}.bak.$backup_stamp"
  cp -a "$CONFIG_FILE" "$backup"
  log "backed up existing configuration to $backup"
fi
if [[ -f $WEB_FILTER_FILE ]]; then
  web_backup="${WEB_FILTER_FILE}.bak.$backup_stamp"
  cp -a "$WEB_FILTER_FILE" "$web_backup"
fi
if [[ -f $AI_FILTER_FILE ]]; then
  ai_backup="${AI_FILTER_FILE}.bak.$backup_stamp"
  cp -a "$AI_FILTER_FILE" "$ai_backup"
fi
install -m 0644 "$TMP_FILE" "$CONFIG_FILE"
install -m 0644 "$TMP_WEB_FILTER" "$WEB_FILTER_FILE"
install -m 0644 "$TMP_AI_FILTER" "$AI_FILTER_FILE"

if ! fail2ban-client -t; then
  if [[ -n $backup ]]; then
    cp -a "$backup" "$CONFIG_FILE"
  else
    rm -f -- "$CONFIG_FILE"
  fi
  if [[ -n $web_backup ]]; then cp -a "$web_backup" "$WEB_FILTER_FILE"; else rm -f -- "$WEB_FILTER_FILE"; fi
  if [[ -n $ai_backup ]]; then cp -a "$ai_backup" "$AI_FILTER_FILE"; else rm -f -- "$AI_FILTER_FILE"; fi
  die "configuration validation failed; all managed files were rolled back"
fi

restart_fail2ban

log "Fail2ban is installed and running. Enabled detected jails:"
fail2ban-client status
log "Inspect a jail with: fail2ban-client status sshd"
