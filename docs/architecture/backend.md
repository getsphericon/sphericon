# Backend wiring and API surfaces

[Architecture](../architecture.md) → Backend wiring and API surfaces

## Dependency injection

**DI**: `samber/do` container, the one composition root. `internal/app/container.go`
`Container(cfg)` registers every singleton (the clock, the pools, the ent client, the senders,
the domain modules, the surfaces' `Deps`, the handlers) as a lazy provider in a `do.Package`;
a provider takes what it needs with `do.MustInvoke` while it builds and keeps no injector for a
later lookup (a deferred lookup hides a cycle from the container, fails once the order of
building changes and undoes the shutdown order). The server (`app.New`), the commands
(`app.NewCommandApp`) and the test harness (`testhelper.Setup`) all build from it, the harness
replacing only the seams `Container` names (`do.Override`) and setting the rest through the
config. A new dependency is a provider there, never a constructor call in a handler, the
server package, the job client (`jobs.NewClient` takes its modules in `jobs.Deps`) or the
harness. No domain module or handler sees the container: `samber/do` is
imported by `internal/app`, the harness and `internal/kernel/cleanup` (its shutdown report) only
(easement fails an import of it in the core, a surface or `ee/`; `internal/kernel/cleanup` carries
the one suppression).

## HTTP routing

**HTTP**: `internal/server/server.go` `New()` mounts the four public ogen servers (the fifth,
`/service`, is served by `NewInternal` on the internal listener, which also always serves the
edge's on-demand TLS `GET /tls/ask`, core, ADR 0038, from `internal/api/tlsask`) and the
public endpoints (the tracked links of `internal/api/tracking`, the Ingest hook of
`internal/api/hooks`, whose handlers the container builds) onto a stdlib `http.ServeMux`, then wraps it with hand-rolled middleware
(recoverer, requestID, timeout, CORS). Errors render as RFC 7807 `application/problem+json`.

## API surfaces

**Five API surfaces**, each with its own TypeSpec spec, ogen server, handler package,
and auth scheme:

- `/site/*` — frontend SPA API. Auth: **JWT cookie** (issued by the `login` operation through `auth.Sessions`, ADR 0020). Handlers in `internal/api/site`.
- `/api/*` — external/public API. Auth: **Bearer api-token** (workspace-scoped). Handlers in `internal/api/external`.
  Every operation names the scopes a token needs in `auth.ExternalOperationScopes`, generated from the
  `@requiresScopes(#[ApiTokenScope.…])` on each TypeSpec operation and checked by
  `ExternalSecurityHandler.HandleBearerAuth` before the handler runs (ADR 0061): a missing declaration fails
  `tsp compile`, no entry = refused to all, a valid token without a scope = 403, and 401 only for a missing,
  invalid, revoked or expired token. No handler checks a scope; /mcp inherits the check, as each tool call is
  dispatched to /api with the caller's token.
- `/collect/*` — tracking ingestion from customer sites. Auth: **x-collect-key** header. Handlers in `internal/api/collect`.
- `/staff/*` — platform staff (the **Staff member**, ADR 0026, ADR 0063), on every Instance in both Modes and
  without a License key. Auth: its own **JWT cookie** (`STAFF_JWT`, `SPHERICON_STAFF_JWT_SECRET`) after password
  plus mandatory TOTP; without the secret the Staff sign-in is off and the whole prefix answers 404 (the `staff`
  license and the `saas` Mode refuse to start without it). Two contracts share the prefix and the cookie: the
  core one (`typespec/staff`: sign-in, me, the Sign-up items and their Legal documents), handlers and security
  handler in `internal/api/staff` over the Staff member module `internal/domain/identity/staffmembers` (store, login adapter,
  sessions, `Current`) and the Sign-up items module `internal/domain/identity/signupitems` (ADR 0062), whose two anonymous reads
  `/site` serves for the sign-up form and the public Legal document pages; and the Enterprise
  sections (`typespec/staffee`: Workspaces, holds, audit, Sign-up survey readout), handlers in `ee/api/staff`
  over the Enterprise module `ee/domain/insights/workspaceoversight`, served
  only with the license feature `staff` and otherwise 404. `server.NewStaffAPI` mounts the core ogen server
  with the Enterprise one as its not-found handler, so each contract routes its own paths.
- `/service/*` — the service endpoints of the Staff surface, called by a platform service with a
  **Service credential** (ADR 0026): **Bearer** `smsc_…`, limited to scopes (`usage:read`,
  `workspaces:read`, `domains:read`, `holds:write`), created only by `sphericon service-credential`. Served **only by
  the internal listener** (`SPHERICON_INTERNAL_ADDR`, opt-in, ADR 0018 style; `NewInternal` mounts `/service` only with
  the `staff` license and always serves the core `GET /tls/ask`): the public mux answers 404 for it.
  Handlers and security handler in `ee/api/staff` (`service.go`) over the module `ee/domain/insights/workspaceoversight` and, for the credential, the module `ee/domain/identity/servicecredentials`; every operation names its scopes in
  `ServiceOperationScopes`, generated from the `@requiresServiceScopes(#[ServiceScope.…])` on each TypeSpec operation (an empty list = any valid credential; a missing declaration fails `tsp compile`; no entry = refused to all). Tests: `env.Internal`, `env.ServiceClient`
  with a fixture credential (`testhelper.ServiceCredentialValue`).

## Sign-in

**Sign-in sequence**: `internal/domain/identity/login` (ADR 0020) is the one core module for the password and
second steps and for `Prove` (the password check behind second-factor and profile actions):
address normalization, the throttle consulted first (ADR 0025), the decoy hash, the verification
gate (ADR 0030), failure counting and the challenge. It holds no database client; an adapter
supplies the `Store`, `Throttle`, challenge `Parse`/`Verify`, and a surface issues the session.
`internal/domain/identity/userlogin` (the User's Login: the Users, their Second factors and Passkeys, the
verification gate, the challenge and the `user.login` entry of each step that signs a User in) and `internal/domain/identity/staffmembers` (`login.go`, the second adapter, ADR 0026) are
the two adapters,
both in identity: add sign-in rules in the module, not in a handler, which only turns the outcome into a
session. User deletion
(`internal/usecase/userdeletion`) proves the password through the same module's `Prove`, over the
Users' throttle and the same hasher, and counts a wrong Second factor code with `CountFailure`. The throttle is
`throttle.Attempts.For(kind)` (`internal/infra/throttle`), a `Counter` bound to one `Kind` (`KindLogin`, `KindStaffLogin`),
which satisfies `login.Throttle`. Handlers switch on the per-step outcomes (`login.Outcome`,
`login.SecondOutcome`).

## Sign-up

**Sign-up sequence**: `signup.UseCase.SignUp` (ADR 0030, 0031; the use case
`internal/usecase/signup`, ADR 0102, which also holds every other step that makes a User Verified
with its first Workspace) is the one place of a Sign-up with a password, shaped
like `login`: the form first, then the same work for every address (a mail budget slot, the hash, the
lookup, one write and exactly one published event, ADR 0105) so a Taken address is not learnable, the recovery
from a race on the unique email, the replaced password of an Unverified User, and the Sign-up
browser's value (drawn here; its hash is `users.BrowserHash` and the verification link checks it
with `users.SameBrowser`). The Sign-up items' answers (ADR 0062) are checked with the form, before
any work, through the `Consents` seam (`consentrecords.Module.Check`): a required acceptance not
ticked, an item not in effect or a stale version is a field of the same refusal, and the Consent
records are written by `users` in the transaction that creates the User (or replaces an Unverified
User's password; a Verified User gets none). The wiring gives it the budget
(`throttle.Attempts.For(KindSignUp)`), the mails, the token mint and the verification gate
(`signup.SignUpConfig`, a parameter of `signup.New`); the `/site` handler calls it and turns its result into the answer
and the cookie. The
profile rules of a User (a blank name, the password rule, the proof of the current password and
whether the session is issued again) are `users.Module.ChangeProfile`, which takes the login module's
`Prove` as a seam.

## Resource mapping

**Resource DTOs come from goverter, not from struct literals (review rule 1, analyzer `handmapper`).** Add a method to the surface's
`Converter` (`internal/api/{site,external,staff}/resources`, `ee/api/staff/resources`) with
`goverter:map` / `extend` helpers, and attach in the handler only what the mapper cannot see
(marked `goverter:ignore` with the reason). A resource `/site` and `/api` both answer is built once
below them (ADR 0111): the Integration's `integrations.View`, the Segment preview of `segments`,
the Workspace summary of `workspacesummary`; each surface maps the same values into its own
contract. What one contract shows and the other does not is a separate operation, so the other never
pays for it: `/site` calls `broadcasts.Views` (a Broadcast's figures, Progress and lint) and
`emailtemplates.Views` (the lint of a save), `/api` the modules, which answer the row. Hand-built
values are limited to response wrappers (`*ListOK`, `*Headers`) and aggregates with no single source
(setup checklist, lint findings, the integration config `oneOf`).

## Authentication context

Auth security handlers live in `internal/api/auth/auth.go`. External requests carry a
`*TokenAuth` (scopes + the Workspace's `*ent.Scoped`) in context — query through
`auth.TokenScoped(ctx)` (see [Workspace scoping](workspace-scoping.md)).

## Error mapping

The answer of every refusal and failure on every surface (ADR 0103) is decided once in
`internal/api/problem`, beside the security handlers: it is the surfaces' shared code, imported by
the surfaces, the Ingest hook (`internal/api/hooks`), `internal/server` and `ee/api` only. A core package returns its refusal and never
imports it (easement component `api-problem`).
