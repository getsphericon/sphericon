# Contributing to sphericon

Thanks for helping. This page covers the legal side, the pull request rules and how to run the
project locally. The architecture is in [`docs/architecture.md`](docs/architecture.md), the coding
standards and the rules every change is reviewed against in [`docs/standards/`](docs/standards/),
the domain language in [`GLOSSARY.md`](GLOSSARY.md) and the decisions behind it in
[`docs/adr/`](docs/adr/). A coding agent starts from [`AGENTS.md`](AGENTS.md).

## Contributor License Agreement

Every contributor signs the [Contributor License Agreement](CLA.md) once, before a pull
request can be merged. It lets the Project Owner distribute your contribution under any
license, which sphericon needs as an open-core project (AGPL core, commercial `ee/`).
You keep the copyright in your work.

When you open your first pull request, the CLA assistant comments on it. Reply with the
sentence it asks for, and the `license/cla` check turns green. The signature is tied to
your GitHub account and covers all your later pull requests.

## Enterprise Edition

The `ee/` directory is not open source ([`ee/LICENSE`](ee/LICENSE)). Pull requests that
change `ee/` are accepted only from people with a separate written agreement with the
Project Owner.

## Pull requests

- Commit messages and the pull request title follow
  [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `docs:`, ...);
  release notes are generated from them, and the `pr-title` check fails a title that does not parse.
- Run `mise run check` and `mise run test` before you push.
- Generated files are never edited by hand; run `mise run generate` instead.
- A change that is more than a typo or a doc line starts from an issue a maintainer has accepted
  (label `ready-for-agent` or `ready-for-human`); name it in the pull request.
- Fill in the pull request template. The `pr-body` check fails until exactly one option of its
  "AI assistance" section is checked, and until each `contract-copy:` marker your change adds has a
  `Contract-Copy: <file>: <reason>` line in the body.

## AI-assisted contributions

Pull requests written with an AI tool are welcome on the same terms as any other. You are the author:
you answer for every line, for its license and for what it does.

- Read the whole diff before you open the pull request, and understand it. Open it yourself; an agent
  does not open it for you. Write the description in your own words: what changed and why.
- Say how an AI tool was used in the "AI assistance" section of the pull request template.
- A commit written with an agent may end with a `Co-Authored-By:` or an `Assisted-by:` trailer naming
  the tool; either is accepted. `Signed-off-by` and the signature of the CLA are yours alone.
- The checks that run on every pull request run on yours; an agent that skips them (`mise run check`,
  `mise run test`) only moves the failure to your review.
- Maintainers may close a pull request that does not meet these terms, without a review.

## Stack

A Go backend and a React/Vite frontend in a single repo. The data model is
workspace-scoped (multi-tenant): contacts, events, API tokens, and tracking entities all
belong to a `workspace`.

