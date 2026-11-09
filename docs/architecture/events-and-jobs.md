# Events, jobs and Notices

[Architecture](../architecture.md) → Events, jobs and Notices

## Notices

**Notices (ADR 0105)**: every mail sphericon sends on its own behalf, through the System sender, is
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

## Transactional reactions

**Transactional reactions (ADR 0114)**: a publish first consumes a stable external fact's
identity, including a nonprojected fact, by a scoped digest in `event_receipts`. Synchronous
subscribers (`events.Subscriber`, registered by the wiring) join the publishing transaction
and call the domain owners; an error rolls back state, receipt and asynchronous work. The
event module imports no domain owner. `OnStored` remains after-commit and is not this seam.
A repeated external observation never runs mandatory writes again after a later reversal.
Transition reactions follow only a real state change, not every observation.

## Event history and replay

**Event history and replay (ADR 0116)**: the bus journals every accepted publication's metadata
in its transaction, independently of River retention. `EventJournalEntry` belongs to the bus;
its Workspace is optional for Instance facts, so the owner's operations explicitly scope their
reads and deletes. Only consumers with a `Replay` policy retain versioned anonymous job args,
initially message observations and Broadcast unsubscribe attribution. The original envelope,
personal source keys and arbitrary request ids are never copied. `Bus.Replay` selects one
supported consumer and a bounded recorded-time interval of one Workspace, validates versions and
scope and queues its normal jobs atomically, under the Workspace's key-share lock, refusing one
deleted or erased; it never publishes or runs mandatory subscribers. An operator names the
Workspace by its id (`sphericon events list` and `replay`): the Event history use case
(`internal/usecase/eventhistory`) resolves it through `workspaces`, since the bus touches a
Workspace row only to lock it, and reads the Instance's own facts when no Workspace is named. History
has no automatic expiry. Contact Erasure preserves anonymous report inputs; Workspace erasure
drains its history in batches, then fences publication and replay's `FOR KEY SHARE` locks with
the Workspace's `FOR UPDATE` row lock
while purging any final history and jobs and committing the tombstone.

## Async jobs

**Async**: one queue, river, for jobs and domain events alike (ADR 0101,
[event flow](../design/domain-events.md)). `events.Bus` publishes an event inside the transaction of
the state change: it writes the Event row (a repeated Workspace-scoped `source_id` publishes nothing) and
inserts one job per consumer whose filter wants the envelope (`events.Consumer`, registered in
`internal/app`: `jobs.EventConsumers` and the Enterprise runner's `ee/jobs.EventConsumers`). A producer never names a
consumer; a new consumer is an args type (embedding `events.EventJob` when the whole envelope
is needed), a worker and a registry
entry, its jobs in the `events` queue. Anonymous report jobs carry only Workspace, delivery reference,
measurement kind and observation time and survive Contact Erasure (ADRs 0113 and 0115); full
personal envelopes keep their purge rules. Every event job runs in the context of handling its
envelope (`events.JobChain`), so what it publishes is caused by that event.
`internal/jobs` uses river (Postgres-backed queue, `database/sql` driver, polling). A job that must
exist iff a state change commits is queued in that transaction with
`events.Bus.WithinScopedEnqueueTx` (a Broadcast's claim and its send job); never claim first and
enqueue after, with a best-effort undo. River keeps a job's args after it ran (a day once
completed, 30 days once discarded) and a waiting one still runs, so every job of a Workspace names
it at the top-level args field `workspaceId`: its args embed `jobkind.Workspace`, or, for an event
job, the envelope carries it. A job of the Instance (a periodic sweep, a Notice to a User) names
none. The erase of a deleted Workspace deletes every job that names it, any kind, queue or state but
running, in its last transaction (`events.WorkspacePurger`), and a test of `internal/kernel/jobkind`
fails on a new args type that neither names a Workspace nor is listed as the Instance's. The job
runner keeps both ends: every insert refuses a job of a Workspace whose Workspace is 0
(`jobs.BaseConfig`), and a job of an erased Workspace is cancelled before its worker runs
(`jobs.WorkerConfig`), so no consumer writes into a Workspace that is gone.
