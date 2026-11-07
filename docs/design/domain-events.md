# Design: internal domain-event system

Status: **implemented**. A domain event is published once, inside the transaction that commits the
state change, and reaches each consumer that wants it as a River job of its own (ADR 0101, which
replaced the watermill-sql outbox of ADR 0019 and ADR 0022; ADR 0105 lets an event belong to the
Instance and makes the mail sphericon sends on its own behalf a consumer). This document says how
the pieces fit; the ADRs say why.

## What it does

1. **Decoupled fan-out**: one publish, any number of consumers. A producer never learns about a
   consumer: it calls `Publish` and nothing else.
2. **Transactional with the state change**: an event is published iff the state that produced it
   commits. Its Event row and its consumers' jobs are written in the producer's transaction, so a
   rolled-back change leaves neither, and an error while publishing rolls the change back.
3. **Durable and retried per consumer**: each consumer's job is retried by River with backoff, on its
   own, without holding up the other consumers or the other events, and kept as `discarded` for 30
   days when its attempts run out, for an operator to retry from River UI.
4. **A Workspace's or the Instance's**: an event of a Workspace (`events.WorkspaceEvent`) carries
   it; an event of a User's account (a Sign-up, a password reset, a Passkey) belongs to no
   Workspace, and its envelope's Workspace is 0.
5. **Queryable log**: the Event table is the durable record the Activity timeline, Segments, Rules,
   Automations and analytics read.
6. **Typed**: our events are Go structs with a stable name and version.

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
   Publish(ev) ── stamp the envelope (id, recorded time, chain)
               ├─ projected event: write its Event row (unique source_id)
               │      the row exists already → a repeat: stop here, publish nothing
               └─ InsertManyTx: one River job per consumer whose filter wants the envelope
commit
```

`events.Bus.WithinTx` (and its scoped variants) opens the transaction; the `Publisher` it hands out
does the rest. A request that changes no state of its own and only asks for something to happen (a
password reset, a verification link sent again) publishes in a transaction of its own
(`events.Bus.Publish`):

- **The Event row** is written for every Projected event (not for an Audit entry, a change told to
  the Workspace's people or an event of the Instance), with the projection, the resolved Contact
  link (left empty when the Contact is gone) and the source id: the event's natural upstream id
  (`DedupKey`: a provider's notification id, a client's event id) or its own envelope id. The
  insert skips a conflict on `source_id`, so a repeated event (a provider's redelivery, a client's
  retried event id, within one batch or across requests) is found by the conflict and is not
  published again: no consumer sees it twice.
- **The jobs**: each consumer is an entry of the consumer registry (`events.Consumer`): a filter that
  reads only the envelope and the event decoded from it, never the database, and the args of its
  job kind. The bus inserts the jobs of every entry that wants the event with one `InsertManyTx` on
  the producer's transaction. The entries are registered on the bus when the application is wired
  (`internal/app`): core's (`jobs.EventConsumers`) and the Enterprise Edition's
  (`ee.EventConsumers`), so the events module imports no consumer module.
- **After the commit**: what must happen once for a newly stored Event, outside any transaction, is
  registered on the bus with `OnStored` and runs after the commit, so neither a repeat nor a
  rolled-back Event reaches it: the send-outcome counter of a provider's bounce or complaint
  (`app.RecordSendOutcome`, which the wiring registers). The bus knows no domain module: the names it catalogs (`email.bounced`,
  `email.complained`) are its own, and `suppressions` refers to them.

A consumer that reads the whole event has args of its own that embed the envelope (`events.EventJob`),
so it reads the typed event, its version, its times and its chain, and its job runs in the `events`
queue. A consumer whose work the event alone decides (the automations and the rules) queues that
work itself: the evaluation, built from the event at publish. There is no ordering promise between jobs: a consumer that writes shared rows takes
its own lock (orders takes one per Workspace).

## The consumers

| Consumer    | Job kind                      | Wants                                                                                       | Does                                                                                |
| ----------- | ----------------------------- | ------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| suppression | `event_suppress`              | a hard bounce or a complaint                                                                | suppresses the address, once, keeping the first reason                              |
| orders      | `event_order`                 | a collected order Event, loaded history included                                            | records the Order and its Movements under the Workspace's lock (ADR 0064)           |
| automations | `automation_evaluate_trigger` | a live, person-made Event of a Contact (no history, no machine engagement, no Audit entry)  | evaluates the Event against the Automations                                         |
| rules       | `rule_evaluate_event`         | what automations wants, except an event a Rule's Action published (`CausedBy`)              | evaluates the Event against the Rules; a Firing is unique on the source id          |
| webhooks    | `event_webhook`               | every live Event and every Audit entry                                                      | queues one delivery per enabled endpoint whose filter matches (ADR 0039)            |
| notices     | `event_notice`                | an event that calls for Notices: an account's, or a change a Workspace's people are told of | queues one `send_notice` per Notice, unique on the event and the address (ADR 0105) |
| broadcasts  | `event_broadcast_unsubscribe` | `telegram.unsubscribed` that names a Broadcast and its Contact                              | counts the unsubscribe on the Broadcast's recipient, once (ADR 0102)                |
| audit (EE)  | `event_audit`                 | an Audit entry                                                                              | stores the entry, once, under the audit license (ADR 0022)                          |
| export (EE) | `event_export_erasure`        | `contact.erased`                                                                            | adds the Erasure to the Erasures stream under the export license (ADR 0045)         |

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

- River's default attempts and backoff apply to event jobs; a discarded one is kept 30 days and
  retried from River UI.
- `river.queue.oldest_available_age` (seconds) and `river.queue.depth`, per queue, are the lag an
  operator alerts on (`SphericonJobQueueBacklog`); River's own metrics, the fetch timings included,
  come from the otelriver plugin.
- **Erasure** (ADR 0021) deletes, in its own transaction, the event jobs that name the Contact by id,
  address or phone (`QueuePurger.PurgeEventJobs`), in any state but running, and the queued webhook
  deliveries that name it. A running job finishes and re-checks what it refers to.

## Testing

Under go-txdb no River runtime works the queue, so a test asserts what a publish left: the Event rows
(`env.StoredEvents`) and the event jobs (`env.QueuedEnvelopes` reads every event job's envelope,
one per event; `env.EventJobs(kind)` one consumer's). The harness registers a consumer of its own
that wants every live event, so the queued jobs show everything a test published, an event no
consumer wants yet included. `env.SystemMail.Messages()` works the Notices' jobs before a read. A test works a consumer's jobs with `env.PerformEnqueuedJobs(t, kinds...)`, through
River's test executor with the application's River configuration, so the real middleware run; a
redelivery is `env.RetryJobs` (River's `JobRetry`) and another `PerformEnqueuedJobs`. The end-to-end suite runs the application with a real River client.
