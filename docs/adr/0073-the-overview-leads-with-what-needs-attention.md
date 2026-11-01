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
  record and a Broadcast held for want of an Integration are items that need attention; a Workspace
  that loses its last Integration with nothing scheduled shows nothing until something is held.
- **Revenue appears with the first Order, not with a currency.** The revenue report returns
  `firstOrderAt` (any currency, any time), and the block is absent while it is null, whatever the
  Reporting currency. Orders in another currency are named inside the block.
- **One period for the page**, the `range` search parameter, 30 Report days by default, above the
  blocks it applies to (Sending, Audience, Revenue). Needs attention, the checklist and Sends are
  live.
- **Sending counts every surface.** The overview's `sending` reads the Outbound messages of the
  period (Broadcast, Automation, Transactional) and their delivery stamps. The open and click rates
  stay the Broadcast cohort's, labelled as such: only a Broadcast recipient carries its own open and
  click.

## Not decided here

- A previous-period comparison beside each figure.
- Per-Sending-domain Complaint and Bounce rates on the overview without the `staff` license
  (ADR 0011 puts them on the dashboard; the warning limits are the detector's configuration).
- A DMARC lookup, which a "not bulk-ready" item would need.
- An item for an active Automation that cannot send, and one for a scheduled Broadcast larger than
  what is left of the Send cap.
- A `failed_at` on the Broadcast: a failure's age is read from `updated_at` today.
- The hold reason as an enum of the contract; it is a string on the Broadcast and the item.
