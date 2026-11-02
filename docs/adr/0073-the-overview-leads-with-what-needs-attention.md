---
status: accepted
---

# The overview leads with what needs attention

The Workspace overview showed the Setup checklist, a revenue block, two cards to create a Broadcast
or an Automation and the things created last. What stops a Workspace's mail (a Sending domain that
lost its record, a provider that stopped the account, a held Broadcast) was each on its own page, and
the revenue block kept a "No orders yet" placeholder on every Workspace that sells nothing. The prior
art is in `docs/research/workspace-overview-prior-art.md`: most email tools lead their home page with
recent items or setup and put problems on a separate health page; Klaviyo leads with an alerts block
that is there only while something is wrong, and no vendor shows an empty revenue figure to an
account without a shop.

## Decisions

- **Blocks are ordered by urgency, then by how many Workspaces they apply to**, and a block with
  nothing to show is not rendered: no placeholder, no "all good" box, no wall of zeros. The order is
  Needs attention, the Setup checklist, Sends, the period's Sending and Audience, Revenue.
- **Needs attention is a live read, never stored** (`internal/attention`, `GET .../attention`). Each
  item is a fact some row already holds (a Sending domain's `verification_lost_at`, an Integration's
  provider standing and Provider limit, a Broadcast's hold reason or failure, an Automation's failed
  runs, a Webhook endpoint's `disabled_at`, the Send cap, the abuse detector's warning tier), read when
  the overview loads, with a severity (blocked, degraded, notice) and the object it is about. A
  failure that is not retried stays an item for 7 days. A Workspace suspension and a Staff hold are
  not items: the layout's banners show them on every page. The abuse warning moves here from the
  analytics overview.
- **The Setup checklist is done for good once it is found complete.** The Workspace stamps
  `setup_completed_at` the first time a read finds every step done; after it the overview shows no
  checklist, and a step that regresses is not onboarding again. A Sending domain that loses its DKIM
  record and a Broadcast held for want of an Integration are items that need attention, and so is an
  active Automation that cannot send (below).
- **Revenue appears with the first Order, not with a currency.** The revenue report returns
  `firstOrderAt` (any currency, any time), and the block is absent while it is null, whatever the
  Reporting currency. Orders in another currency are named inside the block.
- **One period for the page**, the `range` search parameter, 30 Report days by default, above the
  blocks it applies to (Sending, Audience, Revenue). Needs attention, the checklist and Sends are
  live.
- **A failure's age is its own moment, and a hold reason is a contract enum** (#940). A Broadcast
  stamps `failed_at` in the move to failed and the item reads it, so a later edit of the row does
  not bring an old failure back. `holdReason` is the `HoldReason` enum on every resource that
  carries it (a Broadcast, a held Broadcast's or an Automation's attention item), and the
  Broadcast's column checks the same values: both are `internal/holdreason`, the one list of
  reasons Outbound send holds a send for.
- **Sending counts every surface.** The overview's `sending` reads the Outbound messages of the
  period (Broadcast, Automation, Transactional) and their delivery stamps. The open and click rates
  stay the Broadcast cohort's, labelled as such: only a Broadcast recipient carries its own open and
  click.
- **A scheduled Broadcast the Send cap will slow is an item, judged by the bucket** (#939). A
  `scheduled` Broadcast, not held, that starts within the next 24 hours is a degraded item when the
  Workspace's Send cap bucket, read as the send path reads it (`providerlimit.SendCapDrain`, never
  the sends of the last 24 hours) and as it will be at the Broadcast's start, cannot let it out in
  full then. Broadcasts draw from one bucket, so each is judged, in the order they start, by its
  audience, those of every one before it and the recipients sending Broadcasts have left, all taken
  at its start (the refill between their
  starts is left out, so the read errs towards an item). The item carries its own audience
  (`count`) and its start plus the wait (`estimatedCompletionAt`), what the Broadcast's progress
  shows once it sends. The audience is an estimate: the Contacts its plan would take now (the
  Condition copied when it left draft, eligibility), counted by the same query as the plan.
- **An active Automation that cannot send is a blocked item, decided by the send path** (#938). An
  Automation has no stored hold, so the item asks `outbound.Module.Preflight` what a send of its
  email steps would be held for now: the same route and gate Send applies. It is an item when the
  Automation is active and kept, its live version has an email step (a draft sends nothing), and
  the answer is a hold of its own source (`no_integration`, `unverified_domain`,
  `provider_stopped`), the set a held Broadcast is an item for; a Workspace-level hold is the
  banner's. An email step has no From of its own, so the answer depends only on the Integration the
  live version names, or the default: the send path is asked once per Integration, and every
  Automation is an item of its own with that reason. Nothing stores when a route began to hold, so
  the item has no `since`.

- **Every period figure has the previous period beside it** (#935). The overview and revenue
  reports return `current` and `previous`, the shape of the Automation report's results: the
  previous period is as many Report days just before the range (`reportday.Window.Previous`), read
  by the same queries in the same request. A snapshot (the subscribed, the Automations) stays at the
  root and has no previous period. A card shows the previous figure with the signed change: a count
  or an amount as a share of the previous one (+100%), a rate in percentage points (20% to 30% is
  +10 pp), as a share of a rate reads as points. No change is shown when the previous period had
  nothing to compare with: a count of 0, or a rate of no messages or no revenue.

## Not decided here

- Per-Sending-domain Complaint and Bounce rates on the overview without the `staff` license
  (ADR 0011 puts them on the dashboard; the warning limits are the detector's configuration).
