# Handler and module organization: sphericon against six Go projects and cal.com

Researched October 2026 from primary sources: each repo shallow-cloned at its default branch (commit SHAs
below), reading handler code, domain or service code, wiring and contracts. No blog posts. Every claim is
**verified** (file path and line numbers at the commit SHA, so lines stay valid), or marked **inference** or
**unverified**. Nothing in the six clones was built or executed. The metrics come from a small Go program of
ours (`go/parser` only, standard library) that parses the source files and runs nothing in them; its rules
are stated under "Metrics" so the numbers can be reproduced. cal.com, a TypeScript open-core product, was
added afterwards for its two API layers and its permission system, measured with ast-grep under the same rules
where they apply; its section states which tree was read and why.

| Repo                      | Branch | Commit                                     |
| ------------------------- | ------ | ------------------------------------------ |
| getsphericon/sphericon    | main   | `d2d4fd540cc27fe98339c754c74adebbb9fed0e1` |
| knadh/listmonk            | master | `a74f40b346aca27ceed3e2bbf6528712d3d16375` |
| Notifuse/notifuse         | main   | `f635aa09c1050c2df1e8f7b7a0dd6e516019b146` |
| rudderlabs/rudder-server  | master | `cc094269b19bcb5077490c3bfa4d7500f6265222` |
| zitadel/zitadel           | main   | `4977f818a22272b3bc161911ef92aa97c0fa2d0e` |
| coder/coder               | main   | `edd2e070410f27d5408378efc207cf2eae87e800` |
| hatchet-dev/hatchet       | main   | `d1ecfc864d81d5046d71fa5dfdcf6656bf646e95` |
| calcom/cal.com (pre-fork) | main   | `a17f28e9ab44d34a24840cff70082470d775b9b1` |
| calcom/cal.com (HEAD)     | main   | `91eefbf9f460bac7d69327fea4d9560295390e0a` |

Base URL: `https://github.com/<org>/<repo>/blob/<sha>/<path>#L<n>`. Below, paths are given as `path:line`
and the SHA is the one in this table. A sphericon path is at the sphericon commit above, which is the
worktree HEAD this note was written on.

## Question

sphericon splits an HTTP request between a handler package per API surface (`internal/api/site`,
`internal/api/external`, `internal/api/collect`), each implementing the ogen `Handler` interface generated from
TypeSpec, and domain modules under `internal/*` that take a `*ent.Scoped` (see [architecture](../architecture.md)
and review rule 5 in [review rules](../standards/review-rules.md)). How do six Go projects with a comparable
surface (multi-tenant SaaS-style APIs, a marketing tool, an ingestion gateway, an identity server, a dev
platform, a task queue) organize the same split, and where is sphericon ahead, where does it share their
weaknesses, and what could it borrow? sphericon is held to the same rubric as the others, including where that
does not flatter it.

## Summary of findings

- **sphericon's `Handlers` is wide but not a god object in the sense criticised elsewhere.** The site struct has
  43 fields and 222 methods (181 of them the 181 ogen operations), the external one 23 fields and 113 methods.
  Unlike Coder's `API` (64 fields plus an embedded 88-field `Options`, 600 direct `Database` calls in handler
  files), sphericon's handlers reach storage directly at 8 call sites, all single-row reads; writes are banned by
  the `entrywrite` analyzer. The width is a facade over 34 concrete modules, each method uses one or two.
- **About half of a routine sphericon handler is error-to-problem and response mapping.** In a hand-classified
  sample of eight routine handlers, 134 of 274 code lines map errors and responses, 65 resolve the Workspace and
  check the role or scope, 61 map input, 14 call the module. ogen's per-operation response unions force the
  mapping into each handler: `"workspace not found"` is written 117 times in `internal/api/site`, and the three
  surfaces call `problem(http.Status…)` 561 times.
- **Permissions are imperative in sphericon; the contract does not carry them.** 102 `auth.HasScope` checks in
  `internal/api/external` and 34 `accounts.Can*` role checks in `internal/api/site`. The hand-written checks
  have drifted: an insufficient scope answers 401 at 99 sites and 403 at 3. Hatchet (`rbac.yaml` keyed by
  operation id), Zitadel (`auth_option.permission` in the proto) and sphericon's own `/service` surface
  (`x-required-scopes`, enforced by the security handler) declare them instead.
- **site and external duplicate handler code for the same resource,** because two contracts generate two Go
  type sets: 15 unexported helpers exist in both packages, and `SiteIntegrationsUpdate` and `IntegrationsUpdate`
  are near-identical bodies. The rule itself is not duplicated (it is in the module), but the same domain
  error maps to different statuses (`apitokens.ErrNameEmpty`: 422 on site, 400 on external). Zitadel shows the
  same pattern across API versions: the default PAT scope list is copied into its v1 and v2 handlers.
- **Three sphericon handlers hold real business rules,** against rule 5: `SiteAuthRegister` (equal work per
  address, race recovery on a unique violation, password replacement), `SiteUserUpdateMe` (blank name, password
  policy, current-password proof, session re-issue) and the OAuth callback sequencing. In the sample, 134 of the
  198 code lines of the three longest handlers are such rules.
- **Transactions belong to modules and generated wrappers in sphericon.** About 20 core packages open one (plus
  the wiring in `internal/app` and `internal/server`, and `ee/staff`); one handler does (`internal/api/site/audit.go:148`). Notifuse has no transaction around its two-row API key write
  and repairs a failure with a compensating delete; listmonk and Coder write one row; Zitadel pushes events in
  one transaction; rudder-server batches in its writer goroutines.
- **Tenancy is enforced more strongly in sphericon than in any of the six.** The generated scoped client makes a
  foreign row unreachable by type; Notifuse and Hatchet pass a string or UUID tenant id down to the repository,
  Coder enforces RBAC at runtime around the store, Zitadel adds the instance id from the context inside the
  eventstore, listmonk has no tenant.
- **cal.com shows both halves of the permission question.** Its NestJS API v2 has thin controllers (median 13
  lines, no Prisma call) because role and tenant requirements are decorators read by guards, but the guards
  are opt-in, its PBAC guard cannot refuse, and allow decisions are cached for five minutes. Its tRPC layer
  keeps rules in handlers (147 of 344 call Prisma), and v2 reuses those handlers as its domain layer. Its
  `main` is no longer open-core: one commit removed `ee` and left allow-all permission stubs in core.
- **The creation of an API token is the same shape in four projects:** route, authenticate, resolve the tenant,
  check a role, validate, generate a secret, store its hash, return the secret once. Coder's
  `apikey.InheritWithinCaller` / `ErrExceedsCaller` is the same rule as sphericon's `apitokens.MintWithin` /
  `ErrScopeEscalation`. rudder-server has no such operation: write keys come from the control plane's backend
  config.

## Metrics

### Method

One program, the same rules for every tree. A directory is read non-recursively unless written as `dir/...`.
Test files (`_test.go`) and generated files (a `// Code generated … DO NOT EDIT.` line before the `package`
clause) are skipped everywhere. LOC is non-blank lines.

- **Handler functions.** A function counts as a handler when it is an exported method on the receiver named for
  the project, or when its parameter list is exactly the stated signature. Its length is from the `func` line to
  the closing brace. Closures with the same signature (handler factories, and middleware) are counted apart.
- **Fields and methods of the main handler struct** are counted from its declaration; an embedded field counts
  as one.
- **Storage call sites in the handler layer.** A call expression whose selector chain names one of the stated
  identifiers (a field, a variable or a package). For sphericon the rule is ent's shape instead:
  `X.<Entity>().<Query|Create|CreateBulk|Get|GetX|Update|UpdateOne|UpdateOneID|Delete|DeleteOne|DeleteOneID>(…)`,
  plus any chain naming `Client` (the runs with and without `Client` found the same 8 sites). A chain counts once. Every reported site set was spot-checked by reading the
  lines.

| Project   | Handler layer (non-recursive unless `/...`)                                                                       | Handler rule                                                                                                                                                                                          | Logic layer                                                                                  | Storage identifiers                                                                     |
| --------- | ----------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------- |
| sphericon | `internal/api/{site,external,collect}`                                                                            | exported methods on `Handlers`                                                                                                                                                                        | `internal/...` minus `api`, `app`, `server`, `mcpserver`, `testhelper`, `fixtures`, `doctor` | ent chain rule above, `Client`                                                          |
| listmonk  | `cmd` minus `init.go`, `main.go`, `install.go`, `upgrade.go`, `manager_store.go` (wiring)                         | `func(*echo.Context)`                                                                                                                                                                                 | `internal/...` (`internal/core` alone: 2,394)                                                | `db`, `queries`                                                                         |
| Notifuse  | `internal/http`                                                                                                   | `func(http.ResponseWriter, *http.Request)`                                                                                                                                                            | `internal/service/...`, `internal/domain/...`                                                | `repo`, `workspaceRepo`, `userRepo`, `db`, `DB`                                         |
| rudder    | `gateway`                                                                                                         | same as Notifuse; closures are its real handlers                                                                                                                                                      | `processor/...`, `router/...` (downstream pipeline, not called by handlers)                  | `jobsDB`, `errDB`, `db`                                                                 |
| Zitadel   | `internal/api/grpc/...`                                                                                           | exported methods on any `Server` (36 such types)                                                                                                                                                      | `internal/command/...`, `internal/query/...`, `internal/domain/...`                          | `eventstore`, `es`, `db`, `database`, `client`; `query` and `command` counted apart     |
| Coder     | `coderd` minus `coderd.go` (routes and wiring, kept out)                                                          | same as Notifuse                                                                                                                                                                                      | every `coderd/*/...` subpackage except `database`, `httpmw`, `httpapi`, `coderdtest`         | `Database`, `db`, `tx`                                                                  |
| Hatchet   | `api/v1/server/handlers/...`                                                                                      | `func(echo.Context, gen.…RequestObject)` (strict server)                                                                                                                                              | `pkg/repository/...` (hand-written; sqlc output skipped as generated)                        | `V1`, `APIRepository`, `EngineRepository`, `Repository`, `repo`, `queries`              |
| cal.com   | `packages/trpc/server/routers/...` (`*.handler.ts`); `apps/api/v2/src/...` (`*.controller.ts`, `*-controller.ts`) | tRPC: the default or re-exported function of the file, else the exported function with `ctx`/`input` in its parameters, else the single export; v2: a class method preceded by an HTTP verb decorator | `packages/features/...` (incl. `ee`), `packages/lib/...`; v2 `*.service.ts` and repositories | `prisma.<model>.<op>` (any receiver ending in `prisma`), `prisma.$…`, `tx.<model>.<op>` |

### Results

