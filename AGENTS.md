# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

sphericon is an **open-core marketing automation platform** (with a planned SaaS offering).
Go backend + React/Vite frontend in a single repo. The data model is workspace-scoped
(multi-tenant): contacts, events, api tokens, and tracking entities all belong to a
`workspace`.

**The project is greenfield.** There are no production users and no legacy contracts to
preserve: no backward compatibility, no compat shims, deprecation paths or fallbacks.
Rewrite whatever the cleanest design needs: the TypeSpec contract, ent schema, tests and
every caller (regenerate afterwards), rather than layering the new design over the old.

## Codegen pipeline (read this first)

API contracts are **one-directional**: TypeSpec → OpenAPI → generated Go + TS. Never
hand-edit anything under `openapi/`, `gen/`, `ent/` (except `ent/schema/`),
`src/generated/` / `packages/analytics/src/generated/`, or the `*_gen.go` files in the
`internal/api/{site,external}/resources` packages and `internal/fixtures/catalog_gen.go`, or any `*_gen_test.go` mock — regenerate instead.

```
typespec/{site,external,collect,operator,service}   ──tsp compile──▶  openapi/*.openapi.json
  openapi/site      ──ogen──▶ gen/site       (Go server)   ──openapi-ts──▶ src/generated/site (TS client + react-query + zod)
  openapi/external  ──ogen──▶ gen/external    (Go server)
  openapi/collect   ──ogen──▶ gen/collect     (Go server)  ──openapi-ts──▶ packages/analytics/src/generated/collect (types only)
  openapi/operator  ──ogen──▶ gen/operator    (Go server)  ──openapi-ts──▶ src/generated/operator (TS client + react-query + zod)
  openapi/service   ──ogen──▶ gen/service     (Go server, internal listener only)
ent/schema/*.go     ──entc──▶ ent/*           (Go ORM)
ent/schema/*.go + ent/template/scoped*.tmpl  ──entc──▶ ent/scoped.go, ent/scoped_registry.go  (scoped client: `client.Scoped(ws)`)
ent + gen/{site,external}  ──goverter──▶ internal/api/{site,external}/resources/converter_gen.go
fixtures/*.yml (`# fixture: Name` rows)  ──cmd/fixturegen──▶ internal/fixtures/catalog_gen.go (named test constants)
Go interfaces (`mise run generate:mocks` task list)  ──moq──▶ <pkg>/mocks_gen_test.go (test mocks; add an interface to the task list)
```

- **goverter** maps ent entities → ogen resource DTOs. The `Converter` interface and its
  `// goverter:extend` helpers (ids, timestamps, optionals) are hand-written in
  each package's `resources.go`; the impl (`ConverterImpl`) is generated. Adding a DTO
  field that has no source mapping fails generation — that completeness check is the point.

- `mise run generate` — full cycle: typespec → openapi → backend (ent + ogen) → fixture catalog → frontend → i18n types → format.
- `mise run generate:typespec` / `mise run generate:backend` / `mise run generate:openapi` — partial regens.
- The README claims echo + oapi-codegen; that is **outdated**. The HTTP stack is **ogen**
  (`gen/*` are ogen servers, wired in `internal/server/server.go`).

### What is generated vs hand-written

Generated files carry a header (`Code generated … DO NOT EDIT`, or `// @ts-nocheck` on the
frontend) and are flagged `linguist-generated` in `.gitattributes` (GitHub collapses them in
diffs). Generated code lives in dedicated dirs — `internal/` is otherwise hand-written, its
only generated files being the two `converter_gen.go`, `internal/fixtures/catalog_gen.go` and the moq `*_gen_test.go` mocks.

## Common commands

Every task runs natively with the toolchain pinned in `.mise.toml` / `mise.lock` (Go and
Node track `latest`); CI installs the same toolchain with `jdx/mise-action` and runs the same
tasks. There are no dev containers and no Makefile: tasks live in `mise/tasks.toml` (included
from `.mise.toml`); `mise tasks` lists them. The dev stack (Postgres, Mailpit, backend, Vite,
Caddy) is a set of **mise daemons** (`[daemons]` in `.mise.toml`, experimental). Tasks that need
Postgres depend on `db:up` (below) instead of a `db` daemon of their own.

