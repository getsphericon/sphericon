---
status: accepted
---

# The core is layered, and an operation across modules is a use case

> Amends [[0016-mcp-surface-projection-of-external-api]]: its "Use-case modules" item called every domain
> package a use-case module. A use case is now only an operation that spans several domain modules; a
> package that owns one domain's rows is a domain module.
>
> Amended by #1159: a use case is named after its operation's glossary term, a read-only one after
> its report; a protocol's or a console's endpoints are not a use case. See "Names follow one rule"
> and "A use case is an operation that spans several domain modules".
>
> Refined by [[0107-a-domain-module-checks-every-input-to-its-writes]]: no caller holds a rule of a
> module's rows for it.
>
> Refined by [[0110-an-unchecked-operation-lives-where-only-a-use-case-can-import-it]]: an unchecked
> removal lives in a subpackage only use cases may import, not in `forbid_symbol` lists, and easement
> does match a member after its type. Refined by
> [[0111-every-entry-point-is-a-driver-that-calls-one-operation-of-the-core]]: a driver calls one
> operation of the core per function.
>
> Refined by [[0112-an-operation-belongs-to-the-module-that-owns-its-subject]]: "an operation that
> spans several domain modules" is no longer the test. An operation belongs to the module that owns
> its subject, and is a use case only when that owner would call a module above it or no single
> module can own it. The examples of a use case and the use case names below that it moves
> (`eventrecording`, `releasecheck`, `transactionalsend`, `telemetryreport`) no longer hold.
>
> Amended by [[0112-an-opt-out-is-an-event-on-every-channel-and-the-broadcast-counts-it-from-the-event]]: the
> rejected option "Domain events between modules" no longer holds for a counter. Since ADR 0101 an
> event consumer's job is inserted in the producer's transaction, so a count made from the event
> commits with the change: a Broadcast counts every opt-out that way. A cascade that must take
> effect with the change, such as leaving an Automation, stays synchronous.

## Context

`easement.yml` orders the outside of the backend: the contract, the wiring, the API surfaces, the
Enterprise Edition. Inside, the core is one component of about 130 packages, and nothing says which of them
may know which. Go forbids import cycles, so the graph is acyclic, but every cycle a change would close gets a
workaround instead of a decision. The code says so itself:

- **`liveuse`** exists for the modules "that cannot import internal/automations".
- **`ownernotice`** is a leaf "so the modules that accounts itself imports can share it".

The common shape behind those cycles is a base module that calls the modules that use it:

- `tags` asks `segments`, `automations`, `rules` and `broadcasts` whether a Tag is in use before it deletes
  it;
- `contacts` asks `segments`, `rules` and `broadcasts` whether a Custom field can be retyped;
- `erasure` cancels the pending work of `broadcasts`, `rules` and `automations`;
- `consent` ends Enrollments and counts the unsubscribe on the Broadcast.

`docs/architecture.md` forbids exactly this for the bus: "A producer never names a consumer".

Other parts of the core are misplaced too:

- The job runner `internal/jobs` is in the core, although review rule 5 counts jobs with the handlers.
- The bus `events` holds the suppression consumer, and `db` installs other modules' hooks.
- `accounts` is cut by a privilege ("the raw-client home for what the scoped client cannot express"), not by
  a domain. So Workspace settings sit beside sign-up and invitations, and the glossary's avoided word
  "Account" names a package.

## Decisions

- **The core has four tiers, checked by easement:**
  - `internal/kernel/*`: packages that import none of ours.
  - `internal/infra/*`: the bus, the database connection and migrations, the message providers, the
    outbound HTTP call, the throttles. Infrastructure imports only the kernel and never a domain module.
  - `internal/domain/<context>/<module>`: the domain modules.
  - `internal/usecase/<name>`: the use cases.

  Above them sit the drivers, which no domain module or use case imports: the API surfaces, MCP and
  `internal/jobs`. The wiring builds the program, the ent client and its hooks included.

- **A domain module owns its tables and the rules of its own rows.** It calls only domain modules of its
  own context or of contexts below it.
- **Only the owner writes a table.** A table is written only by the domain module that owns it. A use case
  or another module reads it, and changes it only through its owner's functions. The `entrywrite` analyzer,
  which forbids handlers to write through ent, holds an ownership map: each ent entity names its owning
  module, and a write to it from any other package fails.
  An infrastructure package owns the few tables that belong to no domain, with the same rule: the bus
  owns the Events (`Event`), the Login throttle its attempts (`AuthAttempt`) and the database package
  the schema state the migrations keep (`SchemaState`).
- **The contexts are ordered:** identity → contacts → content → sending → orchestration → insights →
  instance. A module that targets or uses another sits above it. A Broadcast targets Contacts and uses a
  Template and an Integration, so orchestration sits above contacts, content and sending.
- **A use case is an operation that spans several domain modules.**
  - Examples: erasing a Contact, an unsubscribe, deleting a Tag, retyping a Custom field, creating a
    Workspace, sign-up.
  - It calls the domain modules, opens the transaction and, where ADR 0099 requires it, takes the Workspace
    lock.
  - Use cases do not import each other (easement `isolated`). A piece two of them need moves down into the
    domain module that owns it. A piece that writes the rows of two modules has no single owner: each
    use case composes those modules itself, as the Workspace creation and the Sign-up both create a
    Workspace with its Owner Membership.
  - A package that serves a protocol's or a console's endpoints is not a use case because it reaches
    many modules. Its endpoints are a driver, and only an operation across modules behind them is a
    use case: the OAuth server is the driver `internal/api/oauth`, and the exchange of a code or a
    refresh token for a Connection is the use case `connectionissuance`. A package whose every
    operation calls one module, and which owns rules of its own, is a domain module: the Workspace
    oversight (`ee/domain/insights/workspaceoversight`) gates the Staff's look across Workspaces and
    makes each change through the one module that owns its rows, since a handler may not build the
    Workspace scope it needs.
