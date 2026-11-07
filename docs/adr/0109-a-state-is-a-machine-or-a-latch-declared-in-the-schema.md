---
status: accepted
---

# A state is a machine or a latch, declared in the schema

> Refines [[0046-lifecycles-are-stateless-tables-with-a-compare-and-swap-write]]: it made every
> machine a `stateless` table with a compare-and-swap write and left one-way latches to "their own
> guarded writes"; this gives a latch one form too, and has the schema say which form a field takes.

## Context

ADR 0046 and review rule 7 decide per field whether a state is a machine, a latch or a plain
attribute. The code holds that decision in three places, and none of them sees every field:

- **The ast-grep rule `no-status-write-outside-lifecycle`** matches columns named `status` and
  `tracking_state`. A bare write to a Tracking domain's `role`, or to any new state column, passes it.
- **The `statemoment` analyzer** lists by hand the moments a background job moves (`verified_at`,
  `disabled_at`, `suspended_at`, …). It leaves out the moments that are "the owner's own act":
  `revoked_at`, `used_at`, `accepted_at`.
- **`versioneddefinition`.** The Automation and Rule definitions are a second lifecycle, built on a row
  lock. They have a status, live and draft pointers and an enforced order, which rule 7.2 calls a
  machine, and the ast-grep rule exempts them by name.

The writes then take five shapes:

1. `lifecycle` with compare-and-swap (Broadcast, AutomationRun, Outbound message, the Workspace
   deletion cycle, a Tracking domain).
2. A row lock with read and write (`versioneddefinition`).
3. A latch written with `…IsNil()` in its `WHERE` (API tokens, OAuth codes and refresh tokens, Recovery
   codes, Bindings, …).
4. A latch written unconditionally. `invitations.MarkAcceptedIn` sets `accepted_at` with no condition,
   and a reissue clears it again.
5. An archived row's read-only rule, enforced three ways:
   - read and check under the Workspace lock (Segment);
   - a condition in the update (Broadcast);
   - a row lock (`versioneddefinition`).

The answers diverge as well. `lifecycle.ErrInvalidTransition` answers 409. A Broadcast that is not a
draft (`ErrNotDraft`) and a change that would leave a Workspace without an Owner answer 422, although
both refuse the request because of the subject's current state.

## Decisions

- **A state field takes one of two forms:**
  - A **machine** is the ADR 0046 `stateless` table with its compare-and-swap write.
  - A **latch** is a primitive in `internal/kernel/lifecycle`. It writes `SET <x>_at = … WHERE <x>_at
    IS NULL` (with the row's other conditions) and answers a miss with `ErrInvalidTransition`. A latch
    is never cleared. A reissued Invitation is a new Invitation, so an accepted one stays accepted.
- **`versioneddefinition` is a machine.**
  - Its table is declared with `lifecycle`.
  - Its live and draft pointers are written in the same conditional update as its status.
  - The exemption by name goes.
- **An archived row is changed by an update conditioned on `archived_at IS NULL`.** A miss is the
  module's refusal "archived". A lock is taken only where Live use needs it (ADR 0108).
- **The schema declares the form.**
  - Each field in `ent/schema` that ends in `_at`, each `status` or `state` enum and each bool carries
    an annotation: `machine`, `latch`, or `attribute` with its reason (`is_default`, a moment that
    only records history).
  - A test over the schema fails on such a field without an annotation.
  - One analyzer, `statemoment` extended, reads the annotations and refuses any write to a `machine`
    or `latch` field except through the `lifecycle` primitives.
  - The ast-grep rule and the hand-kept list are removed.
- **Review rule 15 separates 409 from 422 by cause:**
  - 409 (`Conflict`) means the subject's current state refuses the request: it is in use, in the wrong
    state, its key is taken, or it lost a race.
  - 422 means the input itself is refused, whatever the state.
  - `ErrNotDraft` and the last Owner's refusal become `Conflict`.

## Considered Options

- **Every latch a `stateless` machine.** A one-state move gains a table and nothing else. The latch
  primitive gives it the same compare-and-swap and the same error.
- **Keep `versioneddefinition` on its row lock.** The lock serialises the writers, but it is a second
  idiom the checks must exempt. A compare-and-swap on the status gives the same guarantee in the form
  every other machine has.
- **Name-based checks, extended.** Any list of column names misses the next column. The schema already
  describes every field, so it is where the form is stated.

## Consequences

- The hand-written conditional latches move to the primitive, and `invitations.MarkAcceptedIn` gains
  its condition.
- Review rules 7 and 15 and `docs/architecture.md` change with the code.
- `lifecycle.ErrInvalidTransition` stays the one error of a refused move, for a latch as for a
  machine.
