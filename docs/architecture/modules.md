# Module boundaries and ownership

[Architecture](../architecture.md) → Module boundaries and ownership

## Package boundaries

**Package boundaries are checked by easement (`easement.yml`, `mise run check:arch`).** A surface
package (`internal/api/{site,external,collect,staff}`) never imports another surface, the core (`internal/{kernel,infra,domain,usecase}`)
imports neither a surface nor `ee/`, and only the wiring packages (`internal/app`,
`internal/server`, `internal/testhelper`, `internal/doctor`, `internal/healthcheck`,
`cmd/server`) assemble them; only they import `ee/`. Inside `ee/`, the Enterprise surfaces (`ee/api/**`,
component `ee-api`) call an Enterprise use case or domain module and hold neither the raw client
nor the container, and only the Edition (`ee/ee.go`, component `ee-edition`) builds them: no
Enterprise module imports a handler (see [Enterprise Edition layout](enterprise.md#enterprise-edition-layout)). The drivers (ADR 0102) are the API surfaces, their shared security
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
client (`gen/telemetry`, component `gen-telemetry`), which the Telemetry report of the Instance
state (`internal/domain/instance/instancestates`) sends with. The drivers, the
wiring, the Enterprise surfaces and the Edition may import the surfaces' packages. The ban binds the
Enterprise domain modules and use cases too: the Service credential scopes are generated from the
service contract into the module that owns them (`ee/domain/identity/servicecredentials`, `Scope`), as
the API token scopes are into `apitokens`.
The check is strict: a new Go package under `internal/kernel/` is covered by the `kernel`
component, one under `internal/infra/` by `infra`, one under `internal/usecase/` by `usecase`,
one under `internal/domain/identity/` by `domain-identity` (the raw-client holders by name in
`domain-identity-raw`, see [Domain contexts](#domain-contexts)), one under `internal/domain/contacts/` by
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
see [Workspace scoping](workspace-scoping.md)) and the `samber/do` ban.

## Domain contexts

**Domain contexts (ADR 0102).** A domain module lives in `internal/domain/<context>/<module>`, and
the contexts are ordered from the bottom: identity → contacts → content → sending → orchestration →
insights → instance. A module imports the kernel, the infrastructure, ent, the modules of its own
context and those of the contexts below, never one above and never a use case; an operation that
needs a module above is a use case. easement holds a context as a few components named
`domain-<context>[-<part>]`, one per privilege: `domain-<context>` is the glob of the context's
directory (the modules that hold no raw client), `domain-<context>-raw` is the closed list of its
raw-client holders (with the other holders, the `allow` of `ent.Client` in the top-level
`forbid_symbol`), and a part a rule binds alone gets a component of its own. Each lists in
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
`unsubscribes`, `channelstop`, `suppressions`, `eligibility`, `visitors`, `orders` and
`eventrecording` (the Event recording, which has no table: the bus stores the Events), with the Contact
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
hold the raw client), `upgrade` and `upgradenote` (`domain-instance`). The Release check and the
Telemetry report are `instancestates`' own operations (ADR 0112). An
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

## Table ownership

**Only the owner writes a table (ADR 0102, analyzer `entrywrite`).** Each ent entity names the
package that owns its table in the ownership map (`owners` in `tools/analyzers/entrywrite`, which a
test holds to `ent/schema` and to the directories of the repository), and a write to it from any
other package fails the lint. The owner is the domain module of the entity (a module that owns
several is named after the one the others belong to: `contacts` owns the Contact, its Custom fields
and its join rows with Tags, so a Contact's Tags are applied there, `contacts.ApplyTagIn`, over the
`tags` catalogue's `EnsureIn`); the infrastructure owns the tables of no domain (the bus the Events and external-fact receipts,
`internal/infra/events`, which stores, prunes, stitches (`StitchVisitor`) and erases them and lists
the Event log (`ListLog`, `DistinctActions`); the Login
throttle the attempts; `internal/infra/db` the schema state the migrations keep); a hook package owns
the log it keeps (`usertombstones` the User tombstones, `trackinghostchanges` the Tracking host changes); the
Enterprise Edition owns its own tables in its domain modules (`ee/domain/insights/auditentries` the Audit
entries, `ee/domain/insights/erasures` the Erasures rows, `ee/domain/insights/usages` the Usage rows,
`ee/domain/identity/servicecredentials` the Service credentials). Any other package, a use case
included, reads the table and changes it only through a function of its owner: the Workspace's
standing (`workspaces.SuspendIn`, `SetSendCapIn`, the abuse notice markers, the feed position)
for `suspension`, `ee/domain/insights/abusedetector` and `workspacefeed`; the User's survey marker and feed position
(`users.ClearSurveyDueIn`, `SetFeedPosition`) for `signupsurveyresponses` and `usertombstones`; a Membership an
Invitation or a Join domain admits (`memberships.AddInvitedIn`, `AddByDomainIn`, `SetRoleIn`) for
`users`; a provider's quota answer and refusal of an Integration (`integrations.RecordQuota`,
`RecordRefused`) for `providerlimit` and `outboundmessages`. A write through a generic helper counts where
the helper is instantiated with an ent write builder. The owner also declares each unique
constraint of its tables beside those writes (ADR 0106): `var constraints = db.Declare(...)`, each
constraint by exact name a `db.Refusal` with the module's sentinel or a `db.Fault` with its reason,
and a write that may be refused runs through `constraints.Write`, under a savepoint of whatever
transaction it is in. A use case that answers a refusal differently matches the owner's sentinel
with `errors.Is`; it never reads the violation itself. `TestEveryUniqueConstraintIsDeclaredByItsOwner`
(`internal/app`) fails on an undeclared constraint, a declaration the schema lacks, one made outside
the table's owner in this map, and a refusal no test of its owner asserts with `ErrorIs`. A refusal
that never leaves its owner, which records it as the row's state (`trackingdomains`' host held by
another Workspace), is unexported and caught there with `errors.Is` instead.
