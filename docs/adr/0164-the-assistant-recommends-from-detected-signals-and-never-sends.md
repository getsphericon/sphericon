---
status: accepted
build: [1467]
---

# The assistant recommends from detected signals and never sends

> **Revised before it was built** (#1467): the switch names the protocol, endpoint host and model, as
> ADR 0139's now does, and the assistant with no Member is the author set by the job's context. The
> detectors are a closed list in an order of priority, each with its rule; there is no rate detector in
> the core and no daily pass besides the one job that runs them; a Recommendation, keyed by detector and
> object, is the only state they keep; a sent Broadcast is no longer a detector.

> Extends [[0139-workspace-skills-and-inline-intents-on-an-instance-configured-model]] from intents a
> Member asks for to Recommendations the assistant makes on its own.

## Context

ADR 0139 runs inline intents on a model the Instance configures: a Member asks, the model sees
definitions and figures but never a Contact, and fills a draft the Member saves. Nothing looks at a
Workspace's figures unasked, so a complaint rate creeping up, an Automation that quietly stopped
enrolling or a segment going cold is noticed only by someone who looks. A silent failure found days
later is the larger risk of running marketing through agents.

The practice expected of an agent driving an email platform is to ask before acting by default, to widen
autonomy only for narrow and reversible tasks, to leave strategy and the final send to a person, and to
prove what an optimisation adds with a held-out group, never with last-touch credit. The app is an
author, never a sender, for any agent (ADR 0132), and a Control group can measure what marketing adds
(ADR 0163).

## Decisions

### A Recommendation

- **A Recommendation is an object of the Workspace**: what the assistant observed, with the figures and
  links to the reports it rests on; what it proposes, as zero or more drafts it made (a Segment, a draft
  Broadcast, an inactive Automation); and a status: `open`, `accepted` once a Member sends or publishes
  what it proposed, `dismissed` with an optional reason, or `expired` 14 days after it was made with
  nothing done. Its author is the assistant, with no Member: the detector's job runs in the context
  that sets that author, and no field of a write can. It is not a notification and not a task.

### When the assistant looks

- **Detectors decide when, the model only writes.** A detector is a fixed rule over figures the app
  already has, one of a closed list, in this order of priority:
  1. **An item of Needs attention** (ADR 0073), its kind and its object. A deliverability warning
     reaches the assistant this way where the Instance has one; the core sets no rate limit of its own
     (ADR 0011), so there is no rate detector beside it.
  2. **A Trigger that went quiet**: a live Automation whose Trigger matched no Event in the last 7 days,
     entries skipped included, after at least 10 in the 28 days before. A matched Event that was
     skipped (`once`, a cooldown) is not silence, so an audience used up does not fire it. A date
     Trigger is left out.
  3. **A Segment whose engaged share fell**: among the Contacts in a Segment a Broadcast went to in the
     last 30 days, the share who clicked a message in the last 30 days against the share who clicked
     in the 30 days before, read as two Conditions with their unknown results left out of both the
     count and the base (ADR 0123), so an untracked Contact moves neither. Clicks, never opens, and
     never a machine click (ADR 0053). It fires on a fall of a quarter or more over at least 200
     Contacts whose engagement is known. A Segment whose own Condition tests engagement is left out,
     read from its leaves: "no click in 30 days" would fall by construction.

  When one fires, the model is asked for the rationale and, at the draft level, the drafts. Why it
  looked is a rule anyone can read, not the model's guess.

- **One daily job per Workspace runs every detector**, in the order above, on River in batches that
  persist (ADR 0144). Each reads live figures; nothing is snapshotted for it. No new service.
- **A Recommendation is its own memory.** A detector's firing is keyed by the detector and its object
  (an item of Needs attention, an Automation, a Segment). It makes no Recommendation while one of that
  key is open, nor for 30 days after one was closed, whether accepted, dismissed or expired, so a
  lasting condition is raised again at most once a month.
- **A sent Broadcast is not a signal.** Its outcome is its report's, and what to send next is a
  Member's call from it; a Recommendation per send would crowd out the signals above.

### What it may do

- **Two levels, neither sends.** `suggest`, the default, writes the Recommendation's text only; `draft`
  also makes the drafts. At most 5 Recommendations are open at once unless the Workspace changes it, so
  they never flood. At the limit, a firing of a detector of higher priority than the lowest open
  Recommendation expires that one and takes its place; any other firing makes none. The assistant never schedules, publishes, activates or sends: accepting is a Member
  doing that with the draft, through the gates every send has.
- **Data and consent as in ADR 0139**: the Instance's model, definitions and figures only, never a
  Contact, the Workspace's Skills included. An owner turns it on with a switch of its own, since it runs
  unasked, under `intents:manage`, naming the protocol, endpoint host and model accepted, as ADR
  0139's does. Its tokens count in the same
  metric and under the same daily cap as the inline intents.

### What it learned

- **An accepted Recommendation shows the outcome of what was sent**: a link to the Broadcast or Automation
  a Member sent or published from it, that object's own figures, and, while a Control group is on, the
  comparison of the groups for the period. It claims no incremental revenue of its own: one
  Recommendation's share cannot be isolated without a group of its own. The report of Recommendations
  shows how many were accepted and what the accepted ones led to.

### Who else sees it

- **The API and MCP read Recommendations and may dismiss them** (authoring). Only the assistant inside the
  app makes them. Accepting needs no operation of its own: it is the Schedule or Publish of the draft,
  which is send-class already.

## Consequences

- Recommendations, their detectors, the Workspace's switch, level and limit, and their report.
- Glossary: **Recommendation**.

## Rejected

- **An always-on agent loop**: unbounded cost, and reasons no one can audit.
- **A level that schedules or sends within a budget**: the final send stays a person's, whatever the
  record of earlier Recommendations.
- **Incremental revenue per Recommendation from attribution**: last-touch credit, which overstates what
  marketing adds.
