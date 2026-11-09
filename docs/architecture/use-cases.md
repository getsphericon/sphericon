# Use cases

[Architecture](../architecture.md) → Use cases

## Operation placement and call contract

**Use cases (ADR 0102, ADR 0112, component `usecase`).** An operation belongs to the domain module
that owns its subject, which may compose the modules of its own context and of the contexts below.
It is a use case only when that owner would have to call a module above it (a base module knows no
module that uses it) or when no single module can own it (it composes modules of several contexts
none of which owns it, or the Enterprise License with a core module's rows):
`internal/usecase/<subject><action>` in glossary words (`tagdeletion`). An operation whose
modules its owner may import is the owner's, with its tests: the Contact intake of an import is
`contacts.Module.UpsertBatch`, which records a row's SMS Confirmation through `confirmations` in
the row's transaction; the Transactional send is `outboundmessages.Module.SendTransactional`,
which reads the Template it references, resolves the Contact through `contacts` and sends through
the module's own `Send`; and the Release check and the Telemetry report of the whole Instance are
`instancestates.Module`'s (`CheckRelease`, `ReleaseFor`, `TelemetryReport`,
`SendTelemetryReport`), with their outside parties, the release source and the receiver's client,
handed in by the wiring as parameters of `instancestates.New`; and the Step execution of an
Enrollment is `automations.StepExecution` (`NewStepExecution`, beside the Automation's lifecycle as
the Broadcast's delivery is `broadcasts.Delivery`), which applies a tag step's Tag through
`contacts` and sends a send step through the Outbound send. An operation whose subject
no domain module owns, and which needs only one context, is a domain module of that context
without a table: the Event recording (`internal/domain/contacts/eventrecording`) resolves each
Event's Contact through `contacts`, checks an order Event through `orders` and publishes the
batch on the bus, which owns the Events and the Event log's reads. The use
case is a `UseCase` type built by `New` from what it needs (the bus, the Instance clock: ADR 0104
states the call contract every use case follows, which the `usecaseshape` analyzer checks), registered in
`internal/app/container.go` and handed to the drivers through their `Deps` (the job runner's
included: the Sending domain re-check jobs call `sendingdomainverification`). It takes the
`*ent.Scoped` its driver built, opens the transaction the domain modules it calls join (taking the
Workspace lock where ADR 0099 requires it, so a removal that runs the Removal check opens it with
`WithinWorkspaceLockTx`, as `tagdeletion` and `customfieldretyping` do; `contacterasure` opens `WithinScopedPurgeTx`, takes no Workspace lock and locks the Contact through `contactlock`; `unsubscriberecording` opens one `WithinScopedTx` for the opt-out, the Enrollment's exit and anonymous attribution work), reads them, and changes their rows only through their functions
(`sendingdomainverification` stores a check's outcome with `sendingdomains.RecordDKIM` and `RecordDMARC`; Erasure calls an erase function of each owner: `events.EraseSubject`, `visitors.EraseSubject`, `bindings.EraseSubject`, `outboundmessages.AnonymizeContact`, `confirmations.EraseContact`, `unsubscribes.EraseSMS` and `DetachContact` and the rest, then `contacts.DeleteIn`; an unsubscribe writes the opt-out through `unsubscribes`, drops the Confirmation an opt-out of everything ends through `confirmations.Withdraw`, calls `automations.Exit` and publishes anonymous attribution for the Broadcast consumer).
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
in the closed list of [Workspace scoping](workspace-scoping.md); easement holds it as the `usecase-raw` component,
one pattern per use case (`internal/usecase/secondfactorcheck*`), isolated like the others.
Two more are such use cases: the deletion cycle of a Workspace
(`internal/usecase/workspacedeletion`), which lists the Workspaces that have no scope yet, erases one
table at a time, each batch of listed ids through the removal of the table's owner (`tags.EraseTagsIn`,
`events.EraseEventsIn` and one more per Workspace-owned table; the Enterprise Edition's three, which
no core package may import, the wiring hands in as `workspacedeletion.EditionTables`), and moves the
Workspace row through `workspaces.DiscardIn`, `RestoreIn` and
`TombstoneIn`, with the OAuth codes that name it removed by `oauthclients.DeleteCodesIn` and every
job of the Workspace left in the queue by the bus (`events.Bus.WithinWorkspacePurgeTx`, [Async jobs](events-and-jobs.md#async-jobs)), both in the tombstone's transaction; and the
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
`<import path>.<Name>` is an item of the top-level `forbid_symbol` in `easement.yml` with
`allow: [usecase, usecase-raw]`, which holds in every other component and in the files of none, so
only its use case calls it: no handler, job, MCP tool or other domain module bypasses the check, nor
the owning module itself (easement counts a call there too; the definition and tests are not
checked). The Second factor's writes that skip its checks (the User's
`users.MoveSecondFactorIn`, `recoverycodes.ReplaceIn` and `DeleteIn`, `passkeys.DeleteAllIn`) and the
issue of a grant's credentials (`apitokens.IssueConnection` and `RotateConnection`,
`oauthclients.NewRefreshTokenIn`), the moves of a Workspace's deletion cycle
(`workspaces.DiscardIn`, `RestoreIn` and `TombstoneIn`), the core removals of its erase (each
`Erase…In` of a Workspace-owned table, one shared reason, and `oauthclients.DeleteCodesIn`; the
Edition's three are allowed only in the wiring and the Edition, which name them to hand them in),
the creation of a Workspace with its creator's Membership
(`workspaces.CreateIn`, `memberships.AddCreatorIn`) and each owner's step of a Contact's Erasure
(`visitors.EraseSubject`, `events.EraseSubject`, `rules.EraseFirings` and the others
`internal/usecase/contacterasure` sequences; the purge of its queued jobs is a method of the bus's
publisher, which a name cannot forbid) are such items too. The tests of the operation live in the use case's own `_test` package
(`internal/usecase/tagdeletion`, `internal/usecase/customfieldretyping`, `internal/usecase/unsubscriberecording`; a retype's refusal is also tested through the API's Custom field update, an Erasure through the APIs' Contact deletion and Privacy request), through its exports; the domain module keeps the tests of its own
rows. What the Removal check finds is tested in its own package
(`internal/domain/orchestration/removalcheck`, with the writes that create a use waiting for the
Workspace lock), and each removal that runs it races those writes in its use case's own
`workspace_lock_test.go`. The Secret re-encryption
(`internal/usecase/secretreencryption`, `sphericon encryption-key reencrypt`) spans every
Workspace and the Users and Staff members besides: it takes the `SecretSweeps` of each module that
owns a table with stored secrets (`integrations`, `webhookendpoints`, `sendingdomains`, `users`,
`staffmembers`, `automations`, `rules`), the columns as `secrets.Sweep` values the wiring builds on
the raw client in a fixed order, and sweeps them by id and compare-and-swap
(`secrets.Cipher.Reencrypt`). A sweep only stores the value it read again under the
primary key, so no check guards it and it is not in the `forbid_symbol` of `easement.yml`.
A use case need not guard a removal: an operation whose subject's owner would have to call a
module above it is one too, and so is one that reads modules of several contexts. A Visitor
identification (`internal/usecase/visitoridentification`, built in
`internal/server` with the collect surface) upserts the Contact through `contacts`, binds the
Visitor through `visitors`, announces its chats through `bindings` (sending, above the Contact)
and stitches its Events through the bus, in one transaction. The Ingest hook's reading of a provider's callbacks
(`callbackinterpretation`) opens the request's transaction and changes Outbound messages, Suppressions,
Unsubscribes, Confirmations and Bindings through their modules; the Analytics reports (`analyticsreport`) and what Needs
attention (`needsattentionreport`) only read.
The consent of a Destination (`internal/domain/contacts/confirmations`,
`internal/domain/contacts/unsubscribes` and `internal/domain/contacts/channelstop`, the stops read in
the Channel) sits in the contacts context, below the Broadcasts and Automations an unsubscribe
touches: easement fails its import of `broadcasts`, `automations` or a use case as an import
against the order of the contexts (`domain-contacts`), so only `unsubscriberecording` ends the
Enrollment, in the opt-out's transaction. No opt-out writes a Broadcast row: the recipient row is the
`broadcasts` module's (ADR 0102). Every new opt-out made on a received message, by a link on any
Channel or a Telegram button, publishes the internal `message.unsubscribed` in its transaction
(`unsubscriberecording`, or `channelstop` for an SMS link and a Telegram button), and the broadcasts
consumer (`event_broadcast_unsubscribe`, a job per ADR 0101) is the one caller of
`broadcasts.RecordUnsubscribe`, after the opt-out commits (ADR 0115). The public
`email.unsubscribed`, `telegram.unsubscribed` and `sms.unsubscribed` name the Broadcast on the
Contact's Event only and count nothing. Nothing is lost: the job is inserted in the opt-out's
transaction, and the count is set once.
`unsubscribes` is the lowest of the three: `confirmations` imports it (a START lifts the opt-out),
and `channelstop` composes both (a stop drops the Confirmation).
One unchecked removal is called from a domain module until its use case exists:
`memberships.LeaveDoomedWorkspaceIn`, which lets the only Owner of a Workspace in its deletion window
leave, serves User deletion in `internal/domain/identity/users`, which alone is allowed to call it:
that module is a component of its own, `domain-identity-users`, the only component its
`allow` in `easement.yml` names: no other may call it, the use cases included.
