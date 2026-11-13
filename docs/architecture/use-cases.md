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
included: the Workspace erase job calls `workspacedeletion`). It takes the
`*ent.Scoped` its driver built, opens the transaction the domain modules it calls join (taking the
Workspace lock where ADR 0099 requires it, so a removal that runs the Removal check opens it with
`WithinWorkspaceLockTx`, as `tagdeletion` and `customfieldretyping` do; `contacterasure` opens `WithinScopedPurgeTx`, takes no Workspace lock and locks the Contact through `contactlock`; `unsubscriberecording` opens one `WithinScopedTx` for the opt-out, the Enrollment's exit and anonymous attribution work), reads them, and changes their rows only through their functions
(Erasure calls an erase function of each owner, from its `<module>unchecked` subpackage: `eventsunchecked.EraseSubject`, `visitorsunchecked.EraseSubject`, `outboundmessagesunchecked.AnonymizeContact`, `confirmationsunchecked.EraseNumber`, `unsubscribesunchecked.EraseSMS` and the rest, then `contactsunchecked.DeleteIn`, whose foreign keys delete or detach the Contact's other rows; an unsubscribe writes the opt-out through `unsubscribes`, drops the Confirmation an opt-out of everything ends through `confirmations.Withdraw`, calls `automations.Exit` and publishes anonymous attribution for the Broadcast consumer).
Work that starts before any scope exists, such as a job given only a row's id, has its driver read
the row's Workspace and build the scope the use case takes (a signed unsubscribe link's comes from
`workspaces.Module.LinkScope`). easement
isolates the use cases (`isolated`: none imports another; a piece two of them need moves down into
the domain module that owns it, as the SMS opt-in a START records is `confirmations.StartSMSIn`
for the Ingest hook), lets only the drivers and the wiring import one (`allow_importers`)
and forbids it the raw client. The exception is a use case whose rows belong to no Workspace
when it starts, so there is no `*ent.Scoped` to take: the Second factor reset of a User
(`internal/usecase/secondfactorreset`). Such a use case holds the
raw client, opens the raw transaction its modules' `...In(ctx, tx, ...)` functions join and is
in the closed list of [Workspace scoping](workspace-scoping.md); easement holds it as the `usecase-raw` component,
one pattern per use case (`internal/usecase/secondfactorreset*`), isolated like the others.
Two more are such use cases: the deletion cycle of a Workspace
(`internal/usecase/workspacedeletion`), which lists the Workspaces that have no scope yet, erases one
table at a time, each batch of listed ids deleted by itself through `*ent.Scoped`, the Enterprise Edition's tables included (ADR 0110: the one writer of a Workspace-owned table besides its owner,
[Table ownership](modules.md#table-ownership)), and moves the
Workspace row on through
`TombstoneIn` (deleting one checks only its typed name, so it is `workspaces.Module.Delete`,
ADR 0110; the table of the cycle's moves is `workspaces.DeletionCycle`), with the OAuth codes that name it removed by `oauthclientsunchecked.DeleteCodesIn` and every
job of the Workspace left in the queue by the bus (`events.Bus.WithinWorkspacePurgeTx`, [Async jobs](events-and-jobs.md#async-jobs)), both in the tombstone's transaction; and the
Workspace view (`internal/usecase/workspaceview`), the operations of the Workspace screens, each answering with the Workspace summary as the caller sees it (ADR 0111, amended by #1218): it creates a Workspace, writing it through
`workspacesunchecked.CreateIn` and its Owner Membership through `membershipsunchecked.AddCreatorIn` with the Send cap it
starts with (`instancestates.DefaultSendCapIn`, ADR 0067), and moves a deleted one back through
`workspacesunchecked.RestoreIn`; and the Sign-up
(`internal/usecase/signup`), which composes the same two modules for the User's first Workspace in
every step that makes a User Verified, each on its own transaction: the verification and reset
links, a Sign-up and an Owner made where verification is off, the provider sign-in and the
completion of a pending sign-up, calling the `users` functions for the User's part
(`MarkVerifiedIn`, `CreateVerifiedIn`, `CreateProviderUserIn`, `TakeOverIn`,
`Module.CompletePendingSignupIn`, `ApplyDomainJoinIn`; the completion of a pending sign-up also
resolves its signed reference through `users.Links`, checks the answers and records `user.login`), the sign-in through a Sign-in provider
being its `SignInWithProvider` whole (ADR 0111: the provider's answer, the Invitation, the Sign-up
items, the decision, the link of a link flow, the Second factor and `user.login`; it returns what
happened, signed in, a Second factor due, a Pending sign-up or an identity linked, and the callback
turns that into a cookie or a redirect); and User deletion
(`internal/usecase/userdeletion`), which proves who asks (the password through `login`'s `Prove`,
or the Login throttle and the deletion link, then the Second factor code through
`secondfactors.Module.DeletionCode` in its transaction, a wrong one counted on the Login throttle),
locks the User row and the Owner rows of its Workspaces (`memberships.SoleOwnershipIn`), refuses
the only Owner of a live Workspace, leaves each Workspace (`memberships.LeaveIn`, or
`membershipsunchecked.LeaveDoomedWorkspaceIn` for one in its deletion window) and deletes the row
with `usersunchecked.DeleteIn`, which publishes `UserDeleted`.
What stays unchecked lives in `<module>unchecked` (ADR 0110). A write is left unchecked only
when its check needs a module its owner may not call: one in a higher context, or several at
once; otherwise the check moves into the owner. It still checks its own input (ADR 0107); what it
leaves to the use case is the rule across modules. Such a write is a package-level function of a
subpackage of its owner named `<module>unchecked`
(`internal/domain/contacts/tags/tagsunchecked`), which may import its owner and which the owner
does not import; the owner exports for it only what writes nothing (a state table such as
`workspaces.DeletionCycle` or `secondfactors.Machine`, a compare-and-swap condition, an Audit
entry, a read under a lock), and the write itself is the subpackage's, which `entrywrite` takes as
its owner's. easement holds the subpackages of each context in a component of its own,
`domain-<context>-unchecked` (`internal/domain/<context>/**/*unchecked`, and `infra-unchecked` for
the bus's), each with `allow_importers: [usecase, usecase-raw]`, so only a use case imports one: no
handler, job, MCP tool, command, other domain module, Enterprise package nor the owner bypasses the
check. Each sits in `layers` right above its context, so a subpackage reaches its owner and what
its owner's context may, never a context above. The Enterprise Edition's are
`ee-domain-<context>-unchecked` (`ee/domain/<context>/**/*unchecked`), imported only by its own use
cases; none exists today. No `forbid_symbol`
names an unchecked operation: that list holds only what the program's builders use (the raw
client, the ent hooks and guards, the upgrade boundaries, river's workers). They are the Live
use writes behind the Removal check (`tagsunchecked.Deletion`, the edit
`contactsunchecked.CustomFieldUpdate`, `integrationsunchecked.Deletion` and
`sendingdomainsunchecked.Deletion`, each opened on its row, then checked by the use case and
applied, ADR 0145), User deletion's removals
(`membershipsunchecked.LeaveDoomedWorkspaceIn`, which lets the only Owner of a Workspace in its
deletion window leave, and the User row, `usersunchecked.DeleteIn`), the Second factor reset's
removals (`secondfactorsunchecked.ResetIn`, the factor with its Recovery codes, and
`passkeysunchecked.DeleteAllIn`), the restore and the tombstone of a Workspace's deletion cycle
(`workspacesunchecked.RestoreIn` and `TombstoneIn`), the removal of the OAuth codes its erase
makes (`oauthclientsunchecked.DeleteCodesIn`; the erase deletes a Workspace's rows itself, ADR
0110, so no owner's removal of them has a name), the creation of a Workspace with its creator's Membership (`workspacesunchecked.CreateIn`, `membershipsunchecked.AddCreatorIn`) and each owner's step of a Contact's Erasure (`visitorsunchecked.EraseSubject`, `eventsunchecked.EraseSubject` of
the bus, `suppressionsunchecked.EraseContact` and the others
`internal/usecase/contacterasure`
sequences, with the Contact row last, `contactsunchecked.DeleteIn`; the purge of its queued jobs is a method of the bus's publisher). A new fence for any other caller has the same form: a
subpackage and one component whose `allow_importers` names who may import it, as the readers of
the Removal check are (`<module>use`). The tests of the operation live in the use case's own `_test` package
(`internal/usecase/tagdeletion`, `internal/usecase/customfieldretyping`, `internal/usecase/unsubscriberecording`; a retype's refusal is also tested through the API's Custom field update, an Erasure through the APIs' Contact deletion and Privacy request), through its exports; the domain module keeps the tests of its own
rows, and an unchecked operation's own tests of its rows are its subpackage's (`broadcastsunchecked`, `suppressionsunchecked`). What the Removal check finds is tested in its own package
(`internal/domain/orchestration/removalcheck`, with the writes that create a use waiting for the
Workspace lock), and each removal that runs it races those writes in its use case's own
`workspace_lock_test.go`. The Secret re-encryption
(`internal/usecase/secretreencryption`, `sphericon encryption-key reencrypt`) spans every
Workspace and the Users and Staff members besides: it takes the `SecretSweeps` of each module that
owns a table with stored secrets (`integrations`, `webhookendpoints`, `sendingdomains`, `secondfactors`,
`staffmembers`, `automations`, `rules`), the columns as `secrets.Sweep` values the wiring builds on
the raw client in a fixed order, and sweeps them by id and compare-and-swap
(`secrets.Cipher.Reencrypt`). A sweep only stores the value it read again under the
primary key, so no check guards it and it is no unchecked operation.
A use case need not guard a removal: an operation whose subject's owner would have to call a
module above it is one too, and so is one that reads modules of several contexts. A Visitor
identification (`internal/usecase/visitoridentification`, built in the container like every
use case and handed to the collect surface) upserts the Contact through `contacts`, binds the
Visitor through `visitors`, announces its chats through `bindings` (sending, above the Contact)
and stitches its Events through the bus, in one transaction. The Ingest hook's reading of a provider's callbacks
(`callbackinterpretation`) opens the request's transaction and changes Outbound messages, Suppressions,
Unsubscribes, Confirmations and Bindings through their modules; the Analytics reports (`analyticsreport`), what Needs
attention (`needsattentionreport`) only read. The Workspace view (`workspaceview`) reads the Workspace summary, and
makes the changes of a Workspace a member answers with it, its creation and its restore included:
a change in identity, then the read of sending.
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
