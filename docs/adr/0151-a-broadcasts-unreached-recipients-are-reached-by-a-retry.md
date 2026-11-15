---
status: accepted
---

# A Broadcast's unreached Recipients are reached by a Retry

> Amends [[0143-old-outbound-messages-are-folded-into-stored-figures-and-deleted]] (a cancelled
> Broadcast is frozen too) and [[0149-a-broadcasts-audience-is-chosen-explicitly-and-counted-before-it-is-armed]]
> (a third kind of audience). Settles the retry that
> [[0150-a-sending-broadcast-is-cancelled-and-a-deactivated-automation-may-end-its-enrollments]] left apart.

## Context

A Recipient whose last attempt failed ends with a Failed message, and a Recipient has exactly one
Outbound message (ADR 0046), so nothing can send to it again. Since ADR 0150 the Recipients a Cancel
left without a message are counted as `cancelled`, and the need behind a Pause, fixing a mistake and
reaching the rest, was sent here. A provider outage that outlasts the send's retries leaves the same
gap: part of the audience never got the Broadcast, and the only way to reach them is a new Broadcast
to a Segment no one can write, since "did not get Broadcast B" is not a Condition.

The usual practice of email platforms is to leave a cancelled or partly failed campaign as it ended
and to copy it into a new one addressed to those it did not reach; a few retry the failed recipients
inside the same send instead. An agent driving an email platform is expected never to send failed
mail again without a person's order, since mail that arrives late can do more harm than none.

A Deferral is not a failure: a Recipient deferred by the Send cap, the Provider limit or its Sending
window is still pending, and the Broadcast stays `sending` until it goes. A Recipient Skipped by
Suppression, an Unsubscribe, a missing Confirmation or the Frequency cap is final for that
Destination by design (ADR 0137: a Broadcast is about its moment).

## Decisions

### A Retry is a new Broadcast

- **`Broadcasts.retry` makes a new draft Broadcast from a finished one**: the same Channel,
  Integration and Message content, all editable, with a link to its source. The source keeps its
  state, its Recipients and its figures; a final state stays final and a Recipient keeps its one
  message.
- **A Retry's audience is its source's unreached Recipients**: those whose message is Failed, and on a
  cancelled source those left without a message. It is a third kind of audience beside a Segment and
  all Contacts, named by the link (`retryOf`), and it cannot be switched to a Segment: a different
  audience is an ordinary Broadcast. Skipped Recipients are never in it.
- **It is resolved when the Retry is armed**, as a Segment's Condition is copied then (ADR 0065), and
  each Recipient passes Send-eligibility at send time. A Contact erased since is gone; a Contact whose
  address changed since is reached at its Channel's Destination now, which is often what fixed the
  failure.
- **Only a finished Broadcast with someone to reach can be retried**: `sent` or `failed` with a
  Failed message, `cancelled` with a Failed message or a Recipient left without one. A draft,
  scheduled or sending Broadcast has no outcome yet. Otherwise the call is a 409 naming why, and no
  draft is made.
- **A Broadcast has at most one Retry.** Two Retries of one source, both sent, would mail the same
  people twice. What a Retry fails to reach is retried from the Retry, so retries form a chain. Deleting
  a Retry's draft frees its source for another.
- **No window but retention.** A Broadcast can be retried while its Recipients exist, until ADR 0143
  freezes it. Whether its content still makes sense days later is the sender's call, so the console
  shows how long ago the source went out wherever it offers or schedules a Retry.

### A Retry is an ordinary send

- **Making a Retry is authoring; sending it is send-class.** `Broadcasts.retry` needs
  `broadcasts:write` and is listed over MCP without `mcp:send`. Send and Schedule of a Retry are those of
  any Broadcast, with ADR 0149's count. An agent can prepare a Retry; only one trusted to send can send
  it, which is the order of a person the agent gate asks for.
- **Every check runs again**: Send-eligibility, the Frequency cap (a Failed message already freed its
  place, ADR 0137), the Send cap, the Provider limit, Holds and the Sending window. A Retry gets no
  exception.
- **Figures and attribution are each Broadcast's own.** The source's failed and cancelled stay as they
  were; the Retry has its own Recipients and figures, and an Order credited to a click in it is
  credited to it (the Attribution window's last click). The console links both ways, "Retried by" on
  the source and "Retry of" on the Retry. No report sums a chain.
- **A Retry of a series issue sends under the series' Sending source** (ADR 0135), so an opt-out of the
  series covers it. It is a Broadcast of its own, not an issue: its content can be edited apart from
  the series, and it does not move the series' schedule.

### A cancelled Broadcast is frozen like a sent one

ADR 0143 freezes a Broadcast that has been `sent` or `failed` past the retention window. `cancelled`
is final too (ADR 0150) and now joins them, so a cancelled Broadcast's rows do not outlive the
window.

## Consequences

- The contract gains `Broadcasts.retry` on `/api`, `/site` and MCP, and a Broadcast's `retryOf` (its
  source) and `retriedBy` (its Retry). A Broadcast with `retryOf` has no `segmentId`, and the audience
  operations refuse it.
- The console offers Retry on a finished Broadcast with someone to reach, names how many and how long
  ago the source went out, and shows the audience of a Retry as "Not reached by B".
- Glossary: **Retry**; the Broadcast entry's audience names it.

## Rejected

- **Sending again inside the same Broadcast**: a Recipient would have two messages, a final state
  would move back to `sending`, and the figures of the first send and the second would mix. Content
  could not be fixed, which is the point of retrying after a Cancel.
- **Retrying Skipped Recipients, or deferring them for a Retry**: a Skip is final for that
  Destination by design, and a Retry would turn the Frequency cap's skip into the deferral ADR 0137
  rejected.
- **A fixed retry window** (refused Recipients kept for a few days): it is an opinion about the
  content the app does not have; showing the age is enough for the person who decides.
- **Resending to those who did not open**: it mails people already reached, and opens are unreliable
  under `dont_track` (ADR 0120). It is not a retry and is not decided here.
