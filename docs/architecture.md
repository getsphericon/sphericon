# Architecture

The shape of the backend and the rules its code follows. Review rules are in `docs/standards/review-rules.md`,
coding standards in `CODING_STANDARDS.md`.

## Backend architecture

- **DI**: `samber/do` container, the one composition root. `internal/app/container.go`
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
- **Sign-in sequence**: `internal/domain/identity/login` (ADR 0020) is the one core module for the password and
  second steps and for `Prove` (the password check behind second-factor and profile actions):
  address normalization, the throttle consulted first (ADR 0025), the decoy hash, the verification
  gate (ADR 0030), failure counting and the challenge. It holds no database client; an adapter
  supplies the `Store`, `Throttle`, challenge `Parse`/`Verify` and issues the session. `/site`
  (`internal/api/site/auth_login.go`) and `internal/domain/identity/staffmembers` (`login.go`, the second adapter, ADR 0026) are
  the two adapters: add sign-in rules in the module, not in a handler. The throttle is
  `throttle.Attempts.For(kind)` (`internal/infra/throttle`), a `Counter` bound to one `Kind` (`KindLogin`, `KindStaffLogin`),
  which satisfies `login.Throttle`. Handlers switch on the per-step outcomes (`login.Outcome`,
  `login.SecondOutcome`).
