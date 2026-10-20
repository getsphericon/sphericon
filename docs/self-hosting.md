# Self-hosting Sphericon

Sphericon is **one program and one database**: a Go binary (or Docker image) with the web app, the
tracker snippet and the migrations built in, and PostgreSQL. There is no Redis, no object storage
and no separate worker.

> Would rather not run it? [Sphericon Cloud](/guide/cloud) is the hosted edition.

You need:

- A server or cluster that runs Docker, a plain binary or Kubernetes.
- **PostgreSQL 14 to 18** (the test suite runs on 18). `sphericon doctor` checks it.
- A domain with HTTPS for the public address, such as `https://mail.example.com`.
- A mail provider: any SMTP server, or an SES-compatible service (Amazon SES, Yandex Cloud Postbox and others).

## Quick start

Pick one way. Each takes about five minutes and ends with the same running Instance. Three settings
are needed everywhere (the whole list is in the [Configuration reference](/configuration), but you will
not need most of it):

| Setting                    | What to put there                                                            |
| -------------------------- | ---------------------------------------------------------------------------- |
| `DATABASE_URL`             | The PostgreSQL connection string.                                            |
| `SPHERICON_JWT_SECRET`     | A random value: `openssl rand -hex 32`.                                      |
| `SPHERICON_ENCRYPTION_KEY` | A key you create once: `sphericon genkey` (see [below](#generate-the-keys)). |

and `SPHERICON_APP_URL`, the public `https://` origin, so links in your mail point at the right place.

### Docker Compose (recommended)

Runs PostgreSQL, the migrations and the app together.

1. Download the Compose file and the example environment:

   ```sh
   curl -fsSLO https://raw.githubusercontent.com/getsphericon/sphericon/main/compose.yaml
   curl -fsSL https://raw.githubusercontent.com/getsphericon/sphericon/main/compose.env.example -o .env
   ```

2. Fill in `.env`: `POSTGRES_PASSWORD`, `SPHERICON_JWT_SECRET` (`openssl rand -hex 32`) and
   `SPHERICON_ENCRYPTION_KEY` (`docker run --rm ghcr.io/getsphericon/sphericon:latest genkey`). Back the
   encryption key up with your database.
3. Start it:

   ```sh
   docker compose up -d
   ```

4. Put a [reverse proxy with HTTPS](#https-and-reverse-proxy) in front of port 3000.
5. Go to [First run](#first-run) to create your account.

### Docker

You already have PostgreSQL somewhere.

1. Generate the secrets:

   ```sh
   openssl rand -hex 32                                              # SPHERICON_JWT_SECRET
   docker run --rm ghcr.io/getsphericon/sphericon:latest genkey      # SPHERICON_ENCRYPTION_KEY
   ```

2. Run the image, which migrates the database on start:

   ```sh
   docker run -d --name sphericon -p 3000:3000 \
     -e SPHERICON_ENV=production \
     -e DATABASE_URL="postgres://user:pass@db:5432/sphericon?sslmode=require" \
     -e SPHERICON_JWT_SECRET="<from step 1>" \
     -e SPHERICON_ENCRYPTION_KEY="<from step 1>" \
     -e SPHERICON_APP_URL="https://mail.example.com" \
     -e SPHERICON_DB_AUTO_MIGRATE=true \
     ghcr.io/getsphericon/sphericon:latest
   ```

3. Put a [reverse proxy with HTTPS](#https-and-reverse-proxy) in front of port 3000.
4. Go to [First run](#first-run).

### Binary with systemd

1. Download the release for your platform from the
   [releases page](https://github.com/getsphericon/sphericon/releases) and install it as
   `/usr/local/bin/sphericon`.
2. Put the settings in a file only root can read, `/etc/sphericon/env`:

   ```sh
   sudo install -m 600 -o root /dev/null /etc/sphericon/env
   ```

   ```ini
   SPHERICON_ENV=production
   DATABASE_URL=postgres://user:pass@host:5432/sphericon?sslmode=require
   SPHERICON_JWT_SECRET=<openssl rand -hex 32>
   SPHERICON_ENCRYPTION_KEY=<sphericon genkey>
   SPHERICON_APP_URL=https://mail.example.com
   ```

3. Install the [systemd unit](#systemd) and start it:

   ```sh
   sudo systemctl enable --now sphericon
   ```

4. Put a [reverse proxy with HTTPS](#https-and-reverse-proxy) in front of port 3000.
5. Go to [First run](#first-run).

To try it without systemd, export the same variables and run `./sphericon migrate`, then `./sphericon`.

### Kubernetes

1. Create a Secret with `DATABASE_URL`, `SPHERICON_JWT_SECRET` and `SPHERICON_ENCRYPTION_KEY`:

   ```sh
   kubectl create secret generic sphericon-secrets \
     --from-literal=DATABASE_URL="postgres://user:pass@host:5432/sphericon?sslmode=require" \
     --from-literal=SPHERICON_JWT_SECRET="$(openssl rand -hex 32)" \
     --from-literal=SPHERICON_ENCRYPTION_KEY="<sphericon genkey>"
   ```

2. Install the chart:

   ```sh
   helm install sphericon oci://ghcr.io/getsphericon/charts/sphericon \
     --version 0.5.0 --set existingSecret=sphericon-secrets --set appUrl=https://mail.example.com
   ```

3. Expose the Service through your ingress with TLS, then go to [First run](#first-run). The
   [Kubernetes](#kubernetes-details) section below has the rest.

## First run

A fresh Instance lets anyone who reaches it register, and the first registration owns the first
workspace. Do not expose it before one of these:

1. **Close sign-up and create the first owner yourself** (recommended for a public host). Set
   `SPHERICON_SIGNUP_OPEN=false`, then on the server run

   ```sh
   sphericon user create-owner ada@example.com Ada Lovelace
   ```

   It creates a verified user and its first workspace and prints a one-time password; sign in with
   it and change it in your profile. The workspace is named "Personal". With sign-up closed, every
   later user comes from an owner's invitation, and accepting an invitation keeps working.

2. **Leave sign-up open** and register the first owner yourself before the host is reachable by
   anyone else (a private network or a firewall rule). Close sign-up afterwards if the instance is
   not meant to take other registrations.

Then run `sphericon doctor` (in Docker: `docker exec sphericon sphericon doctor`). It checks the
database, `SPHERICON_APP_URL` and your mail setup and says what to fix
([Diagnosing with doctor](./operations/doctor)). Finish in the app with the
[Quickstart](/guide/quickstart): connect a mail provider, verify a sending domain, send your first email.

---

## In detail

Everything below is reference for the steps above. Read what you need.

## Generate the keys

Both secrets are random values you create once and keep:

```sh
openssl rand -hex 32          # SPHERICON_JWT_SECRET
docker run --rm ghcr.io/getsphericon/sphericon:latest genkey   # SPHERICON_ENCRYPTION_KEY (or: ./sphericon genkey)
```

Back up `SPHERICON_ENCRYPTION_KEY` with your database. It encrypts the stored SMTP and SES-compatible credentials, and
without it they cannot be read again. Rotate it and `SPHERICON_JWT_SECRET` as described in [Security hardening](./operations/security#rotating-secrets).

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

> The binary tracks applied migrations (via goose) in its own `goose_db_version` table.

## Docker and Compose

The image declares a `HEALTHCHECK` against `/healthz`, so `docker ps` reports `healthy`
once the process is serving.

The `compose.yaml` from the quick start runs Postgres (18, in a named volume), a one-shot `migrate` service
(`sphericon migrate`, exits 0) and the app, which starts only after Postgres is healthy and `migrate`
has succeeded. The app keeps the image's `HEALTHCHECK`. Mailpit is not included: configure an SMTP or
SES provider in the app.

`.env` lists the required variables only; any other variable from the [Configuration reference](/configuration) goes into the
`environment` block of the `x-sphericon` anchor in `compose.yaml`. `SPHERICON_VERSION` in `.env` picks the
image tag (pin a release). To upgrade, back up, set the new tag, then `docker compose pull` and
`docker compose up -d`: `up` re-runs `migrate` first, and the app is recreated once it succeeds. See
[Upgrading](./operations/upgrading#docker) for the Majors and refusals.

## systemd

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

## Bounces, complaints and Suppression

Bounces and complaints are ingested only through an Integration's hook, read by a built-in
**Preset** (`ses`, `postmark`, `mailgun`, `generic`) or one of your own from `SPHERICON_PRESETS_DIR`
([ADR 0037](https://github.com/getsphericon/sphericon/blob/main/docs/adr/0037-inbound-provider-callbacks-are-presets-on-an-integration.md)).
**Plain SMTP with no provider callback gets no Suppression**, so SMTP alone is for tests and small
volumes. The hook URL must be reachable from the provider, so serve the Instance over public HTTPS
(see below). Setup steps for each Preset are in
[Deliverability](/guide/deliverability#bounces-and-complaints); the file format is in
[Presets](/guide/presets).

## HTTPS and reverse proxy

Sphericon speaks plain HTTP on `PORT`. Put a reverse proxy in front for TLS, and set `SPHERICON_APP_URL` to the
public `https://` origin: tracking pixels, click links, unsubscribe links and OAuth metadata are
built from it, so a wrong value breaks mail already sent. `sphericon doctor` checks that it is well formed, https
and reachable, and warns when it differs from the origin in recently sent mail
([Diagnosing with doctor](./operations/doctor)).

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

### Tracking domains

A workspace can put its mail's links on `track.<sending domain>` (see
[Tracking domain](guide/deliverability#tracking-domain)). The core never terminates TLS for those hosts:
your proxy does, and Sphericon only answers three questions ([ADR 0038](https://github.com/getsphericon/sphericon/blob/main/docs/adr/0038-tracking-domains-are-core-and-the-edge-is-not.md)).

1. Which edge host should customers CNAME to? Set `SPHERICON_TRACKING_CNAME_TARGET` (for example
   `edge.example.com`, a name that resolves to your proxy). Unset, the console shows no tracking row.
2. Which host did the request ask for? Your proxy sets the header named by
   `SPHERICON_TRACKING_HOST_HEADER` (for example `X-Forwarded-Host`) to the customer's host. It is
   believed only from a peer inside `SPHERICON_TRACKING_EDGE_CIDRS`, which defaults to loopback. When
   your proxy or ingress runs elsewhere (another container, a load balancer in your VPC), list its
   range explicitly, for example `SPHERICON_TRACKING_EDGE_CIDRS=172.18.0.0/16` for a Docker network or
   your VPC CIDR; keep it as narrow as the proxy's addresses, since any peer in it can set the host.
3. May a certificate be issued for this host? `GET /tls/ask?domain=<host>` on the internal listener
   (`SPHERICON_INTERNAL_ADDR`) answers 200 when the host is the `track.` host of a Sending domain whose
   CNAME is verified, 404 otherwise. It is on the internal listener only, so publish that port to the
   proxy and never through the public ingress.

Caddy's on-demand TLS fits directly: it asks the endpoint before it requests a certificate.

```caddyfile
{
  on_demand_tls {
    ask http://127.0.0.1:3400/tls/ask
  }
}

# the Instance's own host: drop the header so nobody can forge a tracking host
mail.example.com {
  reverse_proxy 127.0.0.1:3000 {
    header_up -X-Forwarded-Host
  }
}

# every customer Tracking domain; the certificate is issued on the first request
https:// {
  tls {
    on_demand
  }
  reverse_proxy 127.0.0.1:3000 {
    header_up X-Forwarded-Host {host}
  }
}
```

with `SPHERICON_INTERNAL_ADDR=127.0.0.1:3400`, `SPHERICON_TRACKING_CNAME_TARGET=edge.example.com`
and `SPHERICON_TRACKING_HOST_HEADER=X-Forwarded-Host`. Point `edge.example.com` at the same machine.

A domain goes `pending`, then `dns_verified` (CNAME correct, `ask` answers 200), then `serving`
(Sphericon fetched its own well-known path over HTTPS). A job rechecks every `dns_verified` and `serving` domain every 15 minutes. Until a
domain is `serving`, and again if it regresses, mail uses `SPHERICON_APP_URL`. Removing a Sending domain
or its CNAME leaves links already sent working on the Instance's host and dead on the customer's.

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

### Upgrading to the Workspace id release

Workspace slugs are gone (ADR 0043): every Workspace has an 8-character id, such as `k3Fz9aQ1`, and
it is the only address. The migration refuses to run until every Workspace has one, so upgrade
through the release that backfills the ids first if you skipped it. After the upgrade:

- Bookmarks and links that carry a slug stop working: a Workspace is now at `/workspaces/{id}/...`.
  The id never changes, not even when the Workspace is renamed.
- `sphericon workspace suspend <id> <reason...>` and `sphericon workspace unsuspend <id>` take the
  Workspace id, not the slug.
- A deleted Workspace stays restorable by its Owners for 30 days, then its data is erased and only a
  tombstone row remains.

## Kubernetes details

The chart lives in `charts/sphericon` and is published at every release as an OCI artifact,
`oci://ghcr.io/getsphericon/charts/sphericon`, with the chart version equal to the app version.
Create a Secret with `DATABASE_URL`, `SPHERICON_JWT_SECRET` and `SPHERICON_ENCRYPTION_KEY` first and
pass its name as `existingSecret` (the chart refuses to render without it). To hand the secrets over as
files instead (see [Secrets from files](/configuration#secrets-from-files)), set `secretFiles.secretName` to the Secret
and `secretFiles.keys` to the variables it holds: the chart mounts it at `/run/secrets/sphericon` and sets
each `<KEY>_FILE`; `existingSecret` is then optional.

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
internal listener only). To deploy without the chart, a Deployment needs only the required settings (see the [Configuration reference](/configuration))
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
anywhere. Without the `staff` license the listener still serves `GET /tls/ask?domain=<host>` for the edge
(200 for the Tracking domain host of a Sending domain whose CNAME is verified, 404 otherwise), and `/service` is 404. An address using the same port as `PORT` or `SPHERICON_METRICS_ADDR`, a malformed one, or one already
in use fails startup. The endpoints live under `/service` on that listener.

A credential is created, rotated and revoked only from the command line; no endpoint can mint or
change one. It carries a name and one or more scopes (`usage:read`, `workspaces:read`,
`domains:read`, `holds:write`) and an optional expiry. Its value is shown once and only a hash is stored.

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
