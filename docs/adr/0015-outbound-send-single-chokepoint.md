---
status: accepted
---

# Outbound send: one module, one message record, two outcome scopes

Every email sphericon sends **on a Workspace's behalf** — Broadcast, Automation step, Transactional,
and a user's test send of a Broadcast — passes through **one module, Outbound send**, which
owns everything between "this message is wanted" and "the
provider accepted it": Send-eligibility, the Workspace freeze check, the Sending-domain gate,
rendering, the unsubscribe footer and RFC 8058 headers, DKIM signing, the provider call, and the
record of what happened. The surfaces keep only what is genuinely theirs: choosing _who_ gets
_which_ content _when_ (audience planning, scheduling, enrollment progress, the HTTP contract).

Before this, each surface re-implemented that pipeline and the copies had drifted: a Broadcast
checked eligibility only when planning (an unsubscribe landing before the send still got mail),
a missing tracker silently shipped marketing mail with no unsubscribe link or header, a template
error was swallowed and the raw template was sent, and `email.sent` was published three different
ways. Workspace suspension (ADR 0007) had no single place to be enforced at all.

## Scope: what is and is not an Outbound send

- **In:** Broadcast, Automation step, Transactional. Also the **test send** of a Broadcast: the
  author names the address explicitly, so it skips Send-eligibility and writes no Outbound message
  or `email.sent` Event, but it still passes the freeze check and the Sending-domain gate and is
  DKIM-signed — a suspended Workspace sends nothing, test or not. (Amended by ADR 0041: with no
  verified Sending domain it goes from the System sender instead, to the asking User only.)
- **In, when it exists:** the double-opt-in Confirmation mail (ADR 0013). It goes to a Destination
  on the Workspace's behalf, so it needs Suppression, the freeze check, the domain gate and
  signing; it has no Sending source and no unsubscribe, like Transactional. Today the confirmation
  link and endpoint exist but nothing in sphericon sends the mail, so there is nothing to migrate.
- **Out:** platform mail to sphericon's own Users — welcome, email verification, password reset, member
  invitations, and the "sending domain no longer verified" owner notice. It is sent by the
  instance's system sender, is not scoped to a Workspace's Sending domain, and must keep working
  while a Workspace is suspended or unverified (the owner has to be able to read the notice).

## Decisions

### One Outbound message record per send (replaces per-surface send state)

Each send claims an **Outbound message** row _before_ the provider call, on a unique
`(workspace, idempotency key)` index, then records the outcome and publishes `email.sent` in the
same transaction. The idempotency key is a required input, unique per logical send (broadcast
recipient, enrollment + step, transactional id), so job retries and client retries share one
mechanism. The row carries the surface and its reference, the Sending source (none for
Transactional), the Sending domain, the provider message id and the outcome. This generalises
the `TransactionalEmail` claim from ADR 0005 to all surfaces; Broadcast recipient stays the
frozen audience snapshot and engagement rollup and points at its message: it holds no delivery
state of its own, and every recipient ends with exactly one message (a recipient that cannot
go out ends with a Failed one, recorded by Outbound send). It stores provenance
and outcome, never rendered content (the by-reference rule of ADR 0005 and the copy rule of
ADR 0003 are untouched — they decide which content the caller passes in).

The record gives `email.*` Events a uniform envelope (surface reference, Sending source, Sending
domain, provider message id), which ADR 0011 needs and the old per-surface events could not carry.
The Complaint/Bounce rate stays a **flow rate** with no send↔feedback correlation, as decided in
ADR 0011; the message record makes correlation _possible_, it does not change that decision.

### Two outcome scopes: per-recipient is final, per-source is a reversible hold

The module returns a typed outcome and the caller decides what to do with it:

