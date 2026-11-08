---
status: accepted
---

# Delivery and engagement Events cover every send, and machine engagement is flagged, not dropped

> **Amended below:** the rollup of a human open and click moved from the Broadcast recipient to
> the Outbound message, so it covers every surface.
>
> **Amended by [[0113-outbound-message-projections-follow-events-and-keep-the-earliest-observation]]:**
> human engagement and email delivery projections run asynchronously, retain the earliest observed
> time regardless of handling order and survive Erasure anonymously. That replaces their
> same-transaction and set-only-while-empty requirements below.

Opens and clicks were recorded once per Broadcast recipient, only for Broadcasts, and counted
whatever made the request; delivery was inferred (ADR 0011). We now record the full set a sender
expects (`email.delivered`, `email.delayed`, `email.dropped` beside sent, bounced, complained,
opened, clicked, unsubscribed) for every Outbound message: a Broadcast, an Automation step and, on
request, a Transactional send.

- **The tracking token and the provider's message id name the Outbound message**, not the Broadcast
  recipient. Every surface is tracked the same way, and a Delivery notice is linked to its message
  by the provider's message id, which `email.sent` already stores (SES's id; for SMTP the MIME
  Message-ID we set). A Preset maps it where its provider returns one. No header or tag of our own.
- **Every open and click is an Event**, with `url` on clicks. "First" and "unique" are derived. The
  Broadcast recipient's rollup stays (ADR 0019: reports must not shrink when Events expire): it is
  set once, and only by a human engagement.
- **`machine` is a flag on the Event**, set from the user-agent (a maintained library, not our own
  list) and the iCloud Private Relay egress ranges Apple publishes (refreshed by a river job into a plain
  table). Apple publishes no list for Mail Privacy Protection itself and the Private Relay list is
  known to be incomplete, so detection is a partial signal. Neither the address nor the user-agent is stored. A machine Event is kept, so a wrong
  detection can be fixed later, but it counts nowhere and triggers nothing. No timing heuristic:
  its false positives cost more than it catches.
- **Presets produce Delivery notices** (bounce, complaint, delivery, delay, drop) instead of Delivery
  failures, with an optional provider message id. The format is rewritten, not versioned.
- **Complaint rate's denominator becomes `delivered`**, replacing the `sent − hard bounces` proxy of
  ADR 0011. A Preset that reports no delivery leaves the rate at "insufficient data".
- **The delivery result is stamped once on the Outbound message** (delivered, bounced, complained),
  by the first notice of each kind. Reports of a Broadcast or an Automation read these stamps, so a
  report's delivery figures do not fall as the analytical Events expire (ADR 0019). `delayed` and
  `dropped` are Events only.
- **New Events are analytical** (ADR 0019); permanent bounces, complaints and unsubscribes stay
  evidentiary.
- **Automation triggers** are `email.opened`, `email.clicked` and `email.unsubscribed`, never a
  machine Event and never an Event from the Automation's own email, which closes the loop that
  untracked Automation mail used to avoid.
- **A Transactional send is tracked only when its request says so** (`track`, off by default); it
  stays Transactional (ADR 0005): no unsubscribe link, same Suppression rules.

## Considered options

- **Drop machine Events** (rejected): the loss is permanent and a wrong rule is unfixable.
- **Remove the rollup and count from Events** (rejected): report figures would fall as Events expire.
- **A header or tag of our own to correlate notices** (rejected): the provider's message id is already
  stored and providers return it.
- **Keep `sent − hard bounces`** (rejected): it is only an approximation of delivered, and the
  Delivery notice is now ingested anyway.

## Amendment: the engagement rollup is on the Outbound message

The rollup of the first human open and click moved from the Broadcast recipient to the Outbound
message (`opened_at`, `clicked_at`), for the reason it existed: reports must not shrink when the
Events expire (ADR 0019). The recipient's rollup covered a Broadcast only, because an Automation
message has no recipient row and its Events name the message but expire; the message is the one
record every surface has, and it already carries the delivery stamps.

- **One lasting rollup.** Each stamp is set once (`... WHERE opened_at IS NULL`), only by a human
  hit, in the transaction that publishes the engagement Event, for every tracked message: a
  Broadcast, an Automation and a Transactional send with `track`. A machine hit still stamps
  nothing. The Broadcast recipient has no `opened_at` / `clicked_at` any more (its
  `unsubscribed_at` stays); a Broadcast's stats and report, the contact export and the analytics
  read the message's stamps.
- **The overview's engagement covers the Broadcast and Automation messages** sent in the range (a
  Transactional message is left out), one count per message, not per person, so the rates stay in
  [0, 1]. Its labels no longer say "Broadcast".
- Not changed: the Automation report's own opens and clicks stay repeat counts from Events.
