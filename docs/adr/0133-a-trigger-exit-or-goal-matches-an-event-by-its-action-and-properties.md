---
status: accepted
---

# A Trigger, exit or goal matches an Event by its action and properties

An Automation version names Events by action alone: its Trigger, its exit events and its goal event.
Three decisions since have run into it. A goal of "the Contact got the Tag `qualified`" matches every
`contact.tagged`, so one Automation cannot signal another precisely (ADR 0125). An Automation for one
Score cannot start on `contact.score_reached` of that Score alone; ADR 0129 routes it through a Rule
that tests the Event's `score` and enrolls. And an exit on a failed order and a goal on a paid one
are the same action.

A Rule already reads its triggering Event's properties in its Condition, folded in memory as
constants (ADR 0054), and a Wait for event holds a Condition (ADR 0124). ADR 0122 kept entry
conditions on a Contact out of the Automation: entering only when a Condition holds is a Rule's job.

## Decision

- **A Trigger, each exit event and the goal event is an Event matcher: an action and an optional
  filter on that Event's properties.** For example `contact.tagged` where `tag` is `qualified`,
  `contact.score_reached` where `score` is `readiness`, `order.created` where `status` is `failed`.
  A date Trigger (ADR 0126) has none.
- **The filter says which Event, never which Contact.** It holds only tests of the Event's
  properties, the property leaves a Rule's Condition has (`eq`, `neq`, `contains`, `beginsWith`, `null`, `notNull` and the number comparisons) under `and`, `or` and `not`; no Contact field, Tag, Score or history. A test on the Contact
  stays a Rule's, with its Enroll Action, as ADR 0122 decided. A property the Event lacks, or holds with another type, fails its test, as in a Rule, so a `not` over it matches; property keys have no catalogue and are checked for form only. The tests are two-valued.
- **An Event that fails a Trigger's filter is no entry**: it enrolls no one and is not counted as a
  skipped entry (ADR 0127). Everything else about matching stays: by Contact, a machine open or
  click and loaded history never count, exit and goal Events count from the Enrollment's beginning.
- **Exit events and the goal may share an action with different filters.** An Event that matches both the goal and an exit event reaches the goal: it is the outcome the Automation works for.
- **The Trigger may share an action with an exit or goal event too**, since an Enrollment keeps the id of the Event that began it and that Event never ends it. A Trigger on `contact.tagged` where `tag` is `trial` and a goal on `contact.tagged` where `tag` is `qualified` is the case this decision is for; a goal on the Trigger's own matcher means "did it again".
- **A filter that names a definition is Definition use**: the `tag` of `contact.tagged` and
  `contact.untagged`, the field of `contact.field_changed`, the `score` of `contact.score_reached`.
  The property is `tag`, `key` and `score` respectively. Such a Tag, Custom field or Score cannot be removed while a kept Automation's live version or draft names it. A save that names one not in the catalogue is refused, and the editor offers them from it rather than as free text; a Tag is never renamed, so the name stays its own.
- **What this replaces and keeps.** ADR 0129's Rule that enrolls on one Score is no longer needed:
  the Trigger filters. ADR 0125's rejection of a record-Event Step stands: a Tag with a filtered goal
  is the precise signal between Automations. A Wait for event keeps its full Condition (ADR 0124): it decides the path of a running Enrollment, where the Contact's data matters. Its refusal to await an exit or goal Event now applies when the awaited action is one an exit or goal matches with no filter; with filters it is allowed, and an Event that matches both ends the Enrollment, as an exit or goal Event does wherever the Enrollment stands. Exit on a reply stays an option of the version (ADR 0131): no filter can name the Enrollment that sent the email.

## Consequences

- The version's Trigger, exit events and goal become matchers in the contract, MCP and the editor;
  the trigger job and the matching of exit and goal Events, including the Events recorded before an Enrollment was made, read the stored Event's properties and evaluate the filter in memory, and the matching skips the Enrollment's own triggering Event.
- The Removal check reads matcher filters on System events as Definition use.

## Considered options

- **A full Condition on the Trigger.** It is the entry condition ADR 0122 kept out of the Automation,
  a second way to say what a Rule with an Enroll says.
- **A record-Event Step.** ADR 0125 rejected it; a filtered matcher closes the gap it was for.
- **Exit winning over goal on one Event.** The Event is the outcome the author named as the goal;
  counting it as a dropout would hide conversions.
- **Matching on Contact data in exit and goal.** An exit is about what happened, not who the Contact
  is now; a Branch tests the Contact.