- **Per-recipient (final for that destination):** `Sent` (accepted by the provider — see
  GLOSSARY "Sent"), `Skipped` (Suppression, Unsubscribe, missing Confirmation), or a permanent
  failure of that one message (e.g. a render error on that contact's data). Temporary provider
  failures are retryable errors, retried by the caller's queue.
- **Per-source (`Held`, reversible):** Workspace suspension, Staff hold, an unverified Sending
  domain, a missing Integration. The caller must **not** burn recipients on these: a Broadcast
  pauses with its remaining recipients still pending, Automation Enrollments are held, a
  Transactional request gets a 4xx. Treating a freeze as a per-recipient skip would consume the
  rest of a mid-flight Broadcast so that nobody receives it after the unfreeze, contradicting
  "reversible" in ADR 0007 and "live property" in ADR 0010.

**What resumes a Hold.** The caller defers, it does not poll for an unfreeze. A
per-recipient Broadcast job that gets `Held` returns a River _snooze_ (`JobSnooze` does not
increment the job's attempt count, so a long hold never exhausts `MaxAttempts` — checked in
river v0.40), and runs again later and re-asks the module. An Automation step cannot snooze
(its next run is scheduled by the enrollment's `ResumeAt`), so a held step reschedules itself
the same way and leaves the enrollment active with its current step unchanged. A Broadcast
records the reason in a nullable `hold_reason` while any of its recipients is held and clears it
when one sends, which is the visible "held" state in the UI; a mid-flight Broadcast stays
`sending`, and a hold found when planning leaves the status as it was (the plan simply runs
again later).

**Taking over a pending claim needs a lease, and every write is fenced.** A claim row left
`pending` by a crash and one whose request is still in flight look identical, so a retry may only
take a claim over when it is provably stale: the row carries `claimed_at`, and takeover is a
conditional update `… WHERE status = 'pending' AND claimed_at = <the value read>` after the lease
has expired. Otherwise a repeat call sees the live claim and gets "in progress" (409 for
Transactional; for queue jobs a short snooze, never a spent attempt). Taking over is not enough:
an attempt that outlives its lease must not later record over the one that took the claim, so
every write an attempt makes to its own row (record sent, record skipped/failed, release, delete)
is conditional on the exact `claimed_at` token it holds and the row still being pending. A lost
claim surfaces as "in progress" and records nothing, so the Event log and the row stay
consistent even though a provider call, once made, cannot be un-made.

A template syntax error affects every recipient, so templates are validated (without recipient
data) when a Broadcast is planned and the Broadcast fails as a whole, before any recipient row
exists; a render error on one contact's data fails only that message.

### Eligibility is one destination-keyed rule, evaluated per message at send time

Send-eligibility (ADR 0001, 0013) is a single rule keyed by (channel, Destination) — a
Transactional destination may have no Contact. Broadcast planning uses its batch form (a join
from Contact to destination) to select the audience cheaply, but the authoritative decision is made
for each message at send time and is fail-closed: if the check cannot be made, the message is not
sent. There is no second, hand-written point-lookup implementation of the layers, and the
workspace's require-confirmed-opt-in policy is read inside the rule, never passed in by callers.

### Marketing sends fail closed on unsubscribe

A Broadcast or Automation message that cannot get its unsubscribe footer and RFC 8058 headers
(ADR 0012) is a failed send, never a send without them. Transactional sends carry neither, by
design.

### Transactional stays synchronous

The `/api` request calls the module directly and returns its outcome; the idempotency claim is the
Outbound message row. Queueing it would add latency to password resets and OTPs and a
"accepted but not sent" state for the client to reason about.

## Considered options

- **Unified Outbound message record vs only a common `email.*` Event envelope.** The envelope alone
  is cheaper and fixes the missing Sending-domain stamp, but leaves delivery state and the
  claim/replay logic in three places and keeps the in-transaction callback the unified record
  makes unnecessary. We chose the record because the product is greenfield and it is the one
  primitive that serves idempotency, per-message status, provider correlation and an activity log.
- **Keep Transactional outside the module.** Rejected: it needs the same Suppression, domain gate,
  freeze check and signing; separating it recreates the duplication this ADR removes.
- **Eligibility only at planning time (what Listmonk does; see
  `docs/research/outbound-send-prior-art.md`).** Rejected: a late unsubscribe or Suppression still
  sends within the already-fetched batch.
- **An external workflow engine for the send pipeline.** Rejected for now: Automation is a linear
  sequence with no branching (see GLOSSARY "Step"); revisit when branching is real.

## Consequences

- Workspace suspension and Staff hold (ADR 0007, 0009) are enforced in one place: the module's
  source-level `Held` outcome. EE supplies the freeze reasons; the core only asks the question.
  A `Freezer` reads through the Workspace's scope (`Frozen(ctx, s, ws)`) and holds no raw client; the
  Staff hold one answers from the holds whose expiry is after the clock, so a lapsed hold is
  ignored at once.
- ADR 0010's gate and ADR 0012's headers (including covering them in the DKIM `h=` list) move into
  this module and are tested once, across all three surfaces.
- ADR 0011's requirement that the send path stamp the Sending domain onto `email.sent` is met in
  one place.
- Delivery to the provider remains at-least-once (the provider call precedes the transaction);
  the idempotency key and the Event dedup id make a retry safe.
- `TransactionalEmail` is replaced by Outbound message; the per-surface delivery state on
  Broadcast recipient and Enrollment steps is reduced to a reference to it.
