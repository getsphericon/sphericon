---
status: accepted
---

# A domain event is a River job for each consumer that wants it

Domain events reached their consumers through a transactional outbox (watermill-sql). One row per
event was read by eight consumer groups (persist, automations, rules, webhooks, suppression,
orders, audit, export), each through an offset cursor of its own. Three of them only turned the
event into River jobs, so most events crossed two queues.

The outbox needed its own code for what River already does: retries and a dead-letter table,
replay, a lag metric, a prune bounded by the slowest cursor, and propagation of the trace context.
Its delivery rule (a row is read only once every transaction older than it has finished, so that a
cursor never skips a late commit) made every consumer wait behind the oldest open transaction. Yet
no consumer depends on the order it buys:

- persist and suppression are idempotent;
- an order Event may come before its `Order Completed` (ADR 0064);
- the River jobs of automations, rules and webhooks already run in any order;
- audit deduplicates by event key (ADR 0022).

We drop the outbox. Publishing an event inserts, in the producer's transaction, one River job for
each consumer that wants it. This is how Rails Event Store, Laravel's queued listeners and Spring
Modulith's event publication registry deliver to asynchronous handlers: a job or a row per
handler per event, with no ordering promised. Each of them leaves something to the application:
Rails Event Store, by default, and Laravel, with `after_commit`, enqueue after the commit and lose
the job if the process stops in between, while Spring Modulith has no retry scheduler and no row
claiming. River has all three: an insert that commits with the state, retries with backoff, and
rows claimed with `SKIP LOCKED`.

## Decisions

- **The producer fans out.** A registry, built at startup from core and the Edition (the
  Enterprise audit and export consumers register as they do now), names every consumer: its job
  kind and a filter that reads only the envelope (not loaded history, not machine engagement, has a
  Contact, not caused by a Rule's Action, is an order Event, and so on). Webhooks gets one fan-out
  job per event, which finds the matching endpoints and queues their deliveries. The producer never
  queries endpoints.
- **`Publish` writes the Event row, then the jobs, in the caller's transaction.**
  - For a projected event the row in `events` is written first.
  - A `source_id` already stored marks a repeat (a provider redelivery): no job is inserted for it.
  - The jobs of every consumer whose filter accepts the event are then inserted with one
    `InsertManyTx`.
  - An error comes back from `Publish` and rolls the transaction back.
  - The persist consumer, and the `Persist` call automations and rules made first so that an
    evaluation would see its own Event (#751), are gone.
- **Every job carries the whole envelope.** The Event row holds a projection. Webhook bodies,
  audit and export read the typed event, its version, `RecordedAt`, `CausedBy` and the chain ids,
  which only the envelope has.
- **One job kind per consumer, in the `events` queue.** Webhook deliveries keep their own queue.
  Consumers run concurrently and in any order. A consumer that writes shared rows takes its own
  lock (orders already does).
- **Failures are River's.**
  - Jobs use River's attempts and backoff.
  - A job past its attempts is `discarded` and kept 30 days (`DiscardedJobRetentionPeriod`).
  - An operator retries it from River UI or with `JobRetryTx`.
  - The dead-letter table, its replay and `sphericon events replay` are removed.
- **Tracing and the chain.** The otelriver middleware is registered as a plugin, so its hook
  metrics are emitted, with `EnableTracePropagation` on. The trace-context headers the outbox
  carried are removed. The correlation and causation ids keep travelling in job metadata
  (`JobCorrelation`).
- **One lag metric for every queue:** the age of the oldest available job, per queue. It replaces
  the outbox lag. River itself reports no queue depth.
- **Erasure deletes queued event jobs** whose envelope names the Contact, discarded ones included,
  with `JobDeleteManyTx` and a predicate on the args. That replaces the purge of outbox rows and dead
  letters (ADR 0021). A running job still finishes and re-checks what it refers to.
- **A new migration drops** the outbox, offsets and dead-letter tables. No row is carried over.
  watermill leaves the dependencies.

## Considered Options

- **Keep the outbox.** It gives one row per event on the tracking path, commit order and a 7-day
  window a new consumer can read back. The cost is a second queue, its own retry, lag and prune
  code, and delivery held by any long transaction, for an order nothing uses.
- **Jobs carry only the Event id.** That would keep addresses out of the job table, but webhook
  bodies, audit and export would have to be rebuilt from the projection, which changes the public
  payload.
- **Every job inserted unconditionally, each worker filtering.** The code is simpler, but every
  event writes a job for each consumer even when none of them acts.
- **Collect the jobs and insert them once before commit.** It saves round trips, but an error then
  surfaces far from the `Publish` that caused it.

## Consequences

- **The tracking request now writes, in its own transaction, what the consumers used to write
  later.** For a human open with a Contact that is the Event row, the automations and rules jobs and
  one webhook fan-out job, in place of the one outbox row.
  - Across the whole path the number of rows barely changes: the outbox row goes and the fan-out
    job comes.
- A consumer added later sees only the events published after it is deployed. The `events` table
  stays the durable record, and a one-off job can replay it when a consumer needs the history.
- The outbox part of ADR 0019 (its prune and floor) no longer applies. River's job cleaner keeps
  completed jobs for its default period.
- Tests assert what is now true: the `events` rows and the inserted jobs of a kind. They run a
  consumer's job with `rivertest`, not through an outbox.
