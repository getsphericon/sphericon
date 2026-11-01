# Architecture

The shape of the backend and the rules its code follows. Review rules are in `docs/standards/review-rules.md`,
coding standards in `CODING_STANDARDS.md`.

## Backend architecture

- **DI**: `samber/do` container, the one composition root. `internal/app/container.go`
  `Container(cfg)` registers every singleton (the clock, the pools, the ent client, the senders,
  the domain modules, the surfaces' `Deps`, the handlers) as a lazy provider in a `do.Package`;
  a provider takes what it needs with `do.MustInvoke`. The server (`app.New`), the commands
  (`app.NewCommandApp`) and the test harness (`testhelper.Setup`) all build from it, the harness
  replacing only the seams `Container` names (`do.Override`) and setting the rest through the
  config. A new dependency is a provider there, never a constructor call in a handler, the
  server package or the harness. No domain module or handler sees the container: `samber/do` is
  imported by `internal/app`, the harness and `internal/cleanup` (its shutdown report) only.
- **HTTP**: `internal/server/server.go` `New()` mounts the four public ogen servers (the fifth,
  `/service`, is served by `NewInternal` on the internal listener, which also always serves the
  edge's on-demand TLS `GET /tls/ask`, core, ADR 0038) and the
  public endpoints onto a stdlib `http.ServeMux`, then wraps it with hand-rolled middleware
  (recoverer, requestID, timeout, CORS). Errors render as RFC 7807 `application/problem+json`.
- **Five API surfaces**, each with its own TypeSpec spec, ogen server, handler package,
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
    handler in `internal/api/staff` over the Staff member module `internal/staffmembers` (store, login adapter,
    sessions, `Current`) and the Sign-up items module `internal/signupitems` (ADR 0062), whose two anonymous reads
    `/site` serves for the sign-up form and the public Legal document pages; and the Enterprise
    sections (`typespec/staffee`: Workspaces, holds, audit, Sign-up survey readout), handlers in `ee/staff`, served
    only with the license feature `staff` and otherwise 404. `server.NewStaffAPI` mounts the core ogen server
    with the Enterprise one as its not-found handler, so each contract routes its own paths.
  - `/service/*` — the service endpoints of the Staff surface, called by a platform service with a
    **Service credential** (ADR 0026): **Bearer** `smsc_…`, limited to scopes (`usage:read`,
    `workspaces:read`, `domains:read`, `holds:write`), created only by `sphericon service-credential`. Served **only by
    the internal listener** (`SPHERICON_INTERNAL_ADDR`, opt-in, ADR 0018 style; `NewInternal` mounts `/service` only with
    the `staff` license and always serves the core `GET /tls/ask`): the public mux answers 404 for it.
    Handlers and security handler in `ee/staff` (`service.go`); every operation names its scopes in
    `ServiceOperationScopes`, generated from the `@requiresServiceScopes(#[ServiceScope.…])` on each TypeSpec operation (an empty list = any valid credential; a missing declaration fails `tsp compile`; no entry = refused to all). Tests: `env.Internal`, `env.ServiceClient`,
    `env.IssueServiceCredential`.
- **Sign-in sequence**: `internal/login` (ADR 0020) is the one core module for the password and
  second steps and for `Prove` (the password check behind second-factor and profile actions):
  address normalization, the throttle consulted first (ADR 0025), the decoy hash, the verification
  gate (ADR 0030), failure counting and the challenge. It holds no database client; an adapter
  supplies the `Store`, `Throttle`, challenge `Parse`/`Verify` and issues the session. `/site`
  (`internal/api/site/auth_login.go`) and `internal/staffmembers` (`login.go`, the second adapter, ADR 0026) are
  the two adapters: add sign-in rules in the module, not in a handler. The throttle is
  `accounts.Attempts.For(kind)`, a `Counter` bound to one `Kind` (`KindLogin`, `KindStaffLogin`),
  which satisfies `login.Throttle`. Handlers switch on the per-step outcomes (`login.Outcome`,
  `login.SecondOutcome`).
- **Sign-up sequence**: `internal/signup` (ADR 0030, 0031) is the one core module for a Sign-up with
  a password, shaped like `login`: the form first, then the same
  work for every address (a mail budget slot, the hash, the lookup, one write, a token and exactly
  one mail job) so a Taken address is not learnable, the recovery from a race on the unique email,
  the replaced password of an Unverified User, and the Sign-up browser's value (`BrowserHash`,
  `SameBrowser`). The Sign-up items' answers (ADR 0062) are checked with the form, before any work,
  through the `Consents` seam (`consentrecords.Module.Check`): a required acceptance not ticked, an item
  not in effect or a stale version is a field of the same refusal, and the Consent records go to the
  `Store`, which writes them in the transaction that creates the User (or replaces an Unverified
  User's password; a Verified User gets none). It holds no database client; the `/site` adapter (`internal/api/site/auth.go`)
  supplies the `Store` over `accounts`, the budget (`accounts.Attempts.For(KindSignUp)`), the
  mails and the token mint, and turns the `signup.Outcome` into the answer and the cookie. The
  profile rules of a User (a blank name, the password rule, the proof of the current password and
  whether the session is issued again) are `accounts.ChangeProfile`, which takes the login module's
  `Prove` as a seam.
- **Package boundaries are checked by easement (`easement.yml`, `mise run check:arch`).** A surface
  package (`internal/api/{site,external,collect,staff}`) never imports another surface, `internal/**` core
  imports neither a surface nor `ee/`, and only the wiring packages (`internal/app`,
  `internal/server`, `internal/mcpserver`, `internal/testhelper`, `internal/doctor`, `cmd/server`)
  assemble them; only they import `ee/`. A new wiring package is added to the `wiring` component,
  never to a baseline. The check is strict: a new Go package under `internal/` or `ee/` is covered by
  the `core` and `ee` components, and a new top-level tree must be assigned a component or listed
  as `unowned` in `easement.yml`.
- **Resource DTOs come from goverter, not from struct literals (review rule 1, analyzer `handmapper`).** Add a method to the surface's
  `Converter` (`internal/api/{site,external,staff}/resources`, `ee/staff/resources`) with
  `goverter:map` / `extend` helpers, and attach in the handler only what the mapper cannot see
  (marked `goverter:ignore` with the reason). Hand-built values are limited to response wrappers
  (`*ListOK`, `*Headers`) and aggregates with no single source (setup checklist, lint findings,
  the integration config `oneOf`).
- Auth security handlers live in `internal/api/auth/auth.go`. External requests carry a
  `*TokenAuth` (scopes + the Workspace's `*ent.Scoped`) in context — query through
  `auth.TokenScoped(ctx)` (see "Workspace scoping" below).

## Workspace scoping: the generated scoped client

Every Workspace-owned entity (one with `WorkspaceMixin`) is reached through
`client.Scoped(ws)` (`*ent.Scoped`), generated by the entc templates in
`ent/template/scoped*.tmpl` (ADR 0017). Its `Query`/`Create`/`Get`/`UpdateOneID`/`Update`/
`Delete`/`DeleteOneID`/`CreateBulk`/upsert wrappers apply the Workspace themselves; `Create`
has no `SetWorkspaceID`; a reference to another Workspace's row fails with
`ent.ErrNotInWorkspace` (422). A new Workspace-owned entity is covered on the next
`mise run generate`; nothing is hand-written per entity. A Workspace-owned table may hold at most
one row per Workspace (a unique `workspace_id`): `brand_kits`, and `send_cap_limiters`, the Send
cap's 24-hour bucket (ADR 0067). It is scoped, erased with its Workspace and seeded by fixtures
like any other, and the isolation test lists it in `singletonEntities`. `Scoped.Workspace(ctx)` loads the
tenant row itself (the Workspace is the tenant root, so it has no wrapper). A Versioned definition
(`VersionedMixin` on the object, `VersionMixin` on its versions: the Automation and the Rule, ADR 0065)
also gets `<N>Pointers` (`ent/template/scoped_versioned.tmpl`, `ent.VersionedStore`), the moves of its
live and draft pointers and its status; `internal/drafts` is the one lifecycle over them and the
only caller of their writes (`forbidigo`).

- **Handlers and domain modules hold no `*ent.Client` and no bare `int64` workspace id.**
  They take a `*ent.Scoped` (modules: an explicit `s *ent.Scoped` parameter) and never call
  `client.Scoped(<int64>)`. Tests use `env.DB.Scoped(fixtures.AcmeID)`.
- **`*ent.Scoped` is built only in a closed list of places:**
  1. site: `accounts.Accounts.Scope`, behind the ogen middleware `Handlers.Access`
     (`internal/api/site/access.go`, ADR 0061), from the user's Membership of the Workspace an
     operation's `workspaceId` names, read with `workspaceScope(ctx)` (the Membership with
     `callerMembership(ctx)`). On every operation that carries a session, the middleware then
     admits the User (`accounts.Accounts.Admit`, `internal/accounts/admission.go`, the one decision
     of both surfaces): it refuses first a User with a required acceptance pending (403
     `acceptance_pending`, ADR 0062) unless the operation is marked
     `@allowedWhileAcceptancePending` in TypeSpec, the one list of what a gated User may call;
     an unmarked operation is refused, the mark lifts only the gate (a Workspace's operation still
     applies the Two-factor requirement and its Permissions), it only declares on an anonymous
     operation. On an operation of a Workspace it
     then refuses a User the Two-factor requirement withholds (403 `second_factor_required`,
     ADR 0020) unless the operation is marked `@allowedWhileWithheld` (only lifting the
     requirement), and last applies the Permissions the operation declares in TypeSpec
     (`@requiresPermissions`, generated into `operationAccess`). Two operations name their
     Workspace elsewhere and call `accounts.Accounts.Scope` themselves: `SiteOAuthDecide` (in its
     body, then `accounts.Accounts.Admit` as the middleware does) and `SiteWorkspacesCreate` /
     `SiteWorkspacesRestore` (a Workspace that was not live before);
  2. external (and MCP, which rides the same token auth): `ExternalSecurityHandler.HandleBearerAuth`
     in `internal/api/auth/auth.go`, read with `auth.TokenScoped(ctx)`. A User's own token (an API
     token with a `membership_id`, ADR 0061: a Connection, which names its `oauth_client_id`, or a
     Personal token, which names none; `internal/apitokens/kinds.go` tells them apart) is looked up
     with its Membership: the scope acts as its User, the acceptance gate and the Two-factor
     requirement apply (`accounts.Accounts.Admit`, as on /site: 403 `acceptance_pending` or
     `second_factor_required`), and the Role bounds its scopes on every request (`accounts.MayGrant`,
     the bound the OAuth consent and the minting of a Personal token apply too); the Workspace's API
     token meets neither;
  3. collect: `CollectSecurityHandler` in `internal/api/auth/auth.go`, from the collect key;
  4. jobs and event subscribers, right after the row that names the Workspace is loaded
     (`client.Scoped(run.WorkspaceID)` in `internal/jobs`; `EvaluateTriggerWorker` loads the
     contact and scopes from its Workspace; `events.Persist`/`Suppress`, and the webhooks
     consumer, which builds the scope it hands to `Dispatch` from the event envelope: the
     bus subscriber is the raw-client entry, an envelope carries only a Workspace id; and the
     orders consumer (`events.RecordOrder`, ADR 0064), which opens a transaction on the raw
     client and scopes it to the envelope's Workspace to build the Orders, handing the scope to
     `orders.Module.Record`), and
     `secondfactor.RemindDeadlines` (the deadline reminder job: it lists the Memberships of every
     Workspace with the raw client and scopes each claim to the Workspace of the Membership it
     loaded), and `workspacedeletion.Module.EraseDue`, which lists the due Workspaces with the raw client and
     scopes each erase step's transaction (`tx.Scoped(id)`) to the Workspace it is erasing;
  5. transactions: `events.Bus.WithinScopedTx`, and `tx.Scoped(...)` inside
     `accounts.AcceptInvitation` (its `acceptInvitationIn`, which the completion of a pending
     sign-up also runs in its own transaction), re-scope a transaction's client to the same Workspace;
     `events.Bus.WithinHandOverTx` also scopes it to the Workspace a Join domain moves away from
     (acting as the system), after `joindomains` has read that Workspace off the holder's row;
     the domain-join step of the verification transaction (`applyDomainJoin` in
     `internal/accounts/join.go`, ADR 0051), which builds `tx.Scoped(...)` for the Workspace a
     verified Join domain names, right after `joindomains.HolderIn` has loaded that row, to add
     the joining User's Membership (every step that proves an address calls it explicitly); and
     `accounts.DeleteUser` (User deletion, ADR 0050) scopes its one transaction in turn to each
     Workspace the User belongs to (`tx.Scoped(m.WorkspaceID)`, from the User's own Memberships),
     to lock the Owner rows and remove the Membership;
  6. the Workspace comes from a secret rather than a login: the ingest hook
     `POST /hooks/{key}` (`internal/server/hooks.go`, ADR 0037), where the key belongs to one
     Integration, so the hook resolves the Integration by key with the raw client and then
     scopes to its Workspace (`row.WorkspaceID`); the tracking endpoints (`loadEngaged` in
     `internal/server/tracking.go`), where the token names an outbound message that is loaded
     with the raw client and scoped to its Workspace; signed unsubscribe/confirm tokens
     (`internal/consent`), and `accounts.BootstrapScope` (the bootstrap token);
  7. the platform Staff member (ADR 0026): `ee/staff` scopes over the one Workspace the console
     is looking at, in three reads, each after the Workspace row is loaded and each through
     the module that already serves `/site`: its Audit log (`Module.AuditEntries`, via
     `audit.Log.Entries`), its deliverability rates (`Module.WorkspaceDeliverability`, via
     `reputation.Rates`) and its sends of the last 24 hours shown next to its Send cap
     (`Module.SentLast24h`, via `providerlimit.SentLast24h`, ADR 0067); and the service endpoints (a Service credential) scope over the one
     Workspace a call names, after its row is loaded, to set, clear and list its Staff holds
     (`Module.SetHold`, `ClearHold`, `ActiveHolds`; the send path and the site API read them
     through the `holds` module, which holds no raw client); the abuse detector
     (ADR 0026) scopes over the Workspace being judged, after its row is loaded, to read the
     rates (`abuse.Watch`, which the console's `Module.WorkspaceAbuse` and the `/site`
     dashboard's warning also read through the `abusewatch.Watch` seam (`internal/abusewatch`, a leaf that `ee` implements and `site` consumes)); and the service usage
     read (`Module.WorkspaceUsage`, `ee/staff/service_usage.go`), scoped over the one
     Workspace a call names, after its row is loaded, to read its stored Usage figures; and
     `Module.TrackingHostFeed` (ADR 0038) reads the Instance-wide tracking host change log, which
     holds hosts and states, no Workspace data, so it scopes over nothing; and the contacts peak
     of `ee/usage` (`raise`, run by its ent hook), which scopes over each
     Workspace id it is given only after loading that Workspace's row and finding it live.
- **The raw `*ent.Client` is allowed only in:** `internal/capturedmessages` (only `Purge`, the Instance-wide age purge of Captured messages, ADR 0058, called by a river job; every other function takes a `*ent.Scoped`), `internal/accounts` (User, Membership,
  Workspace incl. Workspace id resolution, invitation by token, and the Pending sign-up of a Sign-in provider, ADR 0062: no User
  or Workspace exists yet, and its completion creates them, its Consent records and an Invitation's Membership in one
  transaction; the prune river job only calls it), `internal/api/auth` (credentials, token and key lookup),
  `internal/consent` (signed unsubscribe/confirm tokens: the Workspace comes from the token,
  so it works on the bus's raw transaction client and scopes from the token's Workspace),
  `internal/oauthserver`, `internal/secondfactor` (a User's TOTP Second factor and Recovery
  codes, ADR 0020: they belong to the User, not a Workspace), `internal/staffmembers` (the Staff member, ADR 0008, 0026, 0063: its table belongs to no Workspace; the store, the login adapter and the session check, on every Instance), `internal/signupitems` (the Sign-up items, their Legal document drafts and published versions, ADR 0062: they belong to the Instance, not to a Workspace), `internal/consentrecords` (the Consent records, ADR 0062: a User's decisions on the Sign-up items belong to the User, not to a Workspace; `Write` adds them on the transaction client of the step that makes the User or changes the answer, `Module.Current` reads a User's latest answer per item, `Module.Pending` and `Gated` are the one definition of a User stopped by the acceptance gate (a required acceptance in effect whose current version they have not accepted) and `AcceptPending` records the re-acceptance, and `Module.OptionalConsents` and `SetOptionalConsent` read and change a User's optional consents in account settings, a change adding one record with the source account settings, and `Mailings` the Users' latest answers on the mailings consent item for the service feed of Users), `internal/signupsurvey` (the Sign-up survey, ADR 0055: a User's answers and the "survey due" marker on the User row belong to no Workspace, and the submit writes both in one transaction), `internal/rekey` (`sphericon encryption-key reencrypt`: it rewrites the stored ciphertext of every Workspace, User and Staff member row under the primary key, by id and compare-and-swap), `internal/machine` (machine engagement, ADR 0053: the iCloud Private Relay ranges table, which no Workspace owns, read per hit by the tracking endpoints and replaced by the refresh river job in `internal/jobs/private_relay.go`, which only calls it), `internal/doctor` (`sphericon doctor`, read-only: it lists every Workspace's Sending domains and the origin of the newest sent mail, and reads the Postgres version), `internal/instance` (the Instance state row
  that no Workspace owns, set by a Staff member in the console, ADR 0032, 0063; it reads the Memberships of every live Workspace to tell an owner of any of them of a newer release; `DefaultSendCapIn` reads the default Send cap on the transaction of `accounts` that creates a Workspace, ADR 0067), `internal/instanceaudit` (the Instance audit log, ADR 0070: its entries belong to the Instance, not to a Workspace; `Record` writes one on the transaction client of the Instance-level Staff change it records, `Entries` reads it for the console), `internal/joindomains` (Join
  domains: a domain is verified by one Workspace across the Instance, so the lookup of the verified row
  by domain runs before any scope exists, and handing a domain over to the Workspace that now proves it
  scopes one transaction to both Workspaces through `events.Bus.WithinHandOverTx`; the periodic
  re-check of verified domains, a river job in `internal/jobs`, lists the due domains of every Workspace with
  `Module.Due` and scopes each `Module.Recheck` to the Workspace of the row it loaded; the Owners of a Workspace that
  loses a domain, by that re-check or by a hand-over, are mailed through the `Notify` seam after the write commits), `internal/domainverify` (the Sending domain verification jobs' module, ADR 0010: it reads a
  Sending domain by id to learn its Workspace before any scope exists, then writes through that scope), `internal/trackingdomains` (the Workspaces' Tracking domains, ADR 0071: the recheck job lists the verified hosts of every
  Workspace with `Module.Due` and loads a row by id in `Module.Recheck` before any scope exists, then writes through that
  Workspace's scope), `internal/automations` (only `FenceOn`, the Automation run fence, ADR 0046: it opens the transaction that holds the share lock on the Enrollment's row; every other function takes a `*ent.Scoped`), `internal/suspension` (Workspace
  suspension and the Staff's other levers on the Workspace row: the abuse exemption and the Send cap, ADR 0067), `internal/workspacedeletion` (the Workspace deletion cycle, ADR 0043: it lists the Workspaces whose window has passed and
  the Owners' deleted ones, which have no scope yet, opens a transaction per erase step (one per
  Workspace-owned table, the Send cap's bucket `send_cap_limiters` among them), scopes to the
  one Workspace being erased inside it, and writes the row the cycle moves), `internal/events`
  (the bus and its subscribers), `internal/jobs` (job entry points), `internal/server`
  (tracking by recipient id, provider hooks, composition, and the Instance-wide Tracking host
  index, ADR 0038: the host gate `trackingHostGate` / `verifiedTrackingHost` in `trackinghost.go`,
  `tlsAskHandler` and `recipientOnHost` resolve a host to its Workspace before any scope exists), `ee/audit` (the Audit log bus
  subscriber: its envelope carries only a Workspace id, ADR 0022), `ee/export` (the Export streams' bus
  subscriber, which writes the Erasures rows from `contact.erased`, and their prune job over every
  Workspace's rows, ADR 0045), `ee/retention` (the
  periodic prune job: a job entry point that lists Workspaces, then scopes), `ee/abuse` (the hourly
  abuse detector, ADR 0026, the same shape: it lists the Workspaces that are neither suspended nor
  exempt, then scopes to judge each, and writes its notice markers by id), `ee/usage` (the periodic
  usage job, same shape, plus the paged read of every Workspace's stored figures, and the ent hook, installed by `Edition.Hooks()` on the client and the bus's transaction client only with the `staff` license, that keeps the contacts peak as Contacts change), `ee/staff` (the
  Enterprise sections of the Staff console, ADR 0026: the Workspace rows, suspension state and
  rates the console shows as tenant metadata, the Service credentials and Staff holds; Workspace data goes
  through the scopes of list item 7), `internal/workspacefeed` (the ent hook, installed on every client, that gives
  a Workspace the next feed position when it is created or an owner is added, removed,
  transferred or changes email or verification: it must reach the Workspace a User or
  Membership change belongs to; `ee/staff` serves the feed in `Module.WorkspaceFeed`), `internal/userfeed` (the same pair of hooks for the service feed of Users, ADR 0062, installed on every client: it gives a User the next feed position when they are created, renamed, change email or verification, make a new decision on the Sign-up item that stands for consent to the operator's mailings or that item's mark is set or cleared, and leaves a tombstone when a User is deleted; it reads Consent records, Legal document versions and Sign-up items to find the Users a change belongs to, and refuses every Consent record written outside a transaction; `ee/staff` serves the feed in `Module.UserFeed`, reading the consents through `consentrecords.Mailings`), `internal/trackinghostlog` (the same pair of hooks for the Tracking host change log, ADR 0038, ADR 0071: it reads the Tracking domains a change touches, across Workspaces for a bulk write, to log the hosts it adds or removes; `ee/staff` serves it in `Module.TrackingHostFeed`) and the composition roots
  (`internal/app`, `internal/db`, `internal/testhelper`, and `ee.Edition`, which only hands the client to the `ee/*` packages listed above). Needing raw access anywhere else
  means a new small package in this list, not a field on `Handlers`.
  `internal/eligibility` holds no raw client: it takes a `*ent.Scoped` and passes
  `s.WorkspaceID()` as a bound SQL argument inside one raw expression (the correlated
  contact check reads the outer row's `workspace_id` column), not as a hand-written
  `WorkspaceID(ws)` predicate.
- **Outside the mixin:** `OAuthCode` keeps a hand-written `workspace_id` and a one-way `workspace`
  edge (an FK that cascades when the Workspace is erased), so it has no scoped wrapper.
- **Lint:** `forbidigo` (`.golangci.yml`) bans entity-level `entity.Update()` repo-wide,
  because a loaded entity still carries the raw client. Update by id instead:
  `s.Tag().UpdateOneID(id)`. There are no exclusions.

## Enterprise Edition, owners, async work and email

- **Enterprise Edition (`ee/`)**: same binary, gated by the runtime license key (`SPHERICON_LICENSE_KEY`,
  verified by `ee/licensekey`; empty = plain core, a bad key fails boot). `ee.New` builds the
  `Edition` (extra bus consumers + read seams) in `internal/app` and in `testhelper.Setup`
  (`WithoutLicense()` for the unlicensed case); core reaches EE only through interfaces
  (`site.AuditLog`, `events.Consumer`). The audit table's ent schema sits in `ent/schema`
  (ent has one schema package) but is written and read only by `ee/audit`. An Audit entry is
  an unprojected event (`events.Unprojected`): persist, automations and webhooks skip it.
  The Audit table's only remover is `ee/retention` (the `retention` license feature, a
  Workspace `retention_days` window, one hourly river job registered through `Edition.Jobs()`).
  An explicit entry, one no audited write produces (a Workspace setting, a login, a membership
  change, the Audit log export `audit.Log.Export`), is recorded one way: `events.RecordAudit`
  inside the transaction of the change (`bus.WithinTx`, or `bus.WithinScopedTx` with the scope's
  `Actor()` when there is a scope); `events.ExplicitAuditActions` lists its actions. Many of them
  have no scope to take the actor from (a User's action goes to each of their Workspaces, a
  suspension names its Staff actor), so the scoped client has no recorder of its own. `ee.New`
  takes the bus for the export: the bus is built first, with the Edition's hooks from `ee.Hooks`.
  In tests the router does not run, so call `env.DeliverToEE(t)` to hand the outbox to the EE
  subscribers.
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
  no cgo. The server starts the runtime in the background at boot (`Warm`; a library caller's first call does it otherwise), limits are enforced before and during a render. Editor and
  bundle dependencies in `packages/emaildoc/package.json` are pinned exactly.
- The tracker snippet (`/t.js`) is the `@sphericon/analytics` IIFE bundle, built and embedded:
  `mise run build:tracker` copies `packages/analytics/dist/t.js` into `internal/server/assets/`.
