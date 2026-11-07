# Go test harness

`internal/testhelper.Setup(t)` is the standard fixture. Each test process gets a database of
its own, cloned from a template with the migrated schema only (built once per change of the
migrations); the committed YAML fixtures from `fixtures/` are loaded into it once per test
process, so their relative dates count from now. Then each test runs inside
a transaction that rolls back on cleanup (`dbcleaner`'s `Transaction` strategy, go-txdb underneath:
full isolation, DB stays at fixture state). Tests drive the real server in-memory through **actors** (typed ogen clients, see
below) — no sockets. See `internal/api/site/contacts_test.go`.

`Setup` builds the Instance from the production container (`app.Container`), replacing only the
seams it names (clock, database, License, senders, DNS, …) with `do.Override`. A module
or a surface gets no wiring of its own in the harness: add its provider to `internal/app/container.go`
and read it from the container in `Setup`. A difference a test needs (no System sender, a volume
floor) is configuration, set on the test's config.

Code under test queues jobs through the real river client, and they wait in the test's transaction.
`env.PerformEnqueuedJobs(t, only...)` works them with their real workers through river's test
executor, ActiveJob's `perform_enqueued_jobs`; a worker it does not know is added to the list in
`Setup`. `env.SystemMail.Messages()` works the queued Notices first (their event consumer and their
sends, ADR 0105), so a test reads them as sent. A test that runs a real river runtime on its
database passes that runtime a sender of its own. The harness registers a consumer of its own that
wants every live event, so `env.QueuedEnvelopes` shows every event a test published, one no
consumer of the application wants yet included.

**Ride the transaction unless the test cannot.** A transaction costs the same however much the
test writes, so `Setup` is the default, for a test of a hook, a refusal, a reader of the database,
a worker called directly. A test that needs a **real commit** (code that opens its own connection,
concurrent workers, a lock one transaction holds against another, a statement
that fails and must not abort the rest, a whole `app.App`, a river runtime) cannot ride one. It takes
a database of its own instead: `testhelper.RealDB(t)` (a `*sql.DB`), `RealDBURL(t)` (a URL, e.g.
`t.Setenv("SPHERICON_DATABASE_URL", ...)` for an app) or `EmptyDBURL(t)` (no schema, to migrate a fresh
instance). `RealDB` is a clone of a second database the process leases and loads with the fixtures
at the same moment as `Setup`'s, so both see the same rows with the same relative dates; the clone
is dropped when the test ends, so no test cleans up by hand or resets state at its start. A test
whose only reason is `rivertest`, which hands a worker a `*sql.Tx`, does not need one: the txdb
connection serves it as long as the worker does not touch the database through another handle while
the `tx` is open (the pool has one connection). The test role is a superuser (the fixture loader
disables foreign-key triggers), as on the shared test database. A module that seals secrets takes
the server's cipher, `env.Cipher`; a test without `Setup` takes `testhelper.Cipher(t)`, the same key
the fixtures are sealed with, and never loads the configuration for it.

**A document is saved through a fake export.** A runtime of the editor's code takes seconds to
start (more on a busy machine), so `testhelper.Renderer()`, the renderer of `Setup` and of a module a
test builds itself (`emailtemplates.New(testhelper.Renderer())`), runs our own checks and fakes the
editor's schema check and export: the body is the document's text as paragraphs. A test of what the
export makes of a document takes `testhelper.RealRenderer()`, and asserts the brand or the markup
only there. The nightly run sets `SPHERICON_TEST_EMAIL_RENDERER=goja`, which makes `Renderer` the real one.

