---
status: accepted
---

# Two Contacts of one person are merged by a deliberate act

A Contact is found by its alias keys, `subject_id`, `email` and `phone`, each unique in the
Workspace (ADR 0002). An Upsert resolves the first one that matches, in that order, and fills the
keys the Contact lacks. When a key it would fill belongs to another Contact, the unique index refuses
it, and the whole write fails (`enrich` fills the key, and the refusal is `ErrEmailTaken` or its
kin). When the first Contact already holds a different value, the key that names the other Contact
is ignored without a trace. That is how duplicates arise and stay: an On-site form makes a Contact
by its email, the customer's app later makes one by its `subject_id`, and the Identify that carries
both keys is refused.

Consent facts (Unsubscribe, Confirmation, Engagement tracking consent) are keyed by Destination, not
by Contact; an Unsubscribe or a Confirmation also names the Contact it belongs to, for display. A
Confirmation is deleted with its Contact (ADR 0021). Two doors already settle a conflict without
failing: an On-site form does not attach a phone another Contact holds and still succeeds (ADR
0086), and a phone confirmed by a message goes to the Contact that holds it (ADR 0088).

## Decision

### Merging is a deliberate act

- **Two Contacts are merged only by a person in the app or by an explicit call of the API with an
  API token**, never by itself when an Identify finds two Contacts. A merge cannot be undone and joins
  two people's data; an Identify from a browser runs on the public tracker key, so automatic merging
  would let anyone join someone's profile to another, the risk ADR 0086 closes for On-site forms.
- **It needs a new Permission, `contacts:merge`**, held by the owner and admin Roles, and an API
  token needs the matching scope.
- **It cannot be undone, so it is previewed**: what moves, which keys are dropped, which
  Enrollments end.

### A key conflict no longer fails the write; it names a Duplicate candidate

- **An Upsert whose keys resolve to two Contacts resolves the first by the order `subject_id`,
  `email`, `phone`, leaves the key another Contact holds unfilled, and writes the rest.** The pair is
  recorded as a **Duplicate candidate**, which the team sees on both Contacts and in a list, and
  merges in one action or dismisses as two people; a dismissed pair is not raised again. An Import
  row does the same. This is the rule ADR 0086 already has for a form's phone, made general. A Create,
  or an Update that sets a key another Contact holds, is still refused: there the caller asserts the
  key for this Contact.
- **A candidate records the door that raised it**: the tracker or an On-site form through the
  public key, the API by a token, an Import. The list and the merge preview show it, since a pair a
  browser raised is a claim anyone could make. There is at most one candidate per pair, and each
  needs two real keys of existing Contacts, so no further bound is set.
- **Only key conflicts raise candidates.** No fuzzy matching on names or other attributes. A browser
  can raise a candidate, and nothing more: the merge stays a person's act.
- **A merge moves the candidates.** A candidate that named the merged Contact names the survivor,
  and one that would pair the survivor with itself goes away. An Erasure of either Contact removes
  its candidates.

### What the merge does

- **The person chooses the Contact that stays, the survivor; the other is merged into it and
  deleted.**
- **Keys**: the survivor keeps its own and takes the other's into empty slots. Where both hold a key
  with different values (two emails), the person picks one.
- **A dropped key becomes a former key of the survivor.** It resolves nothing, receives nothing and
  is no Destination, but a Contact later made with it raises a Duplicate candidate with the survivor,
  so the next form submit with the dropped address does not bring the duplicate back unseen. Former
  keys are the survivor's personal data: its Erasure removes them, and its Data export includes
  them.
- **Consent stays with the person.** The other Contact's Unsubscribes, Confirmations and every
  other consent fact that names it point at the survivor, whichever address it keeps, before the other is deleted, so no Confirmation is lost to the
  delete. They stay keyed by their Destinations, so a dropped address keeps its opt-outs and its
  proof of consent.
- **Refusals travel to the kept Destination, consents do not.** Every opt-out of a dropped
  Destination (an Unsubscribe of any scope, an Engagement tracking `denied`) is recorded again on
  the Destination the survivor keeps on that Channel, with the provenance `merge`; narrowing needs no
  proof from the person (ADR 0121). A Confirmation or a `granted` is not copied: it is proof for the
  address it was given on. So a person who opted out through one address is not mailed at the other
  because their Contacts were merged.
- **Moved Enrollments send to the survivor's Destinations**, under the survivor's consent facts,
  from their next Step on.
- **Attributes**: the survivor's core attributes and Custom fields win; the other fills those the
  survivor lacks. Tags are joined.
- **History moves**: the other's Events, Visitors, Orders, Outbound messages, Captured messages,
  Bindings, Start tokens and Rule Firings move to the survivor; none of them collides, since none is
  unique per Contact. Where both are Recipients of one Broadcast, the other's row keeps its Destination and
  loses its Contact, as on Erasure, so the Broadcast's figures stay and its audience is not rewritten.
- **Enrollments**: where both have an active Enrollment in one Automation, the other's ends as
  `exited`, for the reason `merged`. Every other Enrollment of the other moves to the survivor, active
  ones included, so `once`, the cooldown and an Enrollment's ordinal (ADR 0127) read both histories.
  A moved Enrollment whose entry key the survivor already holds (the same triggering Event, the same
  date occurrence, ADR 0126) keeps its history without that key: the survivor's entry stands for
  both.

### What it records

- **One evidentiary System event, `contact.merged`**, on the survivor: the merged Contact's id, the
  door of the act (a User or an API token, and which), when, and which slots dropped a key, never
  the key itself. It holds no address or number, so it stays proof without personal data, as
  `contact.erased` does (ADR 0021). Webhook endpoints receive it, so a customer's own copy can
  follow.
- **No other Event.** The moved Tags and fields record no `contact.tagged` or
  `contact.field_changed`, as an Import does not: they are earlier state, not something happening
  now. A merge fires no Rule and enrolls no one.

### Ordering

- **A merge takes both Contacts' locks, the lower id first**, so two merges of the same pair, or of
  overlapping pairs, wait for each other instead of deadlocking. In one transaction it ends the
  conflicting Enrollments, moves the rows, records `contact.merged` and deletes the other Contact.

## Consequences

- The Upsert stops failing on a key held by another Contact; Identify, ingest and Import write what
  they can and record a Duplicate candidate.
- A Duplicate candidate is a stored pair with its cause and a dismissed state.
- The contract and MCP gain the merge and the candidates; the Permission catalogue gains
  `contacts:merge`.
- `contact.merged` joins the evidentiary System events and the Webhook event catalogue; opt-outs
  gain the provenance `merge`; a Contact gains its former keys.
- An Erasure or a Data export of the survivor covers what was merged into it.

## Considered options

- **Merging on Identify** (Segment, Customer.io). It turns any browser that can call the tracker
  into a tool for joining two people's data.
- **Failing the write on a conflict, as today.** It loses the rest of the Identify and leaves the
  duplicate with no trace.
- **Fuzzy duplicate detection.** It guesses, and a wrong merge cannot be undone.
- **Unmerge.** The moved rows interleave with the survivor's, and later Events would have to be
  split by a guess.
- **Recording the moved Tags and fields as changes.** It would fire Rules and enroll Contacts on a
  housekeeping act.
- **Deleting the dropped address's consent rows.** An opt-out must outlive the record it came from,
  as it outlives an Erasure (ADR 0021).
