---
status: accepted
build: [1500, 1604]
---

# Double opt-in: confirmation as a positive event, not a subscription status

> **Amended by [[0121-a-preference-page-per-destination-and-only-the-person-lifts-a-refusal]]:** a
> return from `everything` on the Preference page, on fresh proof of the address, records a
> Confirmation with provenance `double_opt_in`; the fresh link that page sends for an older link is
> a message the person asked for, in the mode below that `everything` does not stop.

> **Revised before it was built** (#1500): the flows that send the confirmation email, who asks
> for it and what that changes, the subscription-bombing limit, the email's kind, sender and
> content, and how the policy is turned on. CSV import's two modes are gone: ADR 0138 records an
> imported Confirmation only through the consent columns of the batch upsert, and an import sends
> nothing.

EU regulation requires, for many acquisition flows, a _confirmed_ opt-in: the contact must
actively confirm before marketing may flow, and the sender must be able to **demonstrate** that
consent (GDPR Art. 7). This is table-stakes for anyone sending to the EU — self-hosters included
— so it is **AGPL core**. Unlike ADRs 0011/0012 it needs no MIME/DKIM changes and ships
independently.

The hard part is polarity. The consent model (ADR 0001) is deliberately **subtractive**:
mailability is _derived_ from the **absence** of a (channel, destination)-keyed opt-out — there
is no positive "subscribed" record, and a contact status flag was explicitly rejected. Double
opt-in needs a **positive** fact. This ADR adds it **without** reintroducing a status flag, by
extending — not reversing — ADR 0001.

## Decisions

### Scope: one-time acquisition confirmation, not a consent state machine

Confirmation is a **transient, one-time** fact obtained shortly after acquisition; once an
address is confirmed it is just a normal address under the existing subtractive model. This ADR
deliberately does **not** build an ongoing consent-state machine — no consent versions, no
periodic re-confirmation, no consent TTL, no granular per-topic consent. Those are out of scope.

### The model: a positive Event + a derived gate row, mirroring Unsubscribe

The survival asymmetry that unlocks the design: ADR 0001 keys opt-outs by destination because
_losing an opt-out on delete+re-import re-mails a refuser — a violation_. Run the same test on a
confirmation: losing it forces **re-confirmation before mailing** — friction, not a violation. It
**fails safe**. So the reason that forces destination-keying on the negative fact does not bind
the positive one.

`Unsubscribe` already has the exact shape we need: `recordUnsubscribe` writes **both** a
destination-keyed row (the fast eligibility read-model) _and_ an `email.unsubscribed` Event (the
immutable log + bus signal), in one transaction. Confirmation is its positive mirror:

- a positive, (channel, destination)-keyed **Confirmation row** → the fast eligibility gate (a
  one-line EXISTS predicate, symmetric with the Unsubscribe NOT-EXISTS);
- written in the same transaction as an immutable **`marketing.confirmed` Event** carrying
  _when / how / provenance / IP_ → the source of truth, GDPR Art. 7 proof, and a bus signal (a
  confirmation can trigger a welcome Automation).

Every Confirmation has its Event, whatever its provenance. This is the honest answer to "no
subscribed flag": the truth is an **immutable Event**, never a mutable status column; the row is a
derived read-model — the ADR 0011 pattern (Events are truth, a derived read serves the hot path).

Rejected: **pure event-derived eligibility** (no row, EXISTS against the events table) — the
events table is huge and destination-correlation there needs a new index + normalization
discipline, whereas the small positive row is a trivial, cheap predicate and writing both a row
and an event is exactly what Unsubscribe already does (zero new architecture). Rejected: **a
Contact status flag** — the ADR 0001 hazard (lost on delete+re-import, conflates choice with
deliverability facts).

### Policy: a workspace-level switch, forward-looking

A single **workspace-level `require confirmed opt-in`** policy (default **off** = single opt-in)
drives the **gate**: email eligibility requires a Confirmation. Not per-source: enforcement is
uniform; acquisition nuance is carried by provenance, not a per-source gate. It also decides
whether an On-site form sends the confirmation email to a new address (ADR 0088); a confirmation
email asked for explicitly is sent whatever the policy (below).

**Only a person turns it on, in the console.** An Owner or Admin (`workspace:update`) turns it on
on the Workspace's settings, affirming in the same act that the Workspace's current mailable list
consented: that assertion is what makes the list mailable, and an agent has no knowledge of it to
give, as with the postal address (ADR 0156). `/api` and MCP read the policy on `GET /workspace`
and cannot change it. Turning it on or off is one Audit entry, with the number of Confirmations it
recorded.

**Enabling is forward-looking, never retroactively blocking** — flipping the switch must not make
the existing list un-mailable overnight. Enabling records a Confirmation with provenance
`grandfathered` for every email Destination of a Contact that is mailable at that moment (no
Suppression, no `everything` opt-out, no Confirmation yet), keeping the eligibility gate
_uniform_ ("Confirmation EXISTS" always) rather than a non-auditable time-based carve-out. "Did
this contact confirm?" is answerable per-contact from a record. Each one has its
`marketing.confirmed` Event, recorded as a **Backfill** (ADR 0044): history to segmentation, never a
trigger, so turning the policy on starts no welcome Automation and reaches no webhook. The records
are written before the policy turns on, and the last of them and the policy change share one
transaction, so no Destination mailable at that moment is left unconfirmed and no Broadcast
in flight skips its audience.

Turning it off keeps every Confirmation. Turning it on again grandfathers the Destinations mailable
at that moment, including those that did not confirm while it was on: the person turning it on
asserts it again, and the console says so before they do.

The policy needs a way to confirm: it cannot be turned on until the Workspace has its confirmation
email set up (below), refused with 409 `confirmation_email_required`.

### Import & provenance

An import never sends a confirmation email. A Contact imported or upserted through the API is
unconfirmed unless its row carries consent evidence, which records a Confirmation with provenance
`imported` (ADR 0138). Every Confirmation (row + event) carries a **provenance** enum —
`double_opt_in` (clicked — gold standard), `grandfathered` (policy-enablement), `imported`
(importer-asserted), `keyword` (a START on SMS, ADR 0077). The strongest provenance stands: a real
`double_opt_in` is never downgraded by a later `imported`.

### What sends the confirmation email: a Confirmation request

A **Confirmation request** asks for one confirmation email to a Contact's email address. Nothing
sends one implicitly: creating or upserting a Contact through `/api`, the console, the batch
upsert or an Identify sends nothing, whatever the policy. Those doors also serve migrations and
CRM syncs, where an implicit mail would be a re-permission campaign to a whole list, and an
Identify comes through the public collect key (ADR 0171).

A request is made by one of two parties, and the party decides what `everything` does:

- **The Workspace asks**: a Member from the Contact's page in the console, or `/api` and MCP
  (`POST /contacts/{contactId}/confirmation-requests`, a send-class operation under its own
  `confirmations:send` scope, and `mcp:send` over MCP). An `everything` opt-out or a Suppression of
  the address refuses it (409, naming which): soliciting re-permission from someone who left
  everything is itself a violation, and ADR 0121 puts Members, the API and MCP in one class that
  never undoes a refusal.
- **The person asks**: an On-site form submit (ADR 0088), the "send a new link" of an expired
  confirmation link, and the Preference page's fresh link (ADR 0121). Only Suppression stops it:
  returning after leaving everything goes through this email, as ADR 0088 has it for a form.

A request for an address that already holds a Confirmation sends nothing and answers that it is
confirmed. A request is accepted (202) and sent by a job through Outbound send; a per-source Hold
(ADR 0015) delays it as it delays a Broadcast, it never falls back to the System sender.

### The subscription-bombing limit

Confirmation emails are a **subscription-bombing** vector: a form submits a victim's address
repeatedly, from anywhere. A Destination receives **at most 3 confirmation emails per Workspace in
24 hours**, whoever asks, counted by the Instance's Postgres throttle (ADR 0025), the one that
holds the mail budgets of an address. The number is fixed, not configurable, as the Test send caps
of ADR 0041 are. Over it the Workspace's request gets 429; the person's request gets the same
answer as an accepted one, so a form says nothing about an address. The On-site form keeps its own
proof-of-work and collect limits (ADR 0088).

### The confirmation email

- **It is an Outbound send of its own kind, `confirmation`** (ADR 0015), the word ADR 0130 already
  gives its `Feedback-ID`. It has no Sending source, no unsubscribe footer or `List-Unsubscribe`
  (ADR 0012), no open pixel and no rewritten links, and it is outside the frequency cap and the
  sending window (ADR 0137, ADR 0134). Like any Outbound send it passes the freeze check, the
  Sending-domain gate and DKIM signing.
- **Its eligibility is a named mode of Send-eligibility**, chosen by who asked: Suppression and
  `everything` when the Workspace asked, Suppression only when the person did. Send-eligibility
  takes its mode as a closed type: marketing with its Sending source, transactional, and these two.
  An empty Sending source no longer stands for a mode, so a new kind of message that names no mode
  does not compile.
- **The Workspace sets it up once**, on the settings beside the policy: the From (an address on a
  verified Sending domain and a name), the subject and an introductory paragraph, with defaults in
  the Instance's language naming the Workspace. The rest is fixed by sphericon: the Confirm button
  carrying the link, the line that nothing happens if the person did not ask, the Workspace's
  postal address when it has one (ADR 0156 does not require it: this is not marketing). The link
  cannot be edited out because the author never writes it. The email and the confirmation page are
  in the Workspace's Brand kit, read live, as the Preference page is.
- **A request is refused** (409 `confirmation_email_required`) while the Workspace has no
  confirmation email set up.

### Mechanism & endpoint (reuses the ADR 0012 pattern)

The confirmation link carries a **signed JWT** (reuse the existing HS256 signer) encoding
workspace + destination + contact, with an **`exp` claim** (unlike the eternal unsub/tracking
tokens). The link is method-split: **`GET /e/confirm/{token}` redirects to the SPA page and records
nothing; `POST /site/confirmations/{token}` performs** the confirmation (writes the row + publishes
`marketing.confirmed`, one transaction). The page's explicit "Confirm" button issues the POST
through the generated site client; there is no POST on `/e/confirm`.

Scanner-safety is **load-bearing for legal validity here, not just UX** — and higher-stakes than
ADR 0012: a scanner GET that falsely _unsubscribes_ is annoying; a scanner GET that falsely
_confirms_ **fabricates consent you never had**, poisoning the exact audit trail double opt-in
exists to create. The explicit button-click _is_ the deliberate human act that makes the opt-in
valid.

**The confirmation lifts an `everything` opt-out recorded before its link was issued**: the person
acting on a link delivered to the address after they left is the person coming back (ADR 0121). It
deletes that Unsubscribe and records `email.resubscribed`, then the Confirmation. An opt-out
recorded after the link was issued wins, and the click changes nothing (ADR 0154). A per-source
opt-out is not lifted: the person returns to everything else and keeps the narrower refusal.

### Eligibility ordering: negatives dominate

The gate is the **lowest-priority layer**, after the three negative layers (Suppression,
`everything`, source) — so confirmed-but-suppressed and confirmed-but-unsubscribed both correctly
don't send. An **`everything` unsubscribe invalidates the Confirmation row** (deletes the gate
row; the immutable event preserves "confirmed at T1, left at T2"), so returning after a full
opt-out requires re-confirmation — stale consent never silently reactivates. A _per-source_
unsubscribe does not invalidate confirmation (narrower than leaving entirely).

