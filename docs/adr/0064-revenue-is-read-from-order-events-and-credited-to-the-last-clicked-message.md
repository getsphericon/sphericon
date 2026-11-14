---
status: accepted
---

# Revenue is read from order Events and credited to the last clicked Broadcast or Automation

A Workspace could not say what its email earned: nothing in the model held an order, an amount or a
currency. The Instance sells nothing itself, so revenue is a fact the Workspace's own shop reports. It
arrives as Events in the shape of the RudderStack E-Commerce Events spec, unchanged, so a Workspace that
already sends those events to a customer data platform sends the same ones to Sphericon with no
translation. Attribution is the part the spec leaves to the email tool; the choices below are
Sphericon's.

## Decisions

- **The spec is the contract.** An Event whose `action` is `Order Completed`, `Order Updated`,
  `Order Refunded` or `Order Cancelled` is an order Event, and its properties keep the spec's names
  (`order_id`, `total`, `currency`, `products`, `discount`, `tax`, `shipping`, `cancel_reason`). No
  name or field is translated. The four order Events are checked at ingest, on the tracker and the
  external API alike: an amount with more places than its currency has, or an unknown currency code, is
  refused with a reason in the response, so the shop learns at once and no order is lost silently. Only what Sphericon reads is checked
  (`order_id`, `total`, `currency`, a refund's `refund_id`): an odd `tax` or `revenue` refuses nothing.
  Every other Event is ingested as before, and a valid order Event is stored like any Event and read
  after it is stored. A field the spec has and Sphericon does not use is kept in the Event's properties.
- **An Order is a read model of those Events**, one per (Workspace, `order_id`), and its revenue is
  a list of dated **movements**, not a mutable counter on its Broadcast or Automation. The source
  facts are immutable; movements are a derived report projection that can be rebuilt when late
  facts change the source history. `Order Completed` creates the Order and adds its `total` on the
  day of its Event; an identical repeat of the same `order_id` is a no-op and a conflicting one is
  refused, as specified below. `Order Updated` changes its purchase
  amount, preserving its refunds, and the movement is the resulting change in net revenue.
  `Order Cancelled` takes back what is left on its own day. Cancellation remains in force after
  an Update: a new purchase amount does not reopen a cancelled Order or restore its revenue.
  The spec has no `provider`, so two shops feeding one Workspace must keep their order ids apart.
- **An order Event that arrives before its `Order Completed`** is kept and applied, by its own time,
  once the `Order Completed` arrives. An Order that never gets one counts nothing.
- **A Completed or Updated `total` is the purchase amount before refunds**, with discounts
  already applied. An Update changes that purchase amount; it does not replace net revenue or
  undo earlier refunds. Completed 100, Refunded 20, Updated 130 therefore leaves revenue of 110.
  On Refunded, `total` is instead that refund's amount, as specified below. The optional `revenue`
  field, which leaves out shipping and tax and is absent from refunds, is not read.
- **One `Order Refunded` Event is one refund.** Its `total` is the amount refunded, as PostHog's
  ecommerce spec and GA4's refund `value` also read it; the Segment and RudderStack specs leave the
  word open, so this is the rule Sphericon documents for senders. A shop that refunds twice sends two
  Events. Refunds are only added: an optional `refund_id` property, when present, makes a repeat of the
  same refund of the same `order_id` a no-op when its financial input agrees, and without it each
  distinct Event counts. The net revenue of an
  Order is its purchase amount minus the sum of its refunds, never below zero; a cancelled Order
  counts nothing. Each original refund amount is kept in full even when it exceeds the current
  purchase amount: the floor applies to the revenue projection, not to the fact. Completed 100,
  Refunded 80, Updated 60 leaves zero; a later Update to 120 leaves 40. A refund and a cancellation
  produce movements like the rest.
- **A repeated financial identity must repeat the same fact.** The identity of an Event, a
  Completion's `(Workspace, order_id)` and a refund's `(Workspace, order_id, refund_id)` each
  resolve to the accepted fact. An identical repeat changes nothing. A different purchase or
  refund amount, currency, kind or explicitly supplied occurrence time under that identity is
  a conflict, not a silently ignored correction; a different Order under one Event identity is
  a conflict too. The intake reports the conflict without replacing the accepted financial input
  or its projection. A correction of purchase amount is a new `Order Updated`, with its own
  Event identity. Equality concerns the normalized financial inputs, not ignored customer
  properties or JSON formatting. A retry that omits `occurredAt` reuses its accepted time instead
  of manufacturing a new one. This is the Orders' deduplication contract, not a change to every
  arbitrary customer's Event. A refund without `refund_id` still uses its Event identity; distinct
  Events without a shared refund key are distinct refunds.
- **Money is stored as an integer in the currency's minor unit** with an ISO 4217 code. The spec sends
  plain decimal numbers, which are converted at ingest using the currency's own exponent (two places for
  USD, none for JPY); an amount with more places than the currency has is rejected, not rounded.
- **Time and order are the sender's, within the rules of Backfill.** A movement is dated at its
  Event's `occurredAt`. Facts of one Order are interpreted by `(occurredAt, eventId)`, with the
  stable Event identity breaking equal times deterministically; delivery order decides neither
  the current total nor historical daily amounts. An identity assigned when the source supplies
  none stays the same for that accepted fact. No source revision is required. The id tie-breaker
  is an explicit convention, not a claim to recover a source sequence absent from the input.
  A late fact rebuilds the affected movements and their Report days, even after they were shown;
  it does not publish the source Event again or repeat its marketing reactions.
  A live Event on the external API older than the live window is refused, so a shop's
  history is loaded as a Backfill
  ([[0044-history-arrives-as-a-backfill-and-a-live-event-never-claims-to-be-old]]): its order Events
  build Orders and trigger nothing. A Backfill credits an Order only while the click is still kept: the
  Events' retention is 400 days by default and counts from when an Event is stored (ADR 0019).
