---
status: superseded in part by ADR 0101 (queued work is River jobs: the outbox rows and dead letters it purged no longer exist)
---

# GDPR erasure and subject-access export: delete is erasure, Events are the one append-only exception

> **Amended by [[0141-a-destinations-consent-history-is-read-from-its-evidentiary-events]]:**
> the lookup by address also lists the opt-outs, Suppressions and tracking denials that outlive an
> Erasure, matching a phone number by its hash.

Workspaces sending to the EU need Art. 17 erasure and Art. 15/20 export. Competitors split (Mailchimp
anonymizes reports, Klaviyo keeps a contact-info record, Customer.io suppresses hashed identifiers,
Segment propagates asynchronously); we pick the smallest mechanism consistent with ADR 0001 and the
Event model. Both are **AGPL core** (table-stakes for EU senders); retention policies (auto-purge,
data residency) are a separate backlog item. Age-based Event pruning is already core (ADR 0019); this ADR
adds the per-person path that ADR 0019's "evidentiary Events are never deleted" must yield to.

## Decisions

- **Delete is erasure.** One primitive, no soft variant that keeps PII. Single Contact only;
  accepts a contact id or an email, or a `visitor_id` for an anonymous Visitor with no Contact.
  Workspace action (the Workspace is the controller): Site UI (owner/admin only, enforceable in core
  through `scopedWithRoleFor`) and External API with a new `contacts:erase` scope, separate from
  `contacts:write`; MCP inherits it (ADR 0016). This replaces the existing CRUD contact DELETE
  (and its `contacts:write` scope); there is no second delete path.
  The Site UI erases from a Contact's page and from the Workspace's **Privacy requests** section
  (owner/admin), which takes an address: a lookup first shows counts only (never content; "nothing
  held" is a valid lookup result, so it answers with zeros rather than a 404), then the same erasure
  module as the External API runs by email. Erase is offered only while a Contact or delivery
  records hold the address: opt-outs alone are not erasable, so a lookup after an Erasure shows
  just the kept opt-outs. The Contacts list has no delete action.
- **Events: the one exception to append-only, and to ADR 0019's "evidentiary Events are kept".**
  Customer-tracked Events of the Contact are deleted. Every reserved system Event is anonymized in
  place (`contact_id`, `visitor_id`, `email`, `phone`, `subject_id` and `properties` cleared, except the two non-personal keys the rates are computed
  from, `sendingDomain` and `bounceKind`, which are kept): the
  `email.*` family (`sent`, `opened`, `clicked`, `bounced`, `complained`, `unsubscribed`) so
  complaint/bounce rates (ADR 0011) and metering (ADR 0009) are unchanged, and `contact.created`.
  `marketing.confirmed` is deleted, not anonymized: it is consent proof for a person who is gone and
  carries IP and provenance. Suppression and Unsubscribe do not depend on the Events (ADR 0019), so
  the refusal stays honored. Events are found by `contact_id` and by the Contact's visitor ids
  (Identify stitches earlier anonymous Events onto the Contact, so `contact_id` already covers them).
- **What survives:** `Unsubscribe` and `Suppression` rows keep their plaintext `destination`
  (ADR 0001), `contact_id` cleared (on SMS the number's hash instead, see the amendment below). `Confirmation` is deleted (fails safe: re-confirmation).
  `OutboundMessage` / `BroadcastRecipient` lose their destination and `contact_id`; for a destination
  with no Contact (a transactional send), erasure by email anonymizes `OutboundMessage` rows by
  destination. Visitors, tags, field values and
  `AutomationRun`s are deleted. Domain-event outbox rows and river job args for the Contact are
  cleared; application logs carry no email/phone; backups are not rewritten (they age out; the
  rotation window is documented and a restore re-applies erasures).
- **Synchronous, one transaction**, which also cancels in-flight work: active runs and unsent
  broadcast recipients of the Contact are removed, and send workers re-check the Contact exists.
  A message already handed to the provider is not recalled.
- **Proof without PII:** an immutable system Event `contact.erased` (Workspace, time, requester,
  identifier kind; stored with no email and no contact id) is the accountability record. Its
  webhook delivery additionally carries the Contact's `subject_id` (the customer's own id), or the
  sphericon contact id when there was none, so the customer can erase downstream copies; the
  identifier lives only in the delivered payload, never on the stored Event. No direct downstream
  integrations. In EE the Audit log additionally records the requester's delete.
- **Not blocked on re-creation.** A returning person (Identify, import) becomes a new Contact;
  eligibility still refuses mail through the surviving opt-outs. A tombstone would itself be PII.
- **Export:** one Contact, streamed JSON download, no job and no file storage. Contents: Contact,
  Custom fields, Tags, Visitors, Events, Unsubscribe/Suppression/Confirmation for its Destinations,
  `OutboundMessage` and `BroadcastRecipient` metadata; no rendered bodies. Readable by anyone with
  contact read access. The document is a typed model in the contract (`ContactExportDocument` and
  its members, declared once in `typespec/common/contact-export.tsp`), not the stored rows: each
  member is a goverter projection of its entity, so the ent JSON tags are never the public format.
  ogen can only stream an `io.Reader` for a binary body, so the download is declared as
  `application/octet-stream` and the same operation lists the `application/json` document variant
  for typed clients; the server always streams the first, encoding each typed member as it reads
  keyset pages. A test checks that every stored column of a member has a DTO field or is named as
  deliberately left out, because goverter cannot notice a withheld column.

## Considered options

- **Hashing retained destinations** (Customer.io): rejected. Unsalted email hashes are reversible
  by guessing, so it is pseudonymization only, and it complicates the send-path lookup.
- **Blocking re-creation** (Customer.io, Mailchimp): rejected, see above.
- **Async regulations with status** (Segment): rejected until a real contact's volume demands it.
- **Mutating Events only by hard delete** (no `email.*` anonymization): rejected, it would move
  deliverability and usage numbers after every request.

## Amendment: a phone number leaves only its hash (SMS, #1032)

Erasing a Contact with a phone leaves nothing of its SMS life but the number's hash, the lowercase
hex SHA-256 of the E.164 number that the Erasure record already keeps as its phone hash:

- **SMS opt-outs and Suppressions** of the Contact, and those of its number that name no Contact (a
  STOP from a number before any Contact had it), keep refusing the number, but keyed by that hash
  instead of the number (`phone.Hash`). Every reader on sms matches a stored number or its hash: the
  eligibility layers (in SQL, for one Check and for an Audience alike), STOP, START and the Contact
  card. So a person imported again with the number stays opted out until a START lifts it, and a
  START lifts the hashed opt-out. A lifted Suppression of the number goes. Email keeps the plaintext
  address above; the hash is the exception the SMS spec asked for, and like the hashing rejected
  below it is pseudonymization only: phone numbers are few enough to enumerate, so the hash keeps
  the number out of the rows, not out of reach.
- **SMS Confirmations** of the number go with the Contact's, those that name no Contact included
  (recorded before any Contact had the number, by the API or a START), and so do their
  `sms.confirmed` Events.
- **SMS Outbound messages** lose their destination and the provider's reference (Twilio's SID,
  which names the number and the text in Twilio's log), and the number where Twilio's words on a
  refused text quote it. A delivery report that arrives after the Erasure finds no message and
  records nothing. An anonymized `sms.delivered` or `sms.undelivered` Event loses its source id,
  which carries the SID. `sms.received` (the person's words) is deleted; `sms.sent`,
  `sms.delivered`, `sms.undelivered` and `sms.clicked` are anonymized like their email twins; the
  short links of the Contact's texts are deleted. A Test send's links name no message and carry
  sample data only, and a Test send keeps no number.
- **Queued work**: the outbox envelopes and webhook deliveries that name the number (a contact-less
  `sms.confirmed`) are purged with those that name the Contact.
- **Twilio's copy is not in sphericon's control.** sphericon does not delete messages in Twilio:
  Twilio's message log keeps the number and the text under the Workspace's own account, and the
  Workspace erases them there.
