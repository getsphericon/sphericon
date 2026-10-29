---
status: accepted
---

# Revenue is read from order Events and credited to the last clicked Broadcast or Automation

A Workspace could not say what its email earned: nothing in the model held an order, an amount or a
currency. The Instance sells nothing itself, so revenue is a fact the Workspace's own shop reports. It
arrives as Events in the shape of the RudderStack E-Commerce Events spec, unchanged, so a Workspace that
already sends those events to a customer data platform sends the same ones to Sphericon with no
translation. Attribution is the part the spec leaves to the email tool; the choices below are
Sphericon's. The research behind this is in `docs/research/revenue-and-attribution-prior-art.md`.

## Decisions

- **The spec is the contract.** An Event whose `action` is `Order Completed`, `Order Updated`,
  `Order Refunded` or `Order Cancelled` is an order Event, and its properties keep the spec's names
  (`order_id`, `total`, `currency`, `products`, `discount`, `tax`, `shipping`, `cancel_reason`). The
  ingest paths of the tracker and the external API are not changed; the Event is read after it is
  stored. A field the spec has and Sphericon does not use is kept in the Event's properties.
- **An Order is a read model of those Events**, one per (Workspace, `order_id`). `Order Completed`
  creates it, a repeat of the same `order_id` updates it and never adds to it, `Order Updated` replaces
  its amounts, `Order Cancelled` voids it. The spec has no `provider`, so two shops feeding one Workspace
  must keep their order ids apart.
- **Revenue is `total`.** It is the one amount the spec carries on all four events and already has
  discounts applied. `revenue` (which leaves out shipping and tax) is optional in the spec and absent
  from refunds, so it is not read.
- **A refund is its own row.** The spec has no refund amount, so on `Order Refunded` the `total` is the
  amount refunded: the order's full total for a full refund, less for a partial one. Refunds are only
  added; the net revenue of an Order is its total minus its refunds, and a cancelled Order has none.
- **Money is stored as an integer in the currency's minor unit** with an ISO 4217 code. The spec sends
  plain decimal numbers, which are converted at ingest using the currency's own exponent (two places for
  USD, none for JPY); an amount with more places than the currency has is rejected, not rounded.
- **One reporting currency per Workspace**, set by the Owner or an Admin. An order in another currency
  is stored and listed but counted in no sum. No rate is applied, so no figure is a guess.
- **Attribution is the last click.** An Order is credited to the Broadcast or Automation whose
  `email.clicked` Event the Contact made most recently before the order's Event time, inside the
  **Attribution window**, which the Workspace edits and which is 5 days until set. Opens do not count:
  mail clients open messages by themselves. A click by software
  ([[0053-delivery-and-engagement-events-cover-every-send]]) does not count either. Every
  click is already an Event that carries its Broadcast or Automation, so no tracking is added. An Order
  with no such click in the window is unattributed.
- **Broadcast and Automation revenue are sums over the Orders credited to them**, never a figure stored
  on them, so changing the window or the currency recomputes every report. A click older than the
  Events' retention can no longer credit an Order.
- **The overview and the reports say what they count.** The overview shows revenue for a period and the
  share credited to messages, labelled as the revenue of the orders the Workspace sent in, not of its
  shop. A Broadcast and an Automation show orders and revenue; the denominator of any per-recipient
  figure is delivered messages.

## Considered options

- **Drip's order model** (`provider` + `order_id`, an `action` per state, `refund_amount`): clearer
  refunds, but a Workspace would map its events by hand into a format few others use.
- **Counting opens as well as clicks**: more orders credited, but the credit would follow the mail
  client's behaviour, not the reader's.
- **A currency on every order and a sum per currency**: honest, but every figure then has several
  values. A rate table can come later without changing the stored orders.
- **Storing revenue on the Broadcast or Automation**: cheap to read, but wrong once the window, the
  currency or a late refund changes.
