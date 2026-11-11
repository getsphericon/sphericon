---
status: accepted
---

# Outbound message projections follow events and keep the earliest observation

> **Amended by [[0120-engagement-tracking-is-consented-per-destination]]:** an observation is
> accepted only while its Destination's Engagement tracking consent allows tracking, and a `denied`
> removes the opens and clicks already kept on that Destination's messages.

A recorded human open or click, and an email Delivery notice, cause the Outbound message's
lasting report projection through an asynchronous consumer. The Event and the consumer's work
commit together; the report may lag behind the Event. The Outbound message module owns the
projection's implementation and its rules across all send surfaces and tracked Channels.

This amends ADR 0053's requirement to stamp engagement in the publishing transaction. Keeping
the Event projection synchronous preserves the facts read by Conditions, Rules and Automations;
the delayed stamps serve reports and exports, rather than permission to send.

## Decisions

- **First means earliest observed by this Instance.** Capture the observation time at tracking
  or callback interpretation and carry it through publication. Email callback times describe
  receipt by the Instance, not a provider timestamp. Never use worker execution time.
- **Handling order cannot change the result.** Each applicable field retains the minimum
  observation time. A later-handled earlier observation may move a timestamp earlier, without
  adding a second message to the count. Repeated handling leaves the result unchanged. A
  machine open or click remains an Event and sets no human engagement stamp.
- **Accepted measurements survive Erasure anonymously.** Work accepted before Erasure still
  completes against the anonymous Outbound message record. This includes tracked Transactional
  sends without a Contact. Retain only the scoped delivery reference, kind of measurement and
  observation time needed for that work, not the original personal event envelope. It must
  neither find nor recreate an erased Contact or restore a personal field or relationship.
- **SMS final state stays synchronous.** Its first final report decides whether an Event is
  new and therefore whether it is published. That transition is not this projection consumer's
  responsibility. Neither permission to send nor the send outcome is deferred by this decision.
- **Verification crosses the event seam.** Test publication and handling through the River
  adapter and the application's River test executor adapter: rollback, reversed job order,
  duplicate handling, machine exclusion, Workspace isolation and both orders against Erasure.
  Cover Broadcasts, Automations and tracked Transactional sends. No extra mock-only interface
  is needed; depth comes from concentrating projection policy in its owner.

## Considered options

- **Keep direct stamps at every entry point.** It couples tracking and callbacks to both the
  fact and its lasting projection. A consumer gives locality to the projection module and
  leverage to every producer through one interface.
- **Use the first job to run.** River promises no ordering, so the recorded time would depend
  on scheduling or retries instead of the observations themselves.
- **Purge pending measurement with the personal event.** It makes statistics lose an already
  accepted fact solely because Erasure happened before the projection was handled.

The project is greenfield. Implement this model directly across the producers, consumers and
tests; no compatibility path or data migration from an older behavior is required.
