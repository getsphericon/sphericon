# Review rules

Draft. The concepts a change is reviewed against, the ones asked for again and again in review.
Only conceptual, permanent rules are listed here. A rule about a concrete type, package or tool
(`*ent.Scoped`, `goleak`, how a name is spelled) belongs in `CODING_STANDARDS.md` or `AGENTS.md`.
How a rule is enforced is decided after the list is agreed.

## Enforcement (decided)

- A check is turned on at once and the violations it finds are fixed, with no baseline of known
  violations.
- A check that needs Go types (a hand-written mapper, a reference without a foreign key, logic in a
  job) is an analyzer of our own, built as a `go/analysis` module and run by `golangci-lint custom`.
  A check that needs only syntax is an ast-grep rule (`ast-grep/rules`, run by `mise run check:ast`).
- A rule that code cannot check (2, 3, 5, 12, 13) is the third axis of the `code-review` skill: it
  reads this file and checks the diff against it.
- Every check's message names the rule number and says how to fix the violation.

## AUTO: automation over hand-rolling

1. **Use what can be generated.** An artifact that has a generator comes from it (ent → resource
   mappers, API client, zod, routes, mocks, the fixture catalog) and is never edited by hand.
2. **Look for a ready-made generator or library, or for a way to write one.** A hand-written construct
   that repeats (three or more places) or whose shape another artifact already defines is a sign to
   search for an existing generator or library; if there is none, to consider writing the generator.
   A library that is not in the project is proposed, not replaced by our own code silently.
3. **Ready-made over our own.** A concern a mature library or component covers uses it, adopted in the
   same change. If a library lacks a feature or has a bug, consider a PR to it before a wrapper or
   workaround; a workaround is temporary.
4. **No raw SQL.** Queries are built by ent or a query builder. Raw SQL is allowed only where ent
   cannot express it, with the reason written next to it.

## Layers and boundaries

5. **Business logic is a layer of its own.** Handlers of every surface, MCP, jobs and event subscribers
   only receive input, call a domain module and return the result; they hold no logic.
6. **Knowledge has one place.** A rule or a shape is stated once; no second copy of it elsewhere. For
   the API that place is TypeSpec, and none of the following is declared anywhere else: operations
   (path, method, name), request and response shapes, value constraints (length, range, format,
   pattern), enums and unions, error problems and which an operation returns, the auth scheme of a
   surface, query and header parameters, field and operation docs, the telemetry report schema, and
   the Service credential scopes. (The persistence shape is the ent schema, not TypeSpec.)
7. **A state is stored as the shape its history needs.** Decided per field from the facts:
   1. If anyone needs to know when, why or by whom it changed, store the moment as `<x>_at` (plus a
      reason or actor column); a boolean cannot hold that.
   2. It is a state machine, with legal transitions declared in one place, when there are three or
      more states, or two with an enforced order, or a background job can race a user, or the move
      has side effects (a mail, a retry, a lease).
   3. A two-position toggle with one guarded write, where nobody asks when or why, stays a plain flag.
   4. A boolean that a background job flips on its own (`verified`, a webhook's `enabled`) is the real
      smell: it races with users and needs a timestamp and a conditional write.
   5. A setting the user chooses that does not change by itself (`is_default`) is an attribute, not a
      state.
8. **The database holds declarative invariants and no logic.** Constraints, keys, checks and indexes
   yes; functions, triggers and views no. Logic that runs on every write lives in Go. Every reference
   to a row is a foreign key with an explicit `ON DELETE` (`SET NULL` for a fact that outlives its
   target, `CASCADE` for a row that dies with it). A reference without a key is allowed only for a
   tombstone or a snapshot, with the reason written at the field.

## Data

9. **Deletion is a state, by class** (ADR 0049). A row is one of three kinds:
   1. Configuration without a history trail (Tag, Template, Webhook endpoint, Membership) is deleted
      physically; the Audit log is its trace.
   2. Configuration that history or live work depends on (Segment, Integration, Sending domain,
      Automation, Broadcast) is never removed while something live uses it, and is otherwise discarded
      by one shared soft-delete abstraction; what is archived takes no part in new work.
   3. Facts (Event, Outbound message, Audit entry, Suppression) are never discarded, only removed by
      Retention or Erasure.

## Tests

10. **Test data comes only from fixtures.** A test creates rows itself only when creating them is the
    operation under test (a write through the client, the scoped wrapper or the bus). A shape the
    shared set cannot hold (an old schema for a migration, a generated volume) is solved by tooling
    for fixtures, not by an exception.
11. **A test asserts the result on a real database.** Calls to mocks are not asserted. An outside
    boundary (mail, a provider's HTTP) may be replaced by a stub, as long as the test looks at what
    was left behind and not at the calls.
12. **Tests cover our logic only**, not the behavior of a library.

## Process

13. **Fix the cause, not the symptom, whatever the size of the rework.** Change the contract, schema,
    module and callers instead of patching where the problem showed up.