- **Sign-up sequence**: `signup.UseCase.SignUp` (ADR 0030, 0031; the use case
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
- **Package boundaries are checked by easement (`easement.yml`, `mise run check:arch`).** A surface
  package (`internal/api/{site,external,collect,staff}`) never imports another surface, the core (`internal/{kernel,infra,domain,usecase}`)
  imports neither a surface nor `ee/`, and only the wiring packages (`internal/app`,
  `internal/server`, `internal/testhelper`, `internal/doctor`, `internal/healthcheck`,
  `cmd/server`) assemble them; only they import `ee/`. Inside `ee/`, the Enterprise surfaces (`ee/api/**`,
  component `ee-api`) call an Enterprise use case or domain module and hold neither the raw client
  nor the container, and only the Edition (`ee/ee.go`, component `ee-edition`) builds them: no
  Enterprise module imports a handler (see "Enterprise Edition layout" below). The drivers (ADR 0102) are the API surfaces, their shared security
  handlers and Problem mapping (`internal/api/auth`, `internal/api/problem`), MCP (`internal/mcpserver`,
  component `mcp`, with the surfaces' rules) and the job runner (`internal/jobs`, component `runner`):
  each receives input, calls a domain module or a use case and returns or records the result (review
  rule 5), and no core tier imports one. Only the wiring imports MCP and the runner, and the Edition hands its jobs in as a
  `jobextension.Extension` (`ee/jobs.go`). A domain module or a use case does the work as a function
  and may queue a job (its args, river's insert), but declares no worker and no periodic job: the
  runner or the Edition calls the function, and easement forbids river's `WorkerDefaults`,
  `AddWorker` and `NewPeriodicJob` below them. A new wiring package is added to the `wiring` component, never to a
  baseline. The kernel (`internal/kernel`, component `kernel`, ADR 0102) is the bottom tier: the
  packages that import none of ours, such as the vocabulary every tier shares (`channels`,
  `providers`, `holdreason`, `tokenscope`, `jobkind`) and the helpers that know no domain module
  (`i18n`, `validation`, `pagination`, `patch`, `secrets`, `credentials`, the renderers `emaildoc`,
  `emailrender`, `smsrender` and `telegramrender`). Every component may import it, and easement fails
  its import of any package of ours outside `internal/kernel`, the generated `ent` and `gen` trees
  included; a package that needs one is not kernel. The infrastructure (`internal/infra`, components
  `infra` and `infra-raw`, ADR 0102) sits above it: the bus (`events`), the database connection and
  migrations (`db`, `devdb`, `postgres`), the message providers (`messaging` and its providers, with
  `dkimstate`, the one definition of a verified Sending domain that the DKIM signer reads), the
  outbound HTTP calls (`httpcall`, `webhook`), the request throttle (`ratelimit`), the Login throttle (`throttle`), `logging`,
  OpenTelemetry (`telemetry`, served by `sidelistener`) and the process's configuration (`config`,
  which stays at the root). Every component may import it, and easement fails its import of
  anything of ours but the kernel and the generated ent client: the bus knows no consumer (the
  wiring registers them; the suppression consumer is a job in `internal/jobs`, and the send-outcome
  counter of a bounce or a complaint is `app.RecordSendOutcome`, registered with `Bus.OnStored`), and
  the database package installs no ent hook (`internal/app/entclient` builds every client). No core
  tier (kernel, infrastructure, domain) imports an API surface's generated package (component
  `gen-surfaces`): what the core shares with a contract is generated into the kernel, as the scope
  vocabulary is into `internal/kernel/tokenscope`. The one exception is the Telemetry contract's
  client (`gen/telemetry`, component `gen-telemetry`), which the Telemetry report (the use case
  `internal/usecase/telemetryreport`) uses. The drivers, the
  wiring, the Enterprise surfaces and the Edition may import the surfaces' packages. The ban binds the
  Enterprise domain modules and use cases too: the Service credential scopes are generated from the
  service contract into the module that owns them (`ee/domain/identity/servicecredentials`, `Scope`), as
  the API token scopes are into `apitokens`.
  The check is strict: a new Go package under `internal/kernel/` is covered by the `kernel`
  component, one under `internal/infra/` by `infra`, one under `internal/usecase/` by `usecase`,
  one under `internal/domain/identity/` by `domain-identity` (the raw-client holders by name in
  `domain-identity-raw`, see "Domain contexts" below), one under `internal/domain/contacts/` by
  `domain-contacts`, one under `internal/domain/content/` by `domain-content`, one under
  `internal/domain/sending/` by `domain-sending` (the raw-client holders by name in
  `domain-sending-raw`), and likewise under `internal/domain/orchestration/`,
  `internal/domain/insights/` and `internal/domain/instance/` by `domain-orchestration`,
  `domain-insights` and `domain-instance` (with `domain-orchestration-raw` and
  `domain-instance-raw`), one under `ee/api/` by `ee-api`, one under `ee/usecase/` by `ee-usecase`,
  one under `ee/domain/identity/` or `ee/domain/insights/` by `ee-domain-identity` or
  `ee-domain-insights` (the raw-client holders by name in their `-raw` components), and one
  anywhere else under `ee/` by none: it fails the check until it is placed. There
  is no catch-all for the rest of `internal/`: a new package elsewhere there fails the check until
  it is put in a tier, a context or a component of its own (the drivers, the wiring, `tools` for
  the packages of the repository's commands under `cmd/`, `testsupport` for what the tests share
  below the harness), and a new top-level tree must be assigned a component or listed as `unowned`
  in `easement.yml`. Generated Go (`*_gen.go`) is `target_only`: a handler may import
  its ogen package, whose own imports are not checked. The same file also holds the raw
  `*ent.Client` list (the contexts' `domain-<context>-raw`, `infra-raw` and `usecase-raw`,
  see "Workspace scoping") and the `samber/do` ban.
- **Domain contexts (ADR 0102).** A domain module lives in `internal/domain/<context>/<module>`, and
  the contexts are ordered from the bottom: identity → contacts → content → sending → orchestration →
  insights → instance. A module imports the kernel, the infrastructure, ent, the modules of its own
  context and those of the contexts below, never one above and never a use case; an operation that
  needs a module above is a use case. easement holds a context as a few components named
  `domain-<context>[-<part>]`, one per privilege: `domain-<context>` is the glob of the context's
  directory (the modules that hold no raw client, forbid list `x-core-forbids`),
  `domain-<context>-raw` is the closed list of its raw-client holders (forbid list
  `x-raw-holder-forbids`), and a part a rule binds alone gets a component of its own. Each lists in
  `may_depend_on` the other components of its context and every component of the contexts below,
  in context order, and nothing above, so an import against the order fails `check:arch` with the
  context's reason. Every module of the core is in its context. Identity, the bottom context: the Workspaces and the
  Memberships (`domain-identity-base`, which imports no other domain module: every module may read a
  Workspace, ask a Permission or tell the Owners something), the Users (`domain-identity-users`, the
  one caller of `memberships.LeaveDoomedWorkspaceIn`), the raw-client holders `consentrecords`,
  `invitations`, `joindomains`, `oauthclients`, `passkeys`, `recoverycodes`, `signupitems`,
  `signupsurveyresponses`, `staffmembers`, `suspension`, `usertombstones` and `workspacefeed`
  (`domain-identity-raw`), and `apitokens`, `login`, `signinprovider` and `accountlink` (`domain-identity`).
  An identity module imports no module of another context: not `providerlimit`, not `contacts`.
  Contacts, above it, is one component (`domain-contacts`, the glob of
  `internal/domain/contacts`): `contacts`, `segments`, `tags`, `condition`, `confirmations`,
  `unsubscribes`, `channelstop`, `suppressions`, `eligibility`, `visitors` and `orders`, with the Contact
  row lock (`contactlock`) and the hook that keeps stored addresses lower-cased (`emailcase`, the
  Email case, which targets the Contacts as well as identity's Users and Invitations). None of them holds the raw
  client, so the context has no `domain-contacts-raw`. A contacts module imports no module of a
  context above: not `broadcasts`, `automations` or `rules`, not `integrations`, `bindings` or
  `emailtemplates`, and no use case.
  Content, above contacts, is one component too (`domain-content`, the glob of
  `internal/domain/content`): `emailtemplates`, `brandkits` and `versioneddefinition`, the lifecycle every Versioned
  definition shares. Sending, above content, is `outboundmessages`, `holds`, `providerlimit`,
  `integrations`, `sendingdomains`, `bindings`, `trackingdomains`, `trackinghostchanges`, `messagetoken`,
  `linkclick`, `reputation`, `capturedmessages` and `privaterelayranges` (the spec's list), with one package
  placed by the same rule: `preset`, the engine that reads an Integration's callbacks
  (`integrations` uses it). The DKIM and DMARC primitives of a Sending domain are `sendingdomains`'
  own. Its raw-client holders `bindings`, `capturedmessages`,
  `integrations`, `privaterelayranges`, `sendingdomains`, `trackingdomains` and `trackinghostchanges` are
  `domain-sending-raw`, the rest `domain-sending`. A content module imports no Integration,
  Outbound send or other sending module, and neither context imports `broadcasts`,
  `automations`, `rules`, `webhookendpoints` or a use case.
  Orchestration, above sending, is `broadcasts`, `automations`, `automationstep`, `rules` and
  `webhookendpoints` (the spec's list), with two packages placed by the same rule: `liveuse`, the one shape of a
  refused removal (ADR 0049), a leaf below `broadcasts` and `automations`, which return it, and
  `removalcheck`, the Removal check at the top of the context, which reads the Segments below it and
  the Broadcasts, Automations and Rules beside it through their `<module>use` readers and is a
  component of its own (`domain-orchestration-removalcheck`, imported only by the use cases and the
  wiring). The ent aggregate functions of the Broadcast and Automation reports
  hold no domain rule, so they are infrastructure (`internal/infra/aggregates`). Its raw-client holders `automations`, `rules` and `webhookendpoints` are
  `domain-orchestration-raw`, the rest `domain-orchestration`. Insights, above orchestration, is
  one component (`domain-insights`): the read packages `setupchecklist`, `auditlog`,
  `abusewarning` and `exportstream` (the spec's list), with `dataexport`, the Data export of one
  Contact, which reads Outbound messages and Broadcast recipients and so sits above orchestration.
  Instance, the top context, is `instancestates` and `instanceauditentries` (`domain-instance-raw`, both
  hold the raw client), `upgrade` and `upgradenote` (`domain-instance`). The Telemetry report the
  spec puts there is the use case `internal/usecase/telemetryreport` over `instancestates`. An
  orchestration module imports no insights or instance package, and an insights package no
  instance module.
  Besides the tiers, `internal/` holds the drivers, the wiring (with `internal/healthcheck`, the
  image's HEALTHCHECK command), the packages of the repository's own commands (component `tools`:
  `contractcopy`, `prcheck`, `upgradecheck`, `scopegen`, `goverterenum` and
  `fixtures/fixturegen`, which nothing of the application imports) and what the tests share below
  the harness (component `testsupport`: `fixtures`, `dnstest` and `telemetryschema`, which only the
  harness and `cmd/seed` import).
  To move a module into its context: `git mv` its directory, rewrite the import paths
  (`github.com/getsphericon/sphericon/internal/<module>` to
  `.../internal/domain/<context>/<module>`) and the module-relative paths that name it (the
  `forbid_symbol` names in `easement.yml`, the ownership map of `entrywrite`, `.golangci.yml`,
  the ast-grep rules, scopegen's outputs, the docs), put it in the context's component (by name in
  `domain-<context>-raw` if it holds the raw client), regenerate (`mise run generate:scopes`, goverter, `generate:mocks`) and let
  `easement suggest --check` trim the `may_depend_on` lists.
- **Only the owner writes a table (ADR 0102, analyzer `entrywrite`).** Each ent entity names the
  package that owns its table in the ownership map (`owners` in `tools/analyzers/entrywrite`, which a
  test holds to `ent/schema` and to the directories of the repository), and a write to it from any
  other package fails the lint. The owner is the domain module of the entity (a module that owns
  several is named after the one the others belong to: `contacts` owns the Contact, its Custom fields
  and its join rows with Tags, so a Contact's Tags are applied there, `contacts.ApplyTagIn`, over the
  `tags` catalogue's `EnsureIn`); the infrastructure owns the tables of no domain (the bus the Events,
  `internal/infra/events`, which stores, prunes, stitches (`StitchVisitor`) and erases them and lists
  the Event log (`ListLog`, `DistinctActions`); the Login
  throttle the attempts; `internal/infra/db` the schema state the migrations keep); a hook package owns
  the log it keeps (`usertombstones` the User tombstones, `trackinghostchanges` the Tracking host changes); the
  Enterprise Edition owns its own tables in its domain modules (`ee/domain/insights/auditentries` the Audit
  entries, `ee/domain/insights/erasures` the Erasures rows, `ee/domain/insights/usages` the Usage rows,
  `ee/domain/identity/servicecredentials` the Service credentials). Any other package, a use case
  included, reads the table and changes it only through a function of its owner: the Workspace's
  standing (`workspaces.SuspendIn`, `SetSendCapIn`, the abuse notice markers, the feed position)
  for `suspension`, `ee/usecase/abusedetection` and `workspacefeed`; the User's survey marker and feed position
  (`users.ClearSurveyDueIn`, `SetFeedPosition`) for `signupsurveyresponses` and `usertombstones`; a Membership an
  Invitation or a Join domain admits (`memberships.AddInvitedIn`, `AddByDomainIn`, `SetRoleIn`) for
  `users`; a provider's quota answer and refusal of an Integration (`integrations.RecordQuota`,
  `RecordRefused`) for `providerlimit` and `outboundmessages`. A write through a generic helper counts where
  the helper is instantiated with an ent write builder.
- **Use cases (ADR 0102, component `usecase`).** An operation that spans several domain modules is a
  use case, `internal/usecase/<subject><action>` in glossary words (`tagdeletion`), never a call from
  one domain module into the modules above it: a base module knows no module that uses it. The use
  case is a `UseCase` type built by `New` from what it needs (the bus, the Instance clock: ADR 0104
  states the call contract every use case follows, which the `usecaseshape` analyzer checks), registered in
  `internal/app/container.go` and handed to the drivers through their `Deps` (the job runner's
  included: the Sending domain re-check jobs call `sendingdomainverification`, the step jobs of an
  Enrollment `stepexecution`). It takes the
  `*ent.Scoped` its driver built, opens the transaction the domain modules it calls join (taking the
  Workspace lock where ADR 0099 requires it, so a removal that runs the Removal check opens it with
  `WithinWorkspaceLockTx`, as `tagdeletion` and `customfieldretyping` do; `contacterasure` opens `WithinScopedPurgeTx`, takes no Workspace lock and locks the Contact through `contactlock`; `unsubscriberecording` opens one `WithinScopedTx` for the opt-out, the Broadcast's count and the Enrollment's exit, and `contactintake` one per imported row for the Contact and its SMS Confirmation), reads them, and changes their rows only through their functions
  (`sendingdomainverification` stores a check's outcome with `sendingdomains.RecordDKIM` and `RecordDMARC`; Erasure calls an erase function of each owner: `events.EraseSubject`, `visitors.EraseSubject`, `bindings.EraseSubject`, `outboundmessages.AnonymizeContact`, `confirmations.EraseContact`, `unsubscribes.EraseSMS` and `DetachContact` and the rest, then `contacts.DeleteIn`; an unsubscribe writes the opt-out through `unsubscribes`, drops the Confirmation an opt-out of everything ends through `confirmations.Withdraw`, then calls `broadcasts.RecordUnsubscribe` and `automations.Exit`; an imported row upserts through `contacts.ImportIn` and confirms through `confirmations.ConfirmSMSIn`).
  Work that starts before any scope exists, such as a job given only a row's id, has its driver read
  the row's Workspace and build the scope the use case takes (a signed unsubscribe link's comes from
  `workspaces.Module.LinkScope`). easement
  isolates the use cases (`isolated`: none imports another; a piece two of them need moves down into
  the domain module that owns it, as the SMS opt-in a START records is `confirmations.StartSMSIn`
  for the Ingest hook), lets only the drivers and the wiring import one (`allow_importers`)
  and forbids it the raw client. The exception is a use case whose rows belong to no Workspace
  when it starts, so there is no `*ent.Scoped` to take: the Second factor of a User
  (`internal/usecase/secondfactorcheck`) and the exchange of an OAuth code or refresh token, a
  secret that names its grant (`internal/usecase/connectionissuance`). Such a use case holds the
  raw client, opens the raw transaction its modules' `...In(ctx, tx, ...)` functions join and is
  in the closed list of "Workspace scoping"; easement holds it as the `usecase-raw` component,
  one pattern per use case (`internal/usecase/secondfactorcheck*`), isolated like the others.
  Two more are such use cases: the deletion cycle of a Workspace
  (`internal/usecase/workspacedeletion`), which lists the Workspaces that have no scope yet, erases one
  table at a time, each batch of listed ids through the removal of the table's owner (`tags.EraseTagsIn`,
  `events.EraseEventsIn` and one more per Workspace-owned table; the Enterprise Edition's three, which
  no core package may import, the wiring hands in as `workspacedeletion.EditionTables`), and moves the
  Workspace row through `workspaces.DiscardIn`, `RestoreIn` and
  `TombstoneIn`, with the OAuth codes that name it removed by `oauthclients.DeleteCodesIn`; and the
  creation of a Workspace (`internal/usecase/workspacecreation`), which writes the Workspace through
  `workspaces.CreateIn` and its Owner Membership through `memberships.AddCreatorIn` with the Send cap it
  starts with (`instancestates.DefaultSendCapIn`, ADR 0067); and the Sign-up
  (`internal/usecase/signup`), which composes the same two modules for the User's first Workspace in
  every step that makes a User Verified, each on its own transaction: the verification and reset
  links, a Sign-up and an Owner made where verification is off, the provider sign-in and the
  completion of a pending sign-up, calling the `users` functions for the User's part
  (`MarkVerifiedIn`, `CreateVerifiedIn`, `CreateProviderUserIn`, `TakeOverIn`,
  `Module.CompletePendingSignupIn`, `ApplyDomainJoinIn`). A removal (or a change) that a use case guards with a check is the domain module's
  unchecked removal (and so is the store of a check's outcome, `sendingdomains.RecordDKIM`): a package-level function taking the transaction's scoped client (`tags.DeleteIn`, the retype `contacts.UpdateCustomFieldIn`, the Contact row `contacts.DeleteIn`, which only Erasure removes),
  never a method of the module, since easement matches a method only as a use of its type. Its
  `<import path>.<Name>` is an anchored item of `x-unchecked-removals` in `easement.yml`, aliased into
  the `forbid_symbol` list of every component but `usecase` and `usecase-raw` that may reach the core
  (`x-core-forbids` for the API surfaces, MCP, the Problem mapping, each context's `domain-<context>`,
  `ee-api` and the Enterprise `ee-domain-<context>` and `ee-usecase`, `x-raw-holder-forbids` for the runner, each context's raw-client components and
  the Enterprise `ee-domain-<context>-raw` and `ee-usecase-raw`, and the whole list
  in `api-auth`, `ee-edition`, `testsupport` and `wiring`), so
  only its use case calls it: no handler, job, MCP tool or other domain module bypasses the check, nor
  the owning module itself (easement counts a call there too; the definition and tests are not
  checked). The Second factor's writes that skip its checks (the User's
  `users.SetPendingSecondFactor`, `AcceptSecondFactorStepIn`, `ConfirmSecondFactorIn` and
  `ClearSecondFactorIn`, `recoverycodes.ReplaceIn` and `DeleteIn`, `passkeys.DeleteAllIn`) and the
  issue of a grant's credentials (`apitokens.IssueConnection` and `RotateConnection`,
  `oauthclients.NewRefreshTokenIn`), the moves of a Workspace's deletion cycle
  (`workspaces.DiscardIn`, `RestoreIn` and `TombstoneIn`), the core removals of its erase (each
  `Erase…In` of a Workspace-owned table, one shared reason, and `oauthclients.DeleteCodesIn`; the
  Edition's three are in every list but `x-unchecked-removals`, since the wiring and the Edition, which
  take that one, name them to hand them in), the creation of a Workspace with its creator's Membership
  (`workspaces.CreateIn`, `memberships.AddCreatorIn`) and each owner's step of a Contact's Erasure
  (`visitors.EraseSubject`, `events.EraseSubject`, `rules.EraseFirings` and the others
  `internal/usecase/contacterasure` sequences; the purge of its queued jobs is a method of the bus's
  publisher, which a name cannot forbid) are such items too. The tests of the operation live in the use case's own `_test` package
  (`internal/usecase/tagdeletion`, `internal/usecase/customfieldretyping`, `internal/usecase/unsubscriberecording`, `internal/usecase/contactintake`; a retype's refusal is also tested through the API's Custom field update, an Erasure through the APIs' Contact deletion and Privacy request), through its exports; the domain module keeps the tests of its own
  rows. What the Removal check finds is tested in its own package
  (`internal/domain/orchestration/removalcheck`, with the writes that create a use waiting for the
  Workspace lock), and each removal that runs it races those writes in its use case's own
  `workspace_lock_test.go`. An operation that belongs to no Workspace has no `*ent.Scoped` to take: the Release check and
  the Telemetry report (`internal/usecase/releasecheck`, `internal/usecase/telemetryreport`) take
  the Instance state module (`internal/domain/instance/instancestates`), whose functions make the Instance-wide
  reads on its raw client, and hold no client themselves. The Secret re-encryption
  (`internal/usecase/secretreencryption`, `sphericon encryption-key reencrypt`) spans every
  Workspace and the Users and Staff members besides: it takes the `SecretSweeps` of each module that
  owns a table with stored secrets (`integrations`, `webhookendpoints`, `sendingdomains`, `users`,
  `staffmembers`, `automations`, `rules`), the columns as `secrets.Sweep` values the wiring builds on
  the raw client in a fixed order, and sweeps them by id and compare-and-swap
  (`secrets.Cipher.Reencrypt`). A sweep only stores the value it read again under the
  primary key, so no check guards it and it is not in `x-unchecked-removals`.
  A use case need not guard a removal: an operation that writes no table itself but reads or
  changes several modules' rows is one too. A Visitor identification (`internal/usecase/visitoridentification`, built in
  `internal/server` with the collect surface) upserts the Contact through `contacts`, binds the
  Visitor through `visitors`, announces its chats through `bindings` and stitches its Events through
  the bus, in one transaction. The Ingest hook's reading of a provider's callbacks
  (`callbackinterpretation`) opens the request's transaction and changes Outbound messages, Suppressions,
  Unsubscribes, Confirmations and Bindings through their modules; the Transactional send
  (`transactionalsend`) hands its request to the Outbound send module; recording a customer's Event
  (`eventrecording`) resolves its Contact and publishes it on the bus, which owns the Event log's reads; the Analytics reports (`analyticsreport`) and what Needs
  attention (`needsattentionreport`) only read.
  The consent of a Destination (`internal/domain/contacts/confirmations`,
  `internal/domain/contacts/unsubscribes` and `internal/domain/contacts/channelstop`, the stops read in
  the Channel) sits in the contacts context, below the Broadcasts and Automations an unsubscribe
  touches: easement fails its import of `broadcasts`, `automations` or a use case as an import
  against the order of the contexts (`domain-contacts`), so only `unsubscriberecording` calls
  `broadcasts.RecordUnsubscribe` and ends the Enrollment. A Telegram stop (`channelstop.StopTelegram`, from the
  Ingest hook's button press or a send that finds the bot blocked) writes no Broadcast row: the
  recipient row is the `broadcasts` module's (ADR 0102), and neither `channelstop` nor `outboundmessages`, where a
  blocked send records the stop in its own transaction, may reach it. Its `telegram.unsubscribed`
  names the Broadcast, and the broadcasts consumer (`event_broadcast_unsubscribe`, a job per ADR 0101) counts the unsubscribe with `broadcasts.RecordUnsubscribe` after the opt-out commits. Nothing
  is lost: the job is inserted in the opt-out's transaction, and the count is set once.
  `unsubscribes` is the lowest of the three: `confirmations` imports it (a START lifts the opt-out),
  and `channelstop` composes both (a stop drops the Confirmation).
  One unchecked removal is called from a domain module until its use case exists:
  `memberships.LeaveDoomedWorkspaceIn`, which lets the only Owner of a Workspace in its deletion window
  leave, serves User deletion in `internal/domain/identity/users`, which alone is allowed to call it:
  that module is a component of its own, `domain-identity-users`, whose forbid list
  (`x-user-deletion-forbids`) is the raw-holder list without it, and every other component that may
  reach it forbids it, the use cases included.
- **Resource DTOs come from goverter, not from struct literals (review rule 1, analyzer `handmapper`).** Add a method to the surface's
  `Converter` (`internal/api/{site,external,staff}/resources`, `ee/api/staff/resources`) with
  `goverter:map` / `extend` helpers, and attach in the handler only what the mapper cannot see
  (marked `goverter:ignore` with the reason). Hand-built values are limited to response wrappers
  (`*ListOK`, `*Headers`) and aggregates with no single source (setup checklist, lint findings,
  the integration config `oneOf`).
- Auth security handlers live in `internal/api/auth/auth.go`. External requests carry a
  `*TokenAuth` (scopes + the Workspace's `*ent.Scoped`) in context — query through
  `auth.TokenScoped(ctx)` (see "Workspace scoping" below).
- The answer of every refusal and failure on every surface (ADR 0103) is decided once in
  `internal/api/problem`, beside the security handlers: it is the surfaces' shared code, imported by
  the surfaces, `internal/server` and `ee/api` only. A core package returns its refusal and never
  imports it (easement component `api-problem`).

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
live and draft pointers and its status; `internal/domain/content/versioneddefinition` is the one lifecycle over them and the
only caller of their writes (`forbidigo`). An entity with `ArchiveMixin` gets `Archive`, `Unarchive`,
`QueryArchived`, `QueryWithArchived` and `Purge` in place of `Delete`, one with `DiscardMixin`
`Discard` and `Purge` (ADR 0100); `internal/scopedclient` tests them on the production entities.

- **Handlers and domain modules hold no `*ent.Client` and no bare `int64` workspace id.**
  They take a `*ent.Scoped` (modules: an explicit `s *ent.Scoped` parameter) and never call
  `client.Scoped(<int64>)`. Tests use `env.DB.Scoped(fixtures.AcmeID)`.
- **`*ent.Scoped` is built only in a closed list of places:**
  1. site: `memberships.Module.Scope`, behind the ogen middleware `Handlers.Access`
     (`internal/api/site/access.go`, ADR 0061), from the user's Membership of the Workspace an
     operation's `workspaceId` names, read with `workspaceScope(ctx)` (the Membership with
     `callerMembership(ctx)`). On every operation that carries a session, the middleware then
     admits the User (`users.Module.Admit`, `internal/domain/identity/users/admission.go`, the one decision
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
     Workspace elsewhere and call `memberships.Module.Scope` themselves: `SiteOAuthDecide` (in its
     body, then `users.Module.Admit` as the middleware does) and `SiteWorkspacesCreate` /
     `SiteWorkspacesRestore` (a Workspace that was not live before);
  2. external (and MCP, which rides the same token auth): `ExternalSecurityHandler.HandleBearerAuth`
     in `internal/api/auth/auth.go`, read with `auth.TokenScoped(ctx)`. A User's own token (an API
     token with a `membership_id`, ADR 0061: a Connection, which names its `oauth_client_id`, or a
     Personal token, which names none; `internal/domain/identity/apitokens/kinds.go` tells them apart) is looked up
     with its Membership: the scope acts as its User, the acceptance gate and the Two-factor
     requirement apply (`users.Module.Admit`, as on /site: 403 `acceptance_pending` or
     `second_factor_required`), and the Role bounds its scopes on every request (`memberships.MayGrant`,
     the bound the OAuth consent and the minting of a Personal token apply too); the Workspace's API
     token meets neither;
  3. collect: `CollectSecurityHandler` in `internal/api/auth/auth.go`, from the collect key;
  4. jobs and event consumers, right after the row that names the Workspace is loaded
     (`client.Scoped(run.WorkspaceID)` in `internal/jobs`; `EvaluateTriggerWorker` loads the
     contact and scopes from its Workspace; the suppression consumer (`jobs.SuppressEventWorker`), and the webhooks
     consumer (`jobs.WebhookEventWorker`), which builds the scope it hands to `Dispatch` from the
     event envelope: an event job is the raw-client entry, an envelope carries only a Workspace
     id; and the orders consumer (`events.RecordOrder`, ADR 0064), which opens a transaction on the raw
     client and scopes it to the envelope's Workspace to build the Orders, handing the scope to
     `orders.Module.Record`), and
     `users.RemindSecondFactorDeadlines` (the deadline reminder job: it lists the Memberships of every
     Workspace with the raw client and scopes each claim, `memberships.ClaimSecondFactorReminder`,
     to the Workspace of the Membership it loaded), and `workspacedeletion.UseCase.EraseDue` (`internal/usecase/workspacedeletion`), which lists the due Workspaces with the raw client and
     scopes each erase step's transaction (`tx.Scoped(id)`) to the Workspace it is erasing;
  5. transactions: `events.Bus.WithinScopedTx` (and its variants: `WithinScopedEnqueueTx`, and
     `WithinWorkspaceLockTx` / `WithinWorkspaceLockEnqueueTx`, which first take the Workspace's
     advisory lock, ADR 0099), and `tx.Scoped(...)` inside
     `users.Module.AcceptInvitation` (its `acceptInvitationIn`, which the completion of a pending
     sign-up also runs in its own transaction), re-scope a transaction's client to the same Workspace;
     `events.Bus.WithinHandOverTx` also scopes it to the Workspace a Join domain moves away from
     (acting as the system), after `joindomains` has read that Workspace off the holder's row;
     the domain-join step of the verification transaction (`applyDomainJoin` in
     `internal/domain/identity/users/join.go`, ADR 0051), which builds `tx.Scoped(...)` for the Workspace a
     verified Join domain names, right after `joindomains.HolderIn` has loaded that row, to add
     the joining User's Membership (every step that proves an address calls it explicitly); and
     `users.Module.DeleteUser` (User deletion, ADR 0050) scopes its one transaction in turn to each
     Workspace the User belongs to (`tx.Scoped(m.WorkspaceID)`, from the User's own Memberships),
     to lock the Owner rows and remove the Membership;
  6. the Workspace comes from a secret rather than a login: the ingest hook
     `POST /hooks/{key}` (`internal/server/hooks.go`, ADR 0037), where the key belongs to one
     Integration, so the hook resolves the Integration by key with the raw client and then
     scopes to its Workspace (`row.WorkspaceID`); the tracking endpoints (`loadEngaged` in
     `internal/server/tracking.go`), where the token names an outbound message that is loaded
     with the raw client and scoped to its Workspace, and the short link `GET /c/{token}` there,
     whose token names a `tracked_links` row that is read with the raw client for the message and
     the Workspace it belongs to; signed unsubscribe/confirm tokens
     (`workspaces.Module.LinkScope`, which the unsubscribe page and the confirmation page build
     the scope with; the one-click POST in `internal/server/tracking.go` scopes with the raw
     client), `workspaces.Module.BootstrapScope` (the bootstrap token), and the token
     endpoint of the OAuth server (`internal/api/oauth`, over the use case
     `internal/usecase/connectionissuance`), where an authorization
     code or a refresh token names its grant: the use case loads the row through
     `internal/domain/identity/oauthclients` and scopes the Connection's writes (`apitokens.IssueConnection`,
     `RotateConnection`, `RevokeGrant`) to the Workspace that row names;
  7. the platform Staff member (ADR 0026): `ee/domain/insights/workspaceoversight` scopes over the one Workspace the console
     is looking at, in three reads, each after the Workspace row is loaded and each through
     the module that already serves `/site`: its Audit log (`Module.AuditEntries`, via
     `auditentries.Log.Entries`), its deliverability rates (`Module.WorkspaceDeliverability`, via
     `reputation.Rates`) and its sends of the last 24 hours shown next to its Send cap
     (`Module.SentLast24h`, via `providerlimit.SentLast24h`, ADR 0067); and the service endpoints (a Service credential) scope over the one
     Workspace a call names, after its row is loaded, to set, clear and list its Staff holds
     (`Module.SetHold`, `ClearHold`, `ActiveHolds`; the send path and the site API read them
     through the `holds` module, which holds no raw client); the abuse detector
     (ADR 0026) scopes over the Workspace being judged, after its row is loaded, to read the
     rates (`abusedetector.Watch` of `ee/domain/insights/abusedetector`, which the console's `UseCase.WorkspaceAbuse` and the `/site`
     dashboard's warning also read through the `abusewarning.Watch` seam (`internal/domain/insights/abusewarning`, a leaf that `ee` implements and `site` consumes)); and the service usage
     read (`Module.WorkspaceUsage`, `ee/domain/insights/workspaceoversight/service_usage.go`), scoped over the one
     Workspace a call names, after its row is loaded, to read its stored Usage figures; and
     `Module.TrackingHostFeed` (ADR 0038) reads the Instance-wide tracking host change log, which
     holds hosts and states, no Workspace data, so it scopes over nothing; and the contacts peak
     of `ee/domain/insights/usages` (`raise`, run by its ent hook), which scopes over each
     Workspace id it is given only after loading that Workspace's row and finding it live.
- **The raw `*ent.Client` is allowed only in:** `internal/domain/sending/bindings` (only `PruneStartTokens`, the Instance-wide purge of lapsed and used Start tokens, ADR 0076, called by a river job; every other function takes a `*ent.Scoped`), `internal/domain/sending/capturedmessages` (only `Purge`, the Instance-wide age purge of Captured messages, ADR 0058, called by a river job; every other function takes a `*ent.Scoped`), `internal/domain/identity/users` (the User,
  ADR 0102, and the Pending sign-up of a Sign-in provider, ADR 0062: no User
  or Workspace exists yet, and its completion creates them, its Consent records and an Invitation's Membership in one
  transaction, which the use case `signup` opens; the prune river jobs only call it), `internal/domain/identity/invitations` (the lookup of a pending Invitation by its
  token, the only handle, before any Workspace is known; issuing and revoking take the Workspace's scope), `internal/domain/identity/workspaces` (the Workspace row, ADR 0043, ADR 0102: it is the
  tenant, not a tenant-owned entity, so the scoped client has no wrapper for it; a setting is written on the Workspace a
  scope names, and the command line resolves a Workspace id), `internal/domain/identity/memberships` (the Memberships, ADR 0061, ADR 0102:
  the lookup of a User's Membership that builds a Workspace's scope, the list of a User's Workspaces, the Owners of
  several Workspaces), `internal/api/auth` (credentials, token and key lookup),
  `internal/domain/identity/oauthclients` (the OAuth clients with their authorization codes and refresh tokens,
  ADR 0016: a client and its codes belong to no Workspace until a code is exchanged, and a code is
  looked up by its secret before any scope exists), `internal/domain/identity/passkeys` and `internal/domain/identity/recoverycodes`
  (a User's Passkeys and Recovery codes, ADR 0052, ADR 0020: they belong to the User, not a
  Workspace), the use cases `internal/usecase/secondfactorcheck` (a User's TOTP Second
  factor, ADR 0020: one transaction across the User, the Recovery codes and the Passkeys, none of
  which a Workspace owns), `internal/usecase/connectionissuance` (the token endpoint's
  exchange of a code or a refresh token, a secret that names its grant, see list item 6),
  `internal/usecase/workspacecreation` (a Workspace with its Owner Membership, ADR 0043: no Workspace
  exists yet), `internal/usecase/signup` (the steps that make a User Verified with its first
  Workspace, ADR 0030, ADR 0043: neither exists yet), `internal/usecase/noticedelivery` (the Notices of
  the domain events, ADR 0105: an event of a User's account belongs to no Workspace, and a
  Workspace's Owners and Members are read by the id an event names) and
  `internal/usecase/workspacedeletion` (the Workspace deletion cycle, ADR 0043: it lists the
  Workspaces whose window has passed and the Owners' deleted ones, which have no scope yet, opens a
  transaction per erase step (one per Workspace-owned table, the Send cap's bucket
  `send_cap_limiters` among them), scopes to the one Workspace being erased inside it, and moves the
  Workspace row through `workspaces`), `internal/domain/identity/staffmembers` (the Staff member, ADR 0008, 0026, 0063: its table belongs to no Workspace; the store, the login adapter and the session check, on every Instance), `internal/domain/identity/signupitems` (the Sign-up items, their Legal document drafts and published versions, ADR 0062: they belong to the Instance, not to a Workspace), `internal/domain/identity/consentrecords` (the Consent records, ADR 0062: a User's decisions on the Sign-up items belong to the User, not to a Workspace; `Write` adds them on the transaction client of the step that makes the User or changes the answer, `Module.Current` reads a User's latest answer per item, `Module.Pending` and `Gated` are the one definition of a User stopped by the acceptance gate (a required acceptance in effect whose current version they have not accepted) and `AcceptPending` records the re-acceptance, and `Module.OptionalConsents` and `SetOptionalConsent` read and change a User's optional consents in account settings, a change adding one record with the source account settings, and `Mailings` the Users' latest answers on the mailings consent item for the service feed of Users), `internal/domain/identity/signupsurveyresponses` (the Sign-up survey, ADR 0055: a User's answers and the "survey due" marker on the User row belong to no Workspace, and the submit writes both in one transaction), `internal/domain/sending/integrations`, `internal/domain/orchestration/webhookendpoints` and `internal/domain/orchestration/rules` (only `SecretSweeps`, the stored secrets of the Integrations, the Webhook endpoints and the Rule versions of every Workspace, which `sphericon encryption-key reencrypt`, the use case `internal/usecase/secretreencryption`, has each owner rewrite under the primary key by id and compare-and-swap, as it has the `SecretSweeps` of `users`, `staffmembers`, `sendingdomains` and `automations`, and the Rules' `PruneFirings`, the Instance-wide age prune of Rule Firings the Event retention job calls; every other function of these three takes a `*ent.Scoped`), `internal/domain/sending/privaterelayranges` (machine engagement, ADR 0053: the iCloud Private Relay ranges table, which no Workspace owns, read per hit by the tracking endpoints and replaced by the refresh river job in `internal/jobs/private_relay.go`, which only calls it), `internal/doctor` (`sphericon doctor`, read-only: it lists every Workspace's Sending domains and the origin of the newest sent mail, and reads the Postgres version), `internal/domain/instance/instancestates` (the Instance state row
  that no Workspace owns, set by a Staff member in the console, ADR 0032, 0063; `HoldsReleaseRead` reads the Memberships of every live Workspace for the Release check to tell an owner of any of them of a newer release, and `Figures` reads the Instance-wide counts the Telemetry report rounds; `DefaultSendCapIn` reads the default Send cap on the transaction of the use case `workspacecreation` or `signup` that creates a Workspace, ADR 0067), `internal/domain/instance/instanceauditentries` (the Instance audit log, ADR 0070: its entries belong to the Instance, not to a Workspace; `Record` writes one on the transaction client of the Instance-level Staff change it records, `Entries` reads it for the console), `internal/domain/identity/joindomains` (Join
  domains: a domain is verified by one Workspace across the Instance, so the lookup of the verified row
  by domain runs before any scope exists, and handing a domain over to the Workspace that now proves it
  scopes one transaction to both Workspaces through `events.Bus.WithinHandOverTx`; the periodic
  re-check of verified domains, a river job in `internal/jobs`, lists the due domains of every Workspace with
  `Module.Due` and scopes each `Module.Recheck` to the Workspace of the row it loaded; the Owners of a Workspace that
  loses a domain, by that re-check or by a hand-over, are told by the `join_domain.lost` event the loss publishes in its transaction; `Tokens` reads every Workspace's claim of a domain for the end-to-end
  suite's resolver), `internal/domain/sending/sendingdomains` (only `Due`, the Sending domains of every Workspace due a DKIM re-check, ADR 0010,
  listed by a river job that then reads each domain by id to learn its Workspace and hands the use case
  `internal/usecase/sendingdomainverification` that Workspace's scope; `DevTXTLookup`, the end-to-end suite's resolver; and `SecretSweeps`, the DKIM private keys the re-encryption rewrites; every other
  function takes a `*ent.Scoped`), `internal/domain/sending/trackingdomains` (the Workspaces' Tracking domains, ADR 0071: the recheck job lists the verified hosts of every
  Workspace with `Module.Due` and loads a row by id in `Module.Recheck` before any scope exists, then writes through that
  Workspace's scope), `internal/domain/orchestration/automations` (only `FenceOn`, the Automation run fence, ADR 0046: it opens the transaction that holds the share lock on the Enrollment's row, and `SecretSweeps`, the sealed HTTP step fields of every Automation version the re-encryption rewrites; every other function takes a `*ent.Scoped`), `internal/domain/identity/suspension` (Workspace
  suspension and the Staff's other levers on the Workspace row: the abuse exemption and the Send cap, ADR 0067; it opens the transaction and writes the row through `workspaces`), `internal/infra/events`
  (the bus, the Event rows it writes and the event consumers' work), `internal/jobs` (job entry points), `internal/server`
  (tracking by recipient id, provider hooks, composition, and the Instance-wide Tracking host
  index, ADR 0038: the host gate `trackingHostGate` / `verifiedTrackingHost` in `trackinghost.go`,
  `tlsAskHandler` and `recipientOnHost` resolve a host to its Workspace before any scope exists), `ee/domain/insights/auditentries` (the Audit log's event
  consumer: its envelope carries only a Workspace id, ADR 0022), `ee/domain/insights/erasures` (the Export streams'
  event consumer, which writes the Erasures rows from `contact.erased`, and their prune job over every
  Workspace's rows, ADR 0045), `ee/usecase/auditlogretention` (the
  periodic prune job: a job entry point that lists Workspaces, then scopes), `ee/usecase/abusedetection` (the hourly
  abuse detector, ADR 0026, the same shape: it lists the Workspaces that are neither suspended nor
  exempt, then scopes to judge each, and writes its notice markers by id through `workspaces`), `ee/domain/insights/usages` (the periodic
  usage job, same shape, plus the paged read of every Workspace's stored figures, and the ent hook, installed by `Edition.Hooks()` on the client and the bus's transaction client only with the `staff` license, that keeps the contacts peak as Contacts change), `ee/domain/insights/workspaceoversight` (the
  Enterprise sections of the Staff console, ADR 0026: the Workspace rows, suspension state and
  rates the console shows as tenant metadata and Staff holds; Workspace data goes
  through the scopes of list item 7), `ee/domain/identity/servicecredentials` (the Service credentials,
  which belong to no Workspace), `internal/domain/identity/workspacefeed` (the ent hook, installed on every client, that gives
  a Workspace the next feed position when it is created or an owner is added, removed,
  transferred or changes email or verification: it must reach the Workspace a User or
  Membership change belongs to; `ee/domain/insights/workspaceoversight` serves the feed in `Module.WorkspaceFeed`), `internal/domain/identity/usertombstones` (the same pair of hooks for the service feed of Users, ADR 0062, installed on every client: it gives a User the next feed position when they are created, renamed, change email or verification, make a new decision on the Sign-up item that stands for consent to the operator's mailings or that item's mark is set or cleared, and leaves a tombstone when a User is deleted; it reads Consent records, Legal document versions and Sign-up items to find the Users a change belongs to, and refuses every Consent record written outside a transaction; `ee/domain/insights/workspaceoversight` serves the feed in `Module.UserFeed`, reading the consents through `consentrecords.Mailings`), `internal/domain/sending/trackinghostchanges` (the same pair of hooks for the Tracking host change log, ADR 0038, ADR 0071: it reads the Tracking domains a change touches, across Workspaces for a bulk write, to log the hosts it adds or removes; `ee/domain/insights/workspaceoversight` serves it in `Module.TrackingHostFeed`), `internal/infra/db` (the guard's read of the schema state row, on a client of its own before the application's exists), `internal/infra/throttle` (the Login throttle and the mail budgets of an address, ADR 0025: an AuthAttempt belongs to an address, not a Workspace, and the purge river job only calls it) and the composition roots
  (`internal/app`, whose `internal/app/entclient` builds every client with the hooks above and hands the bus its transaction client, `internal/testhelper`, and `ee.Edition`, which only hands the client to the `ee/*` packages listed above). Needing raw access anywhere else
  means a new small package in this list, not a field on `Handlers`. easement holds the core
  domain modules of the list as the raw-client components of their contexts in `easement.yml` (the
  identity modules of the list as `domain-identity-base`, `domain-identity-users` and
  `domain-identity-raw`, the sending modules as `domain-sending-raw`, the orchestration modules as
  `domain-orchestration-raw`, the instance modules as `domain-instance-raw`, see "Domain
  contexts"); the job runner as its own `runner` component, which does not forbid the client
  either, the raw use cases as the `usecase-raw` component, and the bus, the database package and the Login throttle,
  `internal/infra/events`, `internal/infra/db` and `internal/infra/throttle`, as the `infra-raw` component, and the
  Enterprise packages of the list as `ee-domain-identity-raw`, `ee-domain-insights-raw` and
  `ee-usecase-raw`. It fails a use of
  `ent.Client` anywhere else in `internal/` and `ee/` (and in a surface package), so a new package goes into
  both lists.
  `internal/domain/contacts/eligibility` holds no raw client: it takes a `*ent.Scoped` and passes
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
  `Edition` (extra event consumers + read seams) in `internal/app` and in `testhelper.Setup`
  (`WithoutLicense()` for the unlicensed case); core reaches EE only through interfaces
  (`site.AuditLog`, `events.Consumer`). The audit table's ent schema sits in `ent/schema`
  (ent has one schema package) but is written and read only by `ee/domain/insights/auditentries`. An Audit entry is
  an event that is not Projected (`events.Projected`): it has no Event row, automations skip it,
  and a Webhook endpoint receives it only when it names `audit.entry`.
  The Audit table's rows are removed only through `ee/domain/insights/auditentries`: by `ee/usecase/auditlogretention`'s prune (the
  `retention` license feature, a Workspace `retention_days` window, one hourly river job registered
  through `Edition.Jobs()`, which calls `auditentries.PruneBefore`) and by the erase of a deleted Workspace.
  An explicit entry, one no audited write produces (a Workspace setting, a login, a membership
  change, the Audit log export `auditentries.Log.Export`), is recorded one way: `events.RecordAudit`
  inside the transaction of the change (`bus.WithinTx`, or `bus.WithinScopedTx` with the scope's
  `Actor()` when there is a scope); `events.ExplicitAuditActions` lists its actions. Many of them
  have no scope to take the actor from (a User's action goes to each of their Workspaces, a
  suspension names its Staff actor), so the scoped client has no recorder of its own. `ee.New`
  takes the bus for the export: the bus is built first, with the Edition's hooks from `ee.Hooks`.
  The Edition's consumers of the domain events (`ee.EventConsumers`: the Audit log's and the
  Erasures stream's) are registered on the bus beside core's; their workers come with
  `Edition.Jobs()` and `ExportJobs()`. In tests no River runtime works the queue, so call
  `env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)` to work the jobs a publish inserted.
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
- **Enterprise Edition layout (ADR 0102).** `ee/` has the core's shape: its domain modules in
  `ee/domain/<context>/<module>`, in the core's contexts and order, its use cases in
  `ee/usecase/<name>`, its surfaces in `ee/api`, the Edition `ee/ee.go`, and beside them the
  License key's gate `ee/licensekey` (component `ee-licensekey`), which imports nothing of ours.
  The same rule places a package: one that owns a table is a domain module in the context of what
  it targets, an operation across modules is a use case, and a package that is both is split.
  Identity: `servicecredentials` (the Service credentials, with their scope vocabulary generated
  from the service contract). Insights: `auditentries` (the Audit entries), `erasures` (the Erasures
  rows), `usages` (the Usage rows), which hold the raw client (`ee-domain-insights-raw`), and
  `abusedetector` (the Abuse detector's limits and its reading of a Workspace's standing,
  `ee-domain-insights`), and the Workspace oversight (`workspaceoversight`, `ee-domain-insights-raw`:
  the Staff console's Enterprise sections and the service endpoints over suspension, holds, the
  Audit log, Usage, the Abuse detector and the service feeds; a module of the top of insights, since
  each of its operations calls one module and what it owns is the rules of looking across
  Workspaces). The use cases (`ee-usecase-raw`, each an isolated instance, both holding the raw
  client): `abusedetection` (the detector's hourly pass, which warns and freezes through
  `workspaces`, `memberships` and `suspension`) and `auditlogretention` (the advanced-retention prune
  over `workspaces` and the Audit log). The `export` license gate is the Edition's own (`ee.ExportStreams`).
  An Enterprise domain module imports the core's modules of its context and the contexts below
  and the Enterprise modules of those contexts, never a module above nor a use case; an Enterprise
  use case imports domain modules only, calls no unchecked removal of the core (unlike a core use
  case) and is imported only by `ee/api`, the Edition and the wiring. The boundary with the core is
  unchanged: no core component may import an `ee` component, and each `ee` component names its
  importers.
- **Owners and suspension are defined once.** `internal/domain/identity/memberships` owns "Owner of a
  Workspace": `Owners`, `OwnersOf`, `LockOwners`, `IsOwner`, `OwnerRole`;
  `internal/domain/identity/suspension` owns "suspended" (`Is`, `Suspended`, `NotSuspended`,
  `BySystem`). A Workspace is suspended through the container's one `suspension.Suspender` (the
  command line, the Staff console and the abuse detector), whose event tells its Owners (see
  "Notices"). Call them; do not test `suspended_at` or the owner Role anywhere else.
- **Refused removals (ADR 0049)**: a row in Live use or in Definition use is not removed (nor is a
  Custom field retyped while a Condition tests it with a leaf the new type does not take).
  `internal/domain/orchestration/liveuse` is the one shape of that refusal (`*liveuse.Error`: the
  removed `Subject` and the `Users` that hold it, with helpers like `liveuse.Broadcasts` and
  `liveuse.Enrollments`), and every API surface answers it with the contract's `Problem` (409,
  `usedBy`). When the holders are modules above the removed row's module, the check is the Removal
  check, `internal/domain/orchestration/removalcheck`: one method per subject (`Tag`,
  `CustomFieldRetype`, `Integration`, `SendingDomain`), each returning nil or the `*liveuse.Error`
  with its `Subject` set, called by the use case that removes the row under the Workspace lock before
  its unchecked removal (`tagdeletion` before `tags.DeleteIn`, `customfieldretyping` before
  `contacts.UpdateCustomFieldIn`, `integrationdeletion` and `sendingdomaindeletion` before
  `integrations.DeleteIn` and `sendingdomains.DeleteIn`; see "Use cases"). It is the one list of the
  readers of what may hold a row. Each owner keeps its readers in a subpackage `<module>use` (ADR
  0110's fence by place), which may import its owner and which the owner does not import: Definition
  use is `segmentsuse.UsingTag` and `UnfitForCustomFieldType`, `automationsuse.UsingTag`,
  `rulesuse.UsingTag` and `UnfitForCustomFieldType` and `broadcastsuse.UsingTag` and
  `UnfitForCustomFieldType`; the Live use that sends is `broadcastsuse.LiveSends` and
  `automationsuse.LiveSends`, which return each send as a `liveuse.Send` (who sends, on which
  Channel, through which Integration by `messaging.Through`, from which From), so the Removal check
  and the send path agree on where work sends; a Sending domain's check reads the From an
  Integration sends with from the resolver a send builds its sender with
  (`messaging.EffectiveFrom`). Two easement components, one per context (`domain-contacts-removalcheck-readers`,
  `domain-orchestration-removalcheck-readers`), own the four subpackages with `allow_importers:
  [domain-orchestration-removalcheck]` (ADR 0110), so no other package reads a holder and a new holder is a reader the Removal check adds. The Removal check takes the
  transaction's `*ent.Scoped`; the scope type of the Workspace lock (ADR 0108) replaces it in its
  methods and its readers when that type exists. A domain module that checks the holders of its own
  row (an Automation's archive) returns the same `*liveuse.Error`. `liveuse` is a leaf that holds no
  raw `*ent.Client`: it only maps rows to users and states these shapes.
- **Stored Automation Steps:** `internal/domain/orchestration/automationstep` owns the stored Step model, its kinds,
  decoding and whether a definition sends email. Publish, Needs attention and live-use checks
  share that interpretation: an empty stored string means no Steps; damaged JSON returns an error,
  so a removal cannot treat an unreadable definition as unused. The reader chooses the version:
  dependencies use live, an Enrollment uses its pinned version. `internal/domain/orchestration/automations` keeps
  preparation for writing (normalization, HTML sanitization, validation and secret sealing), masked
  API views and the version lifecycle. The shared reader neither queries nor executes Steps.
- **Notices (ADR 0105)**: every mail sphericon sends on its own behalf, through the System sender, is
  a reaction to a domain event. A module or a use case that changes something a person must be told
  of (a User's account, an Invitation, the Two-factor requirement, a suspension, an abuse warning, a
  lost Join domain or Sending domain verification, a replaced or removed Tracking host, a disabled
  Webhook endpoint) publishes its event in the transaction of the change
  (`internal/infra/events/account.go`, `workspace_changes.go`), and knows neither the recipients nor
  the sender; a request that changes no state publishes with `events.Bus.Publish`: an account's
  requests for a mailed link (a password reset, the verification link sent again, an email change,
  a deletion link) are `users.Requests`, which take the address's mail budget and publish the same
  work for every address (ADR 0031), and the `/site` handlers only call them. The use case
  `internal/usecase/noticedelivery` answers the Notices an event calls for, who is told and what,
  read when the event is handled, a link minted then, and the Notices' consumer
  (`internal/jobs/notices.go`) queues one send job per Notice, unique on the event and the address,
  which the System sender delivers. Do not mail a person anywhere else: publish the event and add its
  Notice to the use case.
- **Async**: one queue, river, for jobs and domain events alike (ADR 0101,
  `docs/design/domain-events.md`). `events.Bus` publishes an event inside the transaction of
  the state change: it writes the Event row (a repeated `source_id` publishes nothing) and
  inserts one job per consumer whose filter wants the envelope (`events.Consumer`, registered in
  `internal/app`: `jobs.EventConsumers` and `ee.EventConsumers`). A producer never names a
  consumer; a new consumer is an args type embedding `events.EventJob`, a worker and a registry
  entry, its jobs in the `events` queue. Every event job runs in the context of handling its
  envelope (`events.JobChain`), so what it publishes is caused by that event.
  `internal/jobs` uses river (Postgres-backed queue, `database/sql` driver, polling). A job that must
  exist iff a state change commits is queued in that transaction with
  `events.Bus.WithinScopedEnqueueTx` (a Broadcast's claim and its send job); never claim first and
  enqueue after, with a best-effort undo.
- **The Workspace lock (ADR 0099)**: a removal that checks Live use or Definition use (archiving a
  Segment; deleting an Integration, a Sending domain or a Tag and editing a Custom field, which run
  the Removal check) and a write that creates a use (a Broadcast's claim, saving a Segment, Create,
  Edit, Publish and Archive of a Versioned definition) run in
  `events.Bus.WithinWorkspaceLockTx` (or `WithinWorkspaceLockEnqueueTx`), whose first statement
  takes the Workspace's transaction-scoped advisory lock, so a check never passes on work that
  becomes live before the removal commits. The lock comes before any row lock. A hot path (a Rule
  firing, an Enrollment, a send) never takes it: it locks the row it acts on and checks it again
  under that lock. Email via `internal/infra/messaging`
  (per-workspace providers: smtp/ses). Both providers share `messaging.BuildMIME`
  (wneessen/go-mail) — smtp sends the `*mail.Msg` directly, ses serializes it to raw
  bytes for SES v2 `SendEmail` (raw content).
- **Automation HTTP Steps**: `stepexecution.UseCase.ExecuteHTTP` (`internal/usecase/stepexecution`) selects the Enrollment's pinned Step,
  checks its lifecycle and position, opens and renders its stored call, and executes it through
  the protected HTTP adapter under the Enrollment's exit fence. Success advances and queues the
  next Step in one transaction; final failure records the Enrollment's failure. Ordinary attempt
  data (`httpcall.Attempt`) produces mutually exclusive retry, cancellation or wait decisions;
  the River worker only establishes Workspace scope and translates those decisions. A rate-limit
  wait spends no ordinary attempt. The stable external request identity is preserved, but a call
  accepted before a failed advancement may be repeated.
- **Rule HTTP Actions**: `rules.Module.ExecuteHTTP` selects the pending Action of a Firing,
  opens its secrets, renders its Contact and Event, executes the protected call and records the
  result. It receives ordinary attempt and timing data (`httpcall.Attempt`) and the existing
  protected HTTP adapter, returning a retry or wait decision without River types. The River
  worker loads the Firing to establish Workspace scope and translates that decision. A final
  failure belongs to the Action's result; it does not cancel the Firing's other Actions or adopt
  Enrollment lifecycle rules. The call retains its stable request identity and existing delivery
  guarantees: a request accepted before a failed result write may be repeated.
- `internal/kernel/emaildoc` (ADR 0040) turns an editor document into email HTML inside the binary: the editor's own
  export code, bundled by `packages/emaildoc` (`mise run build:emaildoc`; rerun it after any change to the pinned
  `@react-email/editor`, then `go test ./internal/kernel/emaildoc -update` to accept the new golden HTML) and run in goja,
  no cgo. The server starts the runtime in the background at boot (`Warm`; a library caller's first call does it otherwise), limits are enforced before and during a render. Editor and
  bundle dependencies in `packages/emaildoc/package.json` are pinned exactly.
- The tracker snippet (`/t.js`) is the `@sphericon/analytics` IIFE bundle, built and embedded:
  `mise run build:tracker` copies `packages/analytics/dist/t.js` into `internal/server/assets/`.

## Frontend resource forms

`src/resources` owns resource form hydration, request conversion and the create/edit lifecycle.
Editors hold typed values, including collections, Condition drafts and message documents, and pass
them directly to the form. `resourceFormSchema` combines explicit empty editor values with the
generated request schema: omitted updates stay absent, nullable clears stay null, and empty
collections remain supplied. Scalar blanks follow the contract's clearing behavior; a null empty
editor value can omit a nonnullable field, such as a document when editing HTML source. Generated
schemas validate the resulting request, including incomplete drafts. `defineResource` preserves
unfinished edits across refetches, reloads on an intentional resource revision change and resets
stateful editors to the normalized saved resource after a successful save.
