# Diagnosing an Instance with `sphericon doctor`

`sphericon doctor` reads the same configuration as the server (the environment and `.env` files)
and checks an Instance from the outside, so you can find what is wrong without asking for help.
Each failed check prints the fix under it. It changes nothing and sends no mail.

```sh
./sphericon doctor            # a binary
docker run --rm --env-file .env ghcr.io/getsphericon/sphericon doctor
```

```text
[ ok ] configuration: the configuration loads
[ ok ] database: schema Major 2, this binary Major 2
[ ok ] postgres version: PostgreSQL 17 (supported: 14 to 18)
[FAIL] system mail: cannot send through SMTP smtp.example.com:587: 535 authentication failed
       fix: Check SPHERICON_SMTP_HOST, SPHERICON_SMTP_PORT, SPHERICON_SMTP_USER and SPHERICON_SMTP_PASS, ...
[warn] app url in sent mail: 12 of the last 200 sent messages carry links on https://old.example.com, ...
       fix: Unsubscribe, confirm and tracking links in mail already delivered still point at the old origin: ...

20 checks, 1 failed, 1 warnings
```

## Exit code and `--json`

The exit code is `0` when no check failed and `1` when any did; a warning does not fail the run.
`--json` prints `{"ok": true|false, "checks": [{"name", "status", "detail", "fix"}]}` on stdout
and nothing else, for a monitor or a deploy pipeline. `status` is `pass`, `warn`, `fail` or `skip`
(the check does not apply, for example no Sending domains yet).

## Secrets are never printed

Every byte of output, text or JSON, passes through a filter that replaces the value of every
secret with `[redacted]`: `DATABASE_URL` (and the password inside it), `SPHERICON_JWT_SECRET`,
`SPHERICON_STAFF_JWT_SECRET`, `SPHERICON_ENCRYPTION_KEY`, `SPHERICON_SMTP_PASS`, the SES
credentials, `SPHERICON_LICENSE_KEY`, `SPHERICON_ACCOUNT_LINK_SIGNING_KEY` and
`SPHERICON_BOOTSTRAP_TOKEN`. An error from the database or the mail relay that echoes one is
filtered too. A value shorter than four characters is left alone.

## The checks

| Check                  | Passes when                                                                                                   | Fails or warns when                                                                                                 |
| ---------------------- | ------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------- |
| `configuration`        | The configuration loads like the server's.                                                                    | Fails, naming the variable, when it does not; the other checks are not run.                                         |
| `database`             | The database answers and this binary can run on its schema Major (what `migrate --status` reads).             | Fails when unreachable or when the start would be refused; warns with migrations pending.                           |
| `postgres version`     | The server's major is between 14 and 18 (the newest the test suite runs).                                     | Fails below 14; warns above 18.                                                                                     |
| `system mail`          | SMTP: the relay accepts a connection (STARTTLS when offered) and the login. SES: the region and keys are set. | Fails when the relay is unreachable or refuses the login, or when no mail is set up while email verification is on. |
| `app url`              | `SPHERICON_APP_URL` is an origin: `http(s)://host[:port]`, no path, query or credentials.                     | Fails otherwise.                                                                                                    |
| `app url https`        | The origin is `https`.                                                                                        | Fails on `http` for a public host; warns for `localhost` and development.                                           |
| `app url reachable`    | `GET /healthz` on the origin answers `200`.                                                                   | Fails on DNS, TLS, proxy or status errors.                                                                          |
| `encryption key`       | `SPHERICON_ENCRYPTION_KEY` is a valid keyset.                                                                 | Fails when missing or malformed.                                                                                    |
| `jwt secret`           | `SPHERICON_JWT_SECRET` is 32+ characters and not a documented placeholder.                                    | Fails otherwise, in every environment, development too.                                                             |
| `license key`          | None (the core), or valid.                                                                                    | Warns when expired (the Instance runs the core); fails when invalid (the server refuses to start).                  |
| `dkim <domain>`        | The DKIM TXT record of the Sending domain resolves and carries its key, through the real DNS resolver.        | Fails when the record is missing, wrong or cannot be resolved.                                                      |
| `app url in sent mail` | The newest 200 sent messages were built on the configured origin.                                             | Warns when some were built on another origin (below).                                                               |

### A changed `SPHERICON_APP_URL`

Every unsubscribe, confirm and tracking link already delivered points at the origin that was
configured when the mail was sent ([ADR 0036](../adr/0036-paths-embedded-in-sent-mail-and-customer-sites-outlive-a-major)).
Each sent message records that origin; when recent mail carries a different one, `doctor` warns.
If you moved the Instance on purpose, keep a redirect from the old origin to the new one at the
reverse proxy; the warning goes away as newer mail replaces the old in the last 200. Mail sent
before this check existed records no origin and is not compared.

Two Workspaces may add the same Sending domain; only one DKIM key can be published at a name, so
`doctor` fails the other. DKIM is always looked up through the real resolver, never the
development stub, so run `doctor` from a machine that resolves your public DNS.