```sh
mise run setup          # deps, db:up, migrate, seed, worktrees:gc
mise run db:up          # ensure the one shared Postgres runs and this checkout's databases exist
mise run dev            # db:up, then mise daemons start caddy — full dev stack (https://sphericon.localhost; a linked worktree has its own URL, see "Dev environment")
mise run dev:down       # stop this checkout's dev stack (never the shared Postgres)
mise run test           # creates the test DB, then `go test ./...` (packages in parallel)
mise run check          # tsc + oxlint + oxfmt --check + knip + golangci-lint (incl. gofmt) + gitleaks + jactionlint + zizmor
mise run test:cover     # Go suite with coverage, fails under the total in .testcoverage.yml (nightly in CI)
mise run check:vulns    # govulncheck (nightly in CI, not part of `check`)
mise run fix            # i18n extract + oxlint --fix + tsp format + oxfmt + go fmt
mise run generate       # typespec -> openapi -> backend -> frontend -> i18n types -> format
```

**One Postgres for every checkout.** The `db` daemon (mise `postgres` preset, fixed port 15432)
is declared in `.mise.toml` but only ever runs in the **primary checkout** (the first
`git worktree list` entry: a stable project root that `mise daemons prune` never sees as
deleted); `mise run db:up` starts it from any checkout and nothing else starts a `db` daemon.
A postmaster per worktree takes one of macOS's 32 SysV shared-memory segments
(`kern.sysv.shmmni`), so ~30 worktrees exhaust them and Postgres fails with `shmget ... No space
left on device`. `db:up` is idempotent: a server already answering at `PGHOST:PGPORT` is a no-op
(so two checkouts calling it at once are safe: at most one start, the other waits for the port),
otherwise it runs `mise -C <primary> daemons start db`, then creates this checkout's databases
if missing under an advisory lock. Every task that needs Postgres (`test`, `test:e2e`, `db:*`)
depends on `db:up`, and so does `dev`; `dev:down` leaves the server running
(`mise -C <primary> daemons stop db` stops it for everyone). The primary checkout must be on a
commit that has this setup.

`PGHOST`, `PGPORT`, `PGUSER`, `PGDATABASE` and `DATABASE_URL` are set in `[env]`, so they are
identical in every checkout without a daemon (a `db` daemon exports its own offset port and
database for a linked worktree, which `[env]` overrides). The database is `sphericon_<dir>`: the
checkout's directory name lowercased to `[a-z0-9_]`, cut to 31 characters plus a hash of the path
when it exceeds 40, so with the `_test`, `_e2e` and `_atlas` twins (`TEST_DB_URL`, `E2E_DB_URL`,
`ATLAS_DEV_URL`; `DB_NAMES` lists all four) every name stays under Postgres's 63-byte identifier
limit and nothing is truncated or collides. Never set `DATABASE_URL` in `.env`: explicit values
override it. `SPHERICON_PG_HOST` / `SPHERICON_PG_PORT` point every checkout at another server (an
external Postgres): one that answers is used as is and never started. CI runs the same path: its
single checkout is its own primary.

Never hand-name a scratch database. `mise run worktrees:gc` (also run by `setup` and
`worktrees:prune`; `--apply` to act, a dry run otherwise) drops the shared server's databases that
no live worktree's `DB_NAMES` covers and runs `mise daemons prune --yes` for dead state; it
refuses to drop anything when it cannot enumerate or read a live worktree, including one on a
commit from before `DB_NAMES` (merge main into it), and never touches the primary's daemon.
It skips the `testdb_tpl_*` databases: those are pgtestdb's template and per-test databases (the
`testhelper.RealDB` family, below), owned by a test that may be running in any checkout, and
pgtestdb drops each when its test ends.
`mise run worktrees:prune -- --apply` first removes the worktrees whose branch is already in
`origin/main`. A worktree from before this setup still runs its own `db` daemon: it retires with
its next merge of main plus `mise daemons stop`. Several tasks in one command need `:::` between
them (`mise run check:fe ::: check:deps`); otherwise the extra names become arguments.

