---
status: accepted
---

# A constraint violation is a refusal its owner declares

## Context

Review rule 8 leaves the last word on uniqueness and checks to the database. The step from a violation
to the refusal a caller sees is written again in each module, and the copies disagree:

- **Three ways to detect a violation:** `ent.IsConstraintError` (`passkeys`, `signupitems`, `holds`),
  `db.IsUniqueViolation` (`sendingdomains`, `joindomains`, `trackingdomains`) and a raw
  `*pgconn.PgError` (`contacts`, `integrations`).
- **Three ways to name the broken rule:** a substring of the constraint name (`contacts`), the exact
  name (`integrations`), and a re-read after the miss (`signupitems`).
- **No shared answer.** `internal/api/problem` has no case for a violation, so one a module does not
  catch answers 500.
- **A violation taken for success.** `settled` is copied three times (`confirmations`, `channelstop`,
  `unsubscriberecording`) and returns nil for any constraint error, a CHECK included. The CHECK
  constraints of the Email case exist to fail loudly when a write goes around the rule.
- **A rule mapped outside its owner.** The unique email of a User is refused by the use cases `signup`
  and `workspacecreation`, not by `users`.
- **The same rule, two answers.** A taken key is 422 in `signupitems` and 409 in `contacts`,
  `joindomains`, `sendingdomains` and `integrations`.
- **Three ways to survive a violation inside a transaction.** A violation aborts the Postgres
  transaction it happens in. `sendingdomains` opens a savepoint first, `rules` catches after its
  transaction ended, and `automations` catches inside its caller's transaction and reports success.
- **Two checks for one rule.** `signupitems` and `staffmembers` read for a duplicate before they insert
  and also catch the violation. Only the catch holds against a concurrent insert.

The workaround behind `settled`, "the Workspace is gone", cannot happen: a Workspace row is never
removed. Its deletion cycle ends in a tombstone (`workspaces.TombstoneIn`, ADR 0043).

## Decisions

- **The class of a violation decides what it can become:**
  - A **unique** (or exclusion) violation is what its owner declared for that constraint: a refusal,
    or a fault.
  - A **foreign key** violation is always a fault. A write that can race the removal of a row it
    references locks that row first (ADR 0099), and does not catch the violation.
  - A **CHECK** violation is always a fault. A CHECK backs a rule the owner already applies, so
    reaching it means a write went around the rule.
- **The owner declares each unique constraint of its tables**, by its exact name, beside the functions
  that write them:
  - as a refusal: the module's exported sentinel, a `Conflict` with the field it concerns (a taken key
    is the subject's state, not a bad input: ADR 0109's reading of review rule 15);
  - or as a fault, with the reason no client can break it (a token hash, an internal key).
- **One function in `internal/infra/db` reads a violation.** It takes the write's error and the
  owner's declarations, recognises the violation by its SQLSTATE and constraint name, and returns the
  declared refusal, or the error unchanged. No module inspects `pgconn` or `ent.IsConstraintError`
  itself.
- **A write that may be refused runs under a savepoint**, opened by the same function, so a refusal
  leaves the caller's transaction usable. This holds whoever opened the transaction, so a module never
  needs to know.
- **A write that is done when the row is already there says so in SQL:** `ON CONFLICT DO NOTHING`, never
  a caught violation. `settled` is removed.
- **No read before an insert** to find a duplicate. The constraint is the one check.
- **No fallback in `problem`.** An undeclared violation stays a 500 and shows in the log. A fallback
  answer would hide a missing declaration.
- **A test holds the declarations to the schema.** It lives in `internal/app`, the one package that
  imports every module, the Enterprise Edition's included, and fails when:
  - a unique constraint of the migrated schema (`pg_catalog`) has no declaration;
  - a declaration names a constraint the schema does not have;
  - a declaration is made by a module that does not own the table, by the ownership map of
    `entrywrite` (ADR 0102);
  - a declared refusal is never asserted with `ErrorIs` in a test of its owner's package. Its test
    breaks the constraint through the module's interface on a real database (review rule 11).

## Considered Options

- **A registry the wiring fills.** It moves the list of refusals out of sight of the module that writes
  the rows. ADR 0102 rejected ports and registries for the same reason.
- **An annotation on the index in `ent/schema`, with generated sentinels.** The schema cannot import a
  domain module, so the refusal would leave its module, and a generated sentinel has no i18n message.
- **A case in `problem` that answers every unique violation with 409.** It is the narrow fix: no module
  would need to declare anything, and a missed declaration would never show.
- **Implicit faults.** If an undeclared constraint were a fault by default, the test could not tell a
  forgotten declaration from a deliberate one.

## Consequences

- Every module that writes a table with a unique constraint declares it, and its catching code goes.
  `users` declares its unique email, and `signup` and `workspacecreation` stop mapping it.
- `signupitems` answers a taken key with `Conflict`, like the others.
- `automations`' enrollment, `confirmations`, `channelstop` and `unsubscriberecording` write their
  idempotent rows with `ON CONFLICT DO NOTHING`.
- Review rules 8 and 15 and the "Only the owner writes a table" paragraph of `docs/architecture/modules.md`
  change with the code, not before it.
