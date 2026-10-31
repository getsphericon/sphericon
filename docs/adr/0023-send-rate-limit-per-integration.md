---
status: accepted
---

# Send rate limit: a per-Integration ceiling, with Deferral as a third Outcome

> **Amended by [[0067-the-send-cap-is-the-platforms-limit-the-provider-limit-the-customers]]:** the
> Send rate limit is now the **Provider limit**. On a `saas` Instance the Workspace's **Send cap**
> joins the minimum, and a spent one is a Deferral too. An SES-compatible Integration needs both
> manual values. SES `EnforcementStatus` is read with the quota, and `SHUTDOWN` is a Hold.

Broadcast and Automation sends are limited only by `QueueBroadcasts: {MaxWorkers: 10}`, one queue shared by every
Workspace. Nothing knows the provider's own limits (SES: messages per second, messages per rolling 24 hours), so a
large send against a new or sandboxed account trips provider throttling and endangers the sender's reputation.
We add a **Send rate limit** to the **Integration** (GLOSSARY) and a new reversible Outcome, **Deferral**.

## Decisions

- **The ceiling lives on the Integration**, because the provider account is what actually limits: `max per second`
  and `max per rolling 24 hours`. Defaults come from the provider where it reports them (SES `GetAccount`); an
  a value a user sets overrides; SMTP has no default (unlimited) because the remote limit is unknown. An Integration
  belongs to one Workspace, so one Workspace's backlog cannot slow another's.
- **Provider quota refresh.** SES `GetAccount` is read when the Integration is saved and then hourly by the
  existing periodic job, because SES quotas grow as an account matures. If the call fails (missing
  `ses:GetAccount` permission, an SES-compatible service such as Postbox), the manual value applies, or the
  Integration is unlimited, with a warning; a failed call never blocks sending. A failure after an earlier success
  keeps the last known provider values (a transient blip must not lift a ceiling), and only raises the warning.
  The save enqueues the lookup as a job, so the save's own response does not carry its result: the provider ceiling
  and the warning appear on the next read, shortly after. The manual ceilings are bounded when set: 1 to 10 000 per
  second and 1 to 100 000 000 per 24 hours, and a value outside the range is a 400.
- **A per-Sending-domain daily cap is reserved in the model, not built.** It is the reputation lever a warmup ramps
  (ADR 0014); the effective ceiling is the minimum of the manual, provider and warmup values.
- **Pace at enqueue time, not by snoozing.** The Broadcast planner spreads recipient jobs over time, setting each
  `ScheduledAt` at a step of `1 / effective rate`, so the limiter is a guard and Deferral is the rare case (a limit
  lowered mid-send, Transactional traffic spending capacity, provider throttling). Without this a 50 000-recipient
  Broadcast wakes its whole backlog every second and 14 of it sends. The ETA is derived from the last `ScheduledAt`
  and the recipients remaining. A Deferral that does happen snoozes for the computed wait plus jitter, scaled by
  the backlog ahead of the job: the pending recipients with a lower id that are awake, that is, not themselves
  asleep on an earlier Deferral (recorded on the recipient as `deferred_until`), since only those queue for a token.
- **Where it is reserved:** inside Outbound send, after Send-eligibility and the `Hold` checks and before the
  provider call, so Skipped or Held messages never spend capacity. Reservation takes one token from both buckets or
  none.
- **Broadcast and Automation are limited; Transactional is not delayed** (ADR 0015: it stays synchronous) **but still
  counts**: it decrements both buckets without waiting and may drive them negative, which makes later Broadcast
  and Automation sends wait longer. Platform mail from the instance's system sender is out of scope.
- **A spent limit is a `Deferral`, not a `Hold`.** The source is healthy, only busy, so there is no `hold_reason`, no
  attempt is consumed, and a Broadcast stays `sending`. The caller defers for the computed time until
  capacity returns (floor about one second), not the fixed 15-minute `Hold` delay: a Broadcast recipient job returns
  a River snooze; an Automation step cannot snooze, so it reschedules itself through the Enrollment's `ResumeAt`,
  exactly as for a `Hold` (ADR 0015). The UI shows progress and an ETA
  derived from the limit and remaining recipients, not a blocked state. A provider reply meaning "too fast" (SES
  `Throttling`, daily quota exceeded) is classified as a Deferral too; the limit is not auto-tuned in the core.
  Transactional never returns a Deferral, a provider reply included: it stays a retryable error (the claim is
  released) and the caller retries.
- **State lives in Postgres**, one row per Integration holding **two token buckets**, per-second (capacity and refill from `max per second`) and
  daily (capacity `max per 24h`, refill `max per 24h / 86400` per second), so the 24-hour limit is rolling rather than
  a fixed-window counter, declared in
  `ent/schema` and reserved atomically in a single `UPDATE … RETURNING` through `ent`; no Redis. It is a small
  package of our own, not a scattering of ad-hoc counters. No ready library fits: `ratelimiter-pg` (checked) has 15
  commits and no releases, is token-bucket only (no rolling 24h window), and owns its table and migrations outside
  Atlas; River Pro (checked) has concurrency limits but no rate limiting. We take its algorithm, including a
  retry-after for the snooze, not the dependency.
- **Core/EE boundary** (ADR 0014): the limit is core. Only **warmup automation** (a schedule that moves the ceiling,
  plus dedicated-IP management) is EE. The core exposes one mutable ceiling field and no plugin interface; an
  interface appears when a second consumer exists.

## Considered options

- **River's own limiting.** River v0.40 offers only `MaxWorkers` per queue; there is no rate limit or per-key limit.
  A queue-wide cap would also starve unrelated Workspaces, the failure reported for Oban's per-queue limit.
- **Instance-wide message rate, blocking `sleep` (Listmonk).** Breaks multi-tenancy and pins a worker for the wait.
- **Cron plus a per-run `--limit` (Mautic).** Throughput is bounded by cron frequency, not a rate; concurrent runs
  duplicate sends.
- **Redis limiters (`redis_rate`, `mennanov/limiters`).** Mature, but add a service the stack does not have.
- **A workflow engine with native per-key limits (Hatchet).** The right semantics, but it replaces River and the
  Outbound send chokepoint (ADR 0015) for one feature.
- **Treating a spent limit as a `Hold`.** Rejected: a Hold means the source is blocked for an external reason and
  shows a reason in the UI; a busy source is neither.

## Consequences

- The Outcome of an Outbound send gains a third reversible variant, and callers (the broadcast and enrollment
  workers) must handle it next to `Held`.
- The Broadcast contract gains progress and an ETA (TypeSpec first, then regenerate).
- Postgres takes one small write per limited send; revisit only if a single Integration approaches thousands of
  messages per second.
- Two Workspaces that configure the same provider account as separate Integrations get separate buckets, so their
  combined rate can exceed the provider's limit. Accepted: Workspaces are isolated by design.
- Throttling the provider is not the same as warming a sending identity: the core guarantees we stay under a
  ceiling, not that the ceiling is the right one for a cold domain.