| Project   | Main handler struct: fields / methods                                                                      | Handler funcs: count, median / p90 / max lines | Handler LOC                                               | Logic LOC                                | Storage calls in handler layer (in handler funcs)                      |
| --------- | ---------------------------------------------------------------------------------------------------------- | ---------------------------------------------- | --------------------------------------------------------- | ---------------------------------------- | ---------------------------------------------------------------------- |
| sphericon | site `Handlers` 43 / 222; external 23 (1 embedded) / 113; collect 1 / 2                                    | 291: 24 / 37 / 95                              | 10,769                                                    | 35,944                                   | 8 (8), all `.Get` reads                                                |
| listmonk  | `App` 27 (1 embedded mutex) / 214                                                                          | 172: 23 / 65 / 244                             | 8,280                                                     | 8,902                                    | 1 (0): a `VACUUM` helper                                               |
| Notifuse  | 35 per-resource structs; `WorkspaceHandler` 7 / 25                                                         | 208: 36 / 61 / 155                             | 11,701                                                    | 72,089                                   | 1 (0): a host lookup helper                                            |
| rudder    | `Handle` 55 / 68                                                                                           | 14 closures: 10 / 52 / 96                      | 2,627                                                     | n/a                                      | 2 (0): the gateway's own writer goroutine                              |
| Zitadel   | 36 `Server` types; user v2 `Server` 9 / 78; `command.Commands` 59 / 1,002                                  | 1,150: 10 / 18 / 236                           | 40,544                                                    | 111,423                                  | 0 executing; read model via `s.query` 844 (393), `s.command` 772 (679) |
| Coder     | `API` 64 (embeds `*Options`, 88 fields) / 552                                                              | 365: 43 / 129 / 390                            | 42,033                                                    | 155,015                                  | 600 (393)                                                              |
| Hatchet   | `apiService` embeds 28 per-area structs, 0 own methods; `APITokenService` 1 field                          | 156: 25 / 73 / 322                             | 8,416                                                     | 35,887                                   | 161 (125)                                                              |
| cal.com   | none (tRPC: 47 area routers of lazy-imported handler files; v2: 80 controller classes with constructor DI) | tRPC 348: 30 / 103 / 793; v2 352: 13 / 22 / 60 | tRPC 38,277 (router tree incl. helpers); v2 not separated | features 137,856 (ee 34,095), lib 14,363 | tRPC 390 (354); v2 controllers 0                                       |

Notes on the numbers:

- sphericon's handler LOC leaves out 1,363 lines of hand-written goverter declarations (`resources/resources.go`
  of site and external), 1,641 lines of HTTP handlers in `internal/server` (provider hooks, tracking, TLS ask),
  which reach the raw client by design (architecture, list item 6), and the `ee/staff` handler types (21
  handler methods, median 18 lines; that package also holds its module, 1,973 LOC together). Counting all of
  them, the handler side is about 15.7k LOC against 35.9k core plus 3.7k `ee`.
- Zitadel's 1,150 handler functions include 204 per-server registration methods (`RegisterServer`, `AppName`,
  `MethodPrefix`, `AuthMethods`, `FileDescriptor`, …), which are not rpc implementations and pull the median
  down; the 66 exported methods of the user v2 `Server` alone give median 13, p90 28, max 61.
- Zitadel's 11 hits on the storage identifiers are false positives of the rule: seven build an eventstore search
  query in the handler (for example `internal/api/grpc/management/org.go:46-55`, which also writes the tenant
  filter `ResourceOwner(authz.GetCtxData(ctx).OrgID)` there), the others are type conversions and a GCS
  `client.Bucket` (`internal/api/grpc/admin/import.go:298`). None executes storage; the query is handed to
  `s.query`.
- rudder-server's handlers are factory closures (`gateway/handle_http.go:41`), so the closure figure is its
  primary one; only one named function matches the exact signature. Its logic layer is the asynchronous
  processor and router, which no handler calls, so a LOC ratio would mean nothing.
- Coder's logic LOC includes infrastructure subpackages (`rbac`, `provisionerdserver`, `telemetry`, test-support
  packages such as `aibridgedtest`); it is an upper bound on its domain code.

## sphericon, held to the same rubric

### The `Handlers` struct

`internal/api/site/handlers.go:97-148` declares 43 fields: 34 pointers to concrete modules or services
(`*broadcasts.Module`, `*accounts.Accounts`, `*login.Module[*ent.User]`, …), 4 interfaces (`WelcomeEnqueuer`
`:58`, `SystemMailEnqueuer` `:66`, `AuditLog` `:153`, `abusewatch.Watch`) and 5 plain values (`appURL`,
`supportEmail`, `now`, `origins`, `requireVerification`). It implements all 181 operations of the generated
`siteapi.Handler`, asserted at `:261`. `internal/api/external/handlers.go:38-63` has 23 fields: the embedded
`externalapi.UnimplementedHandler`, 19 module pointers, `bootstrapToken`, and the two interfaces `AuditLog` and
`ExportStreams`; it implements all 108 operations today, but the embedding means a new operation compiles
without a handler (site has no such embedding).

**Verdict (inference):** a wide facade, not the god struct pattern. The criticism of Coder's `API` and rudder's
`Handle` is that the struct is both the router's receiver and the place where logic and storage access live
(600 and 2 storage sites respectively, handlers of up to 390 lines in Coder). sphericon's methods have a p90 of
37 lines, delegate to modules, and cannot write through ent (rule 5, analyzer `entrywrite`,
`CODING_STANDARDS.md:70-80`). What the width does cost: every module is a constructor argument (`Deps`,
`:175-241`, one literal at `:248-256`), and a test or a reader cannot tell from the type which modules an
operation needs. Hatchet's per-area structs (one field each) show the alternative; see the verdicts.

### Where handler code goes

Sample: the three longest handlers by the metric, plus eight routine ones chosen to cover create, update, read
and delete on site and external, including the token operations traced below. Code lines are non-blank,
non-comment lines of the body, classified by hand into five buckets.

| Handler (file:line)                                    | Code | Auth, tenant, role/scope | Input mapping  | Module call | Error and response mapping | Rules and sequencing |
| ------------------------------------------------------ | ---- | ------------------------ | -------------- | ----------- | -------------------------- | -------------------- |
| `SiteAuthRegister` (`site/auth.go:24`)                 | 63   | 0                        | 3              | 0           | 1                          | 59                   |
| `SiteAuthSigninCallback` (`site/auth_signin.go:64`)    | 68   | 13                       | 7              | 11          | 7                          | 30                   |
| `SiteUserUpdateMe` (`site/me.go:50`)                   | 67   | 5                        | 5              | 10          | 2                          | 45                   |
| `SiteTokensCreate` (`site/tokens.go:45`)               | 32   | 12                       | 4              | 1           | 15                         | 0                    |
| `SiteTokensDelete` (`site/tokens.go:118`)              | 22   | 12                       | 1              | 1           | 8                          | 0                    |
| `SiteBroadcastsCreate` (`site/broadcasts.go:86`)       | 41   | 8                        | 10             | 2           | 21                         | 0                    |
| `SiteIntegrationsUpdate` (`site/integrations.go:137`)  | 54   | 12                       | 20             | 3           | 19                         | 0                    |
| `SiteSendingDomainsGet` (`site/sending_domains.go:93`) | 19   | 8                        | 1              | 1 (ent)     | 9                          | 0                    |
| `AuthTokensCreate` (`external/auth.go:56`)             | 16   | 4                        | 0 (+10 helper) | 1           | 11                         | 0                    |
| `IntegrationsUpdate` (`external/integrations.go:125`)  | 47   | 5                        | 20             | 3           | 19                         | 0                    |
| `EmailsSend` (`external/emails.go:23`)                 | 43   | 4                        | 5              | 2           | 32                         | 0                    |
| **Routine eight**                                      | 274  | 65 (24%)                 | 61 (22%)       | 14 (5%)     | 134 (49%)                  | 0                    |
| **Three longest**                                      | 198  | 18                       | 15             | 21          | 10                         | 134 (68%)            |

Paths in the table are under `internal/api/`. What the buckets hold:

- **Auth, tenant, role/scope.** `scopedFor` / `scopedWithRoleFor` (`site/handlers.go:269-324`) plus the 404 for an
  unknown Workspace, then `accounts.CanManage…(role)` or `auth.HasScope(…)`. The same eight lines open almost
  every site handler; `"workspace not found"` occurs 117 times in the site package.
- **Input mapping.** ogen optionals into module inputs (`convert.StringPtr(req.Subject)`, the integration config
  union in `configInput`, `site/integrations.go:322`).
- **Error and response mapping.** A `switch` of `errors.Is` / `errors.As` on module errors, each case building
  the operation's own typed response (`siteapi.SiteTokensCreateUnprocessableEntity(problemWithErrors(…))`), then
  goverter mapping and attaching what the mapper cannot see (stats, lint warnings, the redacted config). The
  handler files hold 252 `errors.Is(err, …)` calls.
- **Rules and sequencing.** Real business logic in the three long handlers. `SiteAuthRegister`
  (`site/auth.go:57-116`) does equal work per address (ADR 0031): take an attempt, hash the password whatever the
  address, create or find the User, recover from a racing registration on `db.IsUniqueViolation`
  (`:79-85`), replace an unverified password, mint the token, queue exactly one mail. `SiteUserUpdateMe`
  (`site/me.go:63-123`) owns the blank-name rule, the password policy, the current-password proof, the hashing and
  the session re-issue; `accounts.UpdateProfile` receives a ready hash. The blank-name rule therefore lives in a
  handler here and in the module for tokens (`internal/apitokens/apitokens.go:56-59`), the kind of split rule 6
  forbids. The comment at `site/auth.go:35-42` states the reason value checks sit in handlers: ogen validates
  TypeSpec constraints in the decoder and would answer a generic 400 without the per-field map. That is a
  deliberate move of value constraints out of TypeSpec, in tension with rule 6.

The sample was chosen by hand and is small; the shares are **inference** for the whole package, the line counts
are **verified**.

### Direct storage access from handlers

8 call sites in the three packages, all single-row reads on the scope (rule above): `s.SendingDomain().Get` in
`site/sending_domains.go:104,145,180` and `external/sending_domains.go:78,136,163`, `ApiToken().Get` in
`external/auth.go:25` and `Contact().Get` in `external/contacts.go:71`. The `ent/<entity>` packages imported by
non-test handler files are used for enum values only (`contact.SourceAPI`, `membership.Role(…)`,
`signupsurveyresponse.Role(…)`); a search for predicate or ordering use (`Field…`, `Has…With`, `…EQ(`, `IDIn(`)
found none. Writes are excluded mechanically by `entrywrite`. **Verdict:** the bypass is real but limited to
reads that a module `Get` would only wrap; `sendingdomains` and `contacts` could expose them so that a future
read rule (archived rows, redaction) has one place.

### site against external

Both surfaces call the same modules, so a rule is stated once (rule 6 holds at the module level). The handler
code is duplicated instead, because TypeSpec generates two type sets (`siteapi.SiteSmtpConfigInput` and
`externalapi.SmtpConfigInput` are different Go types with the same fields), and easement forbids a surface from
importing the other (`easement.yml:61-78`). Unexported functions defined in both packages: `configInput`,
`conflictProblem`, `createContactAttributes`, `createSMTP`, `fieldProblem`, `inUseProblem`, `lintFindings`,
`optString`, `paragraphDoc`, `problem`, `smtpInput`, `updateContactAttributes`, `validationProblem`,
`webhookResource`, `webhookRuleProblem`. `site/integrations.go:322-340` and `external/integrations.go:280-298` are
the same `configInput` over the two type sets; the bodies of `SiteIntegrationsUpdate` (`:151-193`) and
`IntegrationsUpdate` (`:130-172`) differ in the auth preamble only. Where the copies drift, they drift on the
wire: `apitokens.ErrNameEmpty` answers 422 with a field map on site (`site/tokens.go:66-72`) and 400 on external
(`external/auth.go:69-71`); the error-chain helpers `validationProblem` have different signatures
(`site/contacts.go:177`, `external/handlers.go:140`). This is the analogue of Zitadel's per-version copies, not
of a rule copied into handlers.

