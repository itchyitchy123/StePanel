# StePanel integrations

StePanel can install and expose two companion tools from the StePanel toolkit.
Both are opt-in because they operate across the host and can change service
configuration.

## Fail2ban hardening

Enable it during installation only after declaring the trusted management
network:

```sh
sudo STEPANEL_INSTALL_FAIL2BAN=1 \
  STEPANEL_FAIL2BAN_IGNORE_IP='127.0.0.1/8 ::1 192.0.2.0/24' \
  STEPANEL_FAIL2BAN_JAILS='sshd,recidive,apache-auth,web-scanners' \
  ./install.sh
```

The installer refuses unattended Fail2ban setup without
`STEPANEL_FAIL2BAN_IGNORE_IP` to reduce lockout risk. The integrated script is
the reviewed Fail2ban Hardening Installer and retains dry-run, backup, config
validation, and restore behavior:

```sh
sudo /opt/stepanel/integrations/install-fail2ban.sh --dry-run
sudo /opt/stepanel/integrations/install-fail2ban.sh --restore-backup --dry-run
```

Keep the source project and its versioned release notes in sync when updating
the vendored installer.

## FPM Lens

FPM Lens is review-first: it inventories PHP-FPM pools, observes evidence,
plans memory-bounded changes, renders staged overrides, and can validate them
with the matching `php-fpm -tt` binary. StePanel never applies an unreviewed
plan automatically.

Install a verified FPM Lens release binary during StePanel installation:

```sh
sudo STEPANEL_FPM_LENS_BINARY=/path/to/fpm-lens ./install.sh
```

Then run the review workflow on the host:

```sh
sudo fpm-lens inventory
sudo fpm-lens doktor
sudo fpm-lens assess --samples 12 --interval-seconds 5 \
  --status-url 'pool=http://127.0.0.1/fpm-status?json'
sudo fpm-lens render fpm-lens.plan.json --output-dir /var/lib/ste-panel/fpm-review
```

Review generated files and validate them before deployment through your normal
configuration-management process. See the companion FPM Lens repository for
policy, evidence schemas, and supported layouts.
