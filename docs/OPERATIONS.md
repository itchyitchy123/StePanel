# Operations runbook

## Health check

```sh
curl -fsS http://127.0.0.1:8090/api/health | jq
```

## Logs

```sh
journalctl -u stepanel --since today
journalctl -u apache2 --since today
journalctl -u mysql --since today
```

On RHEL-family systems, the Apache and database unit names may be `httpd` and `mariadb`.

## Recovery

If an import fails, preserve the timestamped staging directory, inspect the service logs, and restore from the pre-import snapshot. Do not repeatedly retry against a live destination without identifying the failure mode.
