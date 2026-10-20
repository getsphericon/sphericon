---
status: accepted
---

# History arrives as a Backfill, and a live Event never claims to be old

A new setup has to bring the Events of its earlier system with it, and an Event that happened last
year must not enroll an Automation or reach a webhook endpoint: a load of history would mail
every Contact again. Marketing tools split here: Customer.io decides by the Event's age (older
than 72 hours never triggers a campaign), Segment leaves it to each call (`integrations`), Mixpanel
has a separate `/import` endpoint. We make it explicit and keep the age as a guard: a **Backfill**
(`backfill: true` on `POST /events/batch`) is recorded and visible to segments but triggers
nothing, requires `occurredAt` and refuses a time in the future; a live Event on the external API
whose `occurredAt` is older than the live window (`SPHERICON_EVENT_LIVE_WINDOW_HOURS`, default 72,
at least 1, never off) is refused with a reason that names `backfill`. A client that forgets the
flag gets an error instead of a mailing.

`clientEventId` (unique per Workspace, own namespace from `/collect`) makes a resent load a no-op.
The system's own actions (`email.*`, `marketing.*`, `contact.*`) are refused from every customer
Event, `/collect` included: they are the evidence behind consent and suppression (ADR 0013, 0019),
so a load of history cannot write them.

## Considered options

- **Decide by age alone, like Customer.io** (rejected): a live Event that arrives late (an app
  syncing offline) is silently dropped as a trigger, and the cut-off is a constant nobody sees.
- **A separate `/import` endpoint, like Mixpanel** (rejected): a second primitive beside
  `/events/batch` for the same record; the flag keeps one.
- **Refuse old Events in `/collect` too** (rejected): browsers and apps queue Events offline and
  send them late, which is real data, not history.

## Consequences

- Retention counts from when an Event is stored, not from `occurredAt` (ADR 0019), so a Backfill
  is not pruned at once and old history is kept for the full window.
- An Event whose subject matches no Contact is stored anonymously and is not linked later: load
  the Contacts first.
- Past engagement (opens, clicks) of an earlier provider cannot be backfilled as `email.*` Events.