**Build test scenarios on the committed fixtures (review rules 10 and 11).** Reference fixture rows through the catalog constants below
(`fixtures.BroadcastDraftID`, `fixtures.ContactAliceID`, `fixtures.SegmentActiveID`, …),
never bare id literals (a deliberately missing id such as `999999` is fine). Query counts
that vary with the dataset dynamically instead of hardcoding them. If a
scenario isn't covered, **add a fixture row** rather than a `client.X.Create()` in the
test. Anchor rows flagged "DO NOT change" in the YAML are referenced across the suite —
leave them. txdb rolls back each test, so mutating a fixture row inside a test is fine, and
it is how a test puts a row into the state it needs at a chosen moment: the fixture Staff holds
and the pending Outbound message are lapsed and stale, and `env.ExtendHold`, `env.ClaimOutbound`,
`env.ClaimRecipient`, `env.PlaceRecipient` (parked recipients), `env.SuppressDestination` and
`env.UnsubscribeFromBroadcasts` (the lifted Suppression and a per-Automation opt-out) move them.
A test that sends through Telegram or Twilio uses the fixture Integrations: `env.AcmeTelegram` and
`env.AcmeTwilio` add their bot and account to the fakes, and `env.AcmeSecondTelegram` a second bot
of Acme for a test that needs two. A test of an SMS or Telegram Broadcast sends one of Lacuna's
drafts through `env.LacunaTwilio` or `env.LacunaTelegram`: a Broadcast reaches every eligible
Contact of its Workspace, and Lacuna's Contacts are no other test's. `ConnectTwilio` connects an
account through the API, for a test of connecting or one that needs another account or an API key.
The `testwrites` analyzer reads the harness as it reads tests: a helper that creates a row names
why no fixture can hold it, with the same `// test writes:` comment.

**`env.DB` refuses a feed change outside a transaction, as production's plain client does.** It
carries the Guards of `internal/domain/identity/workspacefeed`, `internal/domain/identity/usertombstones` and `internal/domain/sending/trackinghostchanges`
(`entclient.New`), and only the bus's transaction client runs their Hooks, so a module that
writes a Workspace's deletion, an owner change, a User, a Consent record, the mailings mark or a
Tracking host outside `events.Bus.WithinTx` fails its tests. A test's own write of such a row (setup
that marks a Workspace deleted or changes a User's address, or the write a test of a feed's Hook
makes) goes through `env.FeedDB`, the client on the same transaction that runs the Hooks. A module
under test only ever gets `env.DB` or a scope built from it, never `env.FeedDB`.

## Domain contexts

