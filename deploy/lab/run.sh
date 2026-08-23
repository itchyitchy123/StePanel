#!/usr/bin/env bash
set -Eeuo pipefail

archive="${1:-/tmp/stepanel-demo.tar.gz}"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/homedir/public_html" "$work/mysql"
printf '%s\n' '<?php echo "StePanel demo";' > "$work/homedir/public_html/index.php"
printf '%s\n' 'CREATE TABLE demo (id INT PRIMARY KEY);' > "$work/mysql/demo.sql"
tar -C "$work" -czf "$archive" homedir mysql

echo "Created synthetic archive: $archive"
echo "Inspect it through the dashboard at http://localhost:8080"
echo "The lab intentionally uses authentication-disabled mode and synthetic data only."
