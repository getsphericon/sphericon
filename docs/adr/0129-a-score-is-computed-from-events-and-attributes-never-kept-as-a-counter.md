---
status: accepted
---

# A Score is computed from Events and attributes, never kept as a counter

Lead scoring ranks Contacts by how close they are to buying: points for a pricing page visit, a
demo request, a plan, minus points for an unsubscribe. The app has none. The common shape is a
number on the contact that actions add to; ADR 0125 left out an increment on Set field for this
decision.

A Condition already counts a Contact's Events of an action within the last N days and compares
numbers. Opens and clicks are unknown for a Destination that is not tracked (ADR 0123), and opens
are inflated by machine opens. The glossary's stance on deliverability, "honest ratios, not a
black-box score", applies here as well: a score a marketer cannot explain will not be trusted.

## Decision

### A Score is a definition, its value is computed

- **A Score is a named definition of the Workspace whose value for a Contact is computed from the
  Contact's Events and attributes**, never a number that actions change. A counter drifts from the
  history it summarises, counts twice when an action is retried, and does not follow a change of the
  rules; a computed value is right after every edit, and decay is a window. Set field gets no
  increment; points given by hand are an Event the customer records (`sales_call`) and a Score
  counts.
- **A Workspace has several Scores** ("product interest", "sales readiness"). A Score has a key that
  never changes and a name that may. Its definition is edited without versions: an edit changes
  every value at once, which is what a computed Score is for.

### What it adds up

- **A Score is a list of components and a window.** An **Event component** gives points for each of
  the Contact's Events of one action, optionally only those whose properties meet a Condition, up to
  a cap on how many count. A **Condition component** gives points when a Condition on the Contact
  holds (has a Tag, a plan of `pro`); it cannot read a Score, so no two Scores name each other. The
  window, a number of days, bounds every Event component of the Score. Points are whole numbers and
  may be negative, and so may the total.
- **The window is no longer than Retention.** Analytical Events are deleted by age (ADR 0019), so a
  Score cannot count what is gone: a window longer than the Instance's retention is refused, and if
  the retention is later shortened the Score counts what is kept and its page says so.
- **No engagement goes into a Score.** An Event component cannot name an open or a click, and a
  Condition component cannot test one. Without consent they are unknown (ADR 0123), which would make
  the Score of everyone who refused tracking partial; opens are also inflated by machine opens.
  Intent shows in site Events, recorded with Tracker consent, and in the customer's own Events.
  Those stay two-valued, as ADR 0123 keeps them: a visitor without Tracker consent has no site
  Events, and they count as none.
- **What a Score names is in Definition use**: the Custom fields and Tags of its Conditions cannot
  be removed or retyped while it names them, and a Score that a Segment's Condition or a kept
  Automation's or Rule's live version or draft names cannot be deleted. A template that names a
  deleted Score renders nothing there, as for a Custom field a Contact lacks.

### Where it is read

- **A Condition reads it as a number**, `score:<key>`, wherever a Condition is read: a Segment, the
  copy an armed Broadcast holds, a Rule, a Branch and a Wait for event. A template renders it. The Contact's page shows its breakdown: how many points each component gave.
- **The value is computed when it is read**, as a Condition is, and stored nowhere.

### A threshold and the Event of reaching it

- **A Score may have one threshold** ("qualified" at 100). When it rises to or above the threshold,
  the app records `contact.score_reached`, an analytical System event naming the Score. A Trigger
  and a Rule may name it, and the target version's Entry policy applies (ADR 0127).
- **For that, and only that, the app keeps one state per Contact and Score: above or below the
  threshold.** It is checked when an Event is recorded for the Contact or the Contact is written,
  for that Contact alone, in a job off the bus as a Rule's evaluation is (ADR 0054), under the Contact
  lock. A rise found there records `contact.score_reached`; a fall only updates the state, so the
  threshold can be reached again.
- **Every other change of the state is silent.** A daily pass updates the states the window moved
  (Events ageing out lower a Score with nothing recorded), and records nothing, in either direction.
  A new Score, a threshold added or moved, any edit of a Score, an Import, a Backfill and a Merge
  set the states they touch without an Event, so adding a threshold of 100 does not enroll every
  Contact already above it. While such a recompute of a Score is running, the checks at Event time
  skip that Score; they resume when it is done.
- **`contact.score_reached` follows the chain of what raised it.** It carries the Rule's `CausedBy`
  (ADR 0054) and the Automations the chain passed through (ADR 0127) of the Event that raised the
  Score, so it does not fire a Rule from a Rule's own Action nor re-enroll an Automation through
  itself. A Backfill counts in the value but records no `contact.score_reached`: loaded history
  triggers nothing (ADR 0044).
- **A Trigger cannot tell two Scores apart.** A Trigger is an action with no filter on its
  properties (ADR 0125), and `contact.score_reached` names the Score in a property, as
  `contact.tagged` names the Tag. Until Triggers filter on properties, an Automation for one Score
  is entered by a Rule on `contact.score_reached` whose Condition tests the Event's `score`, with the
  Enroll Action.
- **An Erasure deletes the Contact's states with it.**

## Consequences

- The contract, MCP and the app gain Scores, their components and the Contact's breakdown; the
  Condition gains the `score` number leaf.
- A table keeps, per Contact and Score with a threshold, whether it is above; nothing else of a Score
  is stored.
- A Segment that reads a Score compiles each Event component to a capped count of Events within the
  window. Two compile paths are new: a Condition on the properties of stored Events (a Rule folds
  property tests in memory against one Event today, ADR 0054) and a count capped before it is
  multiplied by the points.
- `contact.score_reached` joins the System events.

## Considered options

- **A number Custom field that Rules and Steps increment.** It drifts, double counts on retry and
  cannot be recomputed when the points change.
- **One Score per Workspace.** Teams rank interest and readiness differently; one number mixes them.
- **Versioned Scores.** A Score has no in-flight work to protect: the point of editing one is that
  every value changes.
- **Engagement components with unknown carried through the sum** (a Score known only as a range).
  It is exact but unreadable, and the signal opens give is the weakest.
- **Storing every value and updating it on each Event.** It is the counter again, with the decay
  missing until something writes it.
- **A late `contact.score_reached` from the daily pass.** The pass cannot tell a rise the person
  caused from one an edit, an Import or a Backfill made, so it would enroll on housekeeping.
- **Several thresholds or stages.** One threshold covers "qualified"; stages can be several Scores
  until a case needs more.
