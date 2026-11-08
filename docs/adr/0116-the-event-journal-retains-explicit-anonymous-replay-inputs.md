---
status: accepted
---

# The event journal retains explicit anonymous replay inputs

River jobs deliver events, but their retention is not a history of accepted facts, and the Event
log holds projections rather than every internal or Instance fact. The bus journals each accepted
publication in its producer's transaction, including one no consumer wants. Delivery remains
River's: the journal is history, not another queue or outbox. This extends
[[0101-a-domain-event-is-a-river-job-for-each-consumer-that-wants-it]] without restoring its global
cursor. Aggregates remain CRUD rows, not event-sourced state.

## Retention and payloads

- Every entry keeps the random event id, type, version, optional Workspace and occurrence and
  recording times. A null Workspace means an Instance fact. The bus owns the table and explicitly
  confines its Workspace operations; domain modules gain no raw-client access.
- Only a consumer explicitly registered for replay retains its versioned input. Its normal job
  args must be anonymous, safe to keep through Contact Erasure and convergent under repetition.
  Initially these are `event_message_observation` and `event_broadcast_unsubscribe`, whose delivery
  references and observations already survive Erasure (ADRs 0113 and 0115).
- The original envelope is never copied. It may contain personal destinations, arbitrary customer
  properties, provider references, a credential (the Invitation token, ADR 0105), or a client-supplied
  request id. Even an Erasure's envelope has downstream identifiers deliberately absent from its
  Event projection (ADR 0021). None is retained by the journal.
- Entries have no automatic expiry. This adds no retention to personal Event rows or River
  envelopes. Contact Erasure preserves the same anonymous inputs the live report jobs preserve.
  Workspace erasure drains its journal in bounded transactions, then removes any tail with its
  jobs in the final tombstone transaction. Instance history stays.
- Publication takes a share lock of its Workspace before journaling. The final Workspace purge
  takes the update lock before deleting history and jobs, and holds it through the tombstone:
  earlier publishers finish before the final delete; later ones cannot recreate erased history.
  Replay holds the share fence and requires a live, undiscarded Workspace.

Metadata-only entries cannot reconstruct an original event for an arbitrary future consumer.
Retaining another input requires an explicit payload, versioning and Erasure decision.

## Replay

An operator chooses one allowlisted consumer, an existing Workspace and a half-open interval of
recording times. The bus selects at most 1000 matching entries from one query snapshot, checks
their input versions and scopes, and queues their ordinary River jobs in one transaction. A
smaller limit is allowed; an oversized selection is refused in full. A dry run validates and
counts the same selection without queuing work. Unsupported consumers, input versions and queue
failures commit no prefix.

Replay calls neither `Publish`, synchronous subscribers nor general fan-out. It can reapply a
report without mailing, calling Webhooks, starting Automations or changing consent. The initial
consumers converge under repetition and arbitrary order, retain the accepted observation time
and treat a missing or foreign delivery row as a no-op. Repeated commands may queue repeated jobs;
the owner's conditional writes make them safe.

This is reapplication, not destructive projection reset: an existing earliest timestamp cannot
move forward. It repairs missing applications of retained facts, not arbitrary erroneous state.
Sequence ids and recording times are not commit order. There is no advancing cursor: a replay
selects committed rows visible to its query, while concurrent publications retain their live
jobs. A new eligible consumer must be live before its supported history is replayed. Resumable
rebuilds and further historical consumers require their own semantics.

## Considered options

- **Archive every envelope.** It retains secrets and personal data beyond their existing lifetime
  and needs another complete Erasure and subject-export model. Explicit anonymous inputs solve
  the current report recovery need without creating that archive.
- **Treat the Event projection as the original event.** It loses internal and Instance facts and
  the message attribution Erasure removes; it cannot restore accepted anonymous report inputs.
- **Publish history again.** Mandatory subscribers would change current consent and unrelated
  consumers could repeat external actions. Targeted replay keeps those paths separate.
- **Restore the cursor-based outbox.** It adds a second scheduler and its commit-order problem.
  River remains responsible for execution, retry and failed jobs.
