# Design: internal domain-event system

Status: **implemented**. A domain event is published once, inside the transaction that commits the
state change, and runs mandatory subscribers in that transaction, then reaches asynchronous consumers as
River jobs (ADRs 0113–0115 extend ADR 0101, which
replaced the watermill-sql outbox of ADR 0019 and ADR 0022; ADR 0105 lets an event belong to the
Instance and makes the mail sphericon sends on its own behalf a consumer). This document says how
the pieces fit; the ADRs say why.

## What it does

1. **Decoupled fan-out**: one publish, any number of consumers. A producer never learns about a
   consumer: it calls `Publish` and nothing else.
2. **Transactional with the state change**: an event is published iff the state that produced it
   commits. Its receipt, optional Event row, subscriber writes and consumers' jobs share the producer's
   transaction. A subscriber error rolls them all back and permits a retry.
3. **Durable and retried per consumer**: each consumer's job is retried by River with backoff, on its
   own, without holding up the other consumers or the other events, and kept as `discarded` for 30
   days when its attempts run out, for an operator to retry from River UI.
4. **A Workspace's or the Instance's**: an event of a Workspace (`events.WorkspaceEvent`) carries
   it; an event of a User's account (a Sign-up, a password reset, a Passkey) belongs to no
   Workspace, and its envelope's Workspace is 0.
5. **Queryable log**: the Event table is the durable record the Activity timeline, Segments, Rules,
   Automations and analytics read.
6. **Typed**: our events are Go structs with a stable name and version.
7. **Retained publication history**: every accepted fact leaves journal metadata independently of
   River cleanup. Explicitly replayable report consumers retain their anonymous inputs (ADR 0116).

Event sourcing of aggregates is out of scope: aggregates are CRUD rows in ent, and events are a
reaction and log layer beside them.

## Two kinds of events, never conflated

