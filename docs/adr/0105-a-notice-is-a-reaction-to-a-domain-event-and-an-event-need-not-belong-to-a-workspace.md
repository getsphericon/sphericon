---
status: accepted
---

# A notice is a reaction to a domain event, and an event need not belong to a Workspace

> Amends [[0101-a-domain-event-is-a-river-job-for-each-consumer-that-wants-it]]: an event belongs to a
> Workspace or to the Instance, and only a Workspace's event can be an Event with a row. Replaces the
> owner-notice seam of [[0102-the-core-is-layered-and-an-operation-across-modules-is-a-use-case]]
> ("the Owners' notices sit with the Owners").

## Context

sphericon mails people on its own behalf through the System sender: a User their account's links
and notices, a Member their invitation and Second factor deadline, the Owners of a Workspace what
happened to its standing and configuration. Each of those mails was sent by the code that made the
change:

- The `/site` handlers and the Sign-up queued a mail job of their own (`internal/kernel/mailjobs`)
  after the operation, with the insert-only river client, outside its transaction: a mail could be
  lost after a committed change, or queued for one that rolled back.
- Join domains, Sending domain verification, Tracking domains, the webhooks' delivery, the
  suspension and the abuse detector took a `memberships.Notify` and sent the Owners' mail
  synchronously after their commit. A failed send was logged and lost, or, for a suspension,
  turned into `ErrNoticeNotSent`, which every caller had to know about.
- The Second factor reminder claimed a reminder, sent the mail in the claim's loop and released the
  claim when the send failed.

So every module that changes something worth telling knew who is told, through which sender, and
what to do when the send fails. The domain events of ADR 0101 already solve this for the Workspace's
own reactions (automations, Rules, webhooks), but they could not carry these: every event had to
name a Workspace, and a Sign-up, a password reset or a Passkey belongs to a User, not to any
Workspace.

## Decisions

- **An event belongs to a Workspace or to the Instance.** `events.DomainEvent` is only a name and a
  version. A `WorkspaceEvent` names its Workspace; a `Projected` event is a Workspace's event that is
  an Event, with a row of its own, the one kind the automations, the Rules and the webhooks react to.
  The envelope's Workspace is 0 for an event of the Instance. The `Unprojected` marker is gone: not
  being Projected is the default.
- **Only an Event or an Audit entry leaves through a Webhook endpoint.** The webhooks consumer wants
  the Projected events and the Audit entries, nothing else: an event of the Instance has no
  Workspace, and an Invitation's event carries its link's token.
- **Every change a person must be told of publishes an event in its transaction.** The account's
  events (`internal/infra/events/account.go`: a link requested, an address claimed, a Sign-up
  completed, a Sign-in provider linked, a User deleted, a Passkey added or removed) belong to the
  Instance; the Workspace's (`workspace_changes.go`: an Invitation, the Two-factor requirement and a
  Member's reminder, a suspension, an abuse warning, a lost Join domain or Sending domain
  verification, a replaced or removed Tracking host, a disabled Webhook endpoint) to the Workspace.
  A request that changes no state (a password reset, a verification link sent again, an email
  change, a deletion link) publishes in a transaction of its own (`events.Bus.Publish`). A request
  that must not tell whether an address holds a User still publishes for every address, with
  `Deliver` false where no mail is due, so every request does the same work.
- **An event carries no token it can mint later.** A link that signs in or proves something is minted
  when its mail is issued, bound to the User as they are then. The one exception is the Invitation's
  token, which is stored hashed and exists only in the event until it is mailed.
- **Notice delivery is a use case, and the Notices are one event consumer.** `internal/usecase/
  noticedelivery` answers, for an event, the Notices it calls for: the recipients and the copy are
  read when the event is handled (a suspension lifted since, a requirement switched off since, tells
  no one). The Notices' consumer (`internal/jobs/notices.go`) queues one send job per Notice, unique
  on the event and the address, which the System sender delivers; a failed send is retried on its own,
  and a retried event job queues nobody twice. On an Instance with no System sender nothing is
  queued.
- **The modules know no recipient and no sender.** `memberships.Notify`, `OwnerNotifier`,
  `suspension.NotifyOwners`, `suspension.ErrNoticeNotSent` and `internal/kernel/mailjobs` are gone. A
  suspension stands and its notice is retried until it is delivered; the abuse detector marks a
  warning and publishes it in one transaction; a Second factor reminder is claimed and published in
  one transaction, and is no longer released.
- **A module's own follow-up work is not an event.** A job a module queues for its own rows after a
  write (a saved Integration's quota lookup) is queued in the write's transaction
  (`events.Bus.WithinScopedEnqueueTx`), as `docs/architecture/events-and-jobs.md` (Async) requires, not published.

## Considered Options

- **Events only for the Workspace's changes, account mail kept in the handlers.** It would keep the
  envelope as it was, but the account's mail is most of what is mailed, and it would stay lost after
  a committed change and decided by the handlers.
- **A sender port the modules call, with retries behind it.** It would fix the lost mail, but every
  module would still choose who is told and what is said, and a new channel (SMS, Telegram, a page in
  the app) would be one more call in each.
- **A consumer per notice.** The copy of the notices is one concern, read by one person; a consumer
  per notice would scatter it and its tests.
- **Mint every token at the request and carry it in the event.** The request would do the same
  work, but the event, its job and its send job would all hold a live credential, and a password
  changed in between would not end a link that was not mailed yet.

## Consequences

- A notice is queued iff its change commits, and is retried until the System sender takes it. A
  command run from the shell (a suspension) leaves its notice to the server's job runner.
- A new channel for the notices is a change to the Notices' consumer and Notice delivery, not to the
  modules that publish.
- The Staff console no longer reports a suspension whose notice could not be sent: the notice is
  retried. `abuse_freeze_notified_at` records when the suspension's notice was issued.
- A test reads what a request published from the queued event jobs: the harness registers a
  consumer of its own that wants every live event, since an event no consumer wants yet leaves no
  job. `env.SystemMail.Messages()` works the Notices' jobs before a read.
