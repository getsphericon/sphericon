---
status: accepted
---

# One Contact identity: absorb the tracking profile, attach events by stable id

> **Amended by [[0128-two-contacts-of-one-person-are-merged-by-a-deliberate-act]]:** an Upsert whose
> keys resolve to two Contacts no longer fails: it writes to the first by `subject_id`, `email`,
> `phone`, leaves the key another holds unfilled and records a Duplicate candidate; two Contacts of
> one person are joined by a deliberate Merge.

There is a single person record per workspace — the **Contact** — covering both the
behavioral/CDP identity and the marketing audience member. The separate tracking profile is
absorbed into it. A Contact is anchored by a stable internal id, with `subject_id`
(the customer's own user id), `email`, and `phone` as alias keys (each unique per workspace,
any of which may be absent). It owns its Visitors (anonymous devices), and is the thing
Events attach to and the thing campaigns are sent to.

**Events attach to a Contact by stable identity resolved at ingest, never by email.** This is
the core fix. The previous model joined Contact↔Event on the email string, which (1) made all
pre-Identify anonymous activity invisible to segmentation, and (2) discarded the multi-visitor
/ email+phone alias resolution the tracking side had already done. Identity-keyed attachment
plus stitching anonymous Events onto a Contact at Identify time makes the full behavioral
history available to segments.

Anonymous / un-consented people are a **state of the one Contact**, not a separate table.
This composes with [[0001-send-eligibility-model]]: mailability is derived, not a stored
status, so anonymous Contacts in the audience table are harmless — they simply never pass
send-eligibility.

## Considered options

- **Keep two tables (TrackingProfile as CDP staging + Contact), just join events by
  `subject_id`.** Rejected: preserves duplicate identity and a Profile↔Contact sync problem,
  and "join by subject_id" breaks import-only Contacts, which have no subject_id at all. The
  real work is one identity _resolved across every source_ (import, API, form, tracker), not
  a join-column swap.
- **Create a Contact for every anonymous Visitor eagerly.** Not decided here; the Contact
  may exist before identity (anonymous state) but the promotion policy (when a Visitor
  becomes a Contact) is an implementation detail left open.

## Consequences

- `Contact.email` becomes optional (anonymous Contacts have none); identity is multi-key
  (subject_id / email / phone), as the old tracking profile already was.
- Segment event-conditions compile against the identity link, not `event.email`.
- The `prospect` flag is dropped (it was write-only and never read).
- Events remain immutable and append-only, with denormalized identity snapshot fields kept
  for debugging only — the authoritative link is the stable id.

## Note: clearing alias keys

A Contact need not keep any alias key (subject_id / email / phone), so an update may clear
the last remaining one with an explicit null (JSON Merge Patch); such a Contact simply
becomes anonymous-like. The API does not enforce "at least one alias key".
