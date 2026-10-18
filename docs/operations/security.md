# Security hardening

A checklist for running sphericon in production. These are the settings that are yours to get
right; the application enforces the rest.

## Keep metrics on the internal listener

`/metrics` is never served on the public port. It is off until you set `SPHERICON_METRICS_ADDR`, which
starts a separate listener that serves metrics and nothing else (ADR 0018). There is no token
and no allowlist, so the network boundary is the only control:

- Bind to loopback on a single host (`127.0.0.1:9090`), or publish the port only to your
  Prometheus through an internal network or a network policy.
- Never route the metrics port through the public ingress or load balancer.
- Metrics carry only bounded technical labels (handler, status class, outcome), never tenant or
  personal identifiers.

`/healthz` and `/readyz` stay on the public port because orchestrator probes cannot send
credentials; they reveal only liveness and database reachability. See
[Self-hosting](../self-hosting#prometheus-metrics) for the listener setup.

## Secrets

| Secret                     | Requirement                                                                                                                                            |
| -------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `SPHERICON_JWT_SECRET`     | Outside development the server refuses to start if it is empty, shorter than 32 characters, or a known placeholder. Use `openssl rand -hex 32`.        |
| `SPHERICON_ENCRYPTION_KEY` | A base64 Tink keyset (`go run ./cmd/genkey`). Boot fails without it. Back it up separately from the database ([Backup and restore](./backup#secrets)). |
| `DATABASE_URL`             | Contains the database password. Keep it in a secret store, not in an image or a committed file.                                                        |

Supply secrets through your orchestrator's secret mechanism (a Kubernetes `Secret` referenced by
the chart's `existingSecret`, a Docker secret, or your platform's equivalent). Do not copy
example values; the examples contain none on purpose.

## Rotating secrets

Rotate on a schedule and whenever someone with access leaves or a secret may have leaked.

- **`SPHERICON_JWT_SECRET`.** Set a new value and restart every replica. All sessions are invalidated and
  users sign in again; no data is lost. Replicas must share the same value, so roll them
  together.
- **`SPHERICON_ENCRYPTION_KEY`.** The keyset format carries a key id, so a keyset can hold several keys,
  and ciphertext written with an older key stays decryptable as long as that key remains in the
  keyset. To rotate, add a new primary key to the existing keyset (for example with Tink's
  `tinkey`) and deploy. Do not replace the keyset with a freshly generated one: stored provider
  credentials could no longer be decrypted and would have to be re-entered. Keep the previous
  keyset backed up until you have confirmed that sending still works.
- **Database password.** Change it on the server, update `DATABASE_URL`, and restart the
  replicas.
- **API tokens and the collect key.** Create a new one, switch your callers, then revoke the old
  one.
- **`SPHERICON_BOOTSTRAP_TOKEN`.** Treat it as a credential: unset it once you no longer need bootstrap
  access, and change it if it leaked.
- **Provider credentials** (SMTP password, SES keys). Update them in the integration settings;
  they are stored encrypted with `SPHERICON_ENCRYPTION_KEY`.

After any rotation, record the new value in your secret store before you discard the old one.

## TLS to PostgreSQL

Production database connections should be encrypted. Use `sslmode=require` at a minimum:

```
postgres://user:pass@db.example.com:5432/sphericon?sslmode=require
```

`require` encrypts the connection but does not check who answered. To also verify the server
certificate and host name, use `verify-full` with the CA that signed it:

```
postgres://user:pass@db.example.com:5432/sphericon?sslmode=verify-full&sslrootcert=/etc/ssl/db-ca.pem
```

Use `verify-full` whenever the database is reachable over a network you do not fully control.
Never use `sslmode=disable` outside local development. Apply the same to the backup and
migration tooling that connects to the database ([Backup and restore](./backup),
[Upgrading](./upgrading)).

## Public edge

Terminate HTTPS in front of the app (Caddy, a Kubernetes Ingress, a load balancer) and serve the
site, API and tracker over TLS only. Set `SPHERICON_APP_URL` and `SPHERICON_CORS_ORIGINS` to your real origins.
Report vulnerabilities through the repository's `SECURITY.md`.
