# Backend architecture

Workspace scoping is in `docs/agents/workspace-scoping.md`.

- **DI**: `samber/do` container. `internal/app/app.go` `register()` wires every singleton
  (config, sql.DB, ent client, email sender, pubsub, the `http.Handler`). Add new
  dependencies there via `do.Provide`.
- **HTTP**: `internal/server/server.go` `New()` mounts the four public ogen servers (the fifth,
  `/service`, is served by `NewInternal` on the internal listener, which also always serves the
  edge's on-demand TLS `GET /tls/ask`, core, ADR 0038) and the
  public endpoints onto a stdlib `http.ServeMux`, then wraps it with hand-rolled middleware
  (recoverer, requestID, timeout, CORS). Errors render as RFC 7807 `application/problem+json`.
- **Five API surfaces**, each with its own TypeSpec spec, ogen server, handler package,
  and auth scheme:
  - `/site/*` — frontend SPA API. Auth: **JWT cookie** (issued by the `login` operation through `auth.Sessions`, ADR 0020). Handlers in `internal/api/site`.
  - `/api/*` — external/public API. Auth: **Bearer api-token** (workspace-scoped). Handlers in `internal/api/external`.
  - `/collect/*` — tracking ingestion from customer sites. Auth: **x-collect-key** header. Handlers in `internal/api/collect`.
  - `/staff/*` — platform staff (the **Staff member**, ADR 0026), Enterprise (license feature `staff`; unlicensed = 404).
    Auth: its own **JWT cookie** (`STAFF_JWT`, `SPHERICON_STAFF_JWT_SECRET`) after password plus mandatory TOTP. Handlers, store and security handler in `ee/staff`.
  - `/service/*` — the service endpoints of the Staff surface, called by a platform service with a
    **Service credential** (ADR 0026): **Bearer** `smsc_…`, limited to scopes (`usage:read`,
    `workspaces:read`, `domains:read`, `holds:write`), created only by `sphericon service-credential`. Served **only by
    the internal listener** (`SPHERICON_INTERNAL_ADDR`, opt-in, ADR 0018 style; `NewInternal` mounts `/service` only with
    the `staff` license and always serves the core `GET /tls/ask`): the public mux answers 404 for it.
    Handlers and security handler in `ee/staff` (`service.go`); every operation names its scopes in
    `ServiceOperationScopes`, generated from the `@extension("x-required-scopes", #[ServiceScope.…])` on each TypeSpec operation (an empty list = any valid credential; a missing declaration fails generation; no entry = refused to all). Tests: `env.Internal`, `env.ServiceClient`,
    `env.IssueServiceCredential`.
- **Sign-in sequence**: `internal/login` (ADR 0020) is the one core module for the password and
  second steps and for `Prove` (the password check behind second-factor and profile actions):
  address normalization, the throttle consulted first (ADR 0025), the decoy hash, the verification
  gate (ADR 0030), failure counting and the challenge. It holds no database client; an adapter
  supplies the `Store`, `Throttle`, challenge `Parse`/`Verify` and issues the session. `/site`
  (`internal/api/site/auth_login.go`) and `ee/staff` (`login.go`, the second adapter, ADR 0026) are
  the two adapters: add sign-in rules in the module, not in a handler. The throttle is
  `accounts.Attempts.For(kind)`, a `Counter` bound to one `Kind` (`KindLogin`, `KindStaffLogin`),
  which satisfies `login.Throttle`. Handlers switch on the per-step outcomes (`login.Outcome`,
  `login.SecondOutcome`).
- **Resource DTOs come from goverter, not from struct literals (review rule 1, analyzer `handmapper`).** Add a method to the surface's
  `Converter` (`internal/api/{site,external}/resources`, `ee/staff/resources`) with
  `goverter:map` / `extend` helpers, and attach in the handler only what the mapper cannot see
  (marked `goverter:ignore` with the reason). Hand-built values are limited to response wrappers
  (`*ListOK`, `*Headers`) and aggregates with no single source (setup checklist, lint findings,
  the integration config `oneOf`).
- Auth security handlers live in `internal/api/auth/auth.go`. External requests carry a
  `*TokenAuth` (scopes + the Workspace's `*ent.Scoped`) in context — query through
  `auth.TokenScoped(ctx)` (see `docs/agents/workspace-scoping.md`).

- **Enterprise Edition (`ee/`)**: same binary, gated by the runtime license key (`SPHERICON_LICENSE_KEY`,
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
- **Owners and suspension are defined once.** `internal/accounts/owners.go` owns "Owner of a
  Workspace": `Owners`, `OwnersOf`, `LockOwners`, `OwnerMembership`, `IsOwner`, and the one notice to
  them (`NotifyOwners`, the `Notify` seam, `OwnerNotifier`); `internal/suspension` owns "suspended"
  (`Is`, `Suspended`, `NotSuspended`, `BySystem`) and the suspension notice (`NotifyOwners`). Call
  them; do not test `suspended_at` or the owner Role, or mail Owners, anywhere else.
- **Refused removals (ADR 0049)**: a row that live work still uses is not removed. `internal/inuse`
  is the one shape of that refusal (`*inuse.Error`: the removed `Subject` and the `Users` that hold it,
  with helpers like `inuse.Broadcasts` and `inuse.Enrollments`); each domain module returns it from
  its delete and each API surface renders it as the contract's `InUseProblem` (409, `usedBy`).
  `internal/liveuse` finds the live work (today the active email Automations) for the modules that
  cannot import `internal/automations`. Both are leaves that hold no raw `*ent.Client`: `liveuse`
  takes a `*ent.Scoped`, `inuse` only maps rows to users.
- **Owner notices**: a module that tells the Owners of a Workspace something (Join domains, Sending domains,
  webhooks, suspension, the abuse detector) takes an `ownernotice.Notify` (`internal/ownernotice`, a leaf, so
  that modules `accounts` imports can share it) and calls `Tell` or `TellBestEffort`; `accounts.OwnerNotifier`
  builds the one that mails the Owners through the system sender.
- **Async**: `internal/pubsub` (watermill over Postgres) — handlers registered in
  `pubsub.RegisterHandlers`, router run in a goroutine from `cmd/server/main.go`.
  `internal/jobs` uses river (Postgres-backed queue, `database/sql` driver, polling). A job that must
  exist iff a state change commits is queued in that transaction with
  `events.Bus.WithinScopedEnqueueTx` (a Broadcast's claim and its send job); never claim first and
  enqueue after, with a best-effort undo. Email via `internal/messaging`
  (per-workspace providers: smtp/ses). Both providers share `messaging.BuildMIME`
  (wneessen/go-mail) — smtp sends the `*mail.Msg` directly, ses serializes it to raw
  bytes for SES v2 `SendEmail` (raw content).
- `internal/emaildoc` (ADR 0040) turns an editor document into email HTML inside the binary: the editor's own
  export code, bundled by `packages/emaildoc` (`mise run build:emaildoc`; rerun it after any change to the pinned
  `@react-email/editor`, then `go test ./internal/emaildoc -update` to accept the new golden HTML) and run in goja,
  no cgo. The runtime starts on the first call, limits are enforced before and during a render. Editor and
  bundle dependencies in `packages/emaildoc/package.json` are pinned exactly.
- The tracker snippet (`/t.js`) is the `@sphericon/analytics` IIFE bundle, built and embedded:
  `mise run build:tracker` copies `packages/analytics/dist/t.js` into `internal/server/assets/`.
