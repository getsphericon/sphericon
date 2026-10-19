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

| Secret                     | Requirement                                                                                                                                         |
| -------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| `SPHERICON_JWT_SECRET`     | Outside development the server refuses to start if it is empty, shorter than 32 characters, or a known placeholder. Use `openssl rand -hex 32`.     |
| `SPHERICON_ENCRYPTION_KEY` | A base64 Tink keyset (`sphericon genkey`). Boot fails without it. Back it up separately from the database ([Backup and restore](./backup#secrets)). |
| `DATABASE_URL`             | Contains the database password. Keep it in a secret store, not in an image or a committed file.                                                     |

Supply secrets through your orchestrator's secret mechanism (a Kubernetes `Secret` referenced by
the chart's `existingSecret`, a Docker secret, or your platform's equivalent). Do not copy
example values; the examples contain none on purpose.

## Rotating secrets

Rotate on a schedule and whenever someone with access leaves or a secret may have leaked.

- **`SPHERICON_JWT_SECRET`.** Rotate it without signing anyone out; see
  [Rotating the JWT secret](#rotating-the-jwt-secret).
- **`SPHERICON_ENCRYPTION_KEY`.** Rotate it with the `sphericon encryption-key` commands; see
  [Rotating the encryption key](#rotating-the-encryption-key). Do not replace the keyset with a
  freshly generated one: stored provider credentials could no longer be decrypted and would have
  to be re-entered.
- **Database password.** Change it on the server, update `DATABASE_URL`, and restart the
  replicas.
- **API tokens and the collect key.** Create a new one, switch your callers, then revoke the old
  one.
- **`SPHERICON_BOOTSTRAP_TOKEN`.** Treat it as a credential: unset it once you no longer need bootstrap
  access, and change it if it leaked.
- **Provider credentials** (SMTP password, SES keys). Update them in the integration settings;
  they are stored encrypted with `SPHERICON_ENCRYPTION_KEY`.

After any rotation, record the new value in your secret store before you discard the old one.

### Rotating the encryption key {#rotating-the-encryption-key}

`SPHERICON_ENCRYPTION_KEY` is a Tink keyset: it holds several keys, one of them primary. New
secrets are encrypted under the primary key and every key in the keyset can still decrypt, so a
rotation is a sequence you can stop after any step. The commands read the keyset from
`SPHERICON_ENCRYPTION_KEY` (or its `_FILE`); `rotate` and `retire` need no database and print the
new keyset on stdout, so capture it:

1. **Rotate.** `sphericon encryption-key rotate` adds a new primary key and prints the new keyset.
   The old keys stay in it.
2. **Deploy the new keyset to every replica** (set `SPHERICON_ENCRYPTION_KEY` and restart). Do not
   go on while a replica still runs the old keyset: it could not read what the next step writes.
3. **Re-encrypt.** `sphericon encryption-key reencrypt` seals every stored secret under the primary
   key: provider credentials and their parameters, webhook signing secrets, DKIM private keys, the
   secrets of HTTP automation steps and the TOTP secrets of Second factors. It prints, per column,
   how many values it rewrote, how many were already current and how many changed while it ran
   (raced). It is idempotent, and safe to run while the instance serves traffic. It stops with an
   error naming the row when a value cannot be decrypted, which means a key is missing from the
   keyset.
4. **Confirm.** Run `reencrypt` again until it reports no rewritten and no raced values.
5. **Retire the old keys.** `sphericon encryption-key retire` prints the keyset with only the
   primary key. Deploy it to every replica.

Keep the keyset from before step 5 for as long as you keep a database backup taken before step 3:
that backup holds ciphertext under the old key and needs it to be restored
([Backup and restore](./backup#rotating-the-encryption-key)).

### Rotating the JWT secret {#rotating-the-jwt-secret}

`SPHERICON_JWT_SECRET` signs session cookies and the links in sent mail (unsubscribe, confirmation,
open and click tracking). `SPHERICON_JWT_SECRET_SECONDARY` is a second secret that is only ever used
to **verify**: a token that does not verify under the primary secret is checked against it. A
rotation takes two deploys, so that sessions and links signed with the old secret keep working
while replicas disagree about which secret is primary:

1. Generate the new secret (`openssl rand -hex 32`). Set `SPHERICON_JWT_SECRET_SECONDARY` to it on
   every replica, keep `SPHERICON_JWT_SECRET` as it is, and restart. Every replica now accepts both.
2. Swap them: `SPHERICON_JWT_SECRET` is the new secret and `SPHERICON_JWT_SECRET_SECONDARY` the old
   one. Restart every replica. New sessions are signed with the new secret and the old sessions
   still verify.
3. Wait out the transition window, then unset `SPHERICON_JWT_SECRET_SECONDARY` and restart.
   Sessions signed with the old secret stop working and their users sign in again. Wait at least
   `SPHERICON_SESSION_TTL` (24 hours by default) so no one is signed out early. Unsubscribe links
   in mail you already sent are signed with the old secret and never expire: keep the secondary
   secret for as long as you want those links to work, and a recipient who clicks one after you
   removed it sees a link that no longer works. Password reset and email verification links live for
   hours; a user asks for a new one.

If the secret leaked, skip the window: set a new `SPHERICON_JWT_SECRET` and leave the secondary
unset. Everyone signs in again, and links in sent mail stop working.

`SPHERICON_STAFF_JWT_SECRET` must differ from both secrets.

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