Run Go tests only through `mise run test`: it sets `APP_ENV=test` and the test DB, and a bare
`go test` opens the dev database and fails.

Test packages run in parallel, each process on a database of its own, through
`pgcleaner.WithTemplate` (wired in `internal/testhelper`): it leases `<test db>_w<i>` (an advisory
lock on the shared test database, released when the process ends), cloned from
`<test db>_tpl_<hash>`, a schema-only template built once per hash of the schema and the bus. The
fixtures are loaded into the worker at the start of every process, because they use relative
dates. `DATABASE_URL` is rewritten to that database for the process, so a test that loads the
config reaches it too. `worktrees:gc` keeps these with their checkout. The end-to-end suite keeps
one database and `-p 1`.

Run a single Go test (arguments after `--` go to `go test`; the default is `./...`):

```sh
mise run test -- ./internal/api/site -run TestSiteContactsRequireAuth
```

CI runs the Go tests with `-shuffle=on` (the seed is printed; reproduce an order with
`mise run test -- -shuffle=<seed>`), so a test must not depend on another's side effects. Locally
`mise run test` leaves shuffling off because any `-shuffle` flag disables Go's test result cache: an
unchanged package is `(cached)` on the next run (`-count=1` forces a run).

End-to-end suite (ADR 0024, `e2e/` behind the `e2e` build tag, not part of `mise run test`):
`mise run test:e2e` rebuilds the checkout's `<PGDATABASE>_e2e` database, starts its own Mailpit and boots the
app in-process. `HOLD=true mise run test:e2e` keeps the app and Mailpit up after the run (their URLs are
printed) until Ctrl-C, to inspect a failed scenario in the Mailpit UI. Scenarios are domain steps on
`e2e.Workspace` (`env.NewWorkspace(t).Ready()`, `ImportContacts`, `SendBroadcast`), one flat struct whose steps live in non-test files by concept
(`workspace.go`, `integration.go`, `contacts.go`, `broadcast.go`, `automation.go`, `consent.go`); `_test.go`
files hold only scenarios, and bodies are authored with `e2e.MJML(text)`. Mail is observed only through
the Workspace's `Inbox`: one wait (`w.Inbox.Wait(e2e.Match{To, Subject})`) and one absence check
(`w.Inbox.RequireNone`); it deletes every message it saw on cleanup.

Frontend tests: `mise run test:watch`.

**Nightly, not per PR.** `.github/workflows/nightly.yml` runs `mise run test:cover` and
`mise run check:vulns` (govulncheck) once a night and on demand (`workflow_dispatch`). The coverage
floor in `.testcoverage.yml` is raised when coverage rises, never lowered. A new advisory can fail
`check:vulns` with no change on our side, so it is deliberately not part of `check`: put a check
there when its result does not depend on the diff under review.

**A test asserts what an operation left behind** (rows, outbox events, enqueued jobs, the response),
never what was called, how often or with which arguments: no call-verification tests. Do not assert
how many rows a table holds (a new fixture breaks that) or a raw id: count the rows you start from
and assert business facts (slug, order, label). A test that depends on today's date does not read
the wall clock; it injects the clock.

## Database & migrations

- ORM is **ent**; schemas live in `ent/schema/*.go`, generated code in `ent/`.
- Migrations are managed by **Atlas** (`atlas.hcl`, dir `migrations/`), run natively, diffed from the ent schema: `mise run db:generate name=<desc>` then `mise run db:migrate`.
  Atlas reads its target/dev DB URLs from the environment (`DATABASE_URL` / `ATLAS_DEV_URL`),
  using a scratch `<PGDATABASE>_atlas` database on the same server instead of `docker://`.
- `mise run db:reset` / `db:reset-test` to rebuild local DBs.
- Test DB is separate (`APP_ENV=test`); tests create schema via `ent` `Schema.Create`, not Atlas.

## Backend architecture

