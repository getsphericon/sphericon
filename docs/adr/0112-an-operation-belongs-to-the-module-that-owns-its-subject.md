---
status: accepted
---

# An operation belongs to the module that owns its subject

> Refines [[0102-the-core-is-layered-and-an-operation-across-modules-is-a-use-case]]: it admitted
> as a use case "an operation that spans several domain modules"; this replaces that test. It builds
> on [[0110-an-unchecked-operation-lives-where-only-a-use-case-can-import-it]] (the cause goes before
> the fence) and [[0111-every-entry-point-is-a-driver-that-calls-one-operation-of-the-core]] (the use
> case tier stays open).
>
> **Amended by #1361:** the verdict on `secretreencryption` left to the CLI driver's rework (#1210)
> is given here, along with one on `eventhistory`: neither holds an operation of the core, and both
> leave the use case tier. The re-encryption was one call, `secrets.Cipher.Reencrypt` over the
> owners' sweeps, whose list and order are the wiring's: the wiring binds the cipher and the sweeps
> (`app.SecretReencryption`), and `encryption-key reencrypt` runs it and prints its report. The Event
> history resolved the Workspace id an operator names through `workspaces` and asked the bus's
> journal, which is the infrastructure's. It is no domain module of identity without a table
> either, as an operation whose subject no module owns would be: it decides nothing. The bus checks
> the scope, the bound and a deleted or erased Workspace under the Workspace's lock, and turning
> the id an operator types into the Workspace it names is input mapping, as reading a Workspace
> from a request's path is on HTTP. So `events list` and `replay` resolve the id through
> `workspaces` once and ask the bus (`internal/commandline`), and the wiring's binding of the
> re-encryption holds no rule either: a missing or reordered owner fails the run's tests, which
> sweep the wiring's list (`app.SecretSweeps`).

> **Amended by #1357:** the weighing of `callbackinterpretation` deferred to the hooks driver (#1209)
> moves the Callback interpretation into sending, as `internal/domain/sending/callbackinterpretation`,
> a domain module without a table, since neither condition holds: it imports nothing above sending
> (condition 1), and the modules it composes are of sending and the contexts below, the case of a
> domain module without a table rather than of condition 2. Its `Integrations` and `Bots` ports
> stood in front of `integrations.Module`, a module of the core, and go.

## Context

ADR 0102 made a use case of "an operation that spans several domain modules". Its examples were
erasing a Contact, an unsubscribe, deleting a Tag, retyping a Custom field, creating a Workspace and
the Sign-up. Two later decisions moved the line without restating the test:

- ADR 0110 moved a check that needs nothing above its owner into the owner, and left a write
  unchecked only where its check needs a module the owner may not call.
- ADR 0111 kept the use case tier open: a driver calls one operation of a domain module when one
  module holds it, and a use case only when the operation spans several.

The test counts modules, not their order. ADR 0102 already lets a domain module call the modules of
its own context and of the contexts below, yet an operation that does so is a use case as soon as
it calls two of them. The tiers hold 24 use cases under `internal/usecase` and 2 under
`ee/usecase`, and six of them call nothing their subject's owner could not import itself:

- **`contactintake`** upserts a Contact through `contacts` and records its imported SMS
  Confirmation through `confirmations`. Both are contacts modules, and `confirmations` does not
  import `contacts`. `contacts` exports `ImportIn`, `RecordImport` and `ImportSummary` only for it.
- **`transactionalsend`** reads a Template (content), resolves the Contact (contacts), normalises
  the address (`suppressions`, contacts) and hands the request to the Outbound send (sending): all
  of it is the Outbound send's own context or below. It reaches the Outbound send through a `Sender`
  interface, so its test stubs an internal module.
- **`releasecheck`** and **`telemetryreport`** read and write the Instance state's one row through
  `instancestates`, and each calls one outside party (the release source, the Telemetry receiver).
- **`ee/usecase/abusedetection`** reads a Workspace's standing through the Abuse detector's Watch
  (`ee/domain/insights/abusedetector`) and acts through `suspension` and `workspaces`, both in
  identity, below insights.
- **`eventrecording`** resolves a Contact (`contacts`), checks an order Event (`orders`) and
  publishes on the bus. The Events' table is the bus's (infrastructure), not a domain module's.

Each of them splits its subject in two: the rules of the rows live in the owner, the operation in
a package beside it, and the owner exports its steps for that package alone. A reader who looks for
what a Contact intake does reads two packages, and a new operation that composes two modules of one
context gets a package of its own by the same test.

## Decisions

- **An operation belongs to the domain module that owns its subject.** It is a use case only when
  one of two holds:
  1. Its owner would have to call a module above it in the order of contexts: no base module knows
     its users (ADR 0102).
  2. No single module can own it: it composes modules of several contexts none of which owns the
     operation, or the Enterprise License with a core module's rows.

  Otherwise the owner holds it, and the modules it composes are the owner's imports. This replaces
  ADR 0102's "an operation that spans several domain modules" as the test, and its list of examples.