- **No base module knows its users.** A dependency against the order is removed by one of three moves:
  - move the code to where it belongs;
  - move a shared piece into a package below both;
  - compose the modules in a use case above them.

  A port (an interface the lower module declares and the wiring fills) is kept for a seam with two real
  sides, as `login.Store` and `broadcasts.Scheduler` are. It is not used to turn a dependency around.

- **A domain module's unchecked removal is reachable only from its use case.** easement matches names, not
  methods (a call of a method counts as a use of its type), so that removal is a package-level function or
  a type of its own, named in a `forbid_symbol` of the drivers. The check that guards it, its transaction
  and its lock live in the use case.
- **The owner of the transaction publishes the event.** A domain module owns the transaction of an
  operation on its own rows. A use case owns the transaction of an operation across modules, and the
  modules it calls join that transaction.
- **The unit of isolation is the package.** A context only orders packages; it is not a box with a facade,
  and modules keep their own exported API.
- **The Enterprise Edition has the same shape:** `ee/domain/<context>/<module>`, `ee/usecase/<name>`,
  `ee/api` and the Edition `ee/ee.go`. Its boundary with the core is unchanged.
- **Names follow one rule.**
  - A domain module that owns a table is named after its ent entity, in the plural (`contacts`, `tags`,
    `workspaces`, `users`, `memberships`). The plural also keeps it apart from ent's singular packages.
  - A domain module that owns several entities is named after the entity the others belong to, in the
    plural. OAuth codes and refresh tokens belong to an OAuth client, so the module is `oauthclients`.
  - A domain module without a table is named after its glossary term in the singular (`condition`,
    `eligibility`).
  - A use case that changes rows is named after its operation, `<subject><action>` in glossary words
    (`contacterasure`, `tagdeletion`, `sendingdomainverification`, `eventrecording`). When the
    glossary names the operation, its term is the name, and the term may cover the few steps of one
    operation: `signup` (Sign-up), `secondfactorcheck` (Second factor check: enrol, verify, regenerate,
    disable, reset), `releasecheck` (Release check: the lookup and the notice), `transactionalsend`
    (Transactional send), `auditlogretention` (the Retention of the Audit log).
  - A use case that only reads is named after the glossary report it builds (`analyticsreport`,
    `needsattentionreport`). `telemetryreport` also sends the report, which the glossary's Telemetry
    report includes.
  - A name is the glossary's words joined in lower case, with spaces and hyphens dropped: `signup` is
    the package of the Sign-up, although the glossary avoids "signup" in prose.
  - Only glossary words are used. When a name needs a word the glossary lacks, the glossary changes first.
- **`accounts` is split by domain:**
  - Low in identity: the Workspace with its settings, Memberships, Permissions and the Owners with their
    notices.
  - Above them: Users and their credentials, invitations and Join domains.
  - The throttle goes to infrastructure.
  - Workspace creation and sign-up become use cases.
- **The rules are strict from the start.** Each easement rule is switched on in the change that makes it
  hold, and no violation is recorded in a baseline.

## Considered Options

- **Ports for every dependency against the order.** The base module declares what it needs and its users
  implement it. Each rule stays with its owner, but the list of what is asked moves to the wiring and out of
  sight of a reader of the base module. Jeremy Bowers' "Layered Design in Go"
  (https://www.jerf.org/iri/post/2025/go_layered_design/) ranks breaking a cycle with an interface after
  moving code and after composing in a package above, since an interface "still creates a relationship of a
  sort". Shopify reports of its monolith that inverted dependencies "often made code harder to understand"
  (https://shopify.engineering/a-packwerk-retrospective).
- **Domain events between modules.** These cascades must commit with the change that causes them: an
  unsubscribe and the end of the Enrollment, a Live use check and the removal under one lock. An event
  consumer runs after the commit.
- **A domain free of ent, behind repositories.** Every entity's shape would be stated a second time beside
  its ent schema (review rule 6). What that buys is a domain tested against fakes, which review rule 11
  rejects.
- **Contexts as isolated components with a public facade.** Shopify found components drawn by domain
  produced "monstrously large todo files", and removed Packwerk's privacy checks
  (https://shopify.engineering/a-packwerk-retrospective). A facade would also restate each exported
  signature.
- **Vertical slices per operation.** Four API surfaces, MCP and the jobs call the same module for the same
  operation. A slice per entry point would copy the rule; a slice shared by all of them is a module again.
- **Keep the flat core.** Each new cycle would get one more leaf package as its workaround.

## Consequences

- Most packages of the core move and many are renamed. The work lands as a sequence of changes, each with
  its easement rules switched on and its checks green: the tiers and the infrastructure fixes, then the
  split of `accounts`, then the use cases, then the contexts and their directories, then the remaining
  renames.
- `liveuse` and `ownernotice` disappear. The Live use reads become reads of the orchestration context, and
  the Owners' notices sit with the Owners.
- Review rule 5 reads: a driver receives input, calls a domain module or a use case and returns the result.
- The suppression consumer leaves the bus. Its job resolves the Workspace's scope and calls `suppression`,
  and the bounce and complaint names join the bus's catalog of names.
- The scope vocabulary that a domain module reads today from the generated `/site` package is generated
  into a kernel package of its own. TypeSpec stays its single place (ADR 0061).
- `docs/architecture.md`, review rule 5 and `easement.yml` change with the code, not before it.