- **Our** events are a closed, typed union (`ContactCreated`, `EmailEngagement`, `EmailDelivery`,
  `AuditEntry`, `LinkRequested`, `WorkspaceSuspended`, ...). An event that is an Event of its
  Workspace (`events.Projected`) owns its projection onto the Event row (`Project()`); any other (an
  Audit entry, a change told to the Workspace's people, an event of the Instance) has none. Bus
  names are finite and ours (`events.Names`).
- A **customer's** event (`page_view`, `added_to_cart`) is the customer's domain: opaque to us and
  stored as-is. Ingesting one is itself our typed event, `CollectedEvent` (`event.collected`), whose
  `Action` and `Properties` carry the customer's payload. A Trigger, a Rule and a webhook filter
  select on the projected action, never on the bus name.

## Publishing

```
producer (one transaction):
   write state rows
   Publish(ev) ── stamp the envelope (id, observation time, recorded time, chain)
               ├─ claim an externally identified fact: duplicate → stop here
               ├─ projected event: write its Event row (unique workspace_id + source_id)
               ├─ run matching synchronous subscribers, including their derived facts
               ├─ journal publication metadata and explicitly retained anonymous inputs
               └─ InsertManyTx: one River job per matching asynchronous consumer
commit
```

`events.Bus.WithinTx` (and its scoped variants) opens the transaction; the `Publisher` it hands out
does the rest. A request that changes no state of its own and only asks for something to happen (a
password reset, a verification link sent again) publishes in a transaction of its own
(`events.Bus.Publish`):

- **The receipt** claims an upstream identity before any subscriber or job runs, including for
  internal facts without an Event row. Its key is a digest of Workspace, event name and source id;
  it contains no raw destination or source payload. A failed reaction rolls the receipt back.
- **The Event row** is written for every Projected event, with its Contact link when available.
  A natural source id is namespaced by the semantic event name and is unique within a Workspace;
  storage, trigger lookups, Rule firings and Webhook identity use the same value. Internal SMS observations and state transitions have
  no Event row and reach no public Rule, Automation or Webhook; a new SMS opt-out of a Contact's
  number is projected as the public `sms.unsubscribed`, like `email.unsubscribed` and
  `telegram.unsubscribed`.
- **The subscribers** (`events.Subscriber`) run synchronously on the publishing transaction's
  scoped client. The composition root registers them; the bus imports no domain module. SMS stop
  and unreachable observations trigger their owners' consent and suppression writes. Only an
  actual change publishes the corresponding state-transition fact (ADR 0114).
- **The jobs**: each consumer is an entry of the consumer registry (`events.Consumer`): a filter that
  reads only the envelope and the event decoded from it, never the database, and the args of its
  job kind. The bus inserts the jobs of every entry that wants the event with one `InsertManyTx` on
  the producer's transaction. The entries are registered on the bus when the application is wired
  (`internal/app`): core's (`jobs.EventConsumers`) and the Enterprise Edition's
  (`ee.EventConsumers`), so the events module imports no consumer module.
- **The journal** records every accepted publication, even without a consumer or an Event row.
  It stores no original envelope, personal source key or request metadata. A consumer's explicit
  `Replay` policy retains only its versioned anonymous job args. The message observation and
  Broadcast attribution consumers are the initial allowlist. Metadata alone cannot reconstruct
  arbitrary historical typed payloads; retaining another input is an explicit policy decision.
- **After the commit**: what must happen once for a newly stored Event, outside any transaction, is
  registered on the bus with `OnStored` and runs after the commit, so neither a repeat nor a
  rolled-back Event reaches it: the send-outcome counter of a provider's bounce or complaint
  (`jobs.RecordSendOutcome`, an event subscriber of the job runner, which the wiring registers). The bus knows no domain module: the names it catalogs (`email.bounced`,
  `email.complained`) are its own, and `suppressions` refers to them.

A consumer that reads the whole event has args of its own that embed the envelope (`events.EventJob`),
so it reads the typed event, its version, its times and its chain, and its job runs in the `events`
queue. A consumer whose work the event alone decides (the automations and the rules) queues that
work itself: the evaluation, built from the event at publish. Report measurements instead carry only Workspace, message id, action and observation time,
without a personal envelope. Their owner keeps the earliest observation, regardless of handling
order (ADR 0113). There is no ordering promise between jobs: a consumer that writes shared rows takes
its own lock (orders takes one per Workspace).

## The consumers

| Consumer    | Job kind                      | Wants                                                                                       | Does                                                                                             |
| ----------- | ----------------------------- | ------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| suppression | `event_suppress`              | a hard bounce or a complaint                                                                | suppresses the address, once, keeping the first reason                                           |
| orders      | `event_order`                 | a collected order Event, loaded history included                                            | records the Order and its Movements under the Workspace's lock (ADR 0064)                        |
| automations | `automation_evaluate_trigger` | a live, person-made Event of a Contact (no history, no machine engagement, no Audit entry)  | evaluates the Event against the Automations                                                      |
| rules       | `rule_evaluate_event`         | what automations wants, except an event a Rule's Action published (`CausedBy`)              | evaluates the Event against the Rules; a Firing is unique on the source id                       |
| webhooks    | `event_webhook`               | every live Event and every Audit entry                                                      | queues one delivery per enabled endpoint whose filter matches (ADR 0039)                         |
| notices     | `event_notice`                | an event that calls for Notices: an account's, or a change a Workspace's people are told of | queues one `send_notice` per Notice, unique on the event and the address (ADR 0105)              |
| broadcasts  | `event_broadcast_unsubscribe` | internal `message.unsubscribed` naming a received message                                   | attributes a link or button opt-out through the delivery row, including after Erasure (ADR 0115) |
| messages    | `event_message_observation`   | human engagement or an email delivery measurement                                           | updates the anonymous message report stamps with the earliest observation (ADR 0113)             |
| audit (EE)  | `event_audit`                 | an Audit entry                                                                              | stores the entry, once, under the audit license (ADR 0022)                                       |
| export (EE) | `event_export_erasure`        | `contact.erased`                                                                            | adds the Erasure to the Erasures stream under the export license (ADR 0045)                      |

The Event row is written before any consumer runs, so an Automation's exit and goal checks and a
Rule's Condition that counts the Contact's Events see the Event that triggered them (#751).

The notices consumer asks the use case `internal/usecase/noticedelivery` for the Notices of the
event (who is told and what, read when the event is handled, a link minted then) and queues their
sends, which the System sender delivers, each retried on its own. Its events are the account's
(`account.go`) and the Workspace's changes its people are told of (`workspace_changes.go`); none is
Projected, so none reaches a Webhook endpoint (an Invitation's event carries its link's token).

A webhook's body is the public payload (`events.WebhookDeliveryOf`): id (the source id, so a receiver
drops a repeated provider notification), type (the projected action, or `audit.entry`), occurredAt,
workspaceId, subject, contactId and data, with the Staff identity of an Audit entry masked. An
endpoint with no event types receives every customer-facing event and no Audit entry.

## The chain and the trace

Every event carries a correlation id, shared by its whole chain, and a causation id: the event whose
handling published it. An event consumer's job runs in the context of handling its envelope
(`events.JobChain`, a River middleware: correlation from the envelope, cause = the envelope's id), so
an event it publishes, directly or through a job it queues, is caused by the handled one. A job
inserted while handling an event keeps that chain in its metadata.

River's OpenTelemetry integration is a plugin with trace propagation: every job carries the trace
context it was inserted in, and the span of its work links to the inserting span, so a trace leads
from the request to the webhook delivery.

## Failures and operations

```sh
sphericon events replay --workspace <public-id> --consumer event_message_observation \
  --from <RFC3339> --before <RFC3339> --dry-run
```

This validates and counts the retained inputs in the half-open recorded-time interval. Omitting
`--dry-run` queues that consumer's ordinary River jobs. `event_broadcast_unsubscribe` is the other
supported consumer. A selection over `--limit` (default and maximum 1000) is refused entirely:
narrow the interval. Unsupported consumers and input versions queue nothing. Replay never
publishes again, runs mandatory subscribers or fans out to other consumers. It reapplies
convergent measurements; it does not clear or reset a projection.

The selection is one query snapshot, not an advancing cursor or a commit-order promise.
Concurrent publications keep their live jobs. Journal metadata and retained anonymous inputs
have no automatic expiry; Contact Erasure preserves them and Workspace erasure removes its
history through the bus. Other consumers still see future publications only and retry existing
failed work through River UI.

- River's default attempts and backoff apply to event jobs; a discarded one is kept 30 days and
  retried from River UI.
- `river.queue.oldest_available_age` (seconds) and `river.queue.depth`, per queue, are the lag an
  operator alerts on (`SphericonJobQueueBacklog`); River's own metrics, the fetch timings included,
  come from the otelriver plugin.
- **Erasure** (ADR 0021) deletes, in its own transaction, the event jobs that name the Contact by id,
  address or phone (`QueuePurger.PurgeEventJobs`), in any state but running, and the queued webhook
  deliveries that name it. Anonymous report jobs survive: their measurements were accepted before
  Erasure and restore no personal fields. A running job finishes and re-checks what it refers to.

## Testing

Under go-txdb no River runtime works the queue, so a test asserts what a publish left: the Event rows
(`env.StoredEvents`) and the event jobs (`env.QueuedEnvelopes` reads jobs with an envelope,
one per event; `env.EventJobs(kind)` one consumer's; `env.JobsOf` also reads anonymous measurement jobs). The harness registers a consumer of its own
that wants every live event, so the queued jobs show everything a test published, an event no
consumer wants yet included. `env.SystemMail.Messages()` works the Notices' jobs before a read. A test works a consumer's jobs with `env.PerformEnqueuedJobs(t, kinds...)`, through
River's test executor with the application's River configuration, so the real middleware run; a
redelivery is `env.RetryJobs` (River's `JobRetry`) and another `PerformEnqueuedJobs`. The end-to-end suite runs the application with a real River client.
