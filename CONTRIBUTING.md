# Contributing to sphericon

Thanks for helping. This page covers the legal side, the pull request rules and how to run the
project locally. Architecture and conventions are in [`AGENTS.md`](AGENTS.md), the domain language
in [`GLOSSARY.md`](GLOSSARY.md) and the decisions behind it in [`docs/adr/`](docs/adr/).

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

- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/)
  (`feat:`, `fix:`, `docs:`, ...); release notes are generated from them.
- Run `mise run check` and `mise run test` before you push.
- Generated files are never edited by hand; run `mise run generate` instead.

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
mise run test    # creates test DB, then `go test -p 1 ./...`
mise run check   # tsc, oxlint, oxfmt --check, knip, golangci-lint, govulncheck, gitleaks, jactionlint, zizmor
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
Brightpath's Sending domains have no serving Tracking domain in dev, so links in dev mail use
the app URL.

### MCP client

Connect Claude Code to Brightpath through the dev stack's MCP endpoint, `/mcp` on the app origin.
The fixtures carry a token for it, **Dev MCP**, with a known secret. It reads and writes Contacts,
Segments, Broadcasts and Templates and has no `mcp:send`, so the send tools are not listed. With
the stack running (`mise run dev`) and Caddy trusted (`caddy trust`):

```sh
claude mcp add --transport http sphericon https://sphericon.localhost/mcp \
  --header "Authorization: Bearer omtk_devmcp000001_dev-mcp-token-secret"
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

| Daemon   | URL / port            | Notes                                                                  |
| -------- | --------------------- | ---------------------------------------------------------------------- |
| caddy    | `:443`                | TLS + routing, see `Caddyfile`                                         |
| frontend | `:5173` (Vite)        | proxied by Caddy                                                       |
| backend  | `:3300`               | proxied by Caddy under `/site`, `/api`, `/collect`, `/auth`            |
| demo     | `:5373` (Vite)        | Brightpath storefront on https://brightpath.localhost, sends events    |
| db       | `127.0.0.1:15432`     | one mise `postgres` preset server for all checkouts (`mise run db:up`) |
| mailpit  | http://localhost:8025 | captured outbound email (SMTP UI on `:1025`)                           |

> These are the primary checkout's ports. A linked git worktree gets offset ports and its own origin
> (`https://<dir>.sphericon.localhost:<port>`, printed by `mise env`), so stacks never collide; there
> `caddy trust --address "$CADDY_ADMIN"` trusts Caddy's CA. Postgres is one server for every
> checkout (the primary's `db` daemon, started by `mise run db:up`); each checkout gets its own
> databases on it, named `sphericon_<dir>`. See AGENTS.md, "Common commands" and "Dev environment".
>
> The backend runs the real Go server under [air](https://github.com/air-verse/air) for
> hot reload. Migrations run via goose (`mise run db:migrate`); the dev backend itself does not
> self-migrate. A dev database made by an older checkout (before goose became the only runner) needs
> `mise run db:reset` once. On Linux, binding `:443` needs `net.ipv4.ip_unprivileged_port_start=0`.
> The old Grafana/OTLP dev container is gone: leave `OTEL_EXPORTER_OTLP_ENDPOINT` unset
> (the dev stack serves Prometheus metrics on `127.0.0.1:9090/metrics` via the opt-in
> `SPHERICON_METRICS_ADDR`; see [ADR 0018](docs/adr/0018-metrics-opt-in-internal-listener.md)) or point it
> at your own collector.

### Domain verification

The dev stack verifies Sending domains and Join domains against real DNS, as production does: a
domain stays unverified until its DKIM or `_sphericon-join` TXT record is published. The one
exception is **`getsphericon.io`**, which verifies at once in every development Instance, so you
can try the flow without touching DNS. Only that exact domain: `mail.getsphericon.io` goes
through real DNS. A production Instance has no shortcut. The end-to-end suite verifies every
domain it adds.

### Building

The server ships as a self-contained static binary: the React SPA, the `t.js` tracker and the
database migrations are embedded (`go:embed`, built with `-tags embed_spa`).

```sh
mise run build    # → bin/sphericon (build:tracker + build:spa + go build -tags embed_spa)
docker build -t sphericon .   # multi-stage: node build → Go build → Alpine runtime
```
