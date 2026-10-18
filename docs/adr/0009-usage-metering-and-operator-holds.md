---
status: accepted
---

# Usage metering and staff holds: measured in the core, managed from outside

The core **measures** what each Workspace does and lets a Staff member **hold** a Workspace's sending
from outside. It knows nothing of why a Workspace is allowed or held: whoever manages a
Workspace's allowance does so in a separate service that is not part of this repository, through
the Staff surface (ADR 0008, 0026). A self-hosted install has no such service and sees none of
it. This ADR fixes the boundary: what crosses it, and in which direction.

It is unrelated to the **EE license key** (ADR 0014, mechanism (a)): an offline runtime key and an
external service have different lifecycles, and the two are not unified into one "entitlement".

## Decisions

- **The Usage period is the calendar month in UTC, closed by the core**, so a Usage snapshot is
  final before any reader treats it as final.
- **Usage snapshot.** A per-(Workspace, Usage period, metric) materialized aggregate, finalized
  (immutable) at close and computed in the core process, because that is the only place the raw
  Events and their retention live. The Events stay the source of truth for audit and dispute; the
  snapshot is the closed, reproducible figure. v1 metrics: `emails_sent` (sum over `email.sent`)
  and `contacts` (high-water-mark: the peak is unrecoverable after the fact if it is not
  snapshotted live). Events ingested, seats and "active" contacts wait until a reader needs them.
- **Live usage.** The open period's figures with a `final` flag and `computed_at`, served from a
  worker or cache and counting in-flight sends, never a recount on read.
- **The external service reads; the core never calls out.** Through the Staff surface the
  service reads per-Workspace aggregates (Usage snapshot and Live usage), computed from the core's
  own tables so a retry or a replay is idempotent, and the Workspaces created since a cursor, with
  their owner (identity and whether the email is verified). The core holds no address of the
  service and carries no outbound configuration for it.
- **Staff hold.** A reversible Workspace freeze set through the Staff surface for a reason
  that is not reputation. Each hold is stored separately under an opaque key chosen by whoever
  sets it, with a machine reason code, a message shown to the Workspace Owner and an **expiry that
  the setter must renew**, so an unavailable service cannot freeze Workspaces for good. A hold is
  cleared by its own setter or by its expiry; any active hold freezes sending. Every change
  records the Staff member that made it. It reuses the **same core send chokepoint** as suspension
  (ADR 0007, 0015) but is a distinct cause, never a repurposing of it. It freezes all three send
  surfaces, and messages are **held, not failed**: they wait and go out when the hold lifts. The
  core has no grace period or retry logic of its own.
- **The core keeps no allowance of its own.** The one question it answers is "may this Workspace
  send now", with several independent freeze reasons.
- **An account link for the Owner.** When the app is configured with an address and a signing key,
  it shows the Owner a link to that external page and passes a short-lived signed token naming
  the user, the Workspace and the role. The receiving service verifies the token with the core's
  public key and keeps no accounts of its own, so there is one login and no federation. Without
  the configuration there is no link.

## Considered options

- **Compute usage in the external service from a raw Event stream** (rejected): it would need
  reliable access to every tenant's full Event stream and a second copy of retention and audit.
  The core already has the data and aggregates where it lives.
- **Re-scan the raw Event log on every read** (rejected): not reproducible once Events are pruned,
  expensive per read and not tamper-evident; every metering engine pre-aggregates into finalized
  windows instead.
- **The core pushes usage to the service** (rejected): the core would need the service's address
  and delivery guarantees, and self-hosted installs would carry outbound configuration they must
  not have. A pull needs neither. An outbox of CloudEvents-shaped records (stable `id`, subject =
  Workspace id, `time`) maps onto the usual metering engines and can be added later without
  moving this boundary.
- **Overload Workspace suspension** (rejected): suspension is an abuse fact (ADR 0007), and the two
  freezes have different setters, lifecycles and messages.
- **A hold without an expiry** (rejected): an unavailable service would freeze Workspaces for good.
- **Freeze only marketing, spare transactional** (rejected): it leaks the hold's reason into the
  send path by message type and gives a held Workspace a permanent loophole. The setter's own
  grace, not the message type, is what protects a tenant's password-reset mail.
- **Unify the EE license key and the hosted offering into one "entitlement"** (rejected):
  premature abstraction.

## Consequences

- A self-hosted install has no Staff surface (ADR 0008), so it has no external reads, no Staff member
  hold and no account link. The AGPL core gains only a generalized "is sending frozen, and why"
  check, which already existed for suspension.
- The coupling is exactly two contracts: read usage and new Workspaces, write holds. Nothing
  commercial crosses it, and nothing commercial is described in this repository.
- Metering and holds are stored in the core, so the Events and their retention (ADR 0019) stay the
  single source of truth.
