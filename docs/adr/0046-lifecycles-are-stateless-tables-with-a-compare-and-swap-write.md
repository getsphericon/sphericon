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
- **The machines:** Broadcast, AutomationRun, OutboundMessage (its lease
  fence stays inside the write; a Broadcast recipient has no machine of its own, see "Amendment:
  a Broadcast recipient has no state of its own"), the Workspace deletion cycle (`active`, `pending_deletion`,
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
- **Queued jobs resume, they do not plan.** A job is enqueued after the claim (`Send` moves to
  `sending`, `Schedule` to `scheduled`), so the worker runs the sequence with the `resume`
  trigger, legal from `scheduled` and, as a convergent re-run, from `sending`, never from `draft`.
  An unschedule that lands after the job's `Due` check therefore cannot be undone by the plan:
  its compare-and-swap loses and the job ends as stale. `Due` stays as the cheap check of the
  usual case. A reschedule that lands in the same few statements is not closed: the job would
  plan the Broadcast ahead of its new time.

## Enrollment: a side effect is fenced against an exit

A compare-and-swap decides who moves the row, not whether a mail or an HTTP call already left,
so a step that checked `active` and then sent could still send after an exit. The step now runs
its side effect (the Outbound send of an email step, the POST of an HTTP step) under
`jobs.FenceOn`: a `SELECT ... FOR SHARE` of the Enrollment's row, `WHERE status = 'active'`, on a
transaction of its own that does nothing else, held until the effect returns. Every move out of
`active` is an UPDATE of that row and waits for a share lock, so an exit that arrives during the
effect returns after it (the mail is ordered before the exit), and an exit committed before the
lock is seen under it (the effect never starts, the step ends as stale).

- The step's own compare-and-swap (`Advance`, `Complete`, `Fail`) runs after the lock is released,
  so it cannot wait on itself. The outbound claim, lease and idempotency key are unchanged: the
  effect runs on the pool's other connections.
- The hold is as long as the effect: the job's context and the provider and HTTP client timeouts
  bound it. An exit waits that long at most.
- It costs one extra `database/sql` connection per in-flight email or HTTP step. The ordinary
  tests ride one transaction on one connection, where a held lock would starve the effect's own
  queries, so they pass a no-op `Fencer`; the ordering tests use a database with real
  connections (`testhelper.RealDBURL`).
- Not fenced: tag steps (they send nothing; a tag applied just after an exit is a data change,
  not a message) and a `wait`.

## Amendment: a Broadcast recipient has no state of its own

BroadcastRecipient was first listed as a machine (`pending`, then `sent`, `skipped` or `failed`).
That state duplicated the Outbound message's, so every send was recorded twice, in two writes, and
a crash between them left the two rows disagreeing until a retry repaired them. ADR 0015 already
says what the recipient is: the frozen audience snapshot and the engagement rollup, pointing at
its message. So the recipient has no status, no sent time and no error; the Outbound message is
its one machine (above), and a recipient is _pending_ while it has no message, or one that is still
claimed.

- **Every recipient ends with exactly one message.** A recipient that can never go out (its Contact
  has no address, or its last attempt failed) still ends with a Failed message, which Outbound send
  records when no attempt ever claimed the key. A live claim is not failed: it may still reach the
  provider, so the give-up waits out the lease and asks again.
- **A Broadcast's figures are derived, never stored.** Recipients, sent, skipped, failed and pending
  come from the recipient rows and their messages, and opens, clicks and unsubscribes are the
  timestamps on the recipient, set once each. The figures are live while the Broadcast sends and
  cannot drift from the rows. Only the Broadcast keeps a machine here, and finalizing it is the
  `sending` to `sent` move once no recipient is pending.