- **Backend** — Go + [ogen](https://ogen.dev/) (HTTP servers in `gen/*`), ORM
  [ent](https://entgo.io/), queues [river](https://riverqueue.com/), pub/sub
  [watermill](https://watermill.io/), migrations diffed by ent, applied by [goose](https://github.com/pressly/goose).
- **Frontend** — React 19 + Vite, [Mantine](https://mantine.dev/),
  [TanStack Router/Query](https://tanstack.com/), and the generated
  [`@hey-api/openapi-ts`](https://heyapi.dev/) client + react-query hooks.
- **API contracts** — [TypeSpec](https://typespec.io/) as the single source of truth.

## Code generation

The pipeline is one-directional: TypeSpec → OpenAPI → generated Go + TS. Never hand-edit
anything under `openapi/`, `gen/`, `ent/` (except `ent/schema/`), or `src/generated/` /
`packages/analytics/src/generated/` — regenerate instead.

```
typespec/{site,external,collect}   ──tsp compile──▶  openapi/*.openapi.json
  openapi/site      ──ogen──▶ gen/site       (Go server)  ──openapi-ts──▶ src/generated/site (TS client + react-query + zod)
  openapi/external  ──ogen──▶ gen/external   (Go server)
  openapi/collect   ──ogen──▶ gen/collect    (Go server)  ──openapi-ts──▶ packages/analytics/src/generated/collect (types only)
ent/schema/*.go     ──entc──▶ ent/*          (Go ORM)
```

```sh
mise run generate          # full cycle: typespec → openapi → backend (ent + ogen) → frontend → i18n types → format
mise run generate:typespec # typespec → openapi only
mise run generate:backend  # openapi → Go only (ent + ogen)
mise run generate:openapi  # openapi → TS client only
```

After changing TypeSpec or `ent/schema`, run `mise run generate` and commit the generated output.

## Development

**The only prerequisite is [mise](https://mise.jdx.dev).** It installs and pins the whole
toolchain from `.mise.toml` / `mise.lock` (Go, Node, pnpm, golangci-lint, air,
Caddy, Mailpit, gitleaks, jactionlint, zizmor, hk, …), runs the dev stack as daemons and
installs the git hooks.

```sh
mise install # install the pinned toolchain (also installs the git hooks)
mise run setup   # install deps, start Postgres, create dev/test/diff DBs, migrate, seed
mise run dev     # start the whole dev stack (mise daemons)
mise run test    # creates test DB, then `go test ./...` (packages in parallel); never run a bare `go test`
mise run check   # tsc, oxlint, oxfmt --check, knip, golangci-lint (+ our analyzers), easement, gitleaks, jactionlint, zizmor, links
mise run generate # regenerate TypeSpec → OpenAPI → Go + TS
```

CI installs the same toolchain with `jdx/mise-action` and runs the same mise tasks (`mise tasks` lists them all).

### Dev accounts

`mise run setup` seeds the dev database with the same `fixtures/*.yml` the tests use, so a
fresh checkout already has a populated Workspace, **Brightpath** (an online course school with
contacts, events, broadcasts, automations, segments, templates and integrations). Acme and the
other Workspaces (Globex, Initech, Umbrella, …) exist for isolation and edge-case tests. Brightpath's
default Integration is SMTP to the dev Mailpit, so a broadcast or automation sent from it shows up
in the Mailpit UI. These are development fixtures only; never reuse the credentials anywhere else.

Sign in at `https://sphericon.localhost/login`:

| Role   | Workspace  | Email                     | Password   |
| ------ | ---------- | ------------------------- | ---------- |
| owner  | Brightpath | `maya@brightpath.example` | `password` |
| admin  | Brightpath | `ana@brightpath.example`  | `password` |
| member | Brightpath | `milo@brightpath.example` | `password` |
| owner  | Acme       | `info@getsphericon.com`   | `password` |
| admin  | Acme       | `alice@acme.test`         | `password` |
| member | Acme       | `mary@acme.test`          | `password` |
| owner  | Globex     | `jane@globex.test`        | `password` |

Brightpath has all three roles, so one Workspace is enough for development. `sam@globex.test` / `password` has a TOTP Second factor, secret `JBSWY3DPEHPK3PXP`.

Brightpath's Mailpit Integration sends to `127.0.0.1:1025`, the primary checkout's SMTP port. A
linked worktree's Mailpit SMTP port is offset (`SPHERICON_SMTP_PORT`) and a fixture cannot follow
it, because fixtures also load in tests: change the Integration's port in its settings there.
Brightpath's Sending domains keep the default `pending` Tracking state in the fixtures, so no
Tracking domain serves and links in dev mail use the app URL.

### MCP client

Connect Claude Code to Brightpath through the dev stack's MCP endpoint, `/mcp` on the app origin.
The fixtures carry a token for it, **Dev MCP**, with a known secret. It reads and writes Contacts,
Segments, Broadcasts and Templates and has no `mcp:send`, so the send tools are not listed. With
the stack running (`mise run dev`) and Caddy trusted (`caddy trust`):

```sh
claude mcp add --transport http sphericon https://sphericon.localhost/mcp \
  --header "Authorization: Bearer omtk_devmcp000001_dev-mcp-token-secret-0000000000000000000"
```

In a linked worktree use its origin instead (`mise env | grep SPHERICON_APP_URL`), plus `/mcp`.
Then `/mcp` in Claude Code shows `sphericon` connected, and "list the Brightpath contacts" calls
`contacts_list`; "create a contact dev@brightpath.example" calls `contacts_create`. `mise run
db:seed` removes what you wrote.

The Staff console is at `https://sphericon.localhost/console` (Enterprise, license feature
`staff`: without a license key it answers 404). Sign-in needs a password and a TOTP code:

| Staff member                 | Password   | TOTP                            |
| ---------------------------- | ---------- | ------------------------------- |
| `staff@sphericon.test`       | `password` | secret `KRSXG5CTMVRXEZLU`       |
| `fresh-staff@sphericon.test` | `password` | none yet: enrols at first login |

Get the current code with `oathtool --totp -b KRSXG5CTMVRXEZLU`, or add the secret to an
authenticator app.

The values come from `fixtures/users.yml` and `fixtures/staff_members.yml`; when they change,
update this section. `mise run db:seed` reloads the fixtures.

### The dev stack

The local stack is a set of [mise daemons](https://mise.jdx.dev) (`[daemons]` in
`.mise.toml`) behind [Caddy](https://caddyserver.com/) with HTTPS.

1. Install the toolchain, install deps, create the dev/test/diff databases, migrate:

   ```sh
   mise install
   mise run setup
   ```

   No `.env` is required: `.mise.toml` supplies the dev defaults, including `DATABASE_URL`
   (`mise run db:up` starts the Postgres behind it). Do **not** set `DATABASE_URL` in `.env` (real env vars win
   over it). Personal overrides go in the gitignored `.env` (read by the app) or
   `.mise.local.toml` (read by mise).

2. Trust Caddy's local CA once, so the browser accepts `https://sphericon.localhost`:

   ```sh
   caddy trust
   ```

3. Start the stack:

   ```sh
   mise run dev        # mise daemons start caddy (starts everything it depends on)
   mise run dev:down   # stop it
   mise daemons logs backend   # follow a daemon's output; `mise daemons ls` shows status
   ```

The entry point is **https://sphericon.localhost** (Caddy terminates TLS with its internal CA).
Daemons:

| Daemon   | URL / port                   | Notes                                                                           |
| -------- | ---------------------------- | ------------------------------------------------------------------------------- |
| caddy    | `:443`                       | TLS + routing, see `Caddyfile`                                                  |
| frontend | `:5173` (Vite)               | proxied by Caddy                                                                |
| backend  | `:3300`                      | proxied by Caddy under `/site`, `/api`, `/collect`, `/auth`                     |
| demo     | `:5373` (Vite)               | Brightpath storefront on https://brightpath.localhost, sends events             |
| db       | `127.0.0.1:15432`            | one mise `postgres` preset server for all checkouts (`mise run db:up`)          |
| mailpit  | https://mailpit.localhost    | captured outbound email (UI `:8025` behind Caddy, SMTP `:1025`)                 |
| dns      | `127.0.0.1:15353` (UDP, TCP) | CoreDNS, the proof records of Sending domains and Join domains (`dev/Corefile`) |

> These are the primary checkout's ports. A linked git worktree gets offset ports and its own origin
> (`https://<dir>.sphericon.localhost:<port>`, printed by `mise env`), so stacks never collide; there
> `caddy trust --address "$CADDY_ADMIN"` trusts Caddy's CA. Postgres is one server for every
> checkout (the primary's `db` daemon, started by `mise run db:up`); each checkout gets its own
> databases on it, named `sphericon_<dir>`. See docs/agents/dev-environment.md.
>
> The backend runs the real Go server under [air](https://github.com/air-verse/air) for
> hot reload. Migrations run via goose (`mise run db:migrate`); the dev backend itself does not
> self-migrate. A dev database made by an older checkout (before goose became the only runner) needs
> `mise run db:reset` once. On Linux, binding `:443` needs `net.ipv4.ip_unprivileged_port_start=0`.
> The old Grafana/OTLP dev container is gone: leave `OTEL_EXPORTER_OTLP_ENDPOINT` unset
> (the dev stack serves Prometheus metrics on `127.0.0.1:9090/metrics` via the opt-in
> `SPHERICON_METRICS_ADDR`; see [ADR 0018](docs/adr/0018-metrics-opt-in-internal-listener.md)) or point it
> at your own collector.

### The Brightpath demo

Development is built around one demo Workspace, **Brightpath** (an online course school), that
`mise run setup` seeds from `fixtures/*.yml`. Every page has something to show the moment you sign
in, in more than one state:

| Area            | What is seeded                                                                                                                                                                                            |
| --------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| People          | 48 contacts: free, pro and team, with 0 to 7 completed courses, three sources, tags and custom fields of every type (string, number, bool, datetime); three visitors bound to a contact and two anonymous |
| Segments        | six, from a plain filter to nested and event-based conditions                                                                                                                                             |
| Analytics       | about 400 events over three months (viewed pricing, sign-ups, purchases, lessons and courses started or completed, email opens and clicks) behind every chart                                             |
| Broadcasts      | draft, scheduled, sent (six, with recipients and engagement) and failed                                                                                                                                   |
| Automations     | five active and one draft; their runs are active, completed, exited and failed (HTTP status, timeout, network)                                                                                            |
| Rules           | two active (one tags who views pricing, one enrols a new signup in the Welcome series) and one draft; firings of both                                                                                     |
| Messages        | sent, delivered, bounced, skipped and failed, from broadcasts, automations and transactional emails                                                                                                       |
| Suppressions    | a bounce, a complaint and a manual one                                                                                                                                                                    |
| Test inbox      | six captured messages (broadcast, automation, transactional and a test send), all inside the retention window                                                                                             |
| Integrations    | the dev Mailpit (default), Amazon SES and Postmark SMTP, and the Test inbox                                                                                                                               |
| Sending domains | `mail.`, `news.`, `send.brightpath.localhost` verified (mail's Tracking domain is `dns_verified`) and `launch.brightpath.localhost` pending                                                               |
| Join domains    | `brightpath.example` verified, `learn.brightpath.example` pending                                                                                                                                         |
| Webhooks        | one healthy, one failing, one disabled by the owner and one the platform disabled after repeated failures                                                                                                 |
| Team            | owner, admin and member, a pending invitation and an expired one                                                                                                                                          |
| Settings        | a Brand kit, six API tokens, an Audit log of eight entries (it needs a license key with the audit feature), four Confirmations of each provenance                                                         |

To see what a person does on a site, open the demo storefront at https://brightpath.localhost (a
linked worktree prints its own origin in `mise env`): a customer site with the tracker installed, so
what you click there arrives as events through the `collect.` host and shows up in the app, and the
two Rules react to it. The storefront has no login: it identifies whoever types an email in the
sign-up form or the cart, and the events join the contact with that address. So **a different
person is a different email**:

| To play                       | Use                              |
| ----------------------------- | -------------------------------- |
| a new visitor                 | any address not in the fixtures  |
| a free learner, nothing done  | `sofia.marino@northwind.example` |
| a free learner with a course  | `lena.hoffmann@litware.example`  |
| a pro learner                 | `zara.khan@northwind.example`    |
| a team member                 | `omar.salem@tailspin.example`    |
| a webinar attendee            | `layla.haddad@litware.example`   |
| a suppressed address (bounce) | `pavel.novak@adatum.example`     |

Sign in to the app as the Brightpath owner, admin or member (see the table above) in one browser
profile and browse the storefront as someone else in another.

When a fixture lacks a case you need, add a row to `fixtures/` rather than creating it by hand every
time: the same rows feed the tests, and `mise run db:seed` reloads them. Keep a seeded row in a state
no background job rewrites: a past-due scheduled Broadcast would be sent and a pending Outbound
message claimed, and the Test inbox keeps a message for seven days only, so date it in hours.

### Sending mail in development

Nothing leaves your machine. There are two ways to send:

- **Test inbox.** A Workspace's built-in provider that delivers nowhere and keeps a copy of each
  message ([ADR 0058](docs/adr/0058-a-test-inbox-is-a-provider-the-send-path-does-not-know.md)).
  Use it when you only need to see a message.
- **SMTP through Mailpit.** Brightpath's default Integration already points at the dev Mailpit.
  To add the same Integration to any other Workspace, use these SMTP settings:

  | Setting        | Value                                                                    |
  | -------------- | ------------------------------------------------------------------------ |
  | Host           | `127.0.0.1`                                                              |
  | Port           | `1025` (`SPHERICON_SMTP_PORT`; a linked worktree's is offset, see below) |
  | Username, pass | empty: Mailpit takes any message without authentication                  |
  | Encryption     | none (no TLS, no STARTTLS)                                               |

  Every message shows up in the Mailpit UI at https://mailpit.localhost. A private host and any
  port are allowed in development (`SPHERICON_SMTP_ALLOW_PRIVATE_HOSTS`).

The send gate needs a verified Sending domain for the From address; the next section says how
one gets verified.

### Domain verification

A Sending domain or a Join domain is verified against real DNS, as in production, and the dev
stack has its own DNS server for it: a [CoreDNS](https://coredns.io) daemon (`dns`) that the
backend reads through `SPHERICON_DNS_ADDR`, set on the backend daemon only, so the tests and CI
keep their stubs. It answers the zones in `dev/dns/`, one file `db.<domain>` per zone, and
every other name is NXDOMAIN. Nothing reaches the internet.

**Brightpath's domains need nothing from you.** `dev/dns/db.brightpath.localhost` publishes the
DKIM record of `mail.`, `news.` and `send.brightpath.localhost` (the key the fixtures give them) and
the Tracking domain CNAME of `mail.`; `dev/dns/db.brightpath.example` publishes the Join domain
proof of `brightpath.example`. These domains are verified and stay so under the background re-check.
`launch.brightpath.localhost` is `pending` and has no record on purpose: publish it as below and press
**Verify** to see the flow, and a Tracking domain moves from `pending` to `dns_verified` the same way
(its `track.<domain>` CNAME points at `edge.sphericon.localhost`, `SPHERICON_TRACKING_CNAME_TARGET`).
`serving` needs the edge to answer an HTTPS probe, which a development stack cannot offer, so no
fixture sits in it.

**A domain you create yourself** gets a fresh DKIM key (or Join token), which no zone can know
ahead, so it stays unverified until you publish its record:

1. Add the domain in the app (any name, for example `blabla.localhost`) and copy the record it
   shows: the name and the TXT value.
2. Create `dev/dns/db.blabla.localhost` (gitignored, like every `db.*` file but Brightpath's):

   ```
   $ORIGIN blabla.localhost.
   $TTL 60
   @ IN SOA ns.blabla.localhost. hostmaster.blabla.localhost. 1 3600 600 86400 60
   @ IN NS ns.blabla.localhost.
   sphericon._domainkey IN TXT "v=DKIM1; k=rsa; p=<the key>"
   ```

   A Join domain's record goes the same way: `_sphericon-join IN TXT "<token>"`. A TXT string
   holds 255 bytes at most, so split a long key into several quoted strings on one line.

3. CoreDNS reloads the zone within seconds. Check it with
   `dig +short @127.0.0.1 -p "$DNS_PORT" TXT sphericon._domainkey.blabla.localhost`, then press
   **Verify**.

The end-to-end suite does not use this server: it verifies every domain it adds through its own
resolver.

### Building

The server ships as a self-contained static binary: the React SPA, the `t.js` tracker and the
database migrations are embedded (`go:embed`, built with `-tags embed_spa`).

```sh
mise run build    # → bin/sphericon (build:tracker + build:spa + go build -tags embed_spa)
docker build -t sphericon .   # multi-stage: node build → Go build → Alpine runtime
```
