---
status: accepted
---

# A Destination's consent history is read from its evidentiary Events

A Workspace asked by a regulator or by a person "what consent do you hold, and did you honour my
opt-out?" has the facts but no place that shows them together. Consent facts are keyed by
Destination: a Confirmation with its provenance and evidence, an Unsubscribe with its scope and
provenance, a Suppression, an Engagement tracking consent (ADR 0120). Each rests on an evidentiary
System event that Retention never deletes. One Contact's Data export already carries them all as
JSON (ADR 0021). An Erasure deletes the consent proof of a person who is gone (`marketing.confirmed`,
`sms.confirmed`) and keeps what refuses: Unsubscribes, active Suppressions and an Engagement tracking
`denied`. A User's Consent record is about legal documents, not a Contact.

## Decision

- **A Contact's page shows a consent history for each Destination whose consent rows name it**: the
  addresses and numbers it has now, and those it had, a key dropped by a Merge (ADR 0128) or an email
  an Update replaced included, since those are the opt-outs a person asks about. In time order: its
  Confirmations with their provenance and evidence (wording, source, brand), its Unsubscribes with
  their scope and provenance, its returns from an opt-out, its Suppressions with their cause and any
  lifting, and its Engagement tracking consent decisions. Above them stands its state on each Channel
  now, derived from Send eligibility as the CSV export's Channel state is (ADR 0138), so the two never
  disagree.
- **It is read from what is already kept**: the consent rows, which hold the Destination, the time and
  the provenance, and the evidentiary Events that name the Destination, directly (`marketing.confirmed`
  carries the address) or through the Outbound message they answer (an unsubscribe). A fact with no
  Event, such as an opt-out from an unknown sender (ADR 0076), stands on its row alone; a past opt-out
  whose row a return removed stands on its Events. No model of its own is added.
- **Tracker consent is not in it**: it belongs to a Visitor's browser, not to a Destination.
- **The Privacy request lookup by address shows what still stands, not only counts.** It already
  counts what a Workspace holds for an address under `contacts:erase`; it now also lists the opt-outs,
  Suppressions and Engagement tracking `denied` that outlive an Erasure, for a person whose Contact was
  erased or never existed. A phone number typed is hashed and matched by its hash, as an erased number
  is kept (ADR 0021), and is never shown back. It stays in the app: `contacts:erase` is token-only on
  the API and absent from Connections (ADR 0016).
- **There is no separate report for a regulator.** The Data export of the Contact is the document,
  and the history links to it.
- **The API reads it beside the SMS consent it already reads.** `GET /contacts/{id}/sms-consent`
  gives an SMS Confirmation and opt-out under `contacts:read`; the history is a read of its own beside
  it, under the same scope, and the wording and source an import or the API brought are marked
  `@untrusted` (ADR 0132). Reading it is not audited, as reading a Contact is not; the lookup keeps the
  auditing it has.

## Consequences

- The Contact page gains the history per Destination, past ones included; the Privacy request lookup
  lists what stands; the consent resource gains the history beside the SMS consent.

## Considered options

- **A consent ledger of its own.** It would copy what the evidentiary Events already prove.
- **A regulator's report in a new format.** The Data export carries the same facts.
- **Showing an erased person's past Confirmations.** An Erasure deletes them on purpose (ADR 0021).
