---
status: accepted
---

# Background work runs per Workspace, in batches that persist

River runs every job with its defaults: a one-minute timeout, 25 attempts, a crashed job rescued
after an hour. Only the Broadcast plan job sets its own timeout (ten minutes). Several periodic jobs
walk every Workspace in one run (`ee_usage_refresh`, `ee_abuse_detector`, `ee_retention_prune`):
their work grows with the instance, and a run cut off by its timeout keeps nothing and starts again
from the first Workspace, so on a large instance it may never finish. Prior art: Notifuse runs its
long jobs as fifty-second slices that resume from a saved cursor, and one of its loops takes a
`SKIP LOCKED` lock that ends with its statement and so claims nothing; two of its instances can run
the same journey node.

We keep River's machinery and add no cursors or time budgets of our own.

## Decision

- **Work over every Workspace fans out.** A periodic job that concerns all Workspaces only queues
  one job per Workspace, as the sending domain, join domain and send quota rechecks already do. A
  failure then stays in its Workspace, and a larger instance means more jobs, not a longer one.
- **Large work inside one Workspace runs in batches whose results persist at once.** A run cut off
  by a timeout or a shutdown loses at most the batch in flight, and the next run continues from what
  is already done: deleted rows stay deleted, planned recipients stay planned (ADR 0046). The progress
  is the work itself; a stored cursor is added only where nothing done can tell the next run where to
  start. Erasure, retention and the prunes already work this way, and so does the Broadcast plan,
  which converges on a retry; it is left as it is until an audience outgrows its timeout, and then it
  pages by Contact.
- **An attempt is what River counts.** A run stopped by a shutdown is `interrupted` and spends no
  attempt; a timeout and a crash spend one, since a batch is meant to fit its timeout and a miss
  means a slow database or a bug. A worker returns a context error as it got it (wrapped with `%w`
  at most), never as a refusal, a `JobCancel` or an error of its own, or River reads the shutdown as
  a failure.
- **A hand-written claim that holds a row after its statement says what holds it.** A doc comment
  names the column that holds the row (a status, a lease with its fencing token, a latch) and how the
  claim of a holder that crashed ends. A lock inside a transaction holds nothing after the commit
  and needs no such line. `SKIP LOCKED` is River's alone. An Outbound message's lease outlasts every
  attempt that can hold it (ADR 0015): a shorter one would let a retry take a live send over, and the
  provider would get the message twice.

## Considered options

- **Slices with a time budget and a stored cursor, as Notifuse does.** It is the general form, but
  every job would carry a cursor and a budget it does not need: where the work is its own progress, a
  batch that persists gives the same resumption.
- **Raising `Timeout()` where a job is tight.** It moves the wall and keeps the loss: a cut-off run
  still discards everything it did.
- **A test that runs two workers at once against each claim.** Review rule 12 forbids tests that
  repeat an action to provoke a race; the claim that guards a send already has deterministic tests of
  a takeover and of a stale token.
