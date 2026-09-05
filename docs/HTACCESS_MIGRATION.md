# Migrating Apache `.htaccess` rules to Caddy

Caddy is StePanel's default webserver. Caddy does not read Apache `.htaccess`
files, so StePanel provides a conservative migration tool at
`POST /api/caddy/htaccess` and in the dashboard.

The tool currently translates:

- common WordPress, Laravel, and other front-controller rewrites using
  `RewriteCond %{REQUEST_FILENAME} !-f`, `!-d`, and an `index.php` rewrite;
- Apache `Redirect` rules with safe local or HTTP(S) destinations;
- `RewriteEngine On`, root `RewriteBase`, WordPress's authorization-header
  workaround, `Options -Indexes`, and module wrappers when Caddy already
  provides equivalent behavior.

Every other directive is returned as a line-numbered warning. Applying a
conversion with warnings fails closed unless `allow_partial` is explicitly
set. Partial mode applies only the recognized rules; it never copies unknown
Apache syntax into Caddy configuration.

## Dashboard workflow

1. Restore or create the managed site and its isolated PHP-FPM pool.
2. Open **Import .htaccess rules** on the dashboard.
3. Enter the site, domain, and `.htaccess` content.
4. Preview the conversion and resolve every warning where practical.
5. Apply it. StePanel writes a root-owned Caddy PHP site, validates the complete
   Caddyfile, reloads Caddy, and restores the prior configuration on failure.

The importer targets the document-root `.htaccess` behavior. Nested
`.htaccess` files and directives whose meaning depends on Apache module or
directory context require manual review.

## API example

```sh
curl --fail --cookie cookies.txt \
  -H 'Content-Type: application/json' \
  -H "X-CSRF-Token: $csrf" \
  --data "$(jq -n --rawfile content /var/www/sites/example/public/.htaccess \
    '{site:"example",domain:"www.example.com",content:$content,action:"preview"}')" \
  https://panel.example.com/api/caddy/htaccess
```

Change `action` to `apply` after reviewing the returned `caddy_directives` and
`warnings`. Keep `allow_partial` false unless an administrator has manually
accounted for every warning.

For an offline conversion, pipe the file into the StePanel binary:

```sh
stepanel convert-htaccess < .htaccess
```

The command writes translated directives to stdout and warnings to stderr. It
returns status 2 when warnings exist, making unattended migrations fail closed.

## Important limitations

The translator deliberately does not emulate arbitrary `RewriteRule` regular
expressions, access-control directives, authentication, custom error documents,
MIME mappings, Apache environment variables, or module-specific behavior.
Translate those requirements manually using reviewed Caddy matchers and
handlers. Preserve the original file and test authentication, redirects,
uploads, PHP routing, and application-generated permalinks before changing DNS.
