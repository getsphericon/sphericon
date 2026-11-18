---
status: accepted
build: [1549, 1446, 1465, 1563, 1616, 1621]
---

# A property test runs in Postgres as one SQL/JSON path

> Amends [[0054-a-rule-is-a-stateless-reaction-to-an-event-built-on-the-segment-condition-tree]] (where a
> `trigger:<key>` leaf is evaluated), [[0122-an-automation-branches-into-nested-paths-that-rejoin]] (where a
> Branch reads the Enrollment's `event`), [[0124-an-automation-chooses-a-path-by-condition-by-weight-or-by-an-awaited-event]]
> (what a Wait for event awaits), [[0133-a-trigger-exit-or-goal-matches-an-event-by-its-action-and-properties]]
> (where a matcher's filter is evaluated), [[0136-a-wait-for-event-keeps-the-event-that-ended-it-for-later-steps]]
> (where an `awaited` copy is read), [[0153-a-conditions-event-test-filters-the-events-it-counts-by-their-properties]]
> (how a message's System event names what sent it) and
> [[0162-a-condition-tests-a-contacts-orders-and-their-lines]] (the grammar of a line filter).

## Context

The app tests the free properties of a JSON object in many places: an Event's (a Rule's triggering
Event, the Event a Trigger, an exit, a goal or a Wait for event matches, the Events an `event` leaf
counts), a copy of one (an Enrollment's `event`, an `awaited` copy) and an Order's line. ADR 0133 and
ADR 0153 give them one grammar: `eq`, `neq`, `contains`, `beginsWith`, `null`, `notNull` and the
number comparisons, under `and`, `or` and `not`, two-valued, a property the object lacks or holds with
another type failing its test.

One grammar was about to get three evaluators. A Rule folds its triggering Event's properties in Go
(ADR 0054), with one table of operators that has a Go arm and an SQL arm and a test feeding the same
cases to both. The matcher was to evaluate its filter in Go (ADR 0133). An `event` leaf's filter runs
inside the per-Contact count, in SQL (ADR 0153). The line filter of an `order` test was written with
operators of its own. A message's System event was to be filtered by `broadcast` and `automation`
while the Event carries `broadcastId` and `automationId`. Every pair of arms keeps one meaning only as
long as a test catches each drift.

Postgres already has an engine for exactly these tests: the SQL/JSON path language of the SQL
standard, with `jsonb_path_exists`, which the number comparisons already use on a Custom field's
value.

## Decision

- **A property filter compiles once to one SQL/JSON path, and Postgres runs it.** The path is in
  filter form, `$ ? (…)`, and is run with `jsonb_path_exists(target, path, vars, true)`: the values
  of the filter are passed as the path's variables, never spliced into it. Each test is guarded so
  that a value of another type fails it (the path's own result for a type mismatch is unknown, and
  `!` keeps unknown), so a `not` over a test that fails matches, as ADR 0133 says. `eq` and `neq`
  compare text, `contains` is a case-insensitive literal match (`like_regex` with the `q` and `i`
  flags, as `contains` ignores case today), `beginsWith` is `starts with`, and the number comparisons
  compare a JSON number. Nobody writes a path: it is what the compiler emits.
- **It runs where the object is.** On `events.properties` inside an `event` leaf's count; on an
  Order's lines inside an `order` test; and on a `jsonb` parameter of the query for the properties of
  the Event a Rule handles, of the Event a Trigger, an exit, a goal or a Wait for event matches, of
  an Enrollment's `event` copy and of an `awaited` copy. Nothing is folded in Go, and there is no Go
  evaluator of a property test.
- **A message's System event names what sent it by a leaf of its own.** On the sent, delivery and
  engagement Events of every Channel, a filter may hold a sent-by test naming a Broadcast or an
  Automation by id, which compiles to a test of the `broadcastId` or `automationId` the Event
  carries; the link stays the property `url`. A Broadcast or Automation the Workspace lacks is
  refused (ADR 0153). The test is valid where the filtered action is a message's System event.
- **A line filter is the same filter** over the line's keys (`product_id`, `sku`, `name`,
  `category`, `quantity`, `price`), with every operator under `and`, `or` and `not`.
- **A Wait for event awaits Event matchers.** Each is an action and an optional filter, as a
  Trigger's, an exit's and a goal's are. Its optional Condition is an Automation Condition, as a
  Branch path's is (the Contact, its history, the Enrollment's `event`, earlier `awaited` copies),
  and reads unknown as no match (ADR 0124). The properties of the arriving Event are tested by the
  matcher's filter, so the Condition needs no subject for them.
- **The test of an Enrollment's own message stays as ADR 0122 decides it**: three-valued, from the
  Enrollment's records. It reaches the query as a boolean parameter that may be `NULL`, so Postgres
  applies Kleene's rules to the whole Condition.

## Consequences

- The Condition package loses its Go arm of the operators; its tests check the compiler's output
  against Postgres, one table of cases per operator, negated or not, with a missing property and a
  property of each other type.
- A Rule's evaluation, a match of a Trigger, an exit, a goal or a Wait for event and a Branch are
  each a query. A Rule's Condition already is one, and the matching already reads stored Events.
- No index up front, as ADR 0153 decides. If one is needed, a GIN `jsonb_path_ops` index serves the
  `@?` operator, which takes a path with its values inlined.
- #1549 builds the grammar and the compiler; #1446 runs it in the count; #1465 in the line filter;
  #1563 on the Enrollment's `event`; #1616 makes a Wait for event await matchers; #1621 reads an
  `awaited` copy.

## Considered options

- **Two arms, Go and SQL, with a shared table of cases** (what the app has). Every operator is
  written twice and the one meaning holds only as far as the cases reach.
- **A Go port of Postgres's SQL/JSON path**, to run the same path in memory. The port documents its
  own gaps (numbers as floats, Go's regular expressions), so it is a second engine that needs the
  same parity tests.
- **CEL with a translator to SQL.** Two engines, and no translator states that its SQL computes
  what CEL computes; CEL errors on a missing key where SQL yields `NULL`.
- **A language for users to write.** ADR 0054 refuses a second condition language; the path is a
  compile target, never shown.
- **Evaluating everything in Go.** A Segment counts Events across every Contact at once; that count
  belongs in the database.