MCP does not add a third copy: `/mcp` dispatches in-process through the external server
(`internal/server/server.go:94-100`).

### Seams

Modules are concrete `*Module` types (27 `type Module struct` declarations in `internal` and `ee`). The consumer
side declares narrow interfaces where a second implementation or a test double exists:
`broadcasts.Scheduler` (`internal/broadcasts/plan.go:35`, a river adapter and a synchronous one),
`abusewatch.Watch` (`internal/abusewatch/abusewatch.go:23`, implemented by `ee`, consumed by
`site/analytics.go:111`), `login.Store[P]` and `login.Throttle` (`internal/login/login.go:188,170`, two adapters,
site and `ee/staff`), `site.AuditLog` and `external.ExportStreams` (the EE read seams), `outbound.Senders`,
`messaging.EmailSender`, `events.Publisher` and `events.Enqueuer`, `sendingdomains.Verifier`,
`integrations.QuotaEnqueuer`. **Verdict:** interfaces at the seams that have two sides, concrete types
elsewhere; the same policy as Zitadel's concrete `Commands`, the opposite of Notifuse's interface per service and
repository.

### Transactions, validation, permissions, errors

- **Transactions.** The module opens them (`broadcasts.claim`, `internal/broadcasts/broadcasts.go:101-122`, claim
  and job in one transaction) or the generated audited wrapper does: a create of an audited entity runs
  `ent/scoped.go:841-858`, which calls `Scoped.audited` (`ent/scoped_audit.go:93-101`), which opens
  `events.Bus.WithinAuditTx` (`internal/events/scoped.go:36-38`). About 20 core packages open a transaction
  (`WithinScopedTx`, `WithinScopedEnqueueTx`, `WithinHandOverTx` or `Tx(ctx`), besides `internal/app`,
  `internal/server` and `ee/staff`; the one handler that does is `recordAuditExport` (`internal/api/site/audit.go:121-151`), which builds an Audit
  entry and publishes it itself.
- **Validation** is split three ways: TypeSpec constraints in the ogen decoder, a handler `checkFields` for the
  fields with a per-field 422 (`site/auth.go:35-51`), and module refusals (`validation.Refuse`,
  `internal/broadcasts/broadcasts.go:126-142`).
- **Permissions** are imperative per handler: 34 `accounts.Can*` role checks in site, each a named function in
  `internal/accounts/roles.go` (`CanManageTokens` `:38-40`), and 102 `auth.HasScope` checks in external. The
  TypeSpec operations say "Owners and admins only" in a doc comment (`typespec/site/resources/tokens.tsp:71`), not
  in a machine-readable form. The security handler receives the operation name and ignores it
  (`internal/api/auth/auth.go:128`). Insufficient scope answers 401 at 99 external sites and 403 at the 3 token
  operations (`external/auth.go:39-41,57-60,101-104`). sphericon's own `/service` surface already does it the
  other way: `x-required-scopes` on each TypeSpec operation (`typespec/service/resources/holds.tsp:81`), generated
  into `ServiceOperationScopes` by `cmd/scopegen`, enforced default-deny in the security handler
  (`ee/staff/service.go:95-98`).
- **Errors** are mapped twice: each handler returns the operation's typed problem, and anything returned as an
  error reaches `problemErrorHandler` (`internal/server/server.go:239-301`), which maps rate limits, a
  `problemError` (an error that carries its own status and code, `:303-309`; used by
  `secondFactorRequiredError`, `site/handlers.go:329-337`), `lifecycle.ErrInvalidTransition` (409),
  `ent.ErrNotInWorkspace` and `*ent.ValidationError` (422).

### What ogen, TypeSpec and goverter impose

TypeSpec declares each operation and the problems it may return; ogen turns that into one interface method per
operation with a response sum type (`siteapi.SiteTokensCreateRes`), so a declared 403 or 422 is a typed value the
handler must build. That is why the error mapping sits in every handler and why the auth preamble repeats: the
contract makes "workspace not found" an operation's own 404 variant. ogen also validates request constraints
before the handler runs, which is what pushed field checks into handlers (`site/auth.go:35-42`). goverter
generates entity-to-DTO mapping (`mapper.TokenToResource`, checked by `handmapper`), but nothing generates the
other direction, which is where the duplicated input helpers come from.

### Enterprise Edition

The core reaches EE only through interfaces in `Deps`: `Audit: edition.Audit` and `Export: edition.Export` for
external (`internal/app/app.go:1603`), `Holds`, `AbuseWatch` and `Audit` for site (`:1657`, `:1661`). easement
forbids any core import of `ee/` (`easement.yml:27-30`). The handler never asks which edition runs; it calls the
seam (`h.audit.Licensed()`).

## Per project

### listmonk (AGPL-3.0)

- **Shape.** Handlers are methods on the global `*App` in `package main` (`cmd/main.go:36`), registered in one
  `initHTTPHandlers` (`cmd/handlers.go:33`). `App` holds both the logic object (`core *core.Core`) and the raw
  `db *sqlx.DB` and `queries *models.Queries` (`cmd/main.go:40-42`), but no handler function calls those two (0
  sites; the 23 before excluding wiring are in `init.go` and `install.go`).
- **Logic.** `internal/core` (2,394 LOC, `Core` with 7 fields and 117 methods) is thin CRUD by its own package
  doc, and returns `*echo.HTTPError` so handlers can pass errors straight through (`internal/core/core.go:1-5`).
  Field validation lives in the handlers (`cmd/users.go:228-252`).
- **Tenancy, permissions.** No tenant. A permission is named at the route: `pm(a.CreateUser,
  auth.PermUsersManage)` (`cmd/handlers.go:249`), checked by `Auth.Perm` (`internal/auth/auth.go:404-435`).
- **DTO.** One type is request, row and response: `auth.User` is bound from the body (`cmd/users.go:220`) and
  returned (`:274`).
- **Verdict.** Route-level permission naming is the one idea worth noting; the rest (transport errors in the
  logic package, a single type for every layer, no tenant) is what sphericon avoids.

### Notifuse (BSL 1.1 from v40, AGPL for earlier versions, per `LICENSING.md`)

- **Shape.** One handler struct per resource with `RegisterRoutes(mux)` and RPC-style paths
  (`internal/http/workspace_handler.go:61-80`), 35 such structs, wired by hand (`internal/app/app.go:1574`,
  `:1684`). Handler functions are long (median 36, p90 61 lines) because each decodes, validates and maps errors
  itself.
- **Logic.** Services hold repositories and other services: `WorkspaceService` has 18 fields
  (`internal/service/workspace_service.go:22-41`); interfaces are declared in `domain` (`WorkspaceServiceInterface`
  26 methods, `internal/domain/workspace.go:1249`; `WorkspaceRepository` `:1134`) and mocked with gomock
  (`internal/domain/mocks`). The constructor comment at `workspace_service.go:43-47` records a variadic argument
  that left a dependency nil in production while every test was green.
- **Tenancy.** A string workspace id is passed down; each workspace has its own database, opened by
  `GetConnection` (`internal/repository/contact_postgres.go:572`, `pkg/database/connection_manager.go:27`).
- **Permissions** are checked inside each service method (`AuthenticateUserForWorkspace`, then a role or
  permission check; see the trace).
- **Errors.** A shared mapper, `writeServiceError` (`internal/http/utils.go:166`), whose comments say why it
  exists: a missing segment "is a class of error, not a property of one endpoint", and "four of the five" segment
  handlers had failed to see through the wrap. Handlers with their own error chain still bypass it; the API key
  handler explains at length why its licence refusal had fallen through to a 500
  (`internal/http/workspace_handler.go:623-640`).
- **Verdict.** The strongest evidence in this set for central error mapping, and a counter-example for
  interface-per-service (wide interfaces, mocks asserted in handler tests).

### rudder-server (Elastic License 2.0)

- **Shape.** One `Handle` with 55 fields and 68 methods (`gateway/handle.go:76`); handlers are closures built by
  composition, `gw.callType("track", gw.writeKeyAuth(gw.webHandler()))` (`gateway/handle_http.go:41`), mounted on
  chi (`gateway/handle_lifecycle.go:617-626`). The request handler does not write: it queues the payload to a
  worker and waits for its answer (`gateway/regular_handler.go:16-24`); a writer goroutine stores batches in one
  transaction (`gateway/handle_lifecycle.go:501-545`, `WithStoreSafeTx` at `:513`).
- **Tenancy, auth.** The write key maps to a source in memory, rebuilt from the backend config subscription
  (`gateway/handle_lifecycle.go:362-372`, `:406-420`); the source carries the workspace id
  (`gateway/handle_http_auth.go:237`).
- **Errors.** String message keys mapped to statuses through a table (`gateway/response/response.go:88`,
  `GetErrorStatusCode` `:145`).
- **Verdict.** A throughput design (batching writers behind a channel), not a request/module split; relevant only
  to `/collect` if ingestion volume ever needs batching. Nothing to borrow for handler organization.

### Zitadel (AGPL-3.0-only, with Apache-2.0 exceptions)

- **Shape.** 36 gRPC/connect `Server` types, one per service and version; the user v2 one has 9 fields, two of
  them the logic: `command *command.Commands` and `query *query.Queries`
  (`internal/api/grpc/user/v2/server.go:23-35`). Handlers are the shortest in the set (median 13 lines in the user v2 server): map the
  proto request, call one command or query, map the answer. `internal/AGENTS.md:14` writes the rule down: "Keep
  API/service adapters thin".
- **Logic.** CQRS: `internal/command` writes events (`Commands`, 59 fields, 547 exported methods), `internal/query`
  reads projections; both concrete, no interfaces.
- **Tenancy.** The instance comes from the request domain (`InstanceInterceptor`, `internal/api/api.go:219`);
  the eventstore adds it to every search (`internal/eventstore/search_query.go:114-119`) and every new aggregate
  (`internal/eventstore/aggregate.go:24`).
- **Permissions** are declared per rpc in the proto (`auth_option.permission`) and checked by
  `AuthorizationInterceptor` (`internal/api/api.go:223`); object-level checks follow in the command
  (`internal/command/permission_checks.go:52-54`).
- **Errors** are typed by kind (`zerrors`) and mapped once to gRPC codes (`internal/api/grpc/gerrors/zitadel_errors.go:76-100`,
  installed as `ErrorHandler` at `internal/api/api.go:221`).
- **Duplication.** Older API versions stay beside the new ones and call the same command, but the adapter's
  choices are copied: the PAT scope list is written in `internal/api/grpc/management/user.go:873` and again in
  `internal/api/grpc/user/v2/pat.go:24-29`, and the two versions authorize differently (v1 passes the caller's
  org as resource owner, v2 passes a permission check).
- **Verdict.** The model for thin adapters with declared permissions and central error kinds. Borrow the
  declaration and the error-kind mapping, not CQRS.

### Coder (AGPL-3.0, enterprise directory under its own licence)

- **Shape.** All routes in `coderd.New`; handlers are methods on `API` (`coderd/coderd.go:2229`, 64 fields
  embedding `*Options`, 88 fields). The longest handlers in the set (p90 129 lines, max 390), and 600 direct
  calls to `api.Database` in handler files: the handler is the logic layer for most resources, with a few
  extracted packages (`coderd/apikey`, `coderd/wsbuilder`).
- **Permissions and tenancy** live around storage: `dbauthz.New` wraps every `database.Store` method with an RBAC
  check (`coderd/database/dbauthz/dbauthz.go:120`; `InsertAPIKey` at `:6445-6456`), with the actor put into the
  context by the API key middleware (`coderd/httpmw/apikey.go:231`). A handler that forgets a check is still
  refused at the store.
