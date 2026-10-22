---
status: accepted
---

# Delivery and engagement Events cover every send, and machine engagement is flagged, not dropped

Opens and clicks were recorded once per Broadcast recipient, only for Broadcasts, and counted
whatever made the request; delivery was inferred (ADR 0011). We now record the full set a sender
expects (`email.delivered`, `email.delayed`, `email.dropped` beside sent, bounced, complained,
opened, clicked, unsubscribed) for every Outbound message: a Broadcast, an Automation step and, on
request, a Transactional send.

- **The tracking token and the provider's message id name the Outbound message**, not the Broadcast
  recipient. Every surface is tracked the same way, and a Delivery notice is linked to its message
  by the provider's message id, which `email.sent` already stores. No header or tag of our own.
- **Every open and click is an Event**, with `url` on clicks. "First" and "unique" are derived. The
  Broadcast recipient's rollup stays (ADR 0019: reports must not shrink when Events expire): it is
  set once, and only by a human engagement.
- **`machine` is a flag on the Event**, set from the user-agent (a maintained library, not our own
  list) and Apple's published Mail Privacy Protection ranges (refreshed by a river job into a plain
  table). Neither the address nor the user-agent is stored. A machine Event is kept, so a wrong
  detection can be fixed later, but it counts nowhere and triggers nothing. No timing heuristic:
  its false positives cost more than it catches.
- **Presets produce Delivery notices** (bounce, complaint, delivery, delay, drop) instead of Delivery
  failures, with an optional provider message id. The format is rewritten, not versioned.
- **Complaint rate's denominator becomes `delivered`**, replacing the `sent − hard bounces` proxy of
  ADR 0011. A Preset that reports no delivery leaves the rate at "insufficient data".
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
  stored and every provider returns it.
- **Keep `sent − hard bounces`** (rejected): it needs one Delivery notice per message to be wrong
  only slightly, and the notice is now ingested anyway.
