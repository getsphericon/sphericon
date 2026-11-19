---
status: accepted
build: none
---

# An open figure carries low confidence

> Amends [[0053-delivery-and-engagement-events-cover-every-send]] (how an open figure is shown) and
> [[0151-a-broadcasts-unreached-recipients-are-reached-by-a-retry]] (a resend to those who did not
> open is declined).

## Context

An open is the weakest signal the app records. Apple Mail Privacy Protection loads the pixel without
the person, the Private Relay list that catches it is incomplete (ADR 0053), a message sent untracked
has no open at all (ADR 0120, 0123), and a client that blocks images never reports one. Clicks are
filtered the same way but need a person's act. The figures still show an open rate beside the click
rate with nothing to tell them apart, and an agent that drives a Workspace through MCP reads the
same number a person does. The practice is to keep opens but mark them: platforms split human from
machine opens, flag an Apple privacy open on the event, or estimate the real ones from clicks. The
manuals agents load to drive an email platform say to label an open-only read low-confidence and
never to trigger a resend, a sunset or a suppression on opens.

## Decision

- **Every open figure in the contract carries its confidence**: on `/api`, `/site` and MCP an open
  count or rate is a model of its own with `confidence`, a closed enum whose value is `low`, so a
  client cannot read an open figure without the label. A check over the OpenAPI documents refuses a
  property named for opens that is not of that model. A click figure carries none.
- **Machine opens are counted beside the human ones** in a Broadcast's and an Automation's report
  and in the analytics, from the Events while they are kept: they still count in no rate (ADR 0053).
- **The console marks every open rate** as low confidence, and the Condition editor warns on a test
  of an open: the test stays allowed, as every comparable platform allows it, and the unknown of an
  untracked Destination (ADR 0123) still holds.
- **A resend to those who did not open is not a feature.** A follow-up to part of a Broadcast's
  audience is a Segment on its Events (ADR 0153's `event` leaf names the Broadcast) and a new
  Broadcast to it, which passes every check, the Frequency cap included; the docs show it on those
  who did not click, since an open is the weaker signal. An untracked Recipient is unknown there, not
  a non-clicker (ADR 0123). ADR 0151's open point is closed by this.

## Considered options

- **A label in the console only**: an agent reads the contract, not the console, and would take an
  open rate at its face value.
- **Hide open figures**: a Workspace still uses them to compare two subjects of its own over time, and
  the Split test already picks on clicks (ADR 0152).
- **A resend as a copy of the Broadcast** (beside the Retry, ADR 0151) or **as a step inside it**: it
  repeats the Retry with an audience chosen by opens, the weakest signal, for what a Segment and a new
  Broadcast already do.

## Consequences

- The open figures of the contract change shape; their clients are regenerated.
- Glossary: **Engagement rates**, **Machine engagement**.