- **DTO.** `codersdk` types and hand-written `db2sdk` converters (122 functions in
  `coderd/database/db2sdk/db2sdk.go`).
- **EE.** `enterprise/coderd` builds the AGPL `API` (`enterprise/coderd/coderd.go:228`) and swaps implementations
  held in `atomic.Pointer` fields when entitlements change, for example the auditor (`:1036`), which the token
  handler loads per request (`coderd/apikey.go:70`).
- **Verdict.** Authorization at the storage boundary is the idea worth knowing; sphericon's scoped client is the
  same move for tenancy. The fat handlers are the pattern to avoid.

### Hatchet (MIT)

- **Shape.** oapi-codegen strict server (`gen.NewStrictHandler`, `api/v1/server/run/run.go:311`); one package per
  area, one file per operation (`api/v1/server/handlers/api-tokens/{create,list,revoke}.go`); `apiService` embeds
  28 per-area structs (`run.go:67-96`), each holding only the server config (`api-tokens/service.go:7-9`).
- **Logic.** No domain layer: handlers call repositories through the config, `a.config.V1.<Repo>()` (161 sites),
  and the repository interfaces return sqlc rows (`pkg/repository/api_token.go:32-38`). Some logic sits in
  `pkg/auth` (token generation).
- **Tenancy and permissions** are declared in the contract: `x-resources: ["tenant"]` and an operation id
  (`api-contracts/openapi/paths/api-tokens/api_tokens.yaml:1-5`) drive a populator middleware that loads the
  tenant (`api/v1/server/run/run.go:320-335`) and an authorizer that checks membership and the role's
  permissions from `rbac.yaml` (`api/v1/server/authz/middleware.go:161-207`, `:209-226`; `authz/rbac.yaml:6-11`).
  The authorizer is default-allow by its own note (`middleware.go:223-224`) and answers 401 for a refused
  operation (`:222`).
- **DTO.** Generated types plus hand-written `transformers` (15 files in `api/v1/server/oas/transformers`).
- **Verdict.** Borrow the declared permission table keyed by operation id, made default-deny. The per-area
  struct split is cheap but buys little where handlers already hold no storage.

### cal.com (AGPL-3.0 core plus a commercial `ee` licence, at the pre-fork commit; MIT at HEAD)

#### Which tree was read