### Expiry & purge

- The confirmation **token expires** after 7 days. The page of an expired link offers to send a
  new one, a request the person makes: the expired token, still validly signed, names the
  Destination and the Contact.
- Never-confirmed requests are **purgeable** (data minimization: no consent, don't retain PII
  forever) — recorded as a **principle**; the mechanism composes with the GDPR-tooling work
  (erasure/retention), not a second retention engine here.
- **No ongoing re-confirmation / consent expiry** — a confirmed address stays confirmed until it
  unsubscribes.

## Relationship to ADR 0001

This **extends, does not reverse** ADR 0001. It adds a fifth, _positive_ eligibility layer, but
preserves 0001's core principle: the source of truth is an immutable Event plus a derived
destination-keyed row — there is still **no stored subscription status on the Contact**. The
positive layer is optional (off by default) and evaluated last, so single-opt-in workspaces see
the unchanged subtractive model.

## Consequences

- A `Confirmation` and a `Confirmation request` in the glossary, and a `marketing.confirmed` Event,
  backfilled when it grandfathers; the policy and the confirmation email on the Workspace's
  settings, with `requireConfirmedOptIn` read-only on the external `GET /workspace`.
- Send-eligibility takes a closed mode instead of an empty Sending source; Outbound message gains
  the kind `confirmation`.