- **The Orders module retains the minimal financial inputs for as long as the Order exists.**
  Each versioned input contains its kind, original purchase or refund amount, interpreted currency,
  occurrence time and stable opaque identities for the fact and, when given, the refund. A retained
  financial fingerprint supports the repeat check; arbitrary client ids are represented by scoped
  opaque keys, not retained as raw customer strings. These inputs belong to the Order's history,
  not the original Event envelope or a new archive of every publication. They retain neither a
  Contact reference, an address, arbitrary Event properties nor the original payload. They survive
  Contact Erasure with their Order, whose existing shop `order_id` remains the key for later
  refunds, and go with Workspace erasure. Analytical Event retention and receipt expiry do not
  remove these inputs or their deduplication guarantee. Restatement reads them, so it still works
  after the original Event was pruned or erased. This supplies a financial projection's own
  rebuild inputs without expanding the journal's replay allowlist (ADR 0116).
- **One reporting currency per Workspace** (amended below: the first Order sets it), set by the Owner or an Admin, who also set the window; both
  changes enter the audit log. Revenue is not shown until the currency is chosen, and an Order without a
  `currency` counts in it. An Order Completed that names no currency while none is chosen is kept as
  sent and read when the Owner or an Admin chooses one, so a shop that never sends a `currency` loses
  no order; one that names a currency is read at once. An Order keeps the currency it was read in: a
  later change of the Reporting currency leaves it in the old one. An Updated, Refunded or Cancelled
  that names no currency is in the currency of its own Order. An Order in another currency is stored
  and listed but counted in no sum. No rate is applied, so no figure is a guess.
- **Attribution is the last click.** An Order is credited to the Broadcast or Automation whose
  `email.clicked` Event the Contact made most recently before the order's Event time, inside the
  **Attribution window**, which the Workspace edits and which is 5 days until set. Opens do not count:
  mail clients open messages by themselves. A click by software
  ([[0053-delivery-and-engagement-events-cover-every-send]]) does not count either. Every
  click is already an Event that carries its Broadcast or Automation, so no tracking is added; a click
  that carries neither (a Transactional send) is passed over. An Order with no such click in the window
  is unattributed.
- **The credit is fixed when the Order is created.** `Order Completed` finds the winning click once
  and keeps it on the Order; changing the window later affects only later Orders, and a click that
  leaves the Events' retention takes no revenue with it. A click stored after the Order, though dated
  before it, credits nothing: the credit is not looked for again. Of two clicks inside the window, the later
  wins, and equal times go to the click Event stored last. An Order is placed on the Contact the Event
  resolves to by the existing identity rule; an Event for no known Contact is still an Order, counts in
  total revenue and is credited to nothing. An Order never creates a Contact. Restating financial
  movements does not rerun this choice: late amount facts and refunds change the revenue of the
  same credited source; late clicks and later Attribution window changes never move the Order.
- **Broadcast and Automation revenue are sums over the Orders credited to them**, never a figure stored
  on them. Every movement of an Order goes to the Broadcast
  or Automation the Order is credited to, on the day of the movement. Late source facts can refine
  a figure already shown for a past day; there is no promise of a closed accounting period.
- **The overview and the reports say what they count.** The overview shows revenue for 7, 30 or 90 Report
  days (30 by default) in the Workspace time zone and the share of it credited to messages, labelled as the revenue of the orders the Workspace sent in, not of its
  shop. A Broadcast and an Automation show orders and revenue; the denominator of any per-recipient
  figure is delivered messages.

## Considered options

- **Drip's order model** (`provider` + `order_id`, an `action` per state, `refund_amount`): clearer
  refunds, but a Workspace would map its events by hand into a format few others use.
- **Counting opens as well as clicks** (Klaviyo's default, with the same 5 days): more orders credited,
  but the credit would follow the mail client's behaviour, not the reader's.
- **Leaving refunds out of attributed revenue** (Klaviyo does): a bigger figure, and one a returned
  order still flatters; Drip's single-email report leaves returns out, as this does.
- **A `refund_amount` property beside the spec's `total`**: one more field for senders to learn, and
  every store mapper already has the refund's own amount to put in `total`.
- **A currency on every order and a sum per currency**: honest, but every figure then has several
  values. A rate table can come later without changing the stored orders.
- **Storing revenue on the Broadcast or Automation**: cheap to read, but a total to be rewritten by
  every later refund or update, where the movements add up the same on any day.

## Amendment: an Order Completed names its currency, and the first one sets the Reporting currency

The bullet "One reporting currency per Workspace" above asked the Owner or an Admin to choose the
currency before any revenue showed, and held an Order Completed without a `currency` until they did.
No other email tool asks this: each takes the currency from the shop it is connected to. Sphericon
has no shop connection, only the Events, so the Events now carry it.

- **An Order Completed must name its `currency`.** The spec marks no property as required, but it
  lists `currency` on all four order Events, and the shops and pipelines that send them fill it. One
  without it is refused at ingest with a reason, like an amount with too many places. An Updated,
  Refunded or Cancelled may leave it out and is in the currency of its Order; at ingest it is checked
  against that currency when the Order exists, and only as a number when it does not yet.
- **The first Order Completed a Workspace reads sets the Reporting currency** when none is set, a
  Backfilled one included. The change is a `workspace.update` Audit entry by the system. The Owner or
  an Admin can still choose the currency, before the first Order or after it, and a later Order never
  replaces it. Revenue shows as soon as there is an Order.
- **Nothing waits for a currency any more.** No Order Completed is held; only a change that arrives
  before its Order Completed is, so a held Event has no kind `completed` and no Contact.
