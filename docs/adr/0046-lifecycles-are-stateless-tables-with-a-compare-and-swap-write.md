---
status: accepted
---

# Lifecycles are `stateless` transition tables with a compare-and-swap write

Six entities move through states that code changes with a bare `UpdateOneID(...).SetStatus(...)`,
or with a guard written at one call site and forgotten at the next. No transition table exists
anywhere, so "which states does a Broadcast have, and which moves are legal" is answered by
reading every writer, and the writers disagree: `PlanBroadcast` sets `sending` from any status
(a re-run after `sent` regresses it), an AutomationRun's step writes can overwrite `exited` with
`completed`, `restore` and `erase` of a Workspace are unconditional updates, and a Sending
domain's `tracking_state` is written from a row that was read earlier.

## Decisions

- **One library, one idiom: [`qmuntal/stateless`](https://github.com/qmuntal/stateless) with
  external storage** (`NewStateMachineWithExternalStorage`). Each lifecycle is a declared table
  (`Configure(...).Permit(...)`) in the module that owns it, and the table is the answer to
  "what exists". States and triggers are the ent enums, not strings.
- **The write is the compare-and-swap.** The storage `write` function is a conditional
  `UPDATE ... WHERE status = <the state read>`; a miss fails the transition, so the database, not
  the process, decides a race. Exit actions run before the write and are not rolled back: they
  carry no side effects.
- **The machines:** Broadcast, BroadcastRecipient, AutomationRun, OutboundMessage (its lease
  fence stays inside the write), the Workspace deletion cycle (`active`, `pending_deletion`,
  `erased`; restore only from `pending_deletion` inside the window, erase only from
  `pending_deletion`) and a Sending domain's `tracking_state` (every edge listed, demotions
  included). A table for an any-to-any machine is still the map of what exists.
- **Not machines:** one-way latches (`revoked_at`, `used_at`, `email_verified_at`), two-state
  flags (`Integration.enabled`, `Automation.status`) and Workspace suspension keep their own
  guarded writes.
- **One error.** `internal/lifecycle` is a thin constructor over `stateless`: it sets
  `OnUnhandledTrigger` and maps a compare-and-swap miss to the same `ErrInvalidTransition`. The
  error is never swallowed. A worker treats it as a stale event and finishes the job without a
  retry; an API handler answers 409. The tables stay in their modules.
- **Gaps are fixed upstream.** The library returns an untyped `fmt.Errorf` for an unpermitted
  trigger; a typed error goes to `stateless` as a pull request, not blocking, since
  `OnUnhandledTrigger` already replaces the behavior here. A pull request to another
  repository is opened only after review of the diff.

## Consequences

- Each fix above lands with a failing test first: the `PlanBroadcast` regression from `sent` or
  `failed`, `exited` overwritten, `restore` after `erased_at`, a stale `tracking_state` write.
- The first step of the Broadcast change is a spike: a `write` that returns a compare-and-swap
  miss makes `Fire` return that error and leaves the state unchanged. If it does not, the library
  is fixed upstream (a fork under `replace` until the pull request is merged) rather than worked
  around here.

## Rejected

- **Hand-rolled `map[Status][]Status` tables.** They validate the edge but not the race, and each
  module would invent its own error mapping. The repository prefers a maintained library to a
  local helper.
- **`looplab/fsm`.** Its state lives in the machine instance, with string states; its README
  documents no external storage, so the compare-and-swap would sit in callbacks around an
  in-memory copy of the state.

## Broadcast: where the table departs from the plain flow

- **Plan is also legal from `sending`.** The claim (`Send`) moves a Broadcast to `sending` before
  the plan job runs, and a held plan is retried from `sending`, so the table permits plan as a
  re-entry there (a re-plan converges on the same recipient rows). Plan is refused from `sent` and
  `failed`, which is the regression this ADR set out to fix.
- **Revert** (scheduled or sending back to draft) exists for the case where the job cannot be
  enqueued after the claim.
- **Known window.** A queued plan job checks that its Broadcast is due, then plans. An unschedule
  that lands between the two moves the Broadcast to `draft`, and the plan, legal from `draft` for
  direct callers, claims it. Closing it needs the job to carry the state it was enqueued for;
  the window is a few statements wide and left open.