A domain module lives in `internal/domain/<context>/<module>` (ADR 0102; the order of the contexts
and their easement components are "Domain contexts" in `docs/architecture.md`): identity,
contacts, content, sending, orchestration, insights and instance
(`internal/domain/identity/{workspaces,memberships,users,...}`,
`internal/domain/contacts/{contacts,segments,tags,...}`, `internal/domain/content/{emailtemplates,...}`,
`internal/domain/sending/{outboundmessages,integrations,...}`,
`internal/domain/orchestration/{broadcasts,automations,rules,...}`,
`internal/domain/insights/{setupchecklist,auditlog,...}`,
`internal/domain/instance/{instancestates,upgrade,...}`). Nothing of the core is left flat under
`internal/`: what is there besides the tiers is a driver, the wiring, a tool of `cmd/` or test
support (`fixtures`, `dnstest`, `telemetryschema`). A module imports only modules of its own context and
of the contexts below; its tests are not checked, so a test may build what it needs from the harness
as before. A module moves with its tests, in the same `_test` package and with the same assertions.
Only paths change: a relative path a test reads (`signinprovider`'s `../../../../openapi`) gets the new
depth, and the list of `RecordAudit` call sites in `internal/api/site/audit_explicit_test.go` names
the moved files. The move recipe is at the end of that section of `docs/architecture.md`.

## Use cases

A use case (`internal/usecase/<name>`, ADR 0102; the recipe is "Use cases" in
`docs/architecture.md`) owns the tests of its operation, in its own `_test` package through its
exports: when a check moves out of a domain module into a use case, its test moves with it and keeps
its assertions (`internal/usecase/tagdeletion` holds the Tag deletion's race with the writes the
Removal check reads under the Workspace lock, `internal/usecase/customfieldretyping` a Custom field
edit's, `internal/usecase/integrationdeletion` the Integration's and
`internal/usecase/sendingdomaindeletion` the Sending domain's, while what the Removal check finds is
tested in `internal/domain/orchestration/removalcheck` against `env.RemovalCheck`,
`internal/usecase/sendingdomainverification` the Sending domain checks, `internal/usecase/stepexecution` the
steps of an Enrollment, `internal/usecase/secretreencryption` the re-encryption of every stored
secret, `internal/usecase/unsubscriberecording` the refusals of an opt-out the API records,
`internal/usecase/contactintake` an import's rows and `internal/usecase/visitoridentification` what Identify
binds and stitches). The use case is built from the container like a
module, so the API tests reach it through the actors; a direct test builds it with
`New` over `env.Bus`, or over the `Bus` of a `testhelper.NewWorkspaceRace` when it races the
Workspace lock, whose `Queue` runs its writers queued behind the lock. The container's Step
execution is `env.StepExecution`, for a test of how the Instance builds it (its clock); a test that
drives an Automation over senders of its own still builds one with `New`. A use
case of the whole Instance (the Release check, the Telemetry report) is built over the Instance state
module instead (`instancestates.New(env.DB, …)`); the server's own are `env.ReleaseCheck` and
`env.TelemetryReport`. The re-encryption is built over each owner's `SecretSweeps` on `env.DB`
(`integrations.SecretSweeps(env.DB)`, …) with the cipher under test. A use case of rows no Workspace
owns holds the raw client: the Second factor's is `env.SecondFactor`
(`internal/usecase/secondfactorcheck`), the Workspace creation's `env.WorkspaceCreation`
(`internal/usecase/workspacecreation`; a test of its id retry builds it with `New` and an id
generator that collides), the Sign-up's `env.SignUp` (`internal/usecase/signup`, which also makes a User
Verified with its first Workspace and an Owner from a shell), the Workspace deletion cycle
(`internal/usecase/workspacedeletion`) is built with `New(env.DB, env.Bus, testhelper.EditionTables(), clock)`
(the removals of the Enterprise Edition's tables the wiring hands in), and the exchange of an OAuth
grant (`internal/usecase/connectionissuance`) is driven through the server's `/oauth` routes, which
its driver `internal/api/oauth` serves and tests.
The Sign-up (`signup.UseCase.SignUp`) is built with `New` and a `signup.SignUpConfig`; its test
of a race on a new address gives it a `Lookup` whose first lookup misses.
A use case reads the time from the clock `New` takes (ADR 0104), so a test that needs a moment
builds it with a clock it controls; the container's Step execution use case is `env.StepExecution`,
for a test that hands it an Event or a Rule's enrollment.

## Fixture catalog

Anchor fixture rows have **named constants** in `internal/fixtures/catalog_gen.go`
(generated; never hand-edit). Reference `fixtures.AcmeID`, `fixtures.ContactAliceEmail`,
`fixtures.AcmeCollectKey`, … instead of bare id/email/key literals, so renaming or
removing an annotated row is a compile error and renumbering is absorbed by regeneration.

To add a name, put a line comment on the row's first line in `fixtures/*.yml` and run
`mise run generate`:

```yaml
- id: 1 # fixture: Acme
```

The generator (`cmd/fixturegen`, package `internal/fixtures/fixturegen`) reads the raw
files, so every template expression in fixtures must stay quoted (`'{{daysAgo 1}}'`).
It emits `<Name>ID` plus one constant per catalogued column the row carries (the
list is `catalogColumns` in `internal/fixtures/fixturegen/fixturegen.go`); unannotated
rows get nothing. It fails on a duplicate name or an annotation that matches no row.

### Generated volume

A shape that needs many rows (traffic for a rate, a long list) is a template loop inside the
table's one file, written as comments so the file stays valid YAML for the formatter:

```yaml
# {{range $i := seq 30000 30039}}
- id: '{{$i}}'
  email: 'sent-{{$i}}@example.com'
# {{end}}
```

`seq from to` includes both ends. The loop renders before the file is parsed, so it loads in the
same `Load` as the other rows; loading a table a second time would wipe the first (the loader
deletes every table of its files first), so volume never comes from a second loader. The generator
reads a loop body once and refuses a `# fixture:` name inside one. Every row is one INSERT at the
start of every test process, so keep the volume small and scale the code under test's thresholds
down in the test (see `ee/usecase/abusedetection/detector_test.go`, whose scenarios are day offsets in `events.yml`).

### Credentials and anchor identities

Anchor credentials state their plaintext in the fixture and are hashed at load time by
the `argonHash` (user passwords, PHC argon2id at minimum cost; production verify code reads
the cost from the stored hash) and `secretHash` (API token and Service credential secrets,
SHA-256) template funcs. The generator lifts the quoted literal into `<Name>Password` /
`<Name>Secret`, so `fixtures.OwnerJohnPassword`, `fixtures.AnchorTokenSecret` etc. can
authenticate as that identity; the full token is
`credentials.TokenValue(fixtures.AnchorTokenPrefix, fixtures.AnchorTokenSecret)`. An
Invitation's link token (`inviteHash`, its `token_hash`) is lifted into `<Name>Token`, so a test
accepts a fixture Invitation with `fixtures.InvitationAcmeOscarToken` instead of inviting. Use them
only on annotated anchor rows, not dev tokens. The test values are plain constants in the
generated file; `.betterleaks.toml` exempts their throwaway passwords from the generic rules.

Anchor tenancy rows: Acme (owner `OwnerJohn`, member `MemberMary`), a second tenant
`Globex` (owner `OwnerJane`, membership `GlobexOwnerMembership`), and `OutsiderOscar`
with no membership. A test must not assume Acme has a single member or that only one
Workspace exists.

## Actors

An **actor** is a ready typed client for one API surface acting as one identity. Ask the
`TestEnv` for it; JWT signing, token generation and hashing (through the production token
code), security sources and the in-memory transport stay inside
`internal/testhelper/actor.go`. Tests never touch JWTs, headers or transports.

| Surface  | Identity                         | Call                                                        |
| -------- | -------------------------------- | ----------------------------------------------------------- |
| site     | fixture user                     | `env.SiteActor(t, fixtures.OwnerJohnEmail)`                 |
| site     | anonymous                        | `env.SiteAnonymous(t)`                                      |
| external | anchor token                     | `env.ExternalAnchor(t)`                                     |
| external | Acme token with exactly `scopes` | `env.ExternalScoped(t, "contacts:read")`                    |
| external | anonymous / raw bad credential   | `env.ExternalAnonymous(t)`, `env.ExternalWithToken(t, raw)` |
| collect  | Acme collect key                 | `env.CollectAcme(t)`                                        |
| collect  | anonymous / raw bad key          | `env.CollectAnonymous(t)`, `env.CollectWithKey(t, raw)`     |

Clients are the generated ogen clients (`siteapi.Client`, `externalapi.Client`,
`collectapi.Client`). JWT expiry comes from the helper's single `now()`, so a clock seam
can be injected there later. Do not mint JWTs, hash tokens, or build security sources or
clients in a test: ask the env for an actor. For a raw bearer (raw HTTP or MCP) use
`env.ScopedBearer(t, scopes...)`, or `env.ScopedBearerFor(t, fixtures.GlobexID, scopes...)`
for another tenant. Such helpers may persist a token with explicit scopes, but only
through the production hash/generate functions.
`env.MCPClient(t, bearer)` connects an MCP session.

Tenant-isolation tests use the committed Globex rows by name (`fixtures.SegmentGlobexID`,
`fixtures.TemplateGlobexName`, …). Acme's three roles are `OwnerJohn`, `AdminAlice` and
`MemberMary`.
