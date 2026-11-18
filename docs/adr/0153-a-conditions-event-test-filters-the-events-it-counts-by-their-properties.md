---
status: accepted
build: [1446]
---

# A Condition's Event test filters the Events it counts by their properties

> **Amended by [[0180-a-property-test-runs-in-postgres-as-one-sql-json-path]]:** the filter runs in the count as one SQL/JSON path, and a message's System
> event is filtered by a sent-by test naming a Broadcast or Automation by id (it tests the
> `broadcastId` or `automationId` the Event carries), not by keys of its own; the link stays `url`.

> Amends [[0029-segment-rules-compile-to-generated-predicates]] (the `event` leaf) and extends
> [[0133-a-trigger-exit-or-goal-matches-an-event-by-its-action-and-properties]] (its filter, now
> also in a Condition).

## Context

A Condition's `event` leaf counts a Contact's Events of one action, at least or at most a number of
times, optionally within the last N days. It cannot say which Events: "clicked a link to `/pricing`
in Broadcast X", "completed an order with status `paid`" and "was tagged `trial`" are all out of reach
of a Segment, though ADR 0133 lets a Trigger, an exit and a goal filter an Event by its properties.
Event-based conditions that filter on the Event's properties, the campaign of a click and its link
among them, are a standard part of segmenting in marketing platforms.

Events carry two kinds of data. A customer's Event has free properties with no declared type (ADR
0054). A System event about a message also carries typed references: the Broadcast or Automation that
sent it and, for a click, the link. Analytical Events expire after the Instance's retention window
(ADR 0019), so a count with no window only reaches as far back as the kept history.

## Decisions

- **The `event` leaf takes an optional filter, the one an Event matcher has** (ADR 0133): tests of the
  Event's properties (`eq`, `neq`, `contains`, `beginsWith`, `null`, `notNull` and the number
  comparisons) under `and`, `or` and `not`, two-valued, a property the Event lacks or holds with
  another type failing its test. Only Events of the action that pass the filter are counted, within
  the leaf's window. One grammar serves a Trigger, an exit, a goal and every Condition.
- **No property catalogue.** Keys are checked for form only, as in ADR 0133. The editor suggests the
  keys seen on recent Events of the action; a suggestion, not a declaration.
- **A message's System event is filtered by what sent it.** On the sent, delivery and engagement
  Events of every Channel, the filter may name the Broadcast or the Automation that sent the message
  and, on a click, test the link (`eq`, `contains`, `beginsWith`). The editor offers Broadcasts and
  Automations from the Workspace's lists, not as ids.
- **A Broadcast or an Automation is named by its id, and an archived one still matches.** Neither is
  ever removed by its owner (ADR 0100), and archiving one keeps its row and the Events that name it,
  so a filter that names it keeps counting them, and a save may name an archived one: "clicked in last
  month's newsletter" is a question about history. It differs from a Rule's Enroll, whose Publish
  refuses an archived Automation (ADR 0100): an Enroll sends new work into the Automation, a filter
  only reads the Events it already left. A save naming one the Workspace does not have is refused. The editor offers the kept ones, and the archived ones on request, marked as archived. A
  Broadcast or Automation is therefore not Definition use and the Removal check does not read it.
- **A filter that names a Tag, Custom field or Score is Definition use**, as ADR 0133 has it for a
  matcher: the Removal check reads the filters of a Segment's, a Rule's and an Automation's
  Conditions, so such a definition cannot be removed while a kept one names it, and a Segment never
  turns into "no one" without a word.
- **Unknown stays as it is.** A test of an open or a click is unknown for a Destination that is not
  tracked (ADR 0123), and a Segment reads unknown as no match; a filter changes which Events count,
  not that.
- **The horizon is shown, not hidden.** An `event` leaf with no window, or with one longer than the
  Instance's analytical Event retention, counts only the Events kept. The editor and the preview say so
  ("Events of the last N days are kept"). No lasting summary of "ever did it" is kept beside the
  Events: it would have to follow Erasure and a tracking `denied` (ADR 0120) on its own.
- **No new index up front.** The filter runs inside the leaf's count of one Contact's Events of one
  action, which is already indexed. An index on properties is added when profiling real volumes asks
  for it.

## Consequences

- The Condition's `event` leaf gains `filter` in the contract; a Rule's and a Branch's Conditions
  accept it too, as one grammar.
- The Removal check's readers of Segments, Rules and Automations read the `event` leaf's filter for
  the Tag, Custom field and Score it names.
- Glossary: the Condition entry.

## Rejected

- **A typed property catalogue first**: a model of its own (declared keys, types, inference), not
  needed for a filter whose tests already fail safely on a missing or mistyped property.
- **"Bought product Y" through Event properties**: an Order has no lines, so a filter could only test
  a scalar the shop happens to send. Conditions on Orders (count, revenue, lines) are their own
  decision.
- **A Broadcast or Automation named by a filter as Definition use**, kept from Archive while a
  Segment names it: Archive removes neither the row nor its Events, so the guard would protect
  nothing, and it would keep finished work in the lists for a Segment's sake.
- **A lasting "ever" summary per Contact and action**: denormalized state to keep consistent with
  Erasure and consent, for the one case the shown horizon already makes honest.