- **An operation whose subject no domain module owns** (an Event's row is the bus's) and which needs
  only modules of one context and below is a domain module of that context without a table, named
  after its glossary term (ADR 0102's naming).
- **The cause goes before the tier.** As ADR 0110 moves a check into its owner when it needs nothing
  above the owner, an operation moves into its owner by the same reasoning. A function the owner
  exported only for the use case becomes unexported.
- **An outside party stays a port of the owner; an internal module does not.** The release source
  and the Telemetry receiver's client are seams with two real sides, production and the test's stub
  of the outside service, which ADR 0102 keeps. A port in front of a module of the core is not: the
  Transactional send's `Sender` goes, and its tests run through the real Outbound send.
- **The use cases that stay, by the test:**
  - The owner sits below the readers of the Removal check (orchestration): `tagdeletion`,
    `customfieldretyping`, `integrationdeletion` and `sendingdomaindeletion` (condition 1).
  - The subject is low and the operation must touch modules above it: `contacterasure` (the pending
    work of Broadcasts, Rules and Automations), `workspacedeletion` (the rows of every context),
    `unsubscriberecording` (the Broadcast's count, the Enrollment's exit) and
    `visitoridentification` (the chats a Visitor started, in sending) (condition 1).
  - Identity needs `instancestates`, the top context, for the Send cap a Workspace starts with:
    `signup` and `workspacecreation` (condition 1). (Amended by #1218: creating a Workspace is an
    operation of the Workspace view, `workspaceview`, see
    [[0111-every-entry-point-is-a-driver-that-calls-one-operation-of-the-core]].)
  - The Second factor reset also removes Passkeys, above the factor (ADR 0110), and User deletion
    becomes a use case (ADR 0110, ADR 0111, #1206) (condition 1).
  - The License with a core module's rows: `auditlogretention`, whose windows are the Workspaces'
    (`workspaces`) and whose rows are the Audit entries' (condition 2).
  - Across contexts: `analyticsreport` (contacts and sending), `needsattentionreport` (sending,
    orchestration and insights) and `noticedelivery` (identity's Users and Owners, told of what
    happened in other contexts, a Webhook endpoint among them) (condition 2).
- **The use cases that move, each owner in a change of its own:**
  - `contactintake` into `contacts`, as `contacts.Module.UpsertBatch`.
  - `transactionalsend` into `outboundmessages`, as `SendTransactional`.
  - `releasecheck` and `telemetryreport` into `instancestates`.
  - `ee/usecase/abusedetection` into `ee/domain/insights/abusedetector`, which joins the
    Enterprise raw-client holders: the pass lists every Workspace, as the use case did.
  - `eventrecording` into a domain module of the contacts context,
    `internal/domain/contacts/eventrecording`, named after the glossary's Event recording.
- **Others move by the same test in the changes their issues describe:** the verification of a
  Sending domain into `sendingdomains` (#1202), the exchange and refresh of a Connection into
  `apitokens` (#1204), and the Second factor check but its reset into a `secondfactors` module
  (#1203). `stepexecution` moves into `automations` as `automations.StepExecution`: it composes
  `contacts`, `eligibility` and `outboundmessages`, all below orchestration, beside the Enrollment's
  lifecycle it moves, as the Broadcast's delivery is `broadcasts.Delivery`. `callbackinterpretation`
  and `secretreencryption` are weighed by the changes that rework them (the hooks driver, #1209;
  the CLI driver, #1210).
- **A moved operation follows the domain module's conventions.** It reads the Instance clock its
  module holds, never `time.Now`, opens its own transaction and checks every input to its writes
  (ADR 0107). The call contract of ADR 0104 binds the use case tiers only, so `usecaseshape` stops
  reading a moved package. Whether domain modules get a call contract of their own is not decided
  here.

## Considered options

- **Keep "spans several modules".** Every operation that composes two modules of one context stays
  a package of its own, and its owner keeps exporting steps that no other caller may use. The tier
  grows with packages the order of contexts gives no reason for.
- **Drivers call only use cases.** ADR 0111 rejected the closed tier: a forwarding use case for each
  single-module call, with nothing behind it.
- **Every operation in its owner, with ports for the modules above.** ADR 0102 keeps a port for a
  seam with two real sides, not to turn a dependency around.
- **A use case per context.** ADR 0111 rejected it as a facade per context.

## Consequences

- Seven packages leave the tiers, in six changes after this one (the Step execution's included): their tests move to the owner's
  `_test` package with their assertions, and their callers in the API surfaces, the job runner, the
  Edition and the wiring call the owner.
- The Step execution leaves the use case tier the same way. What `automations` exported only for
  it becomes unexported: the moves of an Enrollment (`Advance`, `Complete`, `Fail`, `ExitRun`), the
  steps an Event or a Rule takes (`EndOnEvent`, `TriggeredBy`, `OriginOf`, `EnrollOnTrigger`,
  `EnrollOnRule`), `LockContact`, `IsActive`, `Stale` and the HTTP step's `OpenHTTP`, `RenderBody`
  and failure record. `Enroll`, which only tests called, goes. Their tests run through
  `StepExecution`, the race of a Deactivate or a Publish with an Event's enrollment included;
  `Exit` stays exported for the unsubscribe use case.
- `easement.yml` changes with them: `domain-instance-raw` may import the Telemetry contract's
  client, `abusedetector` joins `ee-domain-insights-raw`, and `ee-usecase-raw` keeps one use case.
  The Outbound send reads the Template a Transactional send references through ent, as a module
  reads another's rows (ADR 0102), so sending imports no content module.
- `docs/architecture/use-cases.md` ("Use cases") states this test, and `internal/AGENTS.md` lists the tests
  that stay with a use case.
- ADR 0104's call contract still binds only the use cases. Its Abuse detector, which it turned from
  a `Detector` into a `UseCase`, is a domain module's operation again.
