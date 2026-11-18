---
status: accepted
build: [1674, 1675, 1676, 1677, 1678, 1679]
---

# A Condition's shape is typed by the place it is used

> Amends [[0029-segment-rules-compile-to-generated-predicates]] (the shape of a leaf),
> [[0054-a-rule-is-a-stateless-reaction-to-an-event-built-on-the-segment-condition-tree]] (`trigger:<key>`),
> [[0122-an-automation-branches-into-nested-paths-that-rejoin]] (where the test of an Enrollment's own
> messages is refused) and [[0136-a-wait-for-event-keeps-the-event-that-ended-it-for-later-steps]]
> (`awaited:<name>.<key>`), on top of [[0180-a-property-test-runs-in-postgres-as-one-sql-json-path]].
> It also makes an exception to [[0033-upgrades-add-within-a-major-and-never-stop-a-running-instance]]
> (a release inside a Major runs on the schema of any other release of it): see Consequences.

## Context

One Condition grammar serves a Segment, a Rule, a Branch, a Wait for event, an Event matcher's
filter and an Order's line filter (ADRs 0054, 0122, 0124, 0133, 0153, 0162, 0180). The contract has
one tree type for all of them. A leaf names what it tests in a string, `field`, whose prefix carries
the kind: `custom:<key>`, `trigger:<key>`, and `awaited:<name>.<key>` where the name runs to the
first dot. The operator is one enum for every kind of value. What a place may hold is a refusal at
runtime: a Segment refuses `trigger:`, a Rule refuses the test of an Enrollment's own messages, a
Wait for event's Condition refuses it too, a matcher's filter refuses everything but property tests.

So a client, an MCP agent and the editor learn what a place takes only from a refusal; a generated
type accepts a tree that the server then rejects; and each new leaf kind or subject is another prefix
to parse.

## Decision

- **A leaf is a member of a discriminated union by `kind`**, each kind with typed fields of its own:
  a Contact attribute, a Tag, Telegram and SMS reachability, a Custom field, a Score, an `event`
  count, an `order` test, a property test, the sent-by test of a message's System event (ADR 0180),
  and the test of an Enrollment's own message. Its operators are typed by the value it tests (text,
  number, moment, presence), so a text operator on a number is not a value the type holds.
- **A property test names its subject in a typed field, never in a prefix.** Inside a filter (an
  Event matcher's, an `event` leaf's, a line filter) it has none: it tests the one Event or line the
  filter is about. In a Condition it is `trigger` (a Rule's triggering Event, an Enrollment's
  `event`) or `awaited` with the name of a Wait for event. The key is a field of its own, so a key
  holding a dot needs no rule.
- **Each place has a tree type of its own**, made from one TypeSpec template over the leaf kinds it
  admits:

  | Place                      | Admits                                                                          |
  | -------------------------- | ------------------------------------------------------------------------------- |
  | Segment                    | the Contact's leaves, `event`, `order`                                          |
  | Rule                       | a Segment's, and a property test of `trigger`                                   |
  | Branch path                | a Rule's, a property test of `awaited`, the test of an Enrollment's own message |
  | Wait for event's Condition | a Branch path's, without the test of an Enrollment's own message                |
  | Event filter               | subjectless property tests, and the sent-by test on a message's System event    |
  | Line filter                | subjectless property tests                                                      |

  A leaf outside its place is refused by the schema and has no value in the generated Go and TS
  types; the editor and MCP are built from the place's type.

- **What depends on data stays a check at save**: a key's form; that a named Tag, Custom field,
  Score, Broadcast or Automation exists; that a named send Step or Wait for event can come before;
  that a sent-by test filters a message's System event.
- **Whether a Condition can be unknown is read from its leaves.** It can when it holds an engagement
  count of `email.opened`, `email.clicked`, `sms.clicked` or `telegram.clicked` (with or without a
  filter, ADR 0123) or the test of an Enrollment's own message (ADR 0122). One function says so, and
  both the Publish check of a Branch's unknown path (ADR 0124) and the editor use it.
- **Stored Conditions are rewritten** to the new shape by one migration; no reader of the old shape
  is kept.

## Consequences

- The contract of Segments, Rules and Automations changes, and with it `/site`, `/api` and MCP.
- A recursive union instantiated per place is checked first through TypeSpec, the OpenAPI emitter,
  ogen and the TS generator, before the trees are moved onto it.
- The runtime refusals of a misplaced leaf are deleted, and their tests become tests that the
  schema refuses the leaf.
- The migration that rewrites the stored Conditions starts a Major. A release before it cannot read
  the typed tree, and no reader of the old shape is kept, so the migration adds the next Major and
  its floor (ADR 0033) and an older release refuses to start on the rewritten data. While it runs,
  the release still serving cannot read what it rewrote: a Segment's preview, a Broadcast due in
  that window and a Rule's evaluation may fail until the new release starts. The pull request that
  lands the move is a breaking change whose `Upgrade:` line says so. Here the
  promise that a running Instance is never stopped does not hold in full; keeping it would need
  the release before the move to read the typed shape, the compatibility reader this ADR rules out.

## Considered options

- **One tree type with refusals at runtime** (what the app has). The contract cannot tell a client
  what a place takes, and every refusal is code and a test of its own.
- **Typed leaves with a string subject**, as a property path in one string. The subject and the
  place it is valid in stay unchecked; a typed union of leaves alone leaves half the problem.
- **A schema per place written by hand.** Six trees drift apart; a template over the leaf kinds
  keeps them one grammar.