- **DI**: `samber/do` container. `internal/app/app.go` `register()` wires every singleton
  (config, sql.DB, ent client, email sender, pubsub, the `http.Handler`). Add new
  dependencies there via `do.Provide`.
- **HTTP**: `internal/server/server.go` `New()` mounts the four public ogen servers (the fifth,
  `/service`, is served by `NewInternal` on the internal listener) and the
  public endpoints onto a stdlib `http.ServeMux`, then wraps it with hand-rolled middleware
  (recoverer, requestID, timeout, CORS). Errors render as RFC 7807 `application/problem+json`.
- **Five API surfaces**, each with its own TypeSpec spec, ogen server, handler package,
  and auth scheme:
  - `/site/*` — frontend SPA API. Auth: **JWT cookie** (issued by the `login` operation through `auth.Sessions`, ADR 0020). Handlers in `internal/api/site`.
  - `/api/*` — external/public API. Auth: **Bearer api-token** (workspace-scoped). Handlers in `internal/api/external`.
  - `/collect/*` — tracking ingestion from customer sites. Auth: **x-collect-key** header. Handlers in `internal/api/collect`.
  - `/operator/*` — platform staff (the **Operator**, ADR 0026), Enterprise (license feature `operator`; unlicensed = 404).
    Auth: its own **JWT cookie** (`OPERATOR_JWT`, `OPERATOR_JWT_SECRET`) after password plus mandatory TOTP. Handlers, store and security handler in `ee/operator`.
  - `/service/*` — the service endpoints of the Operator surface, called by a platform service with a
    **Service credential** (ADR 0026): **Bearer** `omsc_…`, limited to scopes (`usage:read`,
    `workspaces:read`, `holds:write`), created only by `sphericon service-credential`. Served **only by
    the internal listener** (`INTERNAL_ADDR`, opt-in, ADR 0018 style): the public mux answers 404 for it.
    Handlers and security handler in `ee/operator` (`service.go`); every operation names its scopes in
    `ServiceOperationScopes` (no entry = refused to all). Tests: `env.Internal`, `env.ServiceClient`,
    `env.IssueServiceCredential`.