`calcom/cal.com` `main` is no longer the open-core cal.com. At HEAD it is **Cal.diy**, an MIT fork with the
Enterprise Edition removed: the README says "a fork of Cal.com with all enterprise/commercial code removed",
"No enterprise features — Teams, Organizations, Insights, Workflows, SSO/SAML … have been removed" and "no
'Open Core' split" (`HEAD:README.md:41-49`). The cut is one squashed commit, `ab21c7f8` "refactor: Cal.diy
(#28903)", 2026-04-15, whose message reads "Remove Enterprise Edition (EE) features, license checks, and AGPL
restrictions … Switch license from AGPL-3.0 to MIT … 3893 files changed, 20789 insertions(+), 411020
deletions(-)" (**verified**, `git log -1 ab21c7f8`).

The open-core questions are therefore answered at the **parent of that commit**, the last open-core tree, and
HEAD is used only as evidence of how cleanly `ee` could be cut out.

| Tree                     | Commit                                     | Date       | Licence files                                                                                                                                                                       |
| ------------------------ | ------------------------------------------ | ---------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| calcom/cal.com, pre-fork | `a17f28e9ab44d34a24840cff70082470d775b9b1` | 2026-04-14 | `LICENSE` (AGPLv3 except two dirs), `packages/features/ee/LICENSE`, `apps/api/v2/src/ee/LICENSE`, `apps/api/v1/LICENSE`, `apps/web/modules/ee/LICENSE` (Cal.com Commercial License) |
| calcom/cal.com, HEAD     | `91eefbf9f460bac7d69327fea4d9560295390e0a` | 2026-10-28 | `LICENSE` (MIT), `apps/api/v2/src/platform/LICENSE` (MIT)                                                                                                                           |

Paths below are at the pre-fork SHA unless prefixed `HEAD:`. The root `LICENSE:1-8` names exactly two
commercial directories, `packages/features/ee` and `apps/api/v2/src/ee`, everything else AGPLv3 (**verified**);
`apps/api/v1/LICENSE` and `apps/web/modules/ee/LICENSE` also carry the commercial text, and `apps/api/v1/README.md:10`
calls v1 the "Commercial Cal.com Public API" (**verified**). Nothing in either clone was built, installed or
run.

#### Layout and size

Non-test, non-blank LOC of `.ts`/`.tsx` (a script of ours that counts lines and runs nothing in the clones: it skips `*.test.*`, `*.spec.*`,
`*.e2e-spec.*`, `*.integration-test.*`, `__tests__`, `__mocks__`, `test(s)`, `playwright`, `.d.ts`, and files
whose name or first five lines say generated):

| Area                           | Role                                                         | Pre-fork LOC | HEAD LOC |
| ------------------------------ | ------------------------------------------------------------ | -----------: | -------: |
| `apps/web`                     | Next.js app (pages, App Router, `modules/` UI)               |      144,356 |   81,926 |
| `packages/trpc/server`         | tRPC: the web app's private API                              |       38,277 |   17,789 |
| `apps/api/v2/src`              | NestJS public API ("platform"), versioned controllers        |       54,482 |   35,852 |
| of it `apps/api/v2/src/ee`     | commercial v2 (bookings, calendars, event types, schedules)  |       16,661 |        – |
| `apps/api/v1`                  | older Next.js public API, commercial licence                 |       10,014 |  removed |
| `packages/features`            | domain slices: services, repositories, some UI               |      137,856 |   79,873 |
| of it `packages/features/ee`   | commercial slices (teams, organizations, workflows, sso, …)  |       34,095 |        – |
| of it `packages/features/pbac` | permission-based access control                              |        3,310 |  removed |
| `packages/lib`                 | cross-cutting utilities, `ErrorWithCode`                     |       14,363 |   14,058 |
| `packages/platform`            | `platform-libraries` (v2's facade onto features/trpc), atoms |       23,734 |   22,566 |
| `packages/app-store`           | integrations                                                 |       40,638 |   34,821 |
| `packages/prisma`              | schema, client, extensions                                   |        1,992 |        – |

Direction: the written rules say new logic goes to `packages/features/<domain>/{services,repositories}` and
handlers stay thin (`agents/rules/architecture-vertical-slices.md:12-45`, `api-thin-controllers.md:12-27`,
`data-repository-pattern.md:53-57`), and v2 may import only `@calcom/platform-*` and `@calcom/prisma`
(`biome.json:420-445`) (**verified**). tRPC remains the web app's API; v1 is the older public API, v2 the current
one (**inference**: v1 is a fifth of v2's size, v2 has dated controller versions such as
`ee/bookings/2024-04-15` and `2024-08-13`, and the fork dropped v1 entirely). The rule file is candid about the
gap: "we currently have references to Prisma scattered across hundreds of files"
(`agents/rules/data-repository-pattern.md:12`).

#### tRPC layer

- **Composition.** One root `appRouter = router({ viewer })` (`packages/trpc/server/routers/_app.ts:13-15`); the
  viewer router merges 47 area routers (`routers/viewer/_router.tsx:48-96`). Each area has a `_router.ts(x)` and
  one pair of files per procedure, `<op>.schema.ts` (Zod) and `<op>.handler.ts`, loaded lazily inside the
  procedure: `create: authedProcedure.input(ZCreateInputSchema).mutation(async ({ ctx, input }) => { const {
  createHandler } = await import("./create.handler"); … })` (`routers/viewer/apiKeys/_router.tsx:37-44`).
  344 handler files, 49 router files.
- **Procedure middleware.** `publicProcedure` = perf + `errorConversionMiddleware`
  (`procedures/publicProcedure.ts:5`); `authedProcedure` adds `isAuthed`, which loads the session user or throws
  `UNAUTHORIZED` (`procedures/authedProcedure.ts:27`, `middlewares/sessionMiddleware.ts:7-24`);
  `authedAdminProcedure` and `authedOrgAdminProcedure` check `user.role === "ADMIN"` and
  `user.organization.isOrgAdmin` (`authedProcedure.ts:30-31`, `sessionMiddleware.ts:26-41`). Permission
  factories exist: `createTeamPbacProcedure(permission)` reads `input.teamId`, `createOrgPbacProcedure` uses
  `ctx.user.organizationId` (`procedures/pbacProcedures.ts:19-47`, `:57-93`), and per-resource ones such as
  `eventOwnerProcedure` and `createEventPbacProcedure` (`routers/viewer/eventTypes/util.ts:21-92`, `:100`).
- **What a handler receives.** `{ ctx, input }`: `ctx` holds `prisma` (the global client), `insightsDb`,
  `session`, `user` (the Prisma `User` minus secrets, plus credentials and calendars), `locale`, `traceContext`,
  `req`/`res` (`createContext.ts:21-55`, `:66-79`); `input` is the Zod-parsed body. Handler option types narrow
  `ctx` with `Pick<…>` (`apiKeys/create.handler.ts:11-16`), which is the test seam.
- **Thickness.** Measured below: median 30 lines, p90 103, max 793 (`viewer/eventTypes/heavy/update.handler.ts:67`).
  147 of the 344 handler files call Prisma directly (390 call sites), against the repository rule. Others are
  thin adapters over a service, for example `removeMemberHandler`, which picks a service by feature flag and
  calls `checkRemovePermissions`, `validateRemoval`, `removeMembers` (`viewer/teams/removeMember.handler.ts:45-74`).

#### API v2 (NestJS)

- **Shape.** Modules per resource with controller, service, repository, `inputs/`, `outputs/`
  (`modules/api-keys/{api-keys.module.ts,controllers,services,inputs,outputs,api-keys-repository.ts}`;
  `api-keys.module.ts:7-12`). 80 controller classes, 352 endpoints.
- **Guards and decorators.** Authentication is one Passport strategy that accepts OAuth client credentials, an
  API key or an access token as Bearer, a third-party token, or a NextAuth cookie
  (`modules/auth/strategies/api-auth/api-auth.strategy.ts:57-120`). Tenancy and roles are declared per class and
  method: `@UseGuards(ApiAuthGuard, IsOrgGuard, RolesGuard, PlatformPlanGuard, IsAdminAPIEnabledGuard)` on
  `/v2/organizations/:orgId/teams`, then `@UseGuards(IsTeamInOrg) @Roles("ORG_ADMIN")` per method
  (`modules/organizations/teams/index/organizations-teams.controller.ts:49-58`, `:122-137`). `IsOrgGuard` checks
  the path's `orgId` is an organization and puts it on the request (`guards/organizations/is-org.guard.ts:20-41`);
  `IsTeamInOrg` checks `team.parentId === orgId` (`guards/teams/is-team-in-org.guard.ts:42-53`); `RolesGuard`
  compares the membership role in the org and/or team named by the path with the `@Roles` value
  (`guards/roles/roles.guard.ts:19-38`, `:53-170`). The roles and orders are in `lib/roles/constants.ts:3-13`.
- **Validation.** class-validator input classes (`modules/api-keys/inputs/refresh-api-key.input.ts:4-20`) under a
  global `ValidationPipe` with `whitelist: true, transform: true` (`bootstrap.ts:60-68`).
- **Output.** Hand-written output classes, filled with `plainToClass(Dto, row, { strategy: "excludeAll" })`
  (`organizations-teams.controller.ts:77`), wrapped as `{ status: SUCCESS_STATUS, data }`.
- **Thin controllers, services over repositories.** Controllers are short (median 13, p90 22, max 60), comparable
  to Zitadel's user v2 server (13 / 28 / 61), and contain **no** Prisma call; v2 services hold the rules and reach Prisma in only
  4 of 162 service files (8 calls); repositories hold 353 calls in 70 files. Repositories also repeat the tenant
  filter that the guard checked: `team.delete({ where: { id, isOrganization: false, parentId: organizationId } })`
  (`organizations-teams.repository.ts:41-49`, `:76-81`).
- **But v2 also reuses tRPC handlers as its domain layer.** `@calcom/platform-libraries` has 26 import or
  re-export lines naming 20 distinct `@calcom/trpc/...` module specifiers, for example `createHandler as
  createApiKeyHandler` (`packages/platform/libraries/index.ts:122`), `createEventType`/`updateEventType`
  (`libraries/event-types.ts:7-8`), `updateWorkflow` (`libraries/workflows.ts:1`), `confirmBookingHandler`
  (`index.ts:110`), and it re-exports `TRPCError` itself (`index.ts:64`) (**verified**). `createApiKeyHandler` and
  `confirmBookingHandler` each appear in one v2 file (name matches; `createApiKeyHandler` is used at
  `modules/api-keys/services/api-keys.service.ts:11`, `:45`).

#### Business logic: shared or duplicated

- **Shared, the intended way.** Booking creation is one class, `RegularBookingService` in
  `packages/features/bookings/lib/service/RegularBookingService.ts` (re-exported at `libraries/bookings.ts:17`). The
  web API route builds it from a DI container (`apps/web/pages/api/book/event.ts:4`, `:49-59`); v2 subclasses it and
  injects Nest providers for the same dependencies (`apps/api/v2/src/lib/services/regular-booking.service.ts:18-44`).
  PBAC is shared too: v2's guard and tRPC's procedures call the same `PermissionCheckService`
  (`modules/auth/guards/pbac/pbac.guard.ts:17`, `:109-115`; `procedures/pbacProcedures.ts:30-36`).
- **Shared, the costly way.** Where the rule sits in a tRPC handler, v2 calls the handler. The API-key trace below
  is the example: the team-role check, the secret and the insert live in the tRPC handler, and v2 inherits its
  `TRPCError` codes.
- **Duplicated.** API-key creation exists three times: the tRPC handler (`apiKeys/create.handler.ts:18-45`),
  `PrismaApiKeyRepository.createApiKey` in `ee` (`packages/features/ee/api-keys/repositories/PrismaApiKeyRepository.ts:59-87`,
  same prefix logic; `packages/features/calAIPhone/providers/retellAI/services/AgentService.ts` references the
  repository, that it calls `createApiKey` is **inference**), and v2's own default lifetime of 30 days on top
  (`modules/api-keys/services/api-keys.service.ts:40-44`). Lookup by hash exists twice
  (`PrismaApiKeyRepository.ts:14-32`, v2 `api-keys-repository.ts:9-15`). The roles hierarchy is written twice:
  v2's `RolesGuard` (`roles.guard.ts:53-170`, org admins pass team checks at `:136`) and PBAC's fallback
  ("use highest role between team and org", `packages/features/pbac/services/permission-check.service.ts:124-150`).

#### Tenancy

There is no tenant-scoped client. `prisma` is one global client extended with four query extensions, none of
them a tenant filter: locked users, teams pending payment, a booking idempotency key, and a guard that throws on
`deleteMany`/`updateMany` with an `undefined` where-value (`packages/prisma/index.ts:79-83`;
`extensions/disallow-undefined-delete-update-many.ts:3-45`; `extensions/exclude-pending-payment-teams.ts:3-40`)
(**verified**). The last one is the only guard against a forgotten filter, and only against the
`{ teamId: undefined }` shape, which Prisma would otherwise read as "no filter".

Scoping is therefore by convention:

- **tRPC**: from `ctx.user.id`, `ctx.user.organizationId` or an input `teamId` checked in the handler or a
  procedure. Some writes go through the user relation, which scopes them by construction:
  `prisma.user.update({ where: { id: ctx.user.id }, data: { apiKeys: { delete: { id } } } })`
  (`apiKeys/delete.handler.ts:22-33`), while the read just before it is unscoped,
  `prisma.apiKey.findUnique({ where: { id } })` (`:16-20`; it only decides a Zapier cleanup of the caller's own
  webhooks, so no leak follows, **inference**).
- **v2**: path parameters (`:orgId`, `:teamId`) checked by guards, then the same ids passed to repositories that
  filter again (above). A controller without `IsOrgGuard`/`IsTeamInOrg` gets no check: the guards are opt-in per
  class or method (**verified** by their use as decorators; no global tenant guard is registered among
  `APP_GUARD` providers, which hold only `CustomThrottlerGuard`, `app.module.ts:80-83`).

A forgotten `where` reaches every tenant: nothing in the client or the types stops
`prisma.team.findMany({})` (**inference** from the client construction above). The guard-plus-repository
repetition in v2 is defence in depth by habit, not by type.

#### Permissions

- **Two systems.** Legacy membership roles (`OWNER`/`ADMIN`/`MEMBER`) and PBAC: a registry of
  `Resource` × (`CrudAction` | `CustomAction`) strings such as `team.update`, `organization.passwordReset`,
  `booking.readTeamBookings` (`packages/features/pbac/domain/types/permission-registry.ts:1-73`), custom roles
  stored in `Role`/`RolePermission`, three default roles (`packages/features/pbac/README.md:7-48`). PBAC is
  switched on per team by the `pbac` feature flag; when it is off, `checkPermission` falls back to the
  `fallbackRoles` the caller names (`permission-check.service.ts:96-155`). There is no `ApiKey` resource.
- **Declarative or imperative.** Both. Declarative: v2 `@Roles(...)` and `@Pbac([...])` per method, read by guards
  through `Reflector` (`decorators/pbac/pbac.decorator.ts:5`, `roles.guard.ts:30`); tRPC procedure factories
  (`createTeamPbacProcedure("featureOptIn.read")`, `routers/viewer/featureOptIn/_router.ts:41`). Imperative: 62
  non-test tRPC router-tree files reference `PermissionCheckService`, with about 130 matches of
  `checkPermission(`/`getResourcePermissions(`/`hasPermission` (an upper bound: the last also matches variable
  names), against 10 call sites of the two generic factories (text search; resource-specific factories like
  `createEventPbacProcedure` not counted). The API-key check is a hand-written
  Prisma query (`apiKeys/_auth-middleware.ts:6-25`).
- **Default.** `PermissionCheckService` fails closed: an invalid permission string, an exception or no
  membership returns `false` (`permission-check.service.ts:108-112`, `:146-153`). Procedures are default-allow:
  `authedProcedure` only authenticates, so a handler without a check is open to every signed-in user. In v2,
  `RolesGuard` denies when applied without a matching `@Roles` (`roles.guard.ts:68`, `hasMinimumRole` throws on an
  unknown role, `:191-201`), but it is opt-in. `PbacGuard` **never refuses**: every branch returns `true` and only
  sets `request.pbacAuthorizedRequest`, which lets `RolesGuard` skip its check (`pbac.guard.ts:37-78`,
  `roles.guard.ts:22-25`); its `throwForbiddenError` (`pbac.guard.ts:158-176`) is not called. PBAC in v2 can only
  grant beyond the legacy role, never narrow it (**verified** from the two guards; **inference** that this is
  deliberate during the rollout).
- **Caching of decisions.** Both guards cache a positive answer in Redis for 300 s
  (`roles.guard.ts:59-66`, `:167-169`; `pbac.guard.ts:103-120`, `:139-144`), so a demoted member keeps access up to
  five minutes (**inference** from the TTL; no invalidation was found in the two guards).
- **Status drift.** Refusals answer 401 in role checks (`_auth-middleware.ts:24`, `sessionMiddleware.ts:29`, `:38`,
  `removeMember.handler.ts:58`) and 403 in PBAC procedures (`pbacProcedures.ts:39-42`) and v2 guards
  (`roles.guard.ts:50`): the same 401/403 split sphericon has.

#### Errors

- A domain error type, `ErrorWithCode` with an `ErrorCode` enum (`packages/lib/errors.ts:3-25`,
  `packages/lib/errorCodes.ts`), and one status table, `getHttpStatusCode` (`packages/lib/server/getServerErrorFromUnknown.ts:127-…`).
  The written rule: "Use `ErrorWithCode` for errors in non-tRPC files …; use `TRPCError` only in tRPC routers"
  (`AGENTS.md:10`).
- **tRPC** maps it centrally: `errorConversionMiddleware` (`middlewares/errorConversionMiddleware.ts:7-13`) turns
  an `ErrorWithCode` into a `TRPCError` through that table (`lib/toTRPCError.ts:62-73`).
- **v2** registers filters for Prisma, Zod, `HttpException`, `TRPCError` and calendar errors
  (`bootstrap.ts:72-76`); `TRPCExceptionFilter` maps tRPC codes to statuses (`filters/trpc-exception.filter.ts:37-…`,
  `:104-140`). No filter catches `ErrorWithCode` (`@Catch` appears only in those five and two OAuth filters);
  three v2 files test for it by hand (`oauth2-error.service.ts:20`, `atoms-oauth2.controller.ts:48`,
  `ee/bookings/2024-08-13/services/booking-attendees.service.ts:64`, `:165-168`). An `ErrorWithCode` that reaches
  v2 uncaught falls through to `SentryGlobalFilter` (`app.module.ts:66-67`) (**inference**: answered as 500,
  where tRPC would answer the mapped 4xx). The central table exists; only one of the two transports is wired to
  it.

#### Open-core seam

- **By directory and licence**, not by interface: `packages/features/ee` (package `@calcom/ee`) and
  `apps/api/v2/src/ee`, plus `apps/web/modules/ee` and v1. The stated principle is "all 'Singleplayer APIs' are
  open-source under AGPLv3. All 'Multiplayer APIs' are under a commercial license"
  (`apps/api/v2/src/ee/README.md:14`), yet v2's `modules/organizations`, `modules/teams` and
  `packages/features/pbac` sit outside the commercial directories (**verified** by listing).
- **Import direction both ways.** AGPL core imports `ee` directly: the tRPC API-key handler imports
  `generateUniqueAPIKey` from `@calcom/ee/api-keys/lib/apiKeys` (`apiKeys/create.handler.ts:3`); the list handler
  imports `PrismaApiKeyRepository` from `@calcom/features/ee/...` (`apiKeys/list.handler.ts:1`). No lint rule
  forbids it: the `noRestrictedImports` overrides cover `lib`, `app-store`, `features` (no tRPC, no web), `trpc`,
  `testing`, `atoms` and v2 (`biome.json:236-453`), none names `ee`.
- **Runtime licence**: `LicenseKeyService.create` returns a no-op service when no key is configured
  (`packages/features/ee/common/server/LicenseKeyService.ts:37-42`); the UI wraps features in `LicenseRequired`,
  which reads `session.hasValidLicense` and only warns in development (`apps/web/modules/ee/common/components/LicenseRequired.tsx:20-56`).
  Per-team features are feature flags (`pbac` above).
- **What the cut shows.** Removing `ee` took one 411k-line deletion, and core files that imported the deleted
  PBAC service got inline stubs that **allow everything**:
  `class PermissionCheckService { … async checkPermission(..._args) { return true; } async hasPermission(..._args) { return true; } }`
  (`HEAD:packages/trpc/server/routers/viewer/eventTypes/util.ts:14-20`, replacing the import at
  `util.ts:4-5` before the cut). The `ee` API-key helper moved into core as `api-keys-legacy`
  (`HEAD:packages/trpc/server/routers/viewer/apiKeys/create.handler.ts:3`) (**verified**). Imperative permission
  calls spread through handlers are what made the cut produce allow-all stubs (**inference**).

#### Testing seams

Vitest unit tests with a deep Prisma mock reset before each test (`packages/prisma/__mocks__/prisma.ts:1-11`,
`vitest-mock-extended`) and `prismock` (an in-memory Prisma) in `@calcom/testing` (`packages/testing/package.json:25`);
procedure tests mock the permission service by module path (`procedures/pbacProcedures.test.ts:15-21`). v2 has
`*.e2e-spec.ts` suites per controller (for example `organizations-teams.controller.e2e-spec.ts`, Jest,
`apps/api/v2/jest-e2e.ts`), the web app Playwright (`playwright.config.ts` at the root;
`HEAD:apps/web/playwright/fixtures/users.ts`). The narrow `ctx` `Pick<>` types are
what let a handler be called with a hand-built context. Whether the e2e suites use a real database was not
read (**unverified**).

#### Metrics

A script of ours over ast-grep 0.51 (read-only, TypeScript, the same rules as the Go program where they apply):

- **tRPC handler**: per non-test `*.handler.ts`, the function the file exports as default (or re-exports by
  name), else the exported top-level function whose parameter list mentions `ctx` or `input`, else the single
  exported function; length from the `export` line to the closing brace. Every one of the 344 files yields one
  (348 functions).
- **tRPC router closure**: an arrow function passed to `.query(`/`.mutation(` in a `_router` file; counted apart.
- **v2 endpoint**: a method in a non-test `*.controller.ts`/`*-controller.ts` class preceded by `@Get/@Post/@Put/
  @Patch/@Delete/@All`; length from the method name line to the closing brace (decorators excluded).
- **Storage call**: a call whose callee matches `(^|\.)prisma\.(\$\w+|\w+\.\w+)$` or `^tx\.\w+\.\w+$`
  (covers `prisma.x.op`, `ctx.prisma.x.op`, `this.dbRead.prisma.x.op`, `$transaction`, `$queryRaw`). A repository
  holding the client under another name (`this.prismaClient.…`) is missed, so the repository counts are lower
  bounds.

| Layer                    | Count | Median | p90 | Max | Storage calls (inside handler bodies)                  |
| ------------------------ | ----: | -----: | --: | --: | ------------------------------------------------------ |
| tRPC handlers (pre-fork) |   348 |     30 | 103 | 793 | 390 (354), in 147 of 344 files; +12 in `_router` files |
| tRPC router closures     |   403 |      5 |  15 | 103 | (62 closures over 12 lines hold logic inline)          |
| v2 endpoints (pre-fork)  |   352 |     13 |  22 |  60 | 0                                                      |
| of them in `src/ee`      |    78 |     13 |  25 |  58 | 0                                                      |
| tRPC handlers (HEAD)     |   168 |     26 |  80 | 719 | 181 (165), in 71 of 164 files                          |
| v2 endpoints (HEAD)      |   175 |     13 |  23 |  60 | –                                                      |

Logic layer: v2 services 8 Prisma calls in 4 of 162 files, v2 repositories 353 in 70 files; `packages/features`
311 in repository files and **582 in 130 non-repository files** (the rule in `data-repository-pattern.md:53-57`
is the target, not the state). LOC: `packages/trpc/server` 38.3k (handlers plus helper and service files in the
router tree, which hold another 33 storage calls) against features 137.9k + lib 14.4k; v2 54.5k as a whole
(controller-only LOC was not separated, **unverified**), so neither LOC figure is a pure handler-layer number.

#### One operation: create an API key

**tRPC, `viewer.apiKeys.create`** (the web settings page):

1. Route: `apiKeys: apiKeysRouter` (`routers/viewer/_router.tsx:67`), `create: authedProcedure.input(ZCreateInputSchema).mutation(…)`
   with a lazy import of the handler (`apiKeys/_router.tsx:37-44`).
2. Auth: NextAuth session through `isAuthed` (`sessionMiddleware.ts:7-24`).
3. Tenant: none by default (a key belongs to the user); an optional `input.teamId` makes it a team key.
4. Validation: Zod, `note`, `expiresAt`, `neverExpires`, `appId`, `teamId`, all optional (`apiKeys/create.schema.ts:11-17`).
5. Permission: only if `teamId` is given, a hand-written `prisma.team.findFirst` with `members.some({ userId, role in
   [OWNER, ADMIN] })`, else `UNAUTHORIZED` (`apiKeys/create.handler.ts:26`, `apiKeys/_auth-middleware.ts:6-25`); not PBAC.
6. Rule and secret: `generateUniqueAPIKey` from `ee`: 16 random bytes hex, SHA-256 stored (`create.handler.ts:3`,
   `:19`; `packages/features/ee/api-keys/lib/apiKeys.ts:4-10`). No "within caller" rule: the caller is a session.
7. Persistence: `prisma.apiKey.create` in the handler, spreading the remaining input into `data` (`create.handler.ts:28-38`).
8. Transaction: none; the role check and the insert are separate statements.
9. Errors: `TRPCError` from the check; the rest through `errorConversionMiddleware`.
10. Response: the prefixed key string, prefix from `API_KEY_PREFIX` (`create.handler.ts:40-44`). No audit entry
    was found in the handler (**unverified** beyond it).

**v2, `POST /v2/api-keys/refresh`** (v2 has no plain create; it also mints keys inside managed-organization
creation, `modules/organizations/organizations/services/managed-organizations.service.ts:89`):

1. Route: `@Controller({ path: "/v2/api-keys" }) @UseGuards(ApiAuthGuard)`, `@Post("/refresh")`
   (`modules/api-keys/controllers/api-keys.controller.ts:14-43`).
2. Auth: `ApiAuthStrategy` (`api-auth.strategy.ts:57-120`); the service then demands the API-key method
   (`api-keys.service.ts:20-31`).
3. Validation: `RefreshApiKeyInput` (`@IsInt @Min(1) apiKeyDaysValid`, `@IsBoolean apiKeyNeverExpires`,
   `inputs/refresh-api-key.input.ts:4-20`); the old key is looked up by hash (`api-keys.service.ts:63-68`).
4. Rule: both fields set is a 400, default lifetime 30 days (`api-keys.service.ts:34-44`).
5. Permission, secret, persistence: **delegated to the tRPC handler** via `createApiKeyHandler`
   (`api-keys.service.ts:45-57`; `packages/platform/libraries/index.ts:122`), so the team-role check is the tRPC one.
6. Transaction: none; the new key is inserted, then the old one deleted (`api-keys.service.ts:70-76`).
7. Errors: Nest exceptions plus `TRPCExceptionFilter` for the handler's `TRPCError` (`bootstrap.ts:75`).
8. Response: `{ status: SUCCESS_STATUS, data: { apiKey } }` (`api-keys.controller.ts:37-42`).

#### Verdict

- **Better than sphericon; on a par with the best of the six, ahead of them in one point.** (1) v2 controllers are
  as thin as Zitadel's user v2 server (median 13) and hold no storage call, because tenancy and role requirements
  are decorators read by guards, and the guards put the loaded org or team on the request (`IsTeamInOrg` sets
  `request.team`, read with `@GetTeam()`, `organizations-teams.controller.ts:115`); this matches Zitadel's
  declared per-rpc permissions and Hatchet's populator rather than beating them. (2) Ahead of all six: a
  permission model with a typed registry of `resource.action` strings, custom roles stored per tenant, and a
  per-tenant rollout flag with named fallback roles; sphericon's `accounts.Can*` is a set of functions. (3) One
  domain error type with one kind-to-status table (`getHttpStatusCode`), as Zitadel has with `zerrors`, and wider
  than Notifuse's `writeServiceError`; sphericon has neither. (4) The architecture rules are written down with
  their reasons in `agents/rules/*.md`, as Zitadel's `internal/AGENTS.md` is.
- **Worse.** (1) No tenancy by construction: one global Prisma client, every tenant filter by convention; the
  only safety net catches an `undefined` filter value on bulk writes. sphericon's scoped client is strictly
  stronger. (2) tRPC handlers are long (p90 103, max 793) and 147 of 344 call Prisma, against their own rule. (3)
  Authorization is opt-in everywhere: procedures and controllers are open to any authenticated caller unless a
  check is added, and v2's `PbacGuard` cannot refuse. (4) The second API calls the first API's handlers
  (20 `@calcom/trpc` module specifiers in `platform-libraries`), so tRPC's error type, its 401-for-forbidden and its missing
  transaction become v2 behaviour; this is sphericon's site/external duplication solved in the wrong direction. (5)
  Positive authorization decisions are cached for five minutes. (6) The `ee` boundary is a licence line, not a
  dependency rule; cutting it produced allow-all stubs in core.
- **Portable to Go/ogen/TypeSpec/ent, against the note's candidates.**
  - Candidates 1 and 2: cal.com's v2 is the working shape of "declare per operation, check in one place":
    `@Roles("ORG_ADMIN")` per method, path-id guards that load the tenant. Port it as a TypeSpec decorator
    (`x-required-scopes`/`x-required-role`) to a generated table, read in the security handler and in
    `scopedWithRoleFor`. Learn from its two faults: make the declaration mandatory (a generated check that every
    operation has one, default-deny when absent; cal.com's guards are opt-in per controller), and keep the
    declared check able to refuse (its `PbacGuard` only grants). Do not cache allow decisions.
  - Candidates 3 and 6: `ErrorWithCode` + `getHttpStatusCode` is candidate 3 as built, and its v2 gap is the
    warning: a central kind-to-status table must be installed in every surface's error handler (site, external,
    collect, `/service`), or the same error answers 4xx on one and 500 on the other.
  - Candidate 5: strongest support in the set. When rules stay in handlers, the next API reuses the handlers and
    imports their transport errors; cal.com's own booking path (`RegularBookingService` built by two DI wirings)
    is the shape sphericon's modules already have. Move `SiteAuthRegister` and `SiteUserUpdateMe` rules into
    modules before a second surface needs them.
  - Candidate 4: no evidence; cal.com writes both directions by hand (Zod and class-validator inputs,
    `plainToClass` outputs).
  - Not portable / not needed: the per-procedure `.schema.ts` + `.handler.ts` split with lazy imports is a
    Next.js cold-start measure; ogen already generates the per-operation types.
  - For the `ee` seam: cal.com's cut is evidence for sphericon's easement rule that core never imports `ee/`. A
    permission check that core calls must be a core interface with a deny-by-default implementation, so removing
    `ee` cannot turn into allow-all.

#### Unknown / not verified (cal.com)

- Both clones were read only; no suite was run. `ast-grep` counts are structural, not type-aware.
- `SentryGlobalFilter`'s answer to an uncaught `ErrorWithCode` (500 assumed) was not read in `@sentry/nestjs`.
- The count of declarative PBAC procedures covers the two generic factories only.
- Whether v1 is formally deprecated was not found in a document; "legacy" is inference.
- v2 controller-only LOC, audit logging for API keys, and whether v2 e2e tests use a real database were not
  checked.
- `agents/rules/architecture-feature-boundaries.md:35` says cross-feature imports of internals are blocked by
  lint; no such rule is in `biome.json:236-453` (**unverified** elsewhere, for example a custom lint plugin).

## One operation: create an API token

The same operation in each project, hop by hop. rudder-server has no create operation: a write key is a field
of a source in the backend config that the control plane serves (`gateway/handle_lifecycle.go:362-372`); a
search for a route or function that creates or rotates a write key (`create|generate|new|rotate…writekey`,
`createSource`, `WriteKey =` in non-test Go) found only test builders and the replay-source copy
(`backend-config/replay_types.go:27`). Its row shows the write-key authentication path instead.

### sphericon, `/site` (`POST /site/workspaces/{workspaceId}/tokens`)

1. Route: TypeSpec `typespec/site/resources/tokens.tsp:71-85`; ogen server mounted at
   `internal/server/server.go:65-74`.
2. Auth: session cookie, `SiteSecurityHandler.HandleApiKeyAuth` (`internal/api/auth/auth.go:296`).
3. Tenant: `scopedWithRoleFor` (`internal/api/site/handlers.go:291-297`) via `membershipFor` (`:304-313`, the
   Two-factor gate at `:309`) to `accounts.Scope` (`internal/accounts/accounts.go:66-76`), which builds the scope
   with the User as actor (`:81-87`).
4. Permission: `accounts.CanManageTokens(role)` (`internal/api/site/tokens.go:54-57`,
   `internal/accounts/roles.go:38-40`).
5. Validation: ogen decoder for types and the scope enum; the module for the name.
6. Business rule: `apitokens.Mint` (`internal/apitokens/apitokens.go:55-89`): trimmed name required (`:56-59`),
   scopes sorted, deduplicated and checked against the contract enum (`:60-65`, `:148-150`), prefix and secret
   generated (`:67-74`).
7. Persistence: `s.ApiToken().Create()…Save` (`apitokens.go:76-84`).
8. Transaction: the generated audited `Save` (`ent/scoped.go:841-858`) opens `WithinAuditTx`
   (`ent/scoped_audit.go:93-101`, `internal/events/scoped.go:36-38`) and records the Audit entry in it (`:853`).
9. Errors: `ErrNameEmpty` to 422 with a field map (`tokens.go:65-75`); the rest to `problemErrorHandler`.
10. Response: secret once plus `mapper.TokenToResource` (goverter) (`tokens.go:77-80`).

### sphericon, `/api` (`POST /api/auth/tokens`)

1. Route: `typespec/external/resources/auth.tsp:36`; `NewExternalAPI` (`internal/server/server.go:163-170`).
2. Auth and tenant: `HandleBearerAuth` (`internal/api/auth/auth.go:90-100`) looks the token up by prefix in a
   live Workspace, checks revocation, expiry and the secret hash, and builds the scope with the token as actor
   (`:128-167`).
3. Permission: `auth.HasScope(…, ApiTokenScopeTokensWrite)` (`internal/api/external/auth.go:57-60`, 403).
4. Business rule: `apitokens.MintWithin` (`apitokens.go:94-101`): every requested scope must be held by the
   minting token, else `ErrScopeEscalation`; then `Mint` as above.
5. Persistence and transaction: as on site.
6. Errors: escalation 403, name or scope 400 (`external/auth.go:65-74`).
7. Response: `mintedResponse` (`:130-135`).

### listmonk (`POST /api/users`, an API user)

1. Route: `cmd/handlers.go:249`, inside the `/api` group with `a.auth.Middleware` (`:127-139`).
2. Auth: `Auth.Middleware` reads the `Authorization` header or the session (`internal/auth/auth.go:354`).
3. Tenant: none.
4. Permission: `Auth.Perm` with `PermUsersManage`, Super Admin bypass (`internal/auth/auth.go:404-435`).
5. Validation: in the handler (`cmd/users.go:224-252`).
6. Business rule: in `core.CreateUser`: for an API user, a 48-character random token, a synthetic `@api`
   email, the token's SHA-256 stored (`internal/core/users.go:61-73`, `HashAPIToken`
   `internal/auth/auth.go:497-500`).
7. Persistence: one named query (`internal/core/users.go:76`); the SQL hashes login passwords with bcrypt itself
   (`queries/users.sql:1-13`).
8. Transaction: none (one statement).
9. Errors: `core` returns `*echo.HTTPError` (`internal/core/users.go:77-78`).
10. Response: `okResp{user}` after reloading the in-memory token cache (`cmd/users.go:270-274`).

### Notifuse (`POST /api/workspaces.createAPIKey`)

1. Route: `internal/http/workspace_handler.go:80`, registered at `internal/app/app.go:1684`.
2. Auth: `RequireAuth` (JWT) (`internal/http/middleware/auth.go:37`).
3. Validation: method, JSON decode, `CreateAPIKeyRequest.Validate` (`workspace_handler.go:601-615`,
   `internal/domain/workspace.go:1316-1332`).
4. Tenant: in the service, `AuthenticateUserForWorkspace` (`internal/service/workspace_service.go:1462`,
   `internal/service/auth_service.go:139`).
5. Permission: owner only (`workspace_service.go:1473`), then the permission map (`:1478`).
6. Business rules: a licence check for a narrowed key (`:1494-1495`), a guard against an address of a platform
   admin (`:1514`).
7. Persistence: a `users` row (`:1527`) then a `user_workspaces` row in the system database (`:1555`,
   `internal/repository/workspace_postgres.go:510-520`).
8. Transaction: none. A failed membership insert deletes the user row as compensation (`:1567`).
9. Token: a JWT for the API user (`:1578`).
10. Errors and response: the handler's own chain (`workspace_handler.go:620-657`) and an untyped map
    (`:661-665`).

### rudder-server (write key authentication, no create operation)

1. Route: `POST /v1/track` (`gateway/handle_lifecycle.go:626`) to `webTrackHandler` (`gateway/handle_http.go:41`).
2. Auth: `writeKeyAuth` reads HTTP basic auth (`gateway/handle_http_auth.go:32`) and looks the key up in memory
   (`:37`, `:222-229`).
3. Tenant: the source's workspace id copied into the request context (`:233-243`, `:55`).
4. Permission: the source must be enabled (`:50-52`).
5. Validation and job build: `webRequestHandler` reads the payload (`gateway/handle_http.go:117`) and queues it
   (`:122`, `gateway/regular_handler.go:16-24`); a worker builds the job (`gateway/handle.go:314`).
6. Persistence and transaction: the writer goroutine (`gateway/handle_lifecycle.go:501-545`, `:513`).
7. Errors and response: message keys to statuses (`gateway/response/response.go:88-150`;
   `gateway/handle_http.go:106`, `:134`).

### Zitadel (`AddPersonalAccessToken`, user service v2)

1. Route: registered at `cmd/start/start.go:547`; rpc in `proto/zitadel/user/v2/user_service.proto:1643`.
2. Tenant: `InstanceInterceptor` (`internal/api/api.go:219`).
3. Permission, coarse: `auth_option.permission: "authenticated"` (`user_service.proto:1650-1651`), checked by
   `AuthorizationInterceptor` (`internal/api/api.go:223`).
4. Handler: maps the request, sets the fixed scopes and the machine-user restriction, passes a permission check
   (`internal/api/grpc/user/v2/pat.go:17-31`).
5. Validation and rules: `AddPersonalAccessToken` (`internal/command/user_personal_access_token.go:80-101`):
   id generation (`:81-86`), expiration check (`:62`), the user must exist and be a machine user (`:66-78`).
6. Permission, fine: `user.write` on the user, inside the write model load
   (`user_personal_access_token.go:175-189`, `:186`; `internal/command/permission_checks.go:52-54`).
7. Secret: the token is the encrypted `tokenID:userID` (`:116`).
8. Persistence and transaction: one `PersonalAccessTokenAdded` event (`:121-128`), pushed by `eventstore.Push`
   (`:92`), "in a single transaction" (`internal/eventstore/eventstore.go:94-101`).
9. Errors: `zerrors` kinds to codes (`internal/api/grpc/gerrors/zitadel_errors.go:76-100`).
10. Response: id, token and date (`pat.go:36-40`). The v1 management handler does the same with its own scope
    list (`internal/api/grpc/management/user.go:872-884`; permission `user.write` in
    `proto/zitadel/management.proto:2070`).

### Coder (`POST /api/v2/users/{user}/keys/tokens`)

1. Route: `coderd/coderd.go:1776`, under `/users` (`:1689`) with `apiKeyMiddleware` (`:1724`) and
   `ExtractUserParam` (`:1748`).
2. Auth and actor: the API key middleware puts the RBAC actor in the context (`coderd/httpmw/apikey.go:231`).
3. Tenant: none for keys (they belong to a user).
4. Handler: audit request (`coderd/apikey.go:70-79`), decode (`:82`), system users refused (`:88-92`), scope
   names checked (`:97-115`), allow list normalized (`:131-160`), lifetime checked (`:162-173`).
5. Business rule: bounded by the caller's key, `InheritWithinCaller` (`coderd/apikey.go:588-594`,
   `coderd/apikey/apikey.go:179-206`, `ErrExceedsCaller` `:169`); secret generated by `apikey.Generate`
   (`coderd/apikey.go:597`).
