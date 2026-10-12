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
the binary). `APP_ENV` selects the environment (`development` by default; set it to
`production` when self-hosting — the published Docker images already default to `production`).

| Variable                                                            | Required                            | Default                        | Description                                                                                                                                                                                                                                                                                                    |
| ------------------------------------------------------------------- | ----------------------------------- | ------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `DATABASE_URL`                                                      | **yes**                             | —                              | PostgreSQL connection string (`postgres://user:pass@host:5432/db?sslmode=require`). Use `sslmode=verify-full` (with `sslrootcert`) to also verify the server certificate.                                                                                                                                      |
| `JWT_SECRET`                                                        | **yes in prod**                     | —                              | Signing secret for auth tokens. Outside `development`/`test` the server refuses to boot if it is empty, shorter than 32 characters, or a known placeholder. Generate one with `openssl rand -hex 32`.                                                                                                          |
| `SESSION_TTL`                                                       | no                                  | `24h`                          | Lifetime of a dashboard session (a Go duration, e.g. `12h`): the session token expires and the cookie is dropped after it, with no refresh. Must be positive.                                                                                                                                                  |
| `ENCRYPTION_KEY`                                                    | **yes**                             | —                              | Base64 Tink keyset used to encrypt stored provider credentials. Generate one with `go run ./cmd/genkey` (or `sphericon`-side tooling). Boot fails if missing.                                                                                                                                                  |
| `APP_URL`                                                           | no                                  | `http://localhost:3000`        | Public base URL — used when issuing auth tokens and building tracking/unsubscribe links. Set to your real origin.                                                                                                                                                                                              |
| `MCP_URL`                                                           | no                                  | _(empty: `APP_URL`)_           | Public origin of a dedicated MCP host, e.g. `https://mcp.example.com`. The MCP resource is then `${MCP_URL}/mcp` while the OAuth issuer and consent screen stay on `APP_URL`. Leave empty on a single host.                                                                                                    |
| `PORT`                                                              | no                                  | `3000`                         | HTTP listen port.                                                                                                                                                                                                                                                                                              |
| `METRICS_ADDR`                                                      | no                                  | — (off)                        | `host:port` for the opt-in Prometheus listener. Must differ from `PORT`; see [Prometheus metrics](#prometheus-metrics).                                                                                                                                                                                        |
| `INTERNAL_ADDR`                                                     | no                                  | — (off)                        | `host:port` for the opt-in internal listener that serves the Service credential endpoints of the Operator surface (needs the `operator` license). Must differ from `PORT` and `METRICS_ADDR`; see [Service credentials](#service-credentials).                                                                 |
| `AUTO_MIGRATE`                                                      | no                                  | `false`                        | Apply embedded migrations on startup. Convenient for single-replica; see below.                                                                                                                                                                                                                                |
| `CORS_ORIGINS`                                                      | no                                  | —                              | Comma/space-separated origins allowed credentialed CORS on the cookie-authenticated API (`/site`, `/auth`). Empty means same-origin only, which is what the bundled SPA needs; the bearer-token APIs are unaffected.                                                                                           |
| `MAX_BODY_BYTES`                                                    | no                                  | `1048576`                      | Largest accepted request body (bytes) on every surface except `/collect`; larger bodies get `413`.                                                                                                                                                                                                             |
| `COLLECT_MAX_BODY_BYTES`                                            | no                                  | `512000`                       | Largest accepted `/collect` batch (`POST /collect/events`, bytes); larger gets `413`.                                                                                                                                                                                                                          |
| `COLLECT_MAX_EVENT_BYTES`                                           | no                                  | `32768`                        | Largest single `/collect` event (bytes): an identify body, or each event inside a batch; larger gets `413`.                                                                                                                                                                                                    |
| `RATE_LIMIT_HUMAN_PER_MINUTE`                                       | no                                  | `60`                           | Requests per minute per IP and endpoint on signup, invitation accept and consent confirm; over it `429` with `Retry-After`; `0` disables.                                                                                                                                                                      |
| `RATE_LIMIT_API_BURST_PER_SECOND`                                   | no                                  | `20`                           | Requests per second per Workspace on `/api` and `/mcp` (one shared budget); over it `429` with `Retry-After`; `0` disables                                                                                                                                                                                     |
| `RATE_LIMIT_API_PER_MINUTE`                                         | no                                  | `600`                          | Requests per minute per Workspace on `/api` and `/mcp`, stacked on the burst limit; `0` disables                                                                                                                                                                                                               |
| `RATE_LIMIT_FAILED_AUTH_PER_MINUTE`                                 | no                                  | `30`                           | Failed bearer-token authentications per minute per IP; over it `429`; successful ones are not counted; `0` disables                                                                                                                                                                                            |
| `RATE_LIMIT_TRACKING_PER_MINUTE`                                    | no                                  | `600`                          | Open and click events recorded per minute per IP; over it the redirect or pixel is still served and only the recording is skipped (never a `429`); `0` disables.                                                                                                                                               |
| `RATE_LIMIT_LOGIN_FAILURES`                                         | no                                  | `5`                            | Failed logins per account within 15 minutes before login answers `429` with `Retry-After` and a doubling delay (1 s up to 15 min, no lockout), even for a correct password; `0` disables.                                                                                                                      |
| `RATE_LIMIT_LOGIN_IP_PER_MINUTE`                                    | no                                  | `20`                           | Login requests per minute per client IP; over it `429` with `Retry-After`; `0` disables.                                                                                                                                                                                                                       |
| `RATE_LIMIT_COLLECT_PER_MINUTE`                                     | no                                  | `6000`                         | Requests per minute per Workspace on `/collect` (its own budget, apart from `/api`); over it `429` with `Retry-After`; `0` disables.                                                                                                                                                                           |
| `RATE_LIMIT_SERVICE_PER_MINUTE`                                     | no                                  | `600`                          | Requests per minute per Service credential on the internal listener; over it `429` with `Retry-After`; `0` disables.                                                                                                                                                                                           |
| `RATE_LIMIT_COLLECT_IP_PER_MINUTE`                                  | no                                  | `300`                          | Requests per minute per client IP on `/collect`; over it `429` with `Retry-After`; `0` disables.                                                                                                                                                                                                               |
| `RATE_LIMIT_FORGOT_PASSWORD_PER_ADDRESS_PER_HOUR`                   | no                                  | `3`                            | Password-reset mails sent per address per hour; over it forgot-password still answers `202` and sends nothing (the answer never reveals whether the account exists); `0` disables.                                                                                                                             |
| `RATE_LIMIT_FORGOT_PASSWORD_IP_PER_HOUR`                            | no                                  | `10`                           | Forgot-password requests per client IP per hour; over it `429` with `Retry-After`; `0` disables.                                                                                                                                                                                                               |
| `DB_MAX_OPEN_CONNS`                                                 | no                                  | `15`                           | Maximum open connections in the `database/sql` pool (API, ent, event bus).                                                                                                                                                                                                                                     |
| `DB_MAX_IDLE_CONNS`                                                 | no                                  | `DB_MAX_OPEN_CONNS`            | Maximum idle connections kept in that pool. Must not exceed `DB_MAX_OPEN_CONNS`.                                                                                                                                                                                                                               |
| `DB_CONN_MAX_LIFETIME`                                              | no                                  | `30m`                          | Maximum lifetime of a pooled connection (Go duration).                                                                                                                                                                                                                                                         |
| `PGX_MAX_CONNS`                                                     | no                                  | `25`                           | Maximum connections in the river (job queue) pool. river's own queries (fetch, completion, LISTEN, leader election) run on it; job workers do their database work through the `database/sql` pool, so it need not match river's 20 workers (a small pool only queues completions).                             |
| `OUTBOX_RETENTION_FLOOR_DAYS`                                       | no                                  | `7`                            | Minimum age (days) before a domain-event outbox row that every consumer has processed is pruned. The prune job runs every 10 minutes.                                                                                                                                                                          |
| `EVENTS_RETENTION_DAYS`                                             | no                                  | `400`                          | Age (days) after which analytical Events are deleted by a daily job at 03:00 UTC; `0` disables. Evidentiary Events (`marketing.confirmed`, `email.complained`, `email.unsubscribed`, permanent `email.bounced`) are never deleted by age. Segment conditions that look back past this window are capped by it. |
| `APP_LOCALE`                                                        | no                                  | `en`                           | Instance-wide UI and email language: `en`, `ru` or `es`; anything else becomes `en`.                                                                                                                                                                                                                           |
| `LICENSE_KEY`                                                       | no                                  | —                              | Enterprise Edition license key; without it the single binary runs the open-source core only.                                                                                                                                                                                                                   |
| `LOG_LEVEL`                                                         | no                                  | `info`                         | Log level.                                                                                                                                                                                                                                                                                                     |
| `LOG_FORMAT`                                                        | no                                  | `json` (`text` in development) | Log format: `json` or `text`.                                                                                                                                                                                                                                                                                  |
| `OTEL_SERVICE_NAME`                                                 | no                                  | `sphericon`                    | Service name reported to OpenTelemetry.                                                                                                                                                                                                                                                                        |
| `SMTP_HOST` / `SMTP_PORT` / `SMTP_USER` / `SMTP_PASS` / `SMTP_FROM` | no                                  | `SMTP_PORT=1025`               | Outbound email over SMTP.                                                                                                                                                                                                                                                                                      |
| `SYSTEM_EMAIL_PROVIDER`                                             | no                                  | `smtp`                         | Platform (system) email provider: `smtp` or `ses`.                                                                                                                                                                                                                                                             |
| `SYSTEM_EMAIL_FROM`                                                 | no                                  | `noreply@sphericon.localhost`  | From address for platform mail (e.g. welcome emails).                                                                                                                                                                                                                                                          |
| `SES_REGION` / `SES_ACCESS_KEY_ID` / `SES_SECRET_ACCESS_KEY`        | no                                  | —                              | Credentials of an SES-compatible service when using the `ses` provider.                                                                                                                                                                                                                                        |
| `COLLECT_SITE_KEY`                                                  | no                                  | —                              | Tracker ingestion key.                                                                                                                                                                                                                                                                                         |
| `BOOTSTRAP_TOKEN`                                                   | no                                  | —                              | External-API bootstrap token.                                                                                                                                                                                                                                                                                  |
| `OPERATOR_JWT_SECRET`                                               | **yes with the `operator` license** | —                              | Signs the platform Operator session and login challenge (ADR 0026). Required only when the license has the `operator` feature, then at least 32 characters and different from `JWT_SECRET`, or the server refuses to boot. Generate one with `openssl rand -hex 32`.                                           |
| `ACCOUNT_LINK_URL`                                                  | no                                  | — (off)                        | Address of an external account page. With `ACCOUNT_LINK_SIGNING_KEY`, a Workspace Owner sees an Account link that opens it with a signed token; set both or neither. The app never calls this address.                                                                                                         |
| `ACCOUNT_LINK_SIGNING_KEY`                                          | no                                  | — (off)                        | Base64 Ed25519 seed that signs the Account link token (generate with `openssl rand -base64 32`). The app publishes the matching public key at `/.well-known/jwks.json`, so the receiver needs no secret.                                                                                                       |
| `OPERATOR_SESSION_TTL`                                              | no                                  | `4h`                           | Lifetime of an Operator session (a Go duration): shorter than `SESSION_TTL`, with no refresh and no "remember me". Must be positive.                                                                                                                                                                           |

## Generate the keys

Both secrets are random values you create once and keep:

```sh
openssl rand -hex 32          # JWT_SECRET
docker run --rm ghcr.io/getsphericon/sphericon:latest genkey   # ENCRYPTION_KEY (or: ./sphericon genkey)
```

Back up `ENCRYPTION_KEY` with your database. It encrypts the stored SMTP and SES-compatible credentials, and
without it they cannot be read again. Changing `JWT_SECRET` only signs everyone out.

## Database migrations

Migrations are embedded in the binary. You apply them one of two ways:

- **Single replica:** set `AUTO_MIGRATE=true` and the binary migrates on startup.
- **Multiple replicas:** do **not** use `AUTO_MIGRATE` (replicas would race). Run the
  migration step once before rolling out the new version:

  ```sh
  ./sphericon migrate      # applies pending migrations and exits
  ```

  Use it as a pre-deploy job or a Kubernetes init container, then start the servers
  without `AUTO_MIGRATE`.

Before upgrading, take a database backup; see [Upgrading](./operations/upgrading) for the
full procedure and [Backup and restore](./operations/backup) for what to protect.

> The binary tracks applied migrations (via goose) in its own `goose_db_version` table. Don't point
> it at a database previously managed by the Atlas dev-CLI flow (which uses
> `atlas_schema_revisions`).

## Run it

### Docker

```sh
docker run -p 3000:3000 \
  -e APP_ENV=production \
  -e DATABASE_URL="postgres://user:pass@db:5432/sphericon?sslmode=require" \
  -e JWT_SECRET="$(openssl rand -hex 32)" \
  -e ENCRYPTION_KEY="<base64 tink keyset>" \
  -e APP_URL="https://mail.example.com" \
  -e AUTO_MIGRATE=true \
  ghcr.io/getsphericon/sphericon:latest
```

The image declares a `HEALTHCHECK` against `/healthz`, so `docker ps` reports `healthy`
once the process is serving.

### Binary

```sh
export APP_ENV=production
export DATABASE_URL="postgres://user:pass@host:5432/sphericon?sslmode=require"
export JWT_SECRET="$(openssl rand -hex 32)"
export ENCRYPTION_KEY="<base64 tink keyset>"
export APP_URL="https://mail.example.com"

./sphericon migrate   # apply migrations (or set AUTO_MIGRATE=true)
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

`ExecStartPre` runs the migrations before every start, so leave `AUTO_MIGRATE` unset.

## HTTPS and reverse proxy

sphericon speaks plain HTTP on `PORT`. Put a reverse proxy in front for TLS, and set `APP_URL` to the
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

Do not expose `METRICS_ADDR` through the proxy.

The external API can also live on its own host, such as `api.example.com`, if the proxy rewrites
`/*` to `/api/*` (`rewrite ^/(.*)$ /api/$1 break;` in nginx, `rewrite * /api{uri}` in Caddy). The
binary itself stays path-based.

## Backups and upgrades

All state is in PostgreSQL, so a backup is a database backup plus your `ENCRYPTION_KEY`:

```sh
pg_dump --format=custom --file=sphericon-$(date +%F).dump "$DATABASE_URL"
```

To upgrade, back up, then pull the new image (or replace the binary) and restart. Migrations are
applied as described [above](#database-migrations). Pin a version tag rather than `latest` in
production, and read the release notes before a major version. Migrations only move forward, so
to roll back restore the backup taken before the upgrade.

## Kubernetes

There is no Helm chart yet. The image is a plain stateless container, so a Deployment needs only
the environment above, `/healthz` as the liveness probe and `/readyz` as the readiness probe.
Run `sphericon migrate` as a pre-deploy Job or init container and keep `AUTO_MIGRATE` unset when you
run more than one replica.

## Health checks

| Endpoint       | Purpose   | Behaviour                                                                               |
| -------------- | --------- | --------------------------------------------------------------------------------------- |
| `GET /healthz` | Liveness  | Always `200 {"status":"ok"}` while the process serves; touches no dependency.           |
| `GET /readyz`  | Readiness | Pings the database; `200` when reachable, `503` (`application/problem+json`) otherwise. |

Wire `/healthz` to liveness and `/readyz` to readiness probes (Kubernetes, load
balancers, the Docker `HEALTHCHECK`).

## Service credentials

A service can call the Operator surface without a human login, with a **Service credential**
(ADR 0026). It needs the `operator` license feature; without it nothing is served and the commands
refuse.

The service endpoints are served only by a separate internal listener, never on the public port.
It is off by default: set `INTERNAL_ADDR` (`host:port`, for example `127.0.0.1:3400` or
`0.0.0.0:3400` in a container) to bind it, and publish that port only to the calling service, never
through the public ingress. An empty `INTERNAL_ADDR` means no listener and no route for the endpoints
anywhere. An address using the same port as `PORT` or `METRICS_ADDR`, a malformed one, or one already
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

Metrics are off by default and are never served on the public port. Set `METRICS_ADDR` to
start a dedicated listener that serves `GET /metrics` (Prometheus exposition) and nothing
else. There is no token and no allowlist: the network boundary is the control (ADR 0018).

- Single host: `METRICS_ADDR=127.0.0.1:9090` so only local scrapers can reach it.
- Container: `METRICS_ADDR=0.0.0.0:9090`, and publish that port only to your Prometheus
  (a network policy or an internal network), never through the public ingress. An empty host (`:9090`) binds every interface, like
  the container form.

A `METRICS_ADDR` using the same port as `PORT`, or a malformed value, fails startup, as does
a metrics address that is already in use. The Dockerfiles need no change (`EXPOSE 3000` and
the `HEALTHCHECK` stay). Scrape example:

```yaml
scrape_configs:
  - job_name: sphericon
    static_configs:
      - targets: ['sphericon:9090']
```

OTLP push (`OTEL_EXPORTER_OTLP_*`) is independent of this and unchanged.

## Operations

Sizing, [backup and restore](./operations/backup) and [upgrading](./operations/upgrading)
(including PostgreSQL major upgrades) are covered in the [Operations](./operations/) section.
