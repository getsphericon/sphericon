# Design: internal domain-event system

Status: **P0 implemented** (contact.created on the bus, end-to-end). Author date:
2026-06-28. Library facts verified via gh/npm on 2026-06-28. Decisions resolved
2026-06-28: outbox approach **A**, outbox rows **retained**, and — revised during
the spike — **plain Publisher/Subscriber, not the CQRS component** (see Decisions).

## Why

We want an internal event system in the spirit of Rails Event Store: a domain
event (e.g. `contact.created`, `email.opened`, `broadcast.sent`) is published
once, and any number of independent subscribers react. Producers must not know
their consumers. This is the backbone for:

- **Automations** — enrolling contacts is a _subscriber_, not a direct call.
- **Engagement log** — persisting events to the `Event` table is a subscriber.
- **Future**: outgoing webhooks, analytics/reporting projections, audit trail —
  each is just another subscriber, added without touching producers.

Today this is ad-hoc and tangled: events are written with `Event.Create` in three
places (tracking, collect, external), and the Phase-4 automation trigger calls a
river enqueuer directly from the handlers. That couples producers to one consumer
and scatters the "what happened" concept. This design replaces that.

## What it must do (requirements)

1. **Decoupled fan-out** — one publish, N subscribers, each independent.
2. **Transactional with the state change** — if a contact is created, the
   `contact.created` event must be published iff the row commits. No lost events
   (committed state, no event) and no phantom events (event, rolled-back state).
   This is the hard requirement and the main design driver.
3. **Durable + at-least-once** — survive a crash; redeliver on consumer failure.
   ⇒ subscribers must be **idempotent**.
4. **Workspace-scoped** every event carries `workspace_id`.
5. **Queryable log** — the engagement timeline (Contacts → activity) reads stored
   events; analytics will too.
6. **Typed events** in Go — structs, not `map[string]any`, with a stable name +
   version.

**Explicitly NOT required now:** aggregate event-sourcing (rebuilding Contact /
Broadcast state by replaying events). Our aggregates are CRUD rows in ent; events
are a _reaction + log_ layer, not the source of truth. This is the key scoping
decision — it rules out heavyweight ES frameworks.

## Options considered ("or is something else a better fit?")

| Option                        | Maintained (2026-06)   | Fit                                                                                                                                   | Verdict                                                        |
| ----------------------------- | ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------- |
| **watermill** (have it)       | v1.5.2, 9.8k★, active  | Pub/sub framework; Postgres backend (watermill-sql, already a dep); CQRS component for typed events; Forwarder for the outbox pattern | **Recommended**                                                |
| Hand-rolled channel bus       | —                      | Trivial in-process fan-out                                                                                                            | ✗ not durable, no persistence/replay, lost on crash            |
| river as the bus (have it)    | v0.39, active          | Insert one job per subscriber                                                                                                         | ✗ it's a job _queue_, not fan-out pub/sub or a log; wrong tool |
| looplab/eventhorizon          | v0.17.0, 1.7k★, active | Full CQRS+ES: aggregates, projections, event store                                                                                    | ✗ overkill — we don't event-source aggregates                  |
| hallgren/eventsourcing        | v0.9.1, 279★, active   | Focused ES lib (aggregate streams)                                                                                                    | ✗ same — assumes event-sourced aggregates                      |
| modernice/goes, go-eventually | 160★ / 102★, active    | ES toolkits                                                                                                                           | ✗ same family, smaller                                         |
| EventStoreDB client           | archived-ish           | External event-store server                                                                                                           | ✗ another server to run; breaks minimal self-host              |

**Conclusion:** watermill is the right tool and it's already in the stack — but as
a **plain typed event bus with a transactional outbox**, not its CQRS/ES-heavy
cousins. Full event-sourcing frameworks solve a problem we don't have (and the
user stepped back from). river stays — but for _durable execution_ (send email,
wait, fan-out), which a subscriber triggers. Clean split:

> **Event** = "something happened" (watermill, fan-out, logged). **Job** = "do
> this durable/scheduled work" (river, retries). A subscriber reacts to an event
> by enqueuing a job.

## Architecture

```
producer (ONE ent tx):
   write state row  +  append outbox row   ── atomic ──▶ commit
                                   │
                          (relay: watermill-sql subscriber reads outbox topic)
                                   ▼
                         watermill router (consumer groups)
                          ├─▶ persist      → Event table (engagement log/projection)
                          ├─▶ automations  → match active automations, enqueue river RunStep
                          └─▶ (later) webhooks / analytics / audit
```

### Transactional outbox (the load-bearing piece)

