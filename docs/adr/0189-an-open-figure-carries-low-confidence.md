---
status: accepted
build: none
---

# An open figure carries low confidence

> **Amended below:** the check runs when the contract compiles and finds an open figure by its name;
> the click-to-open rate is an open figure; machine opens are a count of messages beside a
> Broadcast's and the analytics' human opens and a count of Events beside an Automation's, unknown
> past the Event horizon; the console marks a figure from its `confidence` and warns wherever an
> editor tests an open.

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

## Amendment: what an open figure is, how machine opens are counted, where the console warns

Read against the contract and the console before the spec, three points were left open: which
properties the check finds, the unit and the horizon of the machine count, and which editors test an
open.

- **The check runs when the contract compiles**, with the contract's other rules, so every document
  generated from it, MCP's included, is held by it. It finds an open figure by name: a numeric
  property whose name holds `opened` or `opens` as a word, or `open` followed by `rate`. Such a
  property is of the open count model or the open rate model, or `null` beside one; a time
  (`openedAt`) is not a figure and is not checked. An open figure the name would miss is renamed
  rather than excused: the opens-by-hour report's total is `opens`. The two models share one closed
  enum, `FigureConfidence`, not a bare `Confidence`, which the Split test uses for significance (ADR
  0152).
- **The click-to-open rate is an open figure**: its denominator is opens, so it carries the same
  `confidence`. So are the opened count of a Report day in the analytics series and an hour's opens
  in the opens-by-hour report.
- **A machine open is counted in the unit of the human figure beside it.** A Broadcast's figures and
  the analytics' Engagement rates count the messages of their cohort with at least one machine open;
  a message opened by a machine and by the person is in both counts. An Automation's report counts
  machine open Events, as its human opens are a repeat count of Events. The series by Report day and
  the opens-by-hour report stay human only. Machine clicks are not counted beside: a click needs an
  act, and a machine click is a scanner's, not a reader's.
- **The machine count is read from the Events and is unknown past their horizon.** No column keeps
  it, and a Broadcast frozen by message retention (ADR 0143) stores none: once the Event retention
  has passed since a Broadcast's last message went out, its machine count is `null`, never a zero
  the deleted Events would make up. Before that the report states the Event retention, as the clicks
  by link report does.
- **The console marks an open figure from its `confidence`**, through one component that takes the
  open count or open rate model and draws the value with the mark, so a figure the contract adds is
  marked without a change to the console. A static check refuses a read of an open figure's value
  outside that component.
- **The warning follows every test of an open**, not one editor: an `event` leaf or matcher on
  `email.opened` in a Condition, a Trigger, an exit or a goal, a Branch's test of its own message's
  open (ADR 0122) and a Wait for event on an open (ADR 0124). One predicate of the console decides
  whether a Condition or a matcher tests an open, and every editor reads it. The API saves such a
  test without a warning: an agent learns the confidence from the figures and from the description of
  `email.opened` in the contract.