- `POST /contacts/{contactId}/confirmation-requests` on `/api` and MCP, the same action on a
  Contact's page in the console, and a request from an expired link's page.
- An agent that drives the app meets the "block when the consent basis is unclear" gate of the
  Email Marketing Bible where a Workspace turns the policy on, and sees whether it is on; with it
  off, email stays single opt-in by the Workspace's choice.

## Considered options

- **Sending on every Contact create while the policy is on.** It mails whole lists from a
  migration or a CRM sync, and lets anyone with the collect key make the Instance mail an address.
  Notifuse, an open-core peer, also lets imports and API writes skip double opt-in.
- **Letting the API's request pass `everything`, as a customer's own sign-up form stands behind
  it.** The API cannot show that a person asked, and a Member would reach through a token what the
  console refuses. The person comes back through an On-site form or the Preference page.
- **A Template for the confirmation email.** A Template edited later could lose the link, and the
  check would be a rule at runtime. A fixed frame around the Workspace's words keeps the link by
  construction.
- **The System sender for a Workspace that cannot send.** It would make the platform's domain mail
  any address typed into a form.
- **Grandfathering without an Event, or with a live one.** Without one, the row is no longer
  derived from the log; a live one starts the welcome Automation for the whole list.
- **The Transactional kind for the confirmation email.** Its counts and reports would mix the
  customer's mail with sphericon's, and its mode would be the empty Sending source this ADR retires.