6. Permission and persistence: `api.Database.InsertAPIKey` (`coderd/apikey.go:602`), which is `dbauthz`'s RBAC
   check on `ResourceApiKey.WithOwner(userID)` (`coderd/database/dbauthz/dbauthz.go:6445-6456`).
7. Transaction: none (one insert).
8. Errors: in the handler (`coderd/apikey.go:176-195`: exceeds caller, a duplicate name to 409, else 500).
9. Response: 201 with the key (`:198`); the audit entry is committed by the deferred `commitAudit` (`:79`).

### Hatchet (`POST /api/v1/tenants/{tenant}/api-tokens`)

1. Route: `api-contracts/openapi/openapi.yaml:172`, operation `api-token:create` with `x-resources: ["tenant"]`
   (`api-contracts/openapi/paths/api-tokens/api_tokens.yaml:1-5`).
2. Tenant: the populator loads the tenant named by the path (`api/v1/server/run/run.go:320-335`).
3. Permission: cookie auth checks membership and the role (`api/v1/server/authz/middleware.go:76-91`,
   `:161-207`, `:209-226`; `ADMIN` holds `ApiTokenCreate` in `authz/rbac.yaml:6-11`); a bearer token may not
   manage tokens at all (`middleware.go:93-98`, `:122-124`).
