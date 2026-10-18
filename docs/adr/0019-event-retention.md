# Event retention: analytical Events expire, evidentiary Events do not

Events and the domain-event outbox grew without bound. We prune them in core with a configurable
per-instance window (default 400 days, `0` disables) that deletes only **analytical** Events;
**evidentiary** Events (consent proof, permanent bounces/complaints, unsubscribes) are kept
indefinitely as the audit and dispute record (GDPR consent proof, "why was this address
suppressed"). Suppression itself does not depend on them: it is a materialized registry that
survives Event deletion. Broadcast reports read the recipient rollup, not live Events, so
deleting analytical Events does not change them; Complaint/Bounce rates (ADR 0011) read a short
live window far inside the retention default. The
outbox is pruned separately and more aggressively: rows strictly below the lowest consumer-group
cursor and older than a floor (7 days), since the `events` table is the durable record and
`source_id` uniqueness already dedupes redelivery. ADR 0009's "pruned" referred to metering only;
this is the actual policy.

Consequence: segment conditions that look back past the window are capped by it, and this is
documented as a product limit, not a bug. Deleting an Event is irreversible, so the window is an
self-hoster choice with a documented default rather than a hidden constant.

## Considered options

- **Time partitioning with drop-partition** (rejected): a unique key must include the partition
  key, which breaks `events.source_id` (the at-least-once dedupe) and the outbox primary key
  `(transaction_id, offset)`. Revisit only if `events` reaches hundreds of millions of rows.
- **Delete by age regardless of kind** (rejected): would destroy consent proof and the audit
  trail behind every Suppression and unsubscribe.
- **Documentation-only, self-hosters run `DELETE`** (rejected): unsafe against the outbox consumer
  cursors and leaves every self-hosted instance growing forever.
