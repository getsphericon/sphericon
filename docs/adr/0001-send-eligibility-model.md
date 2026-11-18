---
status: accepted
---

> **Amended by [[0163-a-workspaces-control-group-receives-no-marketing]]:** a Contact in the
> Workspace's Control group receives no marketing message, a layer of its own.

# Send-eligibility: (channel, destination)-keyed Suppression + scoped Unsubscribe, no contact status

> **Amended by [[0186-a-run-of-soft-bounces-suppresses-a-destination]]:** a run of soft bounces is a reason of Suppression (`soft_bounce_run`).

A Contact has no "subscribed/unsubscribed" status. Whether a message on channel C from
sending source S may reach destination D (the channel-specific address — email, phone,
messenger id) is decided in layers: (1) (C, D) in the **Suppression** list → never;
(2) (C, D) **Unsubscribed** from _everything_ → never; (3) (C, D) **Unsubscribed** from S →
never; (4) otherwise send. Transactional messages skip layers 2–3 but still respect
Suppression. Consent is per-channel — an email opt-out never silences SMS. Targeting is always
a Segment; suppression and unsubscribe only _subtract_ from a send.

A sending source is automatic, never hand-authored — it _is_ the sender: each Automation is
its own unsubscribe scope, all Broadcasts share one ("broadcasts"), plus a reserved
"everything" scope. The default in-email unsubscribe link is scoped to the source; "from
everything" is a separate, deliberate action, so a per-source unsubscribe never silently
loses the whole contact.

Both Suppression and Unsubscribe are **keyed by (channel, destination) per workspace, not by
contact id**, with a nullable `contact_id` kept only for display. This is the deliberate,
surprising part: it is what lets an opt-out survive contact deletion + re-import, which we
require for GDPR erasure — the opt-out is the minimum data retained to honor the refusal.
A contact flag would silently re-enable mailing to someone who unsubscribed the moment they
are deleted and re-imported.

Suppression is global and a compliance ratchet (bounce/complaint do not auto-clear);
Unsubscribe is per-(channel, destination, sending source) and toggleable. The two stay
separate concepts (not one table) despite both being destination-keyed opt-outs.

Keying by `(channel, destination)` rather than bare email is laid in now even though only the
email channel is built: SMS is already reserved on Integration, Contact already carries phone,
and re-keying the one compliance-critical store after the fact is expensive. The destination
(email / phone / messenger id) carries its channel so the same registry serves every future
channel without reshape.

## Considered options

- **A flag on the Contact (`status`/`unsubscribed`).** Rejected: lost on delete+re-import
  (compliance hazard), and conflates a contact's marketing choice with provider deliverability
  facts (bounce/complaint) that may arrive for addresses with no contact at all.
- **Hand-authored Topics as the consent unit.** Rejected: nobody wants to author consent
  categories; the natural, automatic scope is the sender itself — each Automation is its own
  scope, all Broadcasts share one. (See [[GLOSSARY.md]] "Sending source".)
- **Mailing Lists.** Rejected entirely: Lists fuse container + targeting + consent. We keep
  them split — pool = Contacts, targeting = Segment, consent = per-source Unsubscribe.

## Consequences

- The "is this contact unsubscribed?" shown in the UI is **derived** (a lookup against
  unsubscribe/suppression by the contact's destinations), never a stored contact field.
- Segment rules filtering on deliverability compile to a NOT-EXISTS against the
  suppression/unsubscribe tables (same shape as the existing event-correlation rules),
  rather than a column predicate on `contacts.status`.

## Amendment: one rule, evaluated per message at send time

The layers above are implemented **once**, as a rule keyed by (channel, destination) — not by
Contact, because a Transactional destination may have no Contact. Audience selection for a
Broadcast uses a batch form of that same rule (a join from Contact to destination), and the
authoritative check is repeated for every message at send time and fails closed, so an
unsubscribe or Suppression that lands after planning still takes effect. Address normalization
happens in one place. See [[0015-outbound-send-single-chokepoint]].
