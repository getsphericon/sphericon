---
status: accepted
---

# An Automation branches into nested paths that rejoin

> **Amended by [[0123-engagement-history-is-unknown-for-an-untracked-destination]]:** history tests
> of engagement are unknown for an untracked Destination too, and a Branch holding any engagement
> test names the path unknown takes.

> **Amended by [[0124-an-automation-chooses-a-path-by-condition-by-weight-or-by-an-awaited-event]]:**
> a Branch holds an ordered list of paths with Conditions and a last `otherwise`, of which `yes` and
> `no` are the two-path case; paths have ids. The unknown path is a path of its own, with its own
> Steps. A Split and a Wait for event fork too.

> **Amended by [[0126-an-automation-starts-on-a-date-and-waits-until-a-local-time]]:** the one
> Trigger is an Event action or a date; a date Trigger has no entry Condition either.

An Automation version holds a linear array of Steps, and an Enrollment points at one of them. The
canvas draws a graph, but saving flattens it into a chain and drops whatever is off it. A marketer
cannot send people down different paths by their data or behaviour, which is what ROADMAP R12 asks
for, starting with an if/else on the existing Condition language.

Two shapes exist. A free graph of nodes and edges (Notifuse's journeys are one) lets any node lead
to any other, so whatever accepts one must reject dangling nodes and cycles. A structured tree,
like an `if` in code, has neither by construction.

## Decision

- **A Branch is a Step with a Condition and two nested sequences of Steps**, `yes` and `no`. They
  rejoin right after the Branch: an Enrollment that finishes either path goes on to the Step that
  follows it. A path may be empty, meaning "go on". Branches nest: a path may hold another Branch.
- **An End step finishes a path early**, ending the Enrollment as `completed`: the end the author
  designed, not an exit. A Step after an End in the same path is unreachable, and a save with one is
  refused; so is a Branch without a Condition.
- **Every Step has an id**, given when it is created and kept by every version of the Automation,
  as an HTTP step's already is. Everything that names a Step by its position today names it by the
  id instead: the Enrollment's current Step, the Outbound message's step, the send's idempotency
  key and the HTTP call's id, the report's figures per Step across versions, and now the path a
  Branch chose and the engagement test below. Inserting a Step above changes none of them.
- **The Condition is the one language** of Segments and Rules: the Contact's fields, Custom fields,
  Tags and past Events, and the properties of the Event the Enrollment began with (its `event`
  copy), as a Rule reads its triggering Event: decided in memory and folded into the tree as a
  constant, a property the Event lacks (or an Enrollment no Event began) failing the test.
- **It adds one test, on the Enrollment's own messages**: the message a given send Step sent to this
  Enrollment was opened (an email Step only) or clicked (a Step whose Channel tracks clicks). It is
  folded in memory like a triggering Event's property, and it has three values: it is unknown when
  that message went out untracked (ADR 0120), when the Step was skipped, and when the Enrollment
  never passed that Step, as on the other path of an earlier Branch. Unknown passes through the tree
  by Kleene's rules, as `NULL` does in SQL: `AND` is false if any side is false, `OR` is true if any
  side is true, and `NOT` keeps unknown unknown, so "did not open" is unknown too, never true. A
  Condition that ends unknown takes the Branch's unknown path, which a Branch holding this test
  must name, or it cannot be published: otherwise everyone who refused tracking would be sent down
  "did not open" and get the resend. The test may name only a send Step that can come before the
  Branch, never one after it or inside its own paths, and a Step a test names cannot be removed.
  It is valid in a Branch only: a Segment and a Rule refuse it, as a Segment refuses a triggering
  Event's property. History tests stay two-valued.
- **A Branch is evaluated once, when the Enrollment reaches it**, on live data. The path it took is
  recorded on the Enrollment in the same transaction as the move, so a worker that restarts never
  evaluates it twice and the report reads what happened.
- **Entry stays a Rule's job.** An Automation keeps one Trigger. Entering only when a Condition holds
  is a Rule with Trigger events, a Condition and the enroll action, which consumes no Enrollment
  when it does not match; a second mechanism on the Automation would say the same thing again.
- **Versions do not change**: an Enrollment stays on the version it began on (ADR 0065), Branches
  included. Exit and goal events end it wherever it stands.
- **The report shows each Branch's split**: how many Enrollments took `yes`, `no` and, where it
  applies, unknown. Each Step keeps its own figures, wherever it sits.
- **Limits count every path**: the version's limits on Steps (today the 10 HTTP steps) count the
  Steps of all paths; nesting has no limit of its own.

## Consequences

- The version's `steps` become a tree with ids; the contract, MCP and the editor's
  `stepsToGraph` and `graphToSteps` follow it, and the canvas draws a Branch as a split that rejoins.
- `automation_runs.current_step`, `outbound_messages.automation_step`, the send key
  `automation:<run>:<step>` and the report's per-position figures move from a position to a Step
  id; the Enrollment records each Branch it passed with the path taken.
- The Removal check reads the Conditions of Branches as Definition use (Custom fields, Tags).
- Not in this decision, each for its own: an N-way split, an A/B split (the Step id makes its
  assignment stable), waiting for an Event or a timeout, and an exit on a reply, which needs inbound
  mail.

## Considered options

- **A free graph.** It allows merging paths anywhere and loops, at the cost of validating dangling
  nodes and cycles in every editor, in the contract and on Publish; the jumps it adds over nested
  blocks are rare and hard to follow.
- **An entry Condition on the Automation, checked before the once-ever Enrollment.** It duplicates a
  Rule that enrolls.
- **Two-valued engagement tests, untracked read as "not opened".** It would quietly resend to
  everyone who refused tracking.
- **Forbidding a test of a Step in a sibling path.** After two paths rejoin, "opened the email of
  the `yes` path" means something for those who took it; unknown already covers the rest.