4. Handler: reads the tenant from the context (`api/v1/server/handlers/api-tokens/create.go:16`), validates the
   body (`:21-25`) and the duration (`:29-37`).
5. Business rule and secret: `GenerateTenantToken` signs a JWT (`create.go:41`, `pkg/auth/token/token.go:88-107`).
6. Persistence: `tokenRepo.CreateAPIToken` (`token.go:95-101`, `pkg/repository/api_token.go:65-90`, the tenant's
   pool at `:85-88`).
7. Transaction: none (one insert).
8. Errors: typed 400s (`create.go:24`, `:33`); other errors returned untyped.
9. Response: the token once (`create.go:61-63`), audit keys set on the echo context (`:47-48`).

### Side by side

| Hop                  | sphericon                            | listmonk                 | Notifuse                       | Zitadel                  | Coder                       | Hatchet                       | cal.com                                                          |
| -------------------- | ------------------------------------ | ------------------------ | ------------------------------ | ------------------------ | --------------------------- | ----------------------------- | ---------------------------------------------------------------- |
| Route declared in    | TypeSpec                             | Go route table           | Go `RegisterRoutes`            | proto                    | Go chi tree                 | OpenAPI                       | tRPC router object; v2 Nest decorators                           |
| Tenant resolved by   | security handler or `scopedFor`      | (none)                   | service method                 | interceptor + eventstore | (user-owned)                | populator middleware          | (user-owned); optional `teamId` input checked in the handler     |
| Permission           | handler (`Can…`, `HasScope`)         | route wrapper            | service method                 | proto + command          | storage wrapper (`dbauthz`) | middleware + `rbac.yaml`      | hand-written Prisma role query in the tRPC handler, reused by v2 |
| Rule location        | module (`apitokens`)                 | `core`                   | service                        | command                  | handler + `apikey` package  | `pkg/auth`                    | tRPC handler and `ee` helper; v2 service adds lifetime defaults  |
| Minted within caller | yes (`MintWithin`, external only)    | no                       | no                             | n/a                      | yes (`InheritWithinCaller`) | bearer tokens may not mint    | no                                                               |
| Transaction          | generated audited wrapper            | single statement         | none, compensating delete      | event push               | single insert               | single insert                 | none (check, insert; v2 insert then delete)                      |
| Audit                | generated wrapper, same transaction  | none found               | service sets audit metadata    | the event is the record  | handler `audit.InitRequest` | context keys for a middleware | none found                                                       |
| Error mapping        | per-handler typed + central fallback | `echo.HTTPError` in core | handler chain or shared mapper | central by error kind    | per handler                 | typed 400 + untyped           | `TRPCError` in handler; v2 maps it with a global filter          |

