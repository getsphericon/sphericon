# Self-hosting sphericon

sphericon ships as a **single self-contained artifact** — one Go binary (or one Docker image)
with the React SPA, the tracker snippet (`/t.js`), and the database migrations all
embedded. The only external dependency at runtime is **PostgreSQL** (the background queue
and pub/sub both ride on Postgres — no Redis, no object storage, no separate worker).

- **PostgreSQL:** 14+ recommended.

> Would rather not run it? [Sphericon Cloud](/guide/cloud) is the hosted edition.

- Outbound email via SMTP or any SES-compatible service (Amazon SES, Yandex Cloud Postbox and others).

## Configuration

Configuration is read from environment variables (and, if present, `.env` files next to
the binary). `SPHERICON_ENV` selects the environment (`development` by default; set it to
`production` when self-hosting — the published Docker images already default to `production`).

| Variable                                                                                                              | Required                         | Default                                 | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| --------------------------------------------------------------------------------------------------------------------- | -------------------------------- | --------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `DATABASE_URL`                                                                                                        | **yes**                          | —                                       | PostgreSQL connection string (`postgres://user:pass@host:5432/db?sslmode=require`). Use `sslmode=verify-full` (with `sslrootcert`) to also verify the server certificate.                                                                                                                                                                                                                                                                                                 |
| `SPHERICON_JWT_SECRET`                                                                                                | **yes in prod**                  | —                                       | Signing secret for auth tokens. Outside `development`/`test` the server refuses to boot if it is empty, shorter than 32 characters, or a known placeholder. Generate one with `openssl rand -hex 32`.                                                                                                                                                                                                                                                                     |
| `SPHERICON_SESSION_TTL`                                                                                               | no                               | `24h`                                   | Lifetime of a dashboard session (a Go duration, e.g. `12h`): the session token expires and the cookie is dropped after it, with no refresh. Must be positive.                                                                                                                                                                                                                                                                                                             |
| `SPHERICON_ENCRYPTION_KEY`                                                                                            | **yes**                          | —                                       | Base64 Tink keyset used to encrypt stored provider credentials. Generate one with `sphericon genkey` (see [Generate the keys](#generate-the-keys)). Boot fails if missing.                                                                                                                                                                                                                                                                                                |
| `SPHERICON_APP_URL`                                                                                                   | no                               | `http://localhost:3000`                 | Public base URL — used when issuing auth tokens and building tracking/unsubscribe links. Set to your real origin.                                                                                                                                                                                                                                                                                                                                                         |
| `SPHERICON_MCP_URL`                                                                                                   | no                               | _(empty: `SPHERICON_APP_URL`)_          | Public origin of a dedicated MCP host, e.g. `https://mcp.example.com`. The MCP resource is then `${SPHERICON_MCP_URL}/mcp` while the OAuth issuer and consent screen stay on `SPHERICON_APP_URL`. Leave empty on a single host.                                                                                                                                                                                                                                           |
| `PORT`                                                                                                                | no                               | `3000`                                  | HTTP listen port.                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `SPHERICON_METRICS_ADDR`                                                                                              | no                               | — (off)                                 | `host:port` for the opt-in Prometheus listener. Must differ from `PORT`; see [Prometheus metrics](#prometheus-metrics).                                                                                                                                                                                                                                                                                                                                                   |
| `SPHERICON_INTERNAL_ADDR`                                                                                             | no                               | — (off)                                 | `host:port` for the opt-in internal listener that serves the Service credential endpoints of the Staff surface (needs the `staff` license). Must differ from `PORT` and `SPHERICON_METRICS_ADDR`; see [Service credentials](#service-credentials).                                                                                                                                                                                                                        |
| `SPHERICON_DB_AUTO_MIGRATE`                                                                                           | no                               | `false`                                 | Apply embedded migrations on startup. Convenient for single-replica; see below.                                                                                                                                                                                                                                                                                                                                                                                           |
| `SPHERICON_DB_MIGRATE_TIMEOUT`                                                                                        | no                               | `10m`                                   | With auto-migrate on, the longest a start waits for the migration lock and the run before it fails. `0` is no limit. `sphericon migrate` itself has no limit.                                                                                                                                                                                                                                                                                                             |
| `SPHERICON_CORS_ORIGINS`                                                                                              | no                               | —                                       | Comma/space-separated origins allowed credentialed CORS on the cookie-authenticated API (`/site`, `/auth`). Empty means same-origin only, which is what the bundled SPA needs; the bearer-token APIs are unaffected.                                                                                                                                                                                                                                                      |
| `SPHERICON_MAX_BODY_BYTES`                                                                                            | no                               | `1048576`                               | Largest accepted request body (bytes) on every surface except `/collect`; larger bodies get `413`.                                                                                                                                                                                                                                                                                                                                                                        |
| `SPHERICON_COLLECT_MAX_BODY_BYTES`                                                                                    | no                               | `512000`                                | Largest accepted `/collect` batch (`POST /collect/events`, bytes); larger gets `413`.                                                                                                                                                                                                                                                                                                                                                                                     |
| `SPHERICON_COLLECT_MAX_EVENT_BYTES`                                                                                   | no                               | `32768`                                 | Largest single `/collect` event (bytes): an identify body, or each event inside a batch; larger gets `413`.                                                                                                                                                                                                                                                                                                                                                               |
| `SPHERICON_RATE_LIMIT_HUMAN_PER_MINUTE`                                                                               | no                               | `60`                                    | Requests per minute per IP and endpoint on signup, invitation accept and consent confirm; over it `429` with `Retry-After`; `0` disables.                                                                                                                                                                                                                                                                                                                                 |
| `SPHERICON_RATE_LIMIT_API_BURST_PER_SECOND`                                                                           | no                               | `20`                                    | Requests per second per Workspace on `/api` and `/mcp` (one shared budget); over it `429` with `Retry-After`; `0` disables                                                                                                                                                                                                                                                                                                                                                |
| `SPHERICON_RATE_LIMIT_API_PER_MINUTE`                                                                                 | no                               | `600`                                   | Requests per minute per Workspace on `/api` and `/mcp`, stacked on the burst limit; `0` disables                                                                                                                                                                                                                                                                                                                                                                          |
| `SPHERICON_RATE_LIMIT_FAILED_AUTH_PER_MINUTE`                                                                         | no                               | `30`                                    | Failed bearer-token authentications per minute per IP; over it `429`; successful ones are not counted; `0` disables                                                                                                                                                                                                                                                                                                                                                       |
| `SPHERICON_RATE_LIMIT_TRACKING_PER_MINUTE`                                                                            | no                               | `600`                                   | Open and click events recorded per minute per IP; over it the redirect or pixel is still served and only the recording is skipped (never a `429`); `0` disables.                                                                                                                                                                                                                                                                                                          |
| `SPHERICON_RATE_LIMIT_LOGIN_FAILURES`                                                                                 | no                               | `5`                                     | Failed logins per account within 15 minutes before login answers `429` with `Retry-After` and a doubling delay (1 s up to 15 min, no lockout), even for a correct password; `0` disables.                                                                                                                                                                                                                                                                                 |
| `SPHERICON_RATE_LIMIT_LOGIN_IP_PER_MINUTE`                                                                            | no                               | `20`                                    | Login requests per minute per client IP; over it `429` with `Retry-After`; `0` disables.                                                                                                                                                                                                                                                                                                                                                                                  |
| `SPHERICON_RATE_LIMIT_COLLECT_PER_MINUTE`                                                                             | no                               | `6000`                                  | Requests per minute per Workspace on `/collect` (its own budget, apart from `/api`); over it `429` with `Retry-After`; `0` disables.                                                                                                                                                                                                                                                                                                                                      |
| `SPHERICON_RATE_LIMIT_SERVICE_PER_MINUTE`                                                                             | no                               | `600`                                   | Requests per minute per Service credential on the internal listener; over it `429` with `Retry-After`; `0` disables.                                                                                                                                                                                                                                                                                                                                                      |
| `SPHERICON_RATE_LIMIT_COLLECT_IP_PER_MINUTE`                                                                          | no                               | `300`                                   | Requests per minute per client IP on `/collect`; over it `429` with `Retry-After`; `0` disables.                                                                                                                                                                                                                                                                                                                                                                          |
| `SPHERICON_RATE_LIMIT_FORGOT_PASSWORD_PER_ADDRESS_PER_HOUR`                                                           | no                               | `3`                                     | Mails sent per address per hour by forgot-password, by registration and by an email-change request (one budget for each, ADR 0031); over it they still answer `202` and send nothing (the answer never reveals whether the address has an account); `0` disables.                                                                                                                                                                                                         |
| `SPHERICON_RATE_LIMIT_FORGOT_PASSWORD_IP_PER_HOUR`                                                                    | no                               | `10`                                    | Forgot-password requests per client IP per hour; over it `429` with `Retry-After`; `0` disables.                                                                                                                                                                                                                                                                                                                                                                          |
| `SPHERICON_DB_MAX_OPEN_CONNS`                                                                                         | no                               | `15`                                    | Maximum open connections in the `database/sql` pool (API, ent, event bus).                                                                                                                                                                                                                                                                                                                                                                                                |
| `SPHERICON_DB_MAX_IDLE_CONNS`                                                                                         | no                               | `SPHERICON_DB_MAX_OPEN_CONNS`           | Maximum idle connections kept in that pool. Must not exceed `SPHERICON_DB_MAX_OPEN_CONNS`.                                                                                                                                                                                                                                                                                                                                                                                |
| `SPHERICON_DB_CONN_MAX_LIFETIME`                                                                                      | no                               | `30m`                                   | Maximum lifetime of a pooled connection (Go duration).                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `SPHERICON_PGX_MAX_CONNS`                                                                                             | no                               | `25`                                    | Maximum connections in the river (job queue) pool. river's own queries (fetch, completion, LISTEN, leader election) run on it; job workers do their database work through the `database/sql` pool, so it need not match river's 20 workers (a small pool only queues completions).                                                                                                                                                                                        |
| `SPHERICON_OUTBOX_RETENTION_FLOOR_DAYS`                                                                               | no                               | `7`                                     | Minimum age (days) before a domain-event outbox row that every consumer has processed is pruned. The prune job runs every 10 minutes.                                                                                                                                                                                                                                                                                                                                     |
| `SPHERICON_UNVERIFIED_USER_RETENTION_DAYS`                                                                            | no                               | `7`                                     | Age (days) after which a registration whose email was never verified, and that holds no Membership, is deleted by a daily job at 04:00 UTC, freeing its address; `0` disables. A pending Invitation to that address is kept.                                                                                                                                                                                                                                              |
| `SPHERICON_EVENTS_RETENTION_DAYS`                                                                                     | no                               | `400`                                   | Age (days) after which analytical Events are deleted by a daily job at 03:00 UTC; `0` disables. Evidentiary Events (`marketing.confirmed`, `email.complained`, `email.unsubscribed`, permanent `email.bounced`) are never deleted by age. Segment conditions that look back past this window are capped by it.                                                                                                                                                            |
| `SPHERICON_TELEMETRY`                                                                                                 | no                               | unset                                   | Decides the anonymous Telemetry report (ADR 0032): `true` sends it and locks the in-app switch on, `false` never sends it and locks it off; unset leaves the choice to the Owner (default on).                                                                                                                                                                                                                                                                            |
| `DO_NOT_TRACK`                                                                                                        | no                               | unset                                   | `1` switches the Telemetry report off and locks the in-app switch; it can never switch the report on.                                                                                                                                                                                                                                                                                                                                                                     |
| `SPHERICON_RELEASE_CHECK`                                                                                             | no                               | `true`                                  | `false` switches the daily Release check off (ADR 0032): it asks GitHub for the newest stable release and shows Owners an update banner. It sends nothing about the Instance and is independent of the Telemetry report and `DO_NOT_TRACK`.                                                                                                                                                                                                                               |
| `SPHERICON_APP_LOCALE`                                                                                                | no                               | `en`                                    | Instance-wide UI and email language: `en`, `ru` or `es`; anything else becomes `en`.                                                                                                                                                                                                                                                                                                                                                                                      |
| `SPHERICON_REQUIRE_EMAIL_VERIFICATION`                                                                                | no                               | `true`                                  | Whether an account must prove its email address before it can sign in (ADR 0030). Only an explicit `false` turns it off, for an install without outbound mail: registration then takes a password and yields a verified user with its workspace at once. Any other value keeps it on. Never inferred from the SMTP settings.                                                                                                                                              |
| `SPHERICON_LICENSE_KEY`                                                                                               | no                               | —                                       | Enterprise Edition license key, a JWT signed with Ed25519 (`features` claim, standard `exp`); without it the single binary runs the open-source core only.                                                                                                                                                                                                                                                                                                                |
| `SPHERICON_LOG_LEVEL`                                                                                                 | no                               | `info`                                  | Log level.                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `SPHERICON_LOG_FORMAT`                                                                                                | no                               | `json` (`text` in development)          | Log format: `json` or `text`.                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `OTEL_SERVICE_NAME`                                                                                                   | no                               | `sphericon`                             | Service name reported to OpenTelemetry.                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `OTEL_CONFIG_FILE`                                                                                                    | no                               | unset (no OTLP export)                  | Path to an OpenTelemetry [declarative configuration](https://opentelemetry.io/docs/specs/otel/configuration/) YAML file: you choose which traces, metrics and logs go to which destination.                                                                                                                                                                                                                                                                               |
| `SPHERICON_SMTP_HOST` / `SPHERICON_SMTP_PORT` / `SPHERICON_SMTP_USER` / `SPHERICON_SMTP_PASS` / `SPHERICON_SMTP_FROM` | no                               | `SPHERICON_SMTP_PORT=1025`              | Outbound email over SMTP.                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `SPHERICON_SMTP_ALLOW_PRIVATE_HOSTS`                                                                                  | no                               | `false` in production, `true` otherwise | Let a Workspace's SMTP integration use a loopback or private host and any port. Off in production: tenants may only reach public hosts on ports 25, 465, 587 and 2525. The platform's own `SPHERICON_SMTP_HOST` is exempt.                                                                                                                                                                                                                                                |
| `SPHERICON_OUTBOUND_ALLOW`                                                                                            | no                               | — (nothing allowed)                     | Comma-separated `host:port` pairs (a name or an IP address, a port each; no wildcards or ranges) that outbound calls (Webhook endpoint deliveries, test events and Automation HTTP steps) may reach although the address is private or loopback, e.g. `n8n:5678,node-red:1880`, so events can go to a service on your own network. Off by default; redirects are still never followed. A malformed value stops the server at boot. Leave it unset on a multi-tenant host. |
| `SPHERICON_SYSTEM_EMAIL_PROVIDER`                                                                                     | no                               | `smtp`                                  | Platform (system) email provider: `smtp` or `ses`.                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `SPHERICON_SYSTEM_EMAIL_FROM`                                                                                         | no                               | `noreply@sphericon.localhost`           | From address for platform mail (e.g. welcome emails).                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `SPHERICON_SES_REGION` / `SPHERICON_SES_ACCESS_KEY_ID` / `SPHERICON_SES_SECRET_ACCESS_KEY`                            | no                               | —                                       | Credentials of an SES-compatible service when using the `ses` provider.                                                                                                                                                                                                                                                                                                                                                                                                   |
| `SPHERICON_COLLECT_SITE_KEY`                                                                                          | no                               | —                                       | Tracker ingestion key.                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `SPHERICON_BOOTSTRAP_TOKEN`                                                                                           | no                               | —                                       | External-API bootstrap token.                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `SPHERICON_STAFF_JWT_SECRET`                                                                                          | **yes with the `staff` license** | —                                       | Signs the platform Staff member session and login challenge (ADR 0026). Required only when the license has the `staff` feature, then at least 32 characters and different from `SPHERICON_JWT_SECRET`, or the server refuses to boot. Generate one with `openssl rand -hex 32`.                                                                                                                                                                                           |
| `SPHERICON_ACCOUNT_LINK_URL`                                                                                          | no                               | — (off)                                 | Address of an external account page. With `SPHERICON_ACCOUNT_LINK_SIGNING_KEY`, a Workspace Owner sees an Account link that opens it with a signed token; set both or neither. The app never calls this address.                                                                                                                                                                                                                                                          |
| `SPHERICON_ACCOUNT_LINK_SIGNING_KEY`                                                                                  | no                               | — (off)                                 | Base64 Ed25519 seed that signs the Account link token (generate with `openssl rand -base64 32`). The app publishes the matching public key at `/.well-known/jwks.json`, so the receiver needs no secret.                                                                                                                                                                                                                                                                  |
| `SPHERICON_STAFF_SESSION_TTL`                                                                                         | no                               | `4h`                                    | Lifetime of a Staff member session (a Go duration): shorter than `SPHERICON_SESSION_TTL`, with no refresh and no "remember me". Must be positive.                                                                                                                                                                                                                                                                                                                         |
| `SPHERICON_STAFF_HOLD_MAX_EXPIRY`                                                                                     | no                               | `720h`                                  | How far ahead a service may set an Staff hold's expiry (a Go duration); a later expiry is refused. A hold that is not renewed lapses by itself. Must be positive.                                                                                                                                                                                                                                                                                                         |
| `SPHERICON_ABUSE_VOLUME_FLOOR`                                                                                        | no                               | `1000`                                  | Abuse detector (`staff` license, ADR 0026): the fewest sends in the trailing 24 hours, per Sending domain, for its rates to be judged; below it nothing is warned or frozen. Must be positive.                                                                                                                                                                                                                                                                            |
| `SPHERICON_ABUSE_WARN_COMPLAINT_PERCENT`                                                                              | no                               | `0.1`                                   | Complaint rate (percent) at which the abuse detector warns the Workspace owners by email, freezing nothing. Above 0 and below the freeze limit.                                                                                                                                                                                                                                                                                                                           |
| `SPHERICON_ABUSE_WARN_BOUNCE_PERCENT`                                                                                 | no                               | `5`                                     | Bounce rate (percent) at which the abuse detector warns the owners. Above 0 and below the freeze limit.                                                                                                                                                                                                                                                                                                                                                                   |
| `SPHERICON_ABUSE_FREEZE_COMPLAINT_PERCENT`                                                                            | no                               | `0.3`                                   | Complaint rate (percent) at which the abuse detector suspends the whole Workspace (actor `system`) and tells its owners. At most 100.                                                                                                                                                                                                                                                                                                                                     |
| `SPHERICON_ABUSE_FREEZE_BOUNCE_PERCENT`                                                                               | no                               | `10`                                    | Bounce rate (percent) at which the abuse detector suspends the whole Workspace. At most 100.                                                                                                                                                                                                                                                                                                                                                                              |
| `SPHERICON_SUPPORT_EMAIL`                                                                                             | no                               | — (none)                                | Support contact shown to a Workspace owner with a suspension (banner and notice email) so they can appeal by email. Empty shows none.                                                                                                                                                                                                                                                                                                                                     |
| `SPHERICON_BILLING_URL`                                                                                               | no                               | — (none)                                | Billing site a Workspace member is pointed to while sending is on hold: Owners and Admins get a link to it, other members are told to contact one. Empty (the self-host default) shows no link and no billing text.                                                                                                                                                                                                                                                       |

## Generate the keys

Both secrets are random values you create once and keep:

```sh
openssl rand -hex 32          # SPHERICON_JWT_SECRET
docker run --rm ghcr.io/getsphericon/sphericon:latest genkey   # SPHERICON_ENCRYPTION_KEY (or: ./sphericon genkey)
```

Back up `SPHERICON_ENCRYPTION_KEY` with your database. It encrypts the stored SMTP and SES-compatible credentials, and
without it they cannot be read again. Changing `SPHERICON_JWT_SECRET` only signs everyone out.

## Database migrations

Migrations are embedded in the binary. You apply them one of two ways:

- **Single replica:** set `SPHERICON_DB_AUTO_MIGRATE=true` and the binary migrates on startup.
- **Multiple replicas:** leave `SPHERICON_DB_AUTO_MIGRATE` off, so a heavy migration never runs
  under an orchestrator's liveness probe. Run the migration step once before rolling out the new
  version:

  ```sh
  ./sphericon migrate      # applies pending migrations and exits
  ```

  `migrate` has no time limit and takes a database lock, so two migrations started together run
  one after the other and the second finds nothing to do. `./sphericon migrate --status` prints the
  schema's current state, the newest state the binary knows and the pending migrations, and changes
  nothing. With auto-migrate on, a start waits for the lock and the run for at most
  `SPHERICON_DB_MIGRATE_TIMEOUT` and fails when it runs out.

  Use it as a pre-deploy job or a Kubernetes init container, then start the servers
  without `SPHERICON_DB_AUTO_MIGRATE`.

Before upgrading, take a database backup; see [Upgrading](./operations/upgrading) for the
full procedure and [Backup and restore](./operations/backup) for what to protect.

> The binary tracks applied migrations (via goose) in its own `goose_db_version` table. Don't point
> it at a database previously managed by the Atlas dev-CLI flow (which uses
> `atlas_schema_revisions`).

## Run it

### Docker

```sh
docker run -p 3000:3000 \
  -e SPHERICON_ENV=production \
  -e DATABASE_URL="postgres://user:pass@db:5432/sphericon?sslmode=require" \
  -e SPHERICON_JWT_SECRET="$(openssl rand -hex 32)" \
  -e SPHERICON_ENCRYPTION_KEY="<base64 tink keyset>" \
  -e SPHERICON_APP_URL="https://mail.example.com" \
  -e SPHERICON_DB_AUTO_MIGRATE=true \
  ghcr.io/getsphericon/sphericon:latest
```

The image declares a `HEALTHCHECK` against `/healthz`, so `docker ps` reports `healthy`
once the process is serving.

### Binary

```sh
export SPHERICON_ENV=production
export DATABASE_URL="postgres://user:pass@host:5432/sphericon?sslmode=require"
export SPHERICON_JWT_SECRET="$(openssl rand -hex 32)"
export SPHERICON_ENCRYPTION_KEY="<base64 tink keyset>"
export SPHERICON_APP_URL="https://mail.example.com"

./sphericon migrate   # apply migrations (or set SPHERICON_DB_AUTO_MIGRATE=true)
./sphericon           # start the server
```

### systemd

For the bare binary, a unit keeps it running and restarts it on failure. Put the environment in a
file only root can read:

```ini
# /etc/systemd/system/sphericon.service
[Unit]
Description=sphericon
After=network-online.target postgresql.service
Wants=network-online.target

[Service]
User=sphericon
EnvironmentFile=/etc/sphericon/env
ExecStartPre=/usr/local/bin/sphericon migrate
ExecStart=/usr/local/bin/sphericon
Restart=on-failure
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
```

```sh
sudo install -m 600 -o root /dev/null /etc/sphericon/env   # then add KEY=value lines
sudo systemctl enable --now sphericon
```

`ExecStartPre` runs the migrations before every start, so leave `SPHERICON_DB_AUTO_MIGRATE` unset.
After `sphericon upgrade` has migrated, it finds nothing left to do.

To upgrade this unit, run one command; there is no second unit file to install:

```sh
sudo /usr/local/bin/sphericon upgrade --restart
```

It reads this unit's `User` and `EnvironmentFile` with `systemctl show`, runs the new binary's
`migrate` as a transient unit with them (`systemd-run`) while the old binary keeps serving,
swaps `/usr/local/bin/sphericon` (the old one stays as `sphericon.old`) and runs
`systemctl restart sphericon`. It exits non-zero if the service is not active afterwards. A unit
with another name is passed as `--service <name>`. Without `--restart` it prints
`systemctl restart sphericon` and leaves that step to you. See [Upgrading](./operations/upgrading)
for `--check`, `--version`, `--from` (offline) and what to do when a step refuses.

## HTTPS and reverse proxy

sphericon speaks plain HTTP on `PORT`. Put a reverse proxy in front for TLS, and set `SPHERICON_APP_URL` to the
public `https://` origin: tracking pixels, click links, unsubscribe links and OAuth metadata are
built from it, so a wrong value breaks mail already sent.

Caddy gets and renews certificates by itself:

```caddyfile
mail.example.com {
  reverse_proxy 127.0.0.1:3000
}
```

nginx:

```nginx
server {
  listen 443 ssl;
  server_name mail.example.com;
  # ssl_certificate / ssl_certificate_key ...

  client_max_body_size 1m;

  location / {
    proxy_pass http://127.0.0.1:3000;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $remote_addr;
    proxy_set_header X-Forwarded-Proto $scheme;
  }
}
```

Do not expose `SPHERICON_METRICS_ADDR` through the proxy.

The external API can also live on its own host, such as `api.example.com`, if the proxy rewrites
`/*` to `/api/*` (`rewrite ^/(.*)$ /api/$1 break;` in nginx, `rewrite * /api{uri}` in Caddy). The
binary itself stays path-based.

## Backups and upgrades

All state is in PostgreSQL, so a backup is a database backup plus your `SPHERICON_ENCRYPTION_KEY`:

```sh
pg_dump --format=custom --file=sphericon-$(date +%F).dump "$DATABASE_URL"
```

To upgrade, back up, then run the one command for how you run the Instance: `sphericon upgrade`
for a bare binary (`--restart` on systemd), `helm upgrade` on Kubernetes, `docker pull` and
recreate for the image. None of them stops a running Instance; see
[Upgrading](./operations/upgrading) for each, the offline path and the refusals. Pin a version tag
rather than `latest` in production. Migrations only move forward, so to roll back restore the
backup taken before the upgrade.

## Kubernetes

The chart lives in `charts/sphericon` and is published at every release as an OCI artifact,
`oci://ghcr.io/getsphericon/charts/sphericon`, with the chart version equal to the app version.
Create a Secret with `DATABASE_URL`, `SPHERICON_JWT_SECRET` and `SPHERICON_ENCRYPTION_KEY` first and
pass its name as `existingSecret` (the chart refuses to render without it):

```sh
helm install sphericon oci://ghcr.io/getsphericon/charts/sphericon \
  --version 0.5.0 --set existingSecret=sphericon-secrets --set appUrl=https://mail.example.com
helm upgrade sphericon oci://ghcr.io/getsphericon/charts/sphericon --version 0.5.0 --reuse-values
```

The chart runs `sphericon migrate` as a pre-install and pre-upgrade hook Job and keeps
`SPHERICON_DB_AUTO_MIGRATE=false` on the Deployment, so a failed migration fails the release before a
pod is replaced. It sets `/healthz` as the liveness probe and `/readyz` as the readiness probe, runs
two replicas behind a disruption budget, and leaves metrics off unless you set `metrics.enabled`
(`metrics.serviceMonitor.enabled` adds a Prometheus Operator ServiceMonitor that scrapes the
internal listener only). To deploy without the chart, a Deployment needs only the environment above
and those two probes, with `sphericon migrate` as a pre-deploy Job.

## Health checks

| Endpoint       | Purpose   | Behaviour                                                                               |
| -------------- | --------- | --------------------------------------------------------------------------------------- |
| `GET /healthz` | Liveness  | Always `200 {"status":"ok"}` while the process serves; touches no dependency.           |
| `GET /readyz`  | Readiness | Pings the database; `200` when reachable, `503` (`application/problem+json`) otherwise. |

Wire `/healthz` to liveness and `/readyz` to readiness probes (Kubernetes, load
balancers, the Docker `HEALTHCHECK`).

## Service credentials

A service can call the Staff surface without a human login, with a **Service credential**
(ADR 0026). It needs the `staff` license feature; without it nothing is served and the commands
refuse.

The service endpoints are served only by a separate internal listener, never on the public port.
It is off by default: set `SPHERICON_INTERNAL_ADDR` (`host:port`, for example `127.0.0.1:3400` or
`0.0.0.0:3400` in a container) to bind it, and publish that port only to the calling service, never
through the public ingress. An empty `SPHERICON_INTERNAL_ADDR` means no listener and no route for the endpoints
anywhere. An address using the same port as `PORT` or `SPHERICON_METRICS_ADDR`, a malformed one, or one already
in use fails startup. The endpoints live under `/service` on that listener.

A credential is created, rotated and revoked only from the command line; no endpoint can mint or
change one. It carries a name and one or more scopes (`usage:read`, `workspaces:read`,
`holds:write`) and an optional expiry. Its value is shown once and only a hash is stored.

```sh
sphericon service-credential create --expires-in 8760h service-a usage:read,workspaces:read
sphericon service-credential rotate 7      # a second credential with the same name and scopes
sphericon service-credential revoke 7      # once the service has switched over
sphericon service-credential list service-a
```

A service may hold two active credentials at once, so a rotation needs no downtime. `list` shows each
credential's state, expiry and when it was last used. The service presents the value as
`Authorization: Bearer <value>`; a revoked or expired credential, and one used outside its scopes, is
refused. Every write made under a credential is recorded in the Audit log under its name. `GET
/service/identity` answers with the credential's own name, scopes and expiry.

## Prometheus metrics

Metrics are off by default and are never served on the public port. Set `SPHERICON_METRICS_ADDR` to
start a dedicated listener that serves `GET /metrics` (Prometheus exposition) and nothing
else. There is no token and no allowlist: the network boundary is the control (ADR 0018).

- Single host: `SPHERICON_METRICS_ADDR=127.0.0.1:9090` so only local scrapers can reach it.
- Container: `SPHERICON_METRICS_ADDR=0.0.0.0:9090`, and publish that port only to your Prometheus
  (a network policy or an internal network), never through the public ingress. An empty host (`:9090`) binds every interface, like
  the container form.

A `SPHERICON_METRICS_ADDR` using the same port as `PORT`, or a malformed value, fails startup, as does
a metrics address that is already in use. The Dockerfiles need no change (`EXPOSE 3000` and
the `HEALTHCHECK` stay). Scrape example:

```yaml
scrape_configs:
  - job_name: sphericon
    static_configs:
      - targets: ['sphericon:9090']
```

Pushing telemetry elsewhere (OTLP, stdout, several destinations at once) is configured in the file `OTEL_CONFIG_FILE` names; it is independent of the scrape endpoint, which stays served whatever the file says. The standard `OTEL_EXPORTER_OTLP_*` variables are not read: name the endpoint in the file (it can reference environment variables as `${VAR}`).

## Operations

Sizing, [backup and restore](./operations/backup) and [upgrading](./operations/upgrading)
(including PostgreSQL major upgrades) are covered in the [Operations](./operations/) section.
What the Instance sends, how to preview it and how to turn it off (`SPHERICON_TELEMETRY`, `DO_NOT_TRACK`,
`SPHERICON_RELEASE_CHECK`) is on [Telemetry and the Release check](./operations/telemetry).
