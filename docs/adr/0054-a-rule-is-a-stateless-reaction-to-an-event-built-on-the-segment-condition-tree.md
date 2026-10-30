---
status: accepted
---

# A Rule is a stateless reaction to an Event, built on the Segment's Condition tree

> Amended by [[0065-a-live-automation-or-rule-changes-only-by-publish]]: a Rule is versioned, an edit
> writes a draft, Publish makes it live, and the lifecycle is `active` and `inactive`.

> **Amended by #749:** the Rules consumer stores the Event's row (the idempotent `Persist` the
> persist consumer also runs) before it queues the evaluation, so the row of the Event that fired a
> Rule exists when its Condition is evaluated, and an `event` leaf counting the Trigger event's
> action counts it. `trigger:<key>` leaves still read the properties the envelope carries.

A Workspace could react to an Event only by building an Automation: a sequence with an
Enrollment, a version and a once-ever fence, even to "tag a Contact who clicked this link" or "tell
the CRM". A **Rule** is the light primitive for that: when any of its Trigger events is recorded for
a Contact and its Condition holds, its Actions run at once. Anything with a delay, an email or a
sequence stays an Automation. The Rule belongs to the core (AGPL), like Segment and Automation.

## Decisions

- **One condition language.** The Segment's rule tree (ADR 0029) is named **Condition** and is
  shared: a Segment is a named Condition and a Rule holds one. A Rule may also test the triggering
  Event's own properties (`trigger:<key>`), allowed only in a Rule. The Event's row may not exist
  yet when a Rule is evaluated (the bus's persist and Rule subscribers are independent), so these
  leaves are evaluated in memory against the properties the envelope carries, and folded into the
  tree as constants; what remains compiles to the same ent predicates as a Segment's. Property
  keys have no catalogue (they come from ingest): any key is valid when saved, and a missing key
  or a value of another type makes that leaf false, not an error. No expression engine and no
  second language.
- **Stateless and forward-only.** No Enrollment, no delay, no email Action. The Condition is
  evaluated when the Event is handled, against the Contact's current data. A Rule fires on every
  matching Event; "once per Contact" is a Condition plus an apply-Tag Action. An Event with no
  Contact, a machine Event (ADR 0053) and an Event an Action published trigger no Rule. An Event
  published by an Action carries a causation marker on its envelope, stamped by the Action (none
  publishes an Event today). Mail from an Automation a Rule enrolled is caused by that Automation,
  not by the Rule; the once-ever Enrollment bounds it. Rules for one Event are independent, with no
  order between them.
- **At most once per Event, executed off the bus.** Like the Automation trigger, the bus consumer
  only enqueues an evaluation job (Contact, action, the envelope's id and properties). The job loads
  the active Rules for that action, evaluates each Condition, and for each that holds writes a
  **Firing** (unique on Rule and the envelope's id, the `source_id` persist uses, so a redelivered
  Event does nothing twice) and enqueues its Actions job in the same transaction
  (`WithinScopedEnqueueTx`). A Rule whose Condition is false leaves no Firing. The Actions job runs
  them in order, each independent: its result is stored on the Firing and a failed Action does not
  cancel the others. An HTTP call is the one Action that outlives the job: it is recorded `pending`
  and runs as a job of its own on the HTTP steps' queue, retried as an HTTP step is (ADR 0039: days,
  `Retry-After` honoured), so the Actions after it do not wait for it, and the Firing finishes when
  the last pending Action has its result. Enrolling into an Automation that is not active, or a
  Contact already enrolled, is recorded as skipped, not as a failure.
- **Actions today:** apply Tag, remove Tag, HTTP call (its template sees the Contact and the
  triggering Event), enroll in an Automation. Adding one is a new implementation of the Action seam;
  `internal/rules` imports none of the modules the Actions call.
- **Lifecycle.** `draft` and `active`; deactivating stops new Firings; a pending HTTP call of
  an existing Firing still runs to its end. A Rule has Firings, which are history, so removing one is Archive (ADR 0049) and only a
  Rule with no Firings is purged. A Tag, Segment or Automation used by an active Rule is not removed
  (`inuse`). A Firing is analytical: it expires with the retention window (ADR 0019) and goes with
  its Contact on erasure (ADR 0021). A dry run on a stored Event shows what would fire and runs and
  writes nothing.
- **Limits and access.** A cap on the Actions in one Rule (20, of which at most 5 call a URL) and on
  the size of a Condition, none on the number of Rules. A Rule acts only on Events recorded at or
  after its activation, so activating one never reaches back into the log; a dry run ignores that
  and tries a draft. Rules are Audited (ADR 0022) and edited by the roles that edit an Automation.
  They are reachable on `/site` and the external API, and so on MCP (ADR 0016).
- **Shape.** `internal/rules` takes a `*ent.Scoped` and holds no raw client; the shared tree and
  its compiler are one package used by Segment and Rule.

## Considered options

- **A rule engine library (GoRules ZEN, CEL, expr).** Rejected: our conditions already read the
  database through the scoped client and the Workspace's catalogue, which an in-memory engine does
  not, and it would be a second language beside the Segment's. ZEN is also a Rust core behind
  native bindings (zen-go ships `cgo.go`, and its platform list excludes linux-musl), where
  `internal/emaildoc` avoided cgo on purpose.
- **A Rule as a one-step Automation.** Rejected: an Enrollment, a version and a once-ever fence
  are the wrong semantics for "every matching Event".
- **A Rule that replaces the Automation's Trigger.** Rejected: it gives no action outside a
  sequence, which is the point.
- **A delay or a "run at the date in a property" Action.** Rejected: it makes the Rule stateful and
  an Automation by another name.
- **A separate library or module now.** Rejected: one consumer, and the core is tied to ent and the
  scoped client; the narrow package surface keeps extraction cheap later.

## Consequences

- Segment's stored `rule query` is renamed Condition in the contract, the schema and the glossary;
  greenfield, so no compatibility layer.
- Tag operations publish no Event today; chains stay impossible until they do, and an Action that
  starts to publish one must stamp the causation marker.
- Event-side leaves are evaluated in Go while Contact-side leaves run in SQL: one operator table
  serves both, and a test runs the same cases through each.