- Auth security handlers live in `internal/api/auth/auth.go`. External requests carry a
  `*TokenAuth` (scopes + the Workspace's `*ent.Scoped`) in context — query through
  `auth.TokenScoped(ctx)` (see "Workspace scoping" below).

### Workspace scoping: the generated scoped client

Every Workspace-owned entity (one with `WorkspaceMixin`) is reached through
`client.Scoped(ws)` (`*ent.Scoped`), generated by the entc templates in
`ent/template/scoped*.tmpl` (ADR 0017). Its `Query`/`Create`/`Get`/`UpdateOneID`/`Update`/
`Delete`/`DeleteOneID`/`CreateBulk`/upsert wrappers apply the Workspace themselves; `Create`
has no `SetWorkspaceID`; a reference to another Workspace's row fails with
`ent.ErrNotInWorkspace` (422). A new Workspace-owned entity is covered on the next
`mise run generate`; nothing is hand-written per entity. `Scoped.Workspace(ctx)` loads the
tenant row itself (the Workspace is the tenant root, so it has no wrapper).

- **Handlers and domain modules hold no `*ent.Client` and no bare `int64` workspace id.**
  They take a `*ent.Scoped` (modules: an explicit `s *ent.Scoped` parameter) and never call
  `client.Scoped(<int64>)`. Tests use `env.DB.Scoped(fixtures.AcmeID)`.
- **`*ent.Scoped` is built only in a closed list of places:**
  1. site: `accounts.Accounts.Scope`, behind `Handlers.scopedFor` / `scopedWithRoleFor`
     (`internal/api/site/handlers.go`), from the user's Membership on `/w/{slug}`;
  2. external (and MCP, which rides the same token auth): `ExternalSecurityHandler.HandleBearerAuth`
     in `internal/api/auth/auth.go`, read with `auth.TokenScoped(ctx)`;
  3. collect: `CollectSecurityHandler` in `internal/api/auth/auth.go`, from the collect key;
  4. jobs and event subscribers, right after the row that names the Workspace is loaded
     (`client.Scoped(run.WorkspaceID)` in `internal/jobs`; `EvaluateTriggerWorker` loads the
     contact and scopes from its Workspace; `events.Persist`/`Suppress`, and the webhooks
     consumer, which builds the scope it hands to `Dispatch` from the event envelope: the
     bus subscriber is the raw-client entry, an envelope carries only a Workspace id);
  5. transactions: `events.Bus.WithinScopedTx`, and `tx.Scoped(...)` inside
     `accounts.AcceptInvitation`, re-scope a transaction's client to the same Workspace;
  6. the Workspace comes from a secret rather than a login: the SES hook ingest key
     (`internal/server/hooks_ses.go`), signed unsubscribe/confirm tokens
     (`internal/consent`), and `accounts.BootstrapScope` (the bootstrap token);
  7. the platform Operator (ADR 0026): `ee/operator` scopes over the one Workspace the console
     is looking at, in two reads, each after the Workspace row is loaded and each through
     the module that already serves `/site`: its Audit log (`Module.AuditEntries`, via
     `audit.Log.Entries`) and its deliverability rates (`Module.WorkspaceDeliverability`, via
     `reputation.Rates`); and the service endpoints (a Service credential) scope over the one
     Workspace a call names, after its row is loaded, to set, clear and list its Operator holds
     (`Module.SetHold`, `ClearHold`, `ActiveHolds`; the send path and the site API read them
     through `holds.Active` / `holds.Freezer`, which hold no raw client); the abuse detector
     (ADR 0026) scopes over the Workspace being judged, after its row is loaded, to read the
     rates (`abuse.Watch`, which the console's `Module.WorkspaceAbuse` and the `/site`
     dashboard's warning also read through the `site.AbuseWatch` seam); and the service usage
     read (`Module.WorkspaceUsage`, `ee/operator/service_usage.go`), scoped over the one
     Workspace a call names, after its row is loaded, to read its stored Usage figures.
- **The raw `*ent.Client` is allowed only in:** `internal/accounts` (User, Membership,
  Workspace incl. slug resolution, invitation by token), `internal/api/auth` (credentials, token and key lookup),
  `internal/consent` (signed unsubscribe/confirm tokens: the Workspace comes from the token,
  so it works on the bus's raw transaction client and scopes from the token's Workspace),
  `internal/oauthserver`, `internal/secondfactor` (a User's TOTP Second factor and Recovery
  codes, ADR 0020: they belong to the User, not a Workspace), `internal/suspension` (Workspace
  suspension), `internal/events`
  (the bus and its subscribers), `internal/jobs` (job entry points), `internal/server`
  (tracking by recipient id, provider hooks, composition), `ee/audit` (the Audit log bus
  subscriber: its envelope carries only a Workspace id, ADR 0022), `ee/retention` (the
  periodic prune job: a job entry point that lists Workspaces, then scopes), `ee/abuse` (the hourly
  abuse detector, ADR 0026, the same shape: it lists the Workspaces that are neither suspended nor
  exempt, then scopes to judge each, and writes its notice markers by id), `ee/usage` (the periodic
  usage job, same shape, plus the paged read of every Workspace's stored figures, and the ent hook, installed by `Edition.Hooks()` on the client and the bus's transaction client only with the `operator` license, that keeps the contacts peak as Contacts change), `ee/operator` (the
  platform Operator, ADR 0026: the Operator table, and the Workspace rows, suspension state and
  rates the console shows as tenant metadata; Workspace data goes through the scopes of
  list item 7), `internal/workspacefeed` (the ent hook, installed on every client, that gives
  a Workspace the next feed position when it is created or an owner is added, removed,
  transferred or changes email or verification: it must reach the Workspace a User or
  Membership change belongs to; `ee/operator` serves the feed in `Module.WorkspaceFeed`) and the composition roots
  (`internal/app`, `internal/db`, `internal/testhelper`). Needing raw access anywhere else
  means a new small package in this list, not a field on `Handlers`.
  `internal/eligibility` holds no raw client: it takes a `*ent.Scoped` and passes
  `s.WorkspaceID()` as a bound SQL argument inside one raw expression (the correlated
  contact check reads the outer row's `workspace_id` column), not as a hand-written
  `WorkspaceID(ws)` predicate.
- **Outside the mixin:** `OAuthCode` keeps a hand-written `workspace_id` with no `workspace`
  edge (no FK; adding one changes the DB schema), so it has no scoped wrapper.
- **Lint:** `forbidigo` (`.golangci.yml`) bans entity-level `entity.Update()` repo-wide,
  because a loaded entity still carries the raw client. Update by id instead:
  `s.Tag().UpdateOneID(id)`. There are no exclusions.
- **Enterprise Edition (`ee/`)**: same binary, gated by the runtime license key (`LICENSE_KEY`,
  verified by `ee/licensekey`; empty = plain core, a bad key fails boot). `ee.New` builds the
  `Edition` (extra bus consumers + read seams) in `internal/app` and in `testhelper.Setup`
  (`WithoutLicense()` for the unlicensed case); core reaches EE only through interfaces
  (`site.AuditLog`, `events.Consumer`). The audit table's ent schema sits in `ent/schema`
  (ent has one schema package) but is written and read only by `ee/audit`. An Audit entry is
  an unprojected event (`events.Unprojected`): persist, automations and webhooks skip it.
  The Audit table's only remover is `ee/retention` (the `retention` license feature, a
  Workspace `retention_days` window, one hourly river job registered through `Edition.Jobs()`).
  Publish through `events.RecordAudit` inside the mutation's transaction. In tests the router
  does not run, so call `env.DeliverToEE(t)` to hand the outbox to the EE subscribers.
  **Audit capture in the scoped client (ADR 0022):** an `*ent.Scoped` carries its actor
  (`s.As(actor, opener)`, set at the construction points: `Accounts.Scope` user,
  `HandleBearerAuth` api_token, collect/SES/consent/bootstrap `events.Ingest`; `bus.Act` for
  system). An entity opts in with `schema.Audited{Action, NameField}` (fields: `schema.Sensitive{}`);
  its generated wrappers then run in their own transaction, or join the one the scope is bound
  to (`WithinScopedTx` keeps the actor), and publish the entry. Ingest and actor-less scopes take
  the plain path. Upserts of an audited entity must be `DoNothing`. Add the annotation, nothing
  per handler.
  Note `ee/licensekey`, not `ee/license`: the path is case-insensitive on macOS and would
  collide with `ee/LICENSE`.
- **Async**: `internal/pubsub` (watermill over Postgres) — handlers registered in
  `pubsub.RegisterHandlers`, router run in a goroutine from `cmd/server/main.go`.
  `internal/jobs` uses river (Postgres-backed queue). Email via `internal/messaging`
  (per-workspace providers: smtp/ses). Both providers share `messaging.BuildMIME`
  (wneessen/go-mail) — smtp sends the `*mail.Msg` directly, ses serializes it to raw
  bytes for SES v2 `SendEmail` (raw content).
- The tracker snippet (`/t.js`) is the `@sphericon/analytics` IIFE bundle, built and embedded:
  `mise run build:tracker` copies `packages/analytics/dist/t.js` into `internal/server/assets/`.

## Frontend architecture

- The README mentions oRPC, but the site client is
  actually the generated `@hey-api/openapi-ts` fetch client + react-query hooks in
  `src/generated/site/` (consume these, don't hand-write fetch calls).
- Route auth guard hits `/site/workspaces` (`fetchWorkspaces` in `src/router.tsx`) and redirects to `/login` on 401.
- i18n via i18next; `locales/`, types generated by `mise run generate:i18n-types`.
- Lint is **oxlint** (all plugins, type-aware via oxlint-tsgolint; `oxlint.config.ts`) and format is **oxfmt** (`oxfmt.config.ts`; also formats Markdown/YAML/JSON). Single quotes, no semicolons, generated dirs ignored. **Never disable a lint rule** (no `off`, no `oxlint-disable`/ignore comments) — fix the code; configure a rule's own options only when it has them.
- Unused deps/files/exports are caught by **knip** (`knip.ts`); secrets by **gitleaks** (`.gitleaks.toml`); workflows by **jactionlint** + **zizmor**; Go vulns by **govulncheck** (`mise run check:vulns`, nightly).
  Type-check is **tsc** (TypeScript 7, the native compiler).

## Dev environment

`mise run dev` starts the mise daemons (`.mise.toml`) behind Caddy with HTTPS at
**https://sphericon.localhost** (run `caddy trust` once). The external API is also exposed at
**https://api.sphericon.localhost** — Caddy rewrites `/*` → `/api/*` to the same backend, so
the subdomain root mirrors the binary's `/api` path (RudderStack-style edge; the binary
stays path-based). In prod the ingress in front of the binary does the same rewrite for
`api.getsphericon.app`. Daemons: `db`, `mailpit` (SMTP UI at :8025, SMTP :1025),
`backend` (real Go server under air on `:3300`, hot reload), `frontend` (Vite on `:5173`),
`caddy` (:443). Inspect with `mise daemons ls|logs|status`. Migrations run via atlas
(`mise run db:migrate`); the backend does not self-migrate. Dev defaults (JWT secret, dev
`ENCRYPTION_KEY`, SMTP) live in the `[env]` table of `.mise.toml`; personal overrides go in
the gitignored `.env` (read by the app) or `.mise.local.toml`.

**Ports and origins per checkout.** Every daemon declares `port = { auto = true, base = <port> }`:
the primary checkout gets exactly the ports above (caddy 443, vite 5173, backend 3300, mailpit
8025/1025, metrics 9090); a linked worktree gets one slot offset (`base + N`, the same `N` for all
of its daemons), so many stacks run at once. The slot comes from a hash of the checkout's path,
not a registry: two worktrees can land on the same one (two of 14 here did), and then the second
stack fails to start with an error naming the daemon and the other project. Pin the later one
in its `.mise.local.toml` (`[daemons.backend]` ... `port = 3601`, the same for `frontend`,
`mailpit` and `caddy`; SMTP, metrics and Caddy's admin/HTTP ports follow from them). mise
resolves the ports at config load, so
`[env]` templates them (`PORT`, `SMTP_PORT`, `METRICS_ADDR`, `APP_URL`, `APP_HOST`, `API_HOST`), and
the Caddyfile (`{$BACKEND_PORT}`, `{$APP_HOST}`, ...), `vite.config.ts` and the `ready_cmd`s read the
exported `<NAME>_PORT` (`ready_port` cannot be templated). Ports with no daemon of their own (SMTP,
metrics) are the base plus that same offset; Caddy's admin API (`CADDY_ADMIN`, 2019 + offset, what
`caddy trust --address "$CADDY_ADMIN"` reads) and HTTP redirect listener (`CADDY_HTTP_PORT`: 80 in
the primary checkout, 4000 + offset elsewhere) are per instance too. A linked worktree's origin is
`https://<dir-name>.sphericon.localhost:<caddy port>` (API: `https://api.<dir-name>.sphericon.localhost:<caddy port>`):
browser cookies are shared across the ports of one host, so a per-worktree host keeps the session
cookies apart; `*.localhost` resolves to loopback and Caddy's internal CA signs each name.
`mise env | grep APP_URL` prints it. Caddy stays the edge instead of pitchfork's own proxy
(`proxy = "<label>"`, stable per-worktree hostnames) because that proxy needs per-machine setup
(`pitchfork proxy setup`: DNS, trust store, ports), would still leave the primary on Caddy's :443,
and Caddy already does the `/api` rewrite and the path routing in one file.

## Code search

`ast-grep` is installed (pinned in `.mise.toml`). For any search that depends on syntax or code
structure (calls, declarations, a method on a type, JSX props), default to
`ast-grep --lang <go|tsx|typescript> -p '<pattern>'` and adjust `--lang` per file type. Use `rg`/grep
only for plain-text searches (strings, comments, config values). Skip generated files (`gen/`, `ent/`
outside `schema/`, `src/generated/`) unless that is the point of the search.

## Conventions

- Commit messages follow **Conventional Commits** (`feat:`, `fix:`, `chore:`, `docs:`,
  `refactor:`, `ci:` …) — release-please uses them for versioning/changelog.
  commitlint runs on every commit, merge commits included: write `chore: merge <what>`, never
  `Merge branch …`; the header fits 120 characters and each body line 100.
- A new environment variable gets one row in the env table of `docs/self-hosting.md`, the only
  table (`mise run check:env`); the README links to it.
- **Agent guidance lives in this file, not in personal agent memory.** The repo has several
  developers: a rule an agent should follow goes here (or in `docs/`), never into one person's
  memory.
- **Clean up after a merge.** Once a PR is merged, remove its worktree and delete its local and
  remote branch (`git worktree remove <path>`, `git branch -D <branch>`,
  `git push origin --delete <branch>`) right away, without asking. Only the worktree and branch
  made for that PR.
- ADR numbers are unique (`mise run check:adr`). Take the next free number when you add an ADR,
  and re-check after merging `main`: parallel branches pick the same one.
- After changing TypeSpec or `ent/schema`, run `mise run generate` and commit the generated output.
- **No direct SQL in tests.** Tests read and write through ent (`env.DB`) or river's own API. The
  domain-event outbox is modelled in ent too (`OutboxMessage`, `OutboxOffset`: watermill owns the
  tables, `entsql.Skip` keeps them out of migrations, only the client is generated), so
  `env.Outbox*`, `env.OutboxCount` and the cursor helpers sit on the generated client.
  `forbidigo` in `.golangci.yml` rejects `*sql.DB` `Exec`/`Query`/`QueryRow` in `_test.go` files.
  Inject faults with ent hooks/interceptors, never DDL or triggers.
- **Tests never clean up by hand, and never reset state at their start.** A test that needs a real
  commit (the outbox router, concurrent workers, a whole app) takes a database of its own from
  `testhelper.RealDB`, `RealDBURL` or `EmptyDBURL` (pgtestdb), dropped when the test ends; every
  other test rides the transaction `pgcleaner` hands out, which rolls back. A `t.Cleanup` that deletes rows, a purge
  or a reset at the top of a test means a test is committing outside its transaction: fix that test.
- **No tests of built-in behaviour.** A test exercises sphericon's own code. One whose only subject is a
  Mantine component, the standard library or another dependency (a copy button showing "Copied", a
  library's defaults or retries) is deleted, and none is added.
- **No custom CSS anywhere in the repo.** Style the frontend exclusively through Mantine — components, style
  props (`p`, `c`, `w`, responsive object syntax), the color system, the theme, and the
  configured breakpoints. Do not add custom `.css`/CSS-module files, inline `style={{…}}`,
  hardcoded colors, or CSS-in-JS (styled-components/emotion). The only CSS imports are the
  library stylesheets in `src/main.tsx`.
- **Theming & responsiveness.** Light and dark color schemes are supported via
  `MantineProvider` (`defaultColorScheme="auto"`) + `ColorSchemeScript` in `index.html`;
  the UI must be responsive using Mantine primitives (responsive props,
  `visibleFrom`/`hiddenFrom`, `AppShell` breakpoints, `SimpleGrid` cols) — never custom CSS.

## Agent skills

### Issue tracker

Issues live in GitHub Issues (`getsphericon/sphericon`), via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Default five-label vocabulary (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `GLOSSARY.md` at the repo root plus `docs/adr/`. See `docs/agents/domain.md`.

### Worktrees

Created in Claude's default `.claude/worktrees/<name>`, never in a sibling directory. Never create a branch in the primary checkout: only in a worktree, and only when needed. See `docs/agents/worktrees.md`.

### Advisor before /to-spec

Before running the `to-spec` skill, call the `advisor` tool first and take its advice into account. The skill is vendored, so this rule lives here rather than in the skill.

### Merge main before marking ready

In `implement-spec`, before marking the draft PR ready, merge a freshly fetched `origin/main` into
the integration branch, push, and merge the PR at once: `main` moves while the integration runs, and a
conflicting PR runs no CI. The skill is vendored, so this rule lives here rather than in the skill.
