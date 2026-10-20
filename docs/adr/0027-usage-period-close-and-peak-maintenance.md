---
status: accepted
---

# Usage metering: one periodic job closes periods, an ent hook keeps the contacts peak

ADR 0009 fixed what the core measures and where the boundary to an external service lies. This
ADR fixes how the two metrics are kept so that a Usage snapshot is exact, final on time and
invisible to a self-hosted install.

## Decisions

- **One periodic job, a pure function of the clock.** `ee/usage` registers one river job through
  `Edition.Jobs()`, run every 5 minutes. For every Workspace it closes each elapsed Usage period
  that is not final and refreshes the open one. Its result depends only on the time it is given and
  the stored data, so the tests drive it with a test clock and never wait.
- **Close bound.** A period closes within one hour after 00:00 UTC on the first of the month: the
  job interval is far below that hour, and a missed run (downtime, a failed job) is caught up by the
  next one for every unclosed past period, from the Workspace's creation month on. The reader of the
  snapshots must set its own invoice grace well above the hour; the core does not enforce it.
- **`emails_sent` is recomputed from the Outbound messages the provider accepted**, by the time
  they were accepted (`sent_at`), so the figure of a closed period is the same whenever it is
  computed: a message accepted at 23:59 UTC and one at 00:01 UTC land in different periods, and a
  send claimed before midnight but accepted after it belongs to the next. Sends claimed and not yet
  answered (in flight) are never in it: the open period reports them as their own `in_flight_sends`
  figure (0 in a snapshot), so an unanswered send is not billed as sent.
- **`contacts` is a peak, so it cannot be recomputed**: an ent hook, installed on every client
  (the long-lived one and the bus's transaction client) only with the `staff` license, raises
  the open period's peak in the same transaction as the Contact change that identifies, gives an
  email Destination to, clears the email of, or erases a Contact. A change that can lower the count
  records the count first. A new period's peak starts at the Contacts there are at the period's
  first refresh or change.
- **A final row is never rewritten.** Closing is idempotent by (Workspace, period, metric): the
  update is conditional on `final = false`, and a re-run, a late run or a later straggler leaves a
  closed figure unchanged.
- **Snapshots are independent of retention.** The retention job (ADR 0014) removes Audit entries
  only and never touches the usage table, so a closed snapshot outlives the Events it was computed
  from and old invoices stay reproducible by their closed figure.
- **Unlicensed means invisible.** Without the `staff` license the job does nothing, the hook is
  not installed (no row is ever written), the send chokepoint never freezes on a Staff hold, no
  listener is bound for the service endpoints and the public port answers 404 for them. Send
  outcomes are identical to a core without any of this.

## Considered options

- **Count at `email.sent` publication into a running counter** (rejected): a second write on the
  hot send path, and a counter that drifts from the messages it summarizes. The messages are the
  truth and the job re-derives the figure from them idempotently.
- **Recompute the contacts peak at close** (rejected): the peak is not recoverable once Contacts
  are deleted or erased.
- **Run the close once a month** (rejected): one missed run would leave a period open for a month.
  A short interval makes catch-up the normal path rather than a special one.

## Consequences

- A Live usage figure is at most one job interval stale and is never recounted on read.
- The first period after rollout may under-count `contacts` for a Workspace nobody touched: it
  falls back to a lower bound (the identified Contacts that existed before the period ended and
  still do).
