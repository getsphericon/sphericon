# Review rules

The concepts a change is reviewed against, the ones asked for again and again in review. This file
is the one home of every rule it lists: `CODING_STANDARDS.md` holds how the repo does things
(mechanics, tools, conventions) and does not restate a rule from here. A rule about a concrete type,
package or tool (`*ent.Scoped`, `goleak`, how a name is spelled) belongs in `CODING_STANDARDS.md`
or `AGENTS.md`.

## Enforcement

- A check is turned on at once and the violations it finds are fixed, with no baseline of known
  violations.
- A rule that needs Go types is checked by an analyzer of our own; one that needs only syntax, by an
  ast-grep rule. Which rules have a check, and how to add one, is in "Project checks" of
  `CODING_STANDARDS.md`: this file does not list them.
- A rule that code cannot check is left to the reviewer, who reads this catalog through the Standards
  axis of the `code-review` skill (`CODING_STANDARDS.md` and `AGENTS.md` both point here). A rule with a check is still reviewed: a check finds the form, a
  reviewer finds the intent.
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
4. **No raw SQL.** Queries are built by ent or a query builder, in code and in tests alike. Raw SQL
   is allowed only where ent cannot express it, with the reason written next to it.

## Layers and boundaries

5. **Business logic is a layer of its own.** Handlers of every surface, MCP, jobs and event subscribers
   only receive input, call a domain module and return the result; they hold no logic.
6. **Knowledge has one place.** A rule or a shape is stated once; no second copy of it elsewhere. For
   the API that place is TypeSpec, and none of the following is declared anywhere else: operations
   (path, method, name), request and response shapes, value constraints (length, range, format,
   pattern), enums and unions, error problems and which an operation returns, the auth scheme of a
   surface, query and header parameters, field and operation docs, the telemetry report schema, the
   Service credential scopes, the scopes an external operation requires, the Permissions a `/site`
   operation requires and the Permission each grantable scope needs (ADR 0061). (The persistence
   shape is the ent schema, not TypeSpec.)
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
11. **A test asserts the result on a real database.** Calls to mocks are not asserted: no test checks
    what was called, how often or with which arguments. An outside boundary (mail, a provider's HTTP)
    may be replaced by a stub, as long as the test looks at what was left behind and not at the calls.
    The facts asserted are business facts, not accidents: no row counts of a whole table or raw ids.
    A test is black-box: it sits in the package's `_test` package and goes through what the package
    exports, and no `export_test.go` opens the package's insides to it. A command (`cmd/*`) is a
    handler like an HTTP one: its logic lives in an internal package, which is tested, and the
    command has no tests.
12. **Tests cover our logic only.** A test exercises a decision the code makes and checks what came
    out, not the behavior of a library, how long something takes, how much memory it uses, a timeout
    firing on the wall clock, a runtime limit, or the machine and the process around the code. A
    test with only a library or component as its subject is deleted and none is added. No load,
    stress or soak test is written: no test repeats an action to provoke a race or a leak, or
    measures throughput. A UI test (a page rendered and clicked in a browser) is written only for a
    critical path; the logic behind a page is tested as logic, in a `*.test.ts` file.

## Process

13. **Fix the cause, not the symptom, whatever the size of the rework.** Change the contract, schema,
    module and callers instead of patching where the problem showed up.

## Test bodies

14. **A test has no branching.** No `if`, `switch`, `select` or loop over cases that chooses what to
    check, in a test body or its subtest closures: a branch lets a wrong run take the path that
    checks nothing. State the expectation on a value read beforehand; keep a table's rows as data
    (split a table whose rows need different assertions, give a row its expected value rather than a
    flag); logic that must branch goes into a named helper.

## Refusals

15. **A refusal answers the same on every surface, and is mapped once.** 400: the request could not
    be decoded. 422: it was decoded and a module's rule refused it. 409: the subject is in use. 401:
    no credential, or an invalid one. 403: a valid credential without the scope or Permission. 404:
    not found, or a Workspace the caller is not a member of. A handler returns the module's error;
    the server's error handler turns it into the problem, so no handler writes the same mapping again.
