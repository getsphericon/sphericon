---
status: accepted
---

# Double opt-in: confirmation as a positive event, not a subscription status

> **Amended by [[0121-a-preference-page-per-destination-and-only-the-person-lifts-a-refusal]]:** a
> return from `everything` on the Preference page, on fresh proof of the address, records a
> Confirmation with provenance `double_opt_in`; the fresh link that page sends for an older link is
> sent in this ADR's third eligibility mode with `everything` waived.

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

This is the honest answer to "no subscribed flag": the truth is an **immutable Event**, never a
mutable status column; the row is a derived read-model — the ADR 0011 pattern (Events are truth,
a derived read serves the hot path).

Rejected: **pure event-derived eligibility** (no row, EXISTS against the events table) — the
events table is huge and destination-correlation there needs a new index + normalization
discipline, whereas the small positive row is a trivial, cheap predicate and writing both a row
and an event is exactly what Unsubscribe already does (zero new architecture). Rejected: **a
Contact status flag** — the ADR 0001 hazard (lost on delete+re-import, conflates choice with
deliverability facts).

### Policy: a workspace-level toggle, forward-looking

A single **workspace-level `require confirmed opt-in`** policy (default **off** = today's
single-opt-in) drives both the **gate** (eligibility requires a Confirmation) and the **trigger**
(acquisition flows send a confirmation email). Not per-source: enforcement is uniform; acquisition
nuance is carried by provenance, not a per-source gate.

Enabling is **forward-looking, never retroactively blocking** — flipping the switch must not make
the existing list un-mailable overnight. Enabling **backfills grandfather Confirmation records**
for existing mailable contacts (the admin affirmatively asserts the prior-consent basis), keeping
the eligibility gate _uniform_ ("Confirmation EXISTS" always) rather than a non-auditable
time-based carve-out. "Did this contact confirm?" is answerable per-contact from a record.

### Import & provenance

CSV import presents **two modes**: _already confirmed (existing relationship)_ → writes a
Confirmation with provenance `imported`; _needs confirmation_ → no record, unconfirmed, sends a
confirmation email. Every Confirmation (row + event) carries a **provenance** enum —
`double_opt_in` (clicked — gold standard), `grandfathered` (policy-enablement), `imported`
(importer-asserted). Dedup/merge (the Contact-import backlog item) must keep the **strongest**
provenance: a real `double_opt_in` is never downgraded by a re-import asserting `imported`.

### The confirmation email's own eligibility

The confirmation email goes to an _unconfirmed_ address, so it **bypasses the confirmation gate
and per-source Unsubscribe** (it carries no marketing sending source — a one-time consent email,
not a campaign). But it is **not** purely transactional: it **respects Suppression** _and_
`Unsubscribe(everything)`. `CheckTransactional` skips all unsubscribe layers; the confirmation
email must not, because soliciting re-permission from someone who opted out of **everything** is
itself a violation — a new form submission must not send them a confirmation email and must not
auto-clear their opt-out (the only way back is a deliberate resubscribe). So it is a **third
eligibility mode**: Suppression + `everything` only.

Guardrail: confirmation sends are a **subscription-bombing** vector (submitting a victim's
address to a form repeatedly). Confirmation emails must be **rate-limited / de-duped per
destination** (no re-send while a recent unconfirmed request exists); detailed limits are left to
implementation.

### Mechanism & endpoint (reuses the ADR 0012 pattern)

The confirmation link carries a **signed JWT** (reuse the existing HS256 signer) encoding
workspace + destination + contact + acquisition context, with an **`exp` claim** (unlike the
eternal unsub/tracking tokens). The link is method-split: **`GET /e/confirm/{token}` redirects to the
SPA page and records nothing; `POST /site/confirmations/{token}` performs** the confirmation
(writes the row + publishes `marketing.confirmed`, one transaction). The page's explicit
"Confirm" button issues the POST through the generated site client; there is no POST on `/e/confirm`.

Scanner-safety is **load-bearing for legal validity here, not just UX** — and higher-stakes than
ADR 0012: a scanner GET that falsely _unsubscribes_ is annoying; a scanner GET that falsely
_confirms_ **fabricates consent you never had**, poisoning the exact audit trail double opt-in
exists to create. The explicit button-click _is_ the deliberate human act that makes the opt-in
valid.

### Eligibility ordering: negatives dominate

The gate is the **lowest-priority layer**, after the three negative layers (Suppression,
`everything`, source) — so confirmed-but-suppressed and confirmed-but-unsubscribed both correctly
don't send. An **`everything` unsubscribe invalidates the Confirmation row** (deletes the gate
row; the immutable event preserves "confirmed at T1, left at T2"), so returning after a full
opt-out requires re-confirmation — stale consent never silently reactivates. A _per-source_
unsubscribe does not invalidate confirmation (narrower than leaving entirely).

### Expiry & purge

- The confirmation **token expires** (~7 days); an expired link → "request a new confirmation".
- Never-confirmed requests are **purgeable** (data minimization: no consent, don't retain PII
  forever) — recorded as a **principle**; the mechanism composes with the 🟡 GDPR-tooling backlog
  item (erasure/retention), not a second retention engine here.
- **No ongoing re-confirmation / consent expiry** — a confirmed address stays confirmed until it
  unsubscribes.

## Relationship to ADR 0001

This **extends, does not reverse** ADR 0001. It adds a fifth, _positive_ eligibility layer, but
preserves 0001's core principle: the source of truth is an immutable Event plus a derived
destination-keyed row — there is still **no stored subscription status on the Contact**. The
positive layer is optional (off by default) and evaluated last, so single-opt-in workspaces see
the unchanged subtractive model.

## Consequences

- A new `Confirmation` glossary term and a `marketing.confirmed` Event; a workspace policy field;
  a third eligibility mode (Suppression + `everything`); a `GET`=page/`POST`=confirm endpoint.
- Enabling the policy backfills grandfather records; import gains a confirm-mode choice; dedup
  must preserve strongest provenance.
- Composes with two backlog items: **Contact import** (confirm-mode + provenance dedup) and
  **GDPR tooling** (purge of never-confirmed contacts).