To satisfy requirement 2, producers do **not** publish to a broker directly.
Inside the same ent transaction that writes the state change, they append a row
to an **outbox table**. A relay then forwards committed outbox rows to the bus.
This is the standard transactional-outbox / RES "append to stream" pattern and is
why watermill-sql fits: its SQL publisher/subscriber treat a Postgres table as a
topic with consumer offsets, so the outbox table _is_ the watermill topic.

**Chosen: approach A** — watermill-sql publisher on the ent tx connection.
Publish using the same `*sql.Tx` ent is using, so the message insert is in the
state tx. Least code, and the outbox table _is_ the watermill SQL topic. The P0
spike proves we can hand watermill-sql ent's transaction executor.

> Fallback **B** (if the tx handoff can't be wired): an ent-owned `OutboxEvent`
> entity appended in the ent tx + a relay (watermill Forwarder or a small river
> periodic job) that publishes unsent rows and marks them sent. More code, fully
> DB-portable, no ent↔watermill-sql tx glue.

### The `Event` table is a projection, not the bus

Clarify the vocabulary so we don't conflate them:

- **Domain event** = the bus message (typed struct, transient, fanned out).
- **`Event` row** = a _projection_ written by the `persist` subscriber — the
  durable engagement log the UI/analytics read. It is one consumer of the bus,
  not the bus itself.

(If we later want full replay-to-new-subscriber, the outbox table is the durable
stream; the `Event` projection can be rebuilt from it.)

## Event taxonomy & envelope

Name: `aggregate.verb` (past tense): `contact.created`, `contact.unsubscribed`,
`email.opened`, `email.clicked`, `broadcast.sent`, … The automation `trigger_event`
matches this name — so the taxonomy and the trigger vocabulary are the same thing.

Envelope (typed in `internal/events`):

```go
type Envelope struct {
    ID          string         // ULID; idempotency key for consumers
    Name        string         // "contact.created"
    Version     int            // schema version of Payload
    WorkspaceID int64
    SubjectID   int64          // usually contact id
    OccurredAt  time.Time
    Payload     json.RawMessage
}
```

Each event also has a typed Go struct (`ContactCreated{WorkspaceID, ContactID,
Email}`) marshaled into `Payload`. Consumers unmarshal by `Name`+`Version`.

## Cross-cutting

- **Idempotency** — at-least-once ⇒ consumers dedupe on `Envelope.ID` (or natural
  keys: automation enroll already check-then-inserts; persist can upsert).
- **Ordering** — per-subject ordering is nice but not required for MVP; document
  as best-effort.
- **Versioning** — additive payload changes bump `Version`; consumers handle known
  versions. No in-place breaking changes.
- **Poison messages** — bus retries with backoff, then dead-letter (watermill
  middleware); never block the topic.
- **Testing** — naturally inert under go-txdb: the outbox insert rolls back with
  the test's transaction and the watermill router isn't running, so no subscriber
  fires. Engine/consumer logic is unit-tested by calling the handler directly
  (same approach we use for `jobs.SendBroadcast` / `jobs.EvaluateTrigger`).

## How this changes current code (migration)

1. New `internal/events`: envelope, typed events, a `Bus` (publish within an ent
   tx) + subscriber registration over watermill.
2. Producers publish instead of doing the work inline:
   - contact create → publish `contact.created` (drop the inline `Event.Create` +
     direct `trigger.OnEvent`).
   - tracking open/click/unsub → publish `email.*` (drop inline `Event.Create` +
     `trigger.OnEvent`).
   - collect / external event ingest → publish the custom event.
3. Subscribers:
   - `persist` → writes the `Event` row (replaces the three scattered
     `Event.Create` sites with one consumer).
   - `automations` → the Phase-4 `EvaluateTrigger` logic, now triggered by the
     bus instead of an injected enqueuer; it enqueues river `RunStep`.
4. Retire the `apisite.AutomationTrigger` injection seam and the demo watermill
   handlers (`log_contact_created`, `send_welcome_email` → re-express
   `user.registered` as a subscriber or a river job).

river, the automation run engine, and the broadcast engine are unchanged — only
_how the trigger fires_ moves from a direct call to a bus subscription.

## Phasing

- **P0 (spike) — DONE**: transactional publish (approach A) proven + one
  subscriber end-to-end (`contact.created` → persist `Event`). `internal/events`
  (Envelope, Bus.WithinTx, persist subscriber, the outbox migration); contact-create now
  publishes through the bus; tests cover commit/rollback atomicity, the projection
  mapping, and live router delivery.
- **P1 — DONE (core)**: automation enrollment moved onto the bus (an `automations`
  subscriber that enrolls via the river client when the envelope carries a
  `ContactID`); the `apisite.AutomationTrigger` seam is deleted. Email engagement
  (`email.opened/clicked/unsubscribed`) now publishes through the bus
  transactionally; the `persist` subscriber writes their `Event` rows. **Deferred:**
  (a) the collect/external custom-event producers still write `Event` inline — they
  have no contact id and don't enroll automations, so moving them buys nothing yet;
  (b) **idempotency** — delivery is at-least-once and `persist` has no dedupe column,
  so a crash-redelivery can write a duplicate `Event` log row. Harm is low (counts
  are producer-side), so it's its own follow-up: add a unique `source_id` column +
  `OnConflict(...).Ignore()` upsert.
- **P2 — DONE (idempotency + webhooks)**: `persist` is idempotent via a unique
  `Event.source_id` (envelope ULID) + `OnConflictColumns(...).Ignore()`. Outbound
  **webhooks** shipped: `WebhookEndpoint` entity + site CRUD/UI, a `webhooks`
  subscriber that fans each event out via the river client to matching endpoints,
  and a delivery worker that signs (Standard Webhooks) and POSTs with retries.
  Security/signing are **maintained libs, not hand-rolled**: `doyensec/safeurl`
  (SSRF, resolved-IP checks) + `standard-webhooks` (interoperable signatures).
  Webhooks currently see `contact.created` + `email.*`.
- **DONE — collect/external on the bus via a typed union.** A first attempt was
  reverted (it tried to make a customer's runtime-named event one of _our_ types,
  forcing a flat envelope + field-bag). The shipped model below is the correct one.

  **Never conflate internal and external events.**
  - _Our_ events are a **closed typed union** — `ContactCreated`, `EmailOpened/
    Clicked/Unsubscribed`, … — each a Go type that owns its projection (`Project()`).
    Bus event names are finite and ours.
  - A _customer's_ event (`page_view`, `added_to_cart`) is the **user's domain, not
    ours**: we know nothing about it and store it **as-is** (opaque action string +
    properties + identity).
  - The bridge: ingesting a customer event is itself _our_ typed event,
    **`CollectedEvent`** (bus name e.g. `event.collected`), whose body carries the
    user's opaque payload. The user's `page_view` is a **field** (`Action`) inside
    `CollectedEvent`, not a bus event name — so there are no "unknown" bus names and
    no catch-all is needed. collect and external both publish `CollectedEvent`.
    Shipped: persist/automations/webhooks `Decode()` the envelope and use
    `Project()`; automations enroll and webhooks filter on `Project().Action` (the
    customer action), not the bus name.

- **Later**: analytics subscribers; per-subject ordering if needed.

## Decisions (resolved 2026-06-28)

1. **Outbox approach A** — publish via watermill-sql on ent's transaction
   connection, so the outbox insert rides the same tx as the state change. The P0
   spike proves the tx-executor handoff; fall back to B only if it can't be wired.
2. **Outbox rows are retained** (not pruned after dispatch). Replay-to-new-subscriber
   is not built now, but keeping the rows immutable leaves the door open and the
   table is cheap. Add a pruning job later if it grows.
3. ~~CQRS component~~ → **plain Publisher/Subscriber + generic Envelope**
   (revised during the P0 spike). watermill's `cqrs.EventProcessor` registers one
   handler per _compile-time_ event type, but our core subscribers don't work that
   way: `persist` writes **every** event, and `automations` match a **runtime**
   `trigger_event` string — including open-ended custom events from the collect
   API that have no Go type. Neither can be a per-type CQRS handler. So a single
   generic `Envelope` on one topic + plain pub/sub is the right shape; typing stays
   producer-side via the `DomainEvent` interface marshaled into the payload.

### Implementation notes (P0, learned in the spike)

- **Consumer group per subscriber.** All subscribers share the one outbox topic,
  so each must run under its **own consumer group** (its own offset cursor) to get
  fan-out; a shared group would make them _compete_ for messages.
  `events.RegisterSubscribers` builds one watermill-sql subscriber per consumer.
- **Outbox schema in the migrations.** The tx publisher can't self-initialize (a CREATE
  TABLE would implicitly commit the tx), so a migration
  (`migrations/*_domain_events_outbox.sql`) creates the message + offsets tables, like the
  rest of the schema.
- **Idempotency is still a gap.** Delivery is at-least-once; on crash, `persist`
  could double-write an `Event` row (no dedupe column yet). Acceptable for P0;
  P1 adds a unique source-id column and upsert. Tracked as an open item.