## Comparison matrix

| Project   | Handler shape                                                                                    | Logic location                                                                             | Interfaces                                                                   | Tenancy enforcement                                                                                   | Permission declaration                                                                   | Error mapping                                              | DTO mapping                                                    | EE seam                                                                         | Test seam                                                           |
| --------- | ------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- | ---------------------------------------------------------- | -------------------------------------------------------------- | ------------------------------------------------------------------------------- | ------------------------------------------------------------------- |
| sphericon | one generated interface per surface, wide facade                                                 | domain modules, `*ent.Scoped` in                                                           | at seams with two sides                                                      | by type (generated scoped client)                                                                     | imperative in handlers (site, external); declared for `/service`                         | per-handler typed + central fallback                       | goverter (out), hand (in)                                      | interfaces in `Deps`, import ban (easement)                                     | ogen clients in memory, real Postgres, rollback                     |
| listmonk  | methods on global `App`                                                                          | thin `core` CRUD                                                                           | few (`manager.Store`, `media.Store`, `manager.Messenger`)                    | none                                                                                                  | named per route                                                                          | `echo.HTTPError` from core                                 | one type for all layers                                        | none                                                                            | almost none in Go                                                   |
| Notifuse  | struct per resource                                                                              | services over repositories                                                                 | per service and repository, mocked                                           | string id, database per workspace                                                                     | in each service method                                                                   | shared mapper plus handler chains                          | domain types, untyped maps                                     | licence checks inside services                                                  | gomock of domain interfaces                                         |
| rudder    | closures on one `Handle`                                                                         | async pipeline after `jobsdb`                                                              | `jobsdb.JobsDB` and friends                                                  | workspace id on the job                                                                               | source enabled                                                                           | message key table                                          | raw JSON                                                       | `enterprise/` packages wired in `app`                                           | ginkgo + gomock                                                     |
| Zitadel   | thin `Server` per service and version                                                            | CQRS commands and queries                                                                  | none on logic                                                                | instance from context, added by eventstore                                                            | proto per rpc + command checks                                                           | central by error kind                                      | proto + converters                                             | none (single edition)                                                           | mocked eventstore expectations; integration dirs                    |
| Coder     | fat methods on `API`                                                                             | mostly in handlers                                                                         | `database.Store`                                                             | RBAC around the store                                                                                 | RBAC policy at the store                                                                 | per handler                                                | `codersdk` + `db2sdk` by hand                                  | enterprise wraps `API`, swaps atomic pointers                                   | full server (`coderdtest.New`) + `dbmock`                           |
| Hatchet   | file per operation, per-area structs                                                             | repositories (no domain layer)                                                             | wide repository interfaces                                                   | populator from `x-resources`                                                                          | `rbac.yaml` by operation id                                                              | typed 400 + untyped                                        | generated + transformers                                       | config hooks (inference)                                                        | few handler tests                                                   |
| cal.com   | tRPC: file per procedure (`.schema` + `.handler`), lazy-loaded; v2: Nest controller per resource | `packages/features` services and repositories, much still in tRPC handlers, which v2 calls | repository interfaces in some slices, DI containers; mostly concrete classes | convention: ids in `where`; v2 guards on path ids plus a repeated repository filter; no scoped client | v2 `@Roles`/`@Pbac` and guards (opt-in); tRPC mostly imperative `PermissionCheckService` | `ErrorWithCode` and one status table, wired into tRPC only | Zod in tRPC, class-validator and `plainToClass` in v2, by hand | `ee` directories by licence; core imports `ee`; licence key service and UI gate | Vitest with a deep Prisma mock or prismock; v2 Jest e2e; Playwright |

## Verdicts

### Where sphericon is ahead

- **Tenancy by construction.** No other project makes a foreign tenant's row unreachable by the type of the
  client. Notifuse, Hatchet and rudder-server pass a tenant id down and rely on every query using it, Coder
  checks RBAC around the store at runtime, Zitadel adds the instance from the context inside the eventstore,
  listmonk has no tenant.
- **Layering checked by tools.** `entrywrite` (handlers cannot write through ent), easement (no surface imports
  another, the core never imports `ee/`) and `handmapper` make rule 5 and rule 1 checkable. Zitadel states the
  same rule in `internal/AGENTS.md`; nobody else enforces it, and Coder's and Hatchet's storage counts show where
  convention ends up.
- **One rule, many entry points.** `apitokens` serves site, external and bootstrap (MCP hides every token
  operation, `typespec/external/mcp.tsp:11-15`), and a module behind an operation that MCP does expose serves it
  through the same external handler; Notifuse and Coder keep comparable rules in handlers or services tied to one transport.
- **Transactions and audit in one place.** The audited scoped wrapper records the entry in the write's own
  transaction; Coder commits audit from a deferred handler call, Notifuse attaches metadata in the service, Hatchet
  sets context keys.

### Where sphericon shares the others' weaknesses

- **Imperative permission checks with drift** (Coder handlers, Notifuse services): 136 hand-written checks and the
  401/403 split.
- **Per-handler error mapping** (Coder, Notifuse): about half of a routine handler, and the same domain error
  answered differently by the two surfaces.
- **Copies across contracts** (Zitadel versions): 15 helper pairs and twin handler bodies.
- **Some rules in handlers** (Coder, listmonk): registration and profile update.
- **A wide constructor** (Coder `API`, rudder `Handle`): mitigated by modules, but every operation sees every
  module.
- **A domain package that depends on generated contract types** (milder than listmonk's `echo.HTTPError` in
  `core` or Hatchet's `pkg` importing `api`, `pkg/config/server/server.go:13`): `apitokens` reads the scope
  vocabulary from `gen/external` (`internal/apitokens/apitokens.go:22`, `:148-150`).

### Candidate improvements

Each is a candidate for a later decision, with its evidence; none is decided here. Candidates 1, 2 and 6 were
later decided in [ADR 0061](../adr/0061-access-to-an-operation-is-declared-in-the-contract.md), and candidate 3
became review rule 15.

1. **Declare the external API's token scopes per operation in TypeSpec and enforce them in
   `ExternalSecurityHandler`.** Extend the `x-required-scopes` extension that `/service` already uses, generate the
   operation-to-scopes map as `cmd/scopegen` does, and look it up in `HandleBearerAuth`, which already receives the
   operation (`internal/api/auth/auth.go:128`), default-deny as in `ee/staff/service.go:95-98`. Evidence: Hatchet's
   operation-keyed `rbac.yaml` (whose authorizer is default-allow and says it should not be), Zitadel's per-rpc
   `auth_option`. Effect: 102 checks leave the handlers, one status for insufficient scope, MCP inherits it, and
   rule 6 gains the scopes of the external surface beside those of `/service`.
2. **Declare the site role requirement per operation as well.** The site security handler cannot apply it, because
   the Workspace comes from the path, so the declaration would be read where the Membership is loaded
   (`scopedWithRoleFor`), naming the `accounts.Can…` decision instead of calling it in each handler. Evidence:
   Hatchet checks the role in middleware after loading the tenant from `x-resources`. Lower confidence than 1:
   ogen does not pass the operation into handler methods, so this needs a generated table keyed by handler.
3. **Map recurring refusals centrally.** Let a Workspace that is not found, and the module refusals that already
   carry a message (`validation.Error`, `inuse.Error`), render through the existing `problemError` seam
   (`internal/server/server.go:303-309`) instead of 117 literal 404s and per-handler switches. Evidence: Notifuse's
   `writeServiceError` comments on the wrap that four of five handlers missed; Zitadel maps error kinds once. To
   check first: that the generated clients decode a problem from the error path the same as the declared variant,
   and that each operation's declared statuses stay true (**unverified**).
4. **Generate the input direction.** goverter (or a small generator) from `gen/*` inputs to module inputs would
   remove `configInput`, `createContactAttributes`, `updateContactAttributes`, `smtpInput` and `mintInput` copies
   (rule 2: three or more places). None of the six projects does this; Coder's hand-written `db2sdk` is the cost
   of not generating even the output direction.
5. **Move the registration and profile rules into modules.** `accounts` (or a `registration` module beside
   `login`) would own equal work per address, race recovery and the password change; the handlers keep the cookie
   and the response. Evidence: sphericon's own `login` module for the same kind of flow, Zitadel's command layer.
6. **Make one domain error answer the same on both surfaces.** `ErrNameEmpty` (422 against 400) and the
   insufficient scope status are the two cases found; candidate 3 removes the cause.
7. **Smaller items.** Drop `externalapi.UnimplementedHandler` from the external struct for the compile-time
   completeness site has; move `recordAuditExport` out of the handler so no handler opens a transaction; give
   `sendingdomains` and `contacts` the `Get` the handlers now run on the scope.
8. **Not recommended now: splitting `Handlers` per area as Hatchet does.** It narrows constructors but adds wiring,
   and the problems above sit in the method bodies, not in the struct's width.

## Unknown / not verified

- Nothing was executed; no handler was run, no test suite either.
- The classification table is a hand count over eleven handlers chosen by hand; shares for the whole package are
  inference.
- Whether an ogen client treats a problem rendered by the error handler exactly like a declared response variant
  (candidate 3) was not checked.
- Hatchet's EE mechanism (config hooks, `AllowedOperations` "for extensions") is inference from the authorizer;
  no enterprise code was read. rudder-server's `enterprise/` packages were located (`app/app.go:17-21`), not traced.
- Coder's `enterprise/coderd` handlers and Zitadel's `backend/v3` tree were not measured.
- Licences were read from the repository headers only (`LICENSE`, `LICENSING.md`).

This note reports findings and candidates only; it changes no ADR.
