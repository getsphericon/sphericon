# Revenue and attribution prior art: Drip and RudderStack

Researched October 2026 from primary sources: the Drip developer API reference (developer.drip.com), the Drip help
center (help.drip.com) and drip.com product pages, and the RudderStack documentation (rudderstack.com/docs).
This note extends [drip-com-feature-analysis.md](drip-com-feature-analysis.md) (which only sketches the Shopper
Activity API and the default attribution rule) and the attribution row of
[mindbox-feature-analysis.md](mindbox-feature-analysis.md) (last-non-direct-touch, 1 to 30 days); it does not repeat
them.

**Source quality caveat.** help.drip.com and drip.com return HTTP 403 to automated fetchers. Every claim sourced to
those hosts below was read through search-engine excerpts of the official pages, not the full page, and is marked
**(excerpt)**. developer.drip.com and rudderstack.com were fetched directly (the Drip reference is one very long page,
and a fetch tool summarised it in slices, so field lists were extracted by summarisation, not read by eye).

## Question

Sphericon has Events, Contacts, Broadcasts, Automations and per-broadcast open/click tracking, and no revenue, order
or currency concept. How do Drip and RudderStack model e-commerce events, money and attribution, and which design
decisions does that force for a "revenue story": revenue per period on the workspace overview and revenue per
broadcast and per automation? This note reports facts and marks inference; it proposes no schema.

## Summary of findings

- Drip stores orders as first-class records keyed by `provider` + `order_id`; refunds and cancellations are further
  `action` values on the same order, and a refund is a separate `refund_amount`, not a negative total.
- Drip's attributed revenue is last-touch: the purchase goes to the email most recently clicked or received, inside a
  window (default 5 days) that is editable per account; a view-through mode and a click-only mode exist.
- Drip's per-email revenue excludes returned and cancelled orders (single-email report); its API reports revenue in
  cents and does not name a currency for the metrics.
- RudderStack specifies order events (Order Completed, Updated, Refunded, Cancelled) with plain-number money and a
  `currency` string, marks nothing required, and does no attribution of its own: it forwards events to destinations.
- The RudderStack to Drip mapping is lossy and renames fields (`total` becomes `grand_total`, `affiliation` becomes
  `provider`), and RudderStack itself says Drip does not strictly follow the spec.

## 1. Drip: Shopper Activity API and e-commerce events

All facts in this section are from the developer reference: https://developer.drip.com/ (sections Shopper Activity,
Order Activity, Orders, Refunds).

### Event streams

Three streams, each with a batch endpoint (the batch path was confirmed for orders:
`POST /v3/:account_id/shopper_activity/order/batch`): product activity, cart activity and order activity. The
drip-com-feature-analysis note already lists the three streams; this note covers orders only.

### Order Activity (current API, v3)

`POST /v3/:account_id/shopper_activity/order` (https://developer.drip.com/, section Order Activity).

- Required: `provider` (lower snake case name of the source platform), `action`, `order_id` (the platform's own
  unique id), each item's `name`, and one of `person_id` or `email`.
- Optional: `occurred_at` (defaults to now), `initial_status`, `order_public_id`, `order_url`, `currency`, the totals
  `grand_total`, `total_discounts`, `total_taxes`, `total_fees`, `total_shipping`, and `refund_amount` (all default
  to 0), per-item fields (`product_id`, `product_variant_id`, `sku`, `price`, `quantity` defaulting to 1, `discounts`,
  `taxes`, `fees`, `shipping`, `total`), billing and shipping addresses, and custom attributes.
- `action` is one of six values: `placed`, `updated`, `paid`, `fulfilled`, `refunded`, `canceled`. Each creates a
  matching timeline event. `placed` fixes when the order originated (from `occurred_at`); if other actions arrive
  without a prior `placed`, the time of the first event received is used.
- Update key: sending the same `provider` and `order_id` updates the existing order.
- Refunds and cancellations: set `refund_amount` to the refunded amount and leave `grand_total` unchanged. A full
  refund lowers the customer's lifetime value by that amount.
- Lifetime value: a `placed` order adds `grand_total` to the customer's LTV; later updates adjust it by the change in
  `grand_total`.
- Response: `202 Accepted` with a `request_id`; processing is asynchronous.
- Batch: 1 to 1,000 orders per request; batch endpoints are limited to 50 requests per hour, other endpoints to 3,600
  per hour.
- Money units: the v3 page shows decimals (for example a `grand_total` of 54.00) and does not state a unit
  (not documented). `currency` is an optional string.
- Idempotency and dedup: not documented. Only the `provider` + `order_id` update key is described; the page does not
  say whether resending a `placed` is a no-op or how a repeat affects LTV.

### Legacy Orders and Refunds (v2, still functional)

`POST /v2/:account_id/orders` and `POST /v2/:account_id/refunds` (https://developer.drip.com/, sections Orders and
Refunds).

- Orders: `amount` is required and is in cents (tax, fees and discount too); `currency_code` is optional (ISO 4217);
  `financial_state`, `fulfillment_state`, `closed_at`, `cancelled_at` are optional. `provider` + `upstream_id`
  identify an order for updates. Setting `cancelled_at` fires a "Canceled an order" event.
- Refunds are their own record: `provider`, `order_upstream_id` and `amount` are required; `upstream_id` identifies a
  refund for updates; creating one fires a "Refunded an order" event and lowers LTV by the refund amount. The units of
  the refund `amount` are not stated (not documented; the example looks like cents).
- The v3 reference recommends the v3 batch endpoint over the legacy batch endpoint
  (https://developer.drip.com/, section Order Activity).
- Net: Drip changed units between API generations (cents in v2, decimals in v3 examples).

## 2. Drip: what "attributed revenue" means

Sources: https://help.drip.com/hc/en-us/articles/4424710492685-Revenue-Attribution-Settings,
https://help.drip.com/hc/en-us/articles/4424710496653-Revenue,
https://help.drip.com/hc/en-us/articles/4424710436365-Data-to-Help-Drive-Your-Marketing-Strategy,
https://help.drip.com/hc/en-us/articles/4424710547213-Single-Email-Campaign-Metrics, and
https://www.drip.com/product/insights. All **(excerpt)**.

- **Model.** The Revenue article describes Drip-attributed revenue as last-touch **(excerpt)**. Total revenue is the
  store's gross revenue and is a separate figure, shown under Analytics > Revenue **(excerpt)**.
- **Default rule.** The Attribution Settings article says a purchase is credited to the email the customer most
  recently clicked or received, provided the purchase falls in the window **(excerpt)**. The Insights page states the
  rule as: a click within 5 days of the purchase, or an on-site visit within a day after an email followed by a
  purchase within 5 days **(excerpt)**. These two descriptions are not fully consistent: "most recently clicked or
  received" versus click or on-site visit. Which email wins when several qualify is therefore only partly documented.
- **Window.** Default 5 days, editable at Settings > Account > Attribution Window **(excerpt)**. The upper limit is not
  documented.
- **Opens vs clicks.** Opens are not named as a trigger in any excerpt. The credit comes from a click, or from an
  on-site visit after an email (the "seen on-site" path, on by default). A toggle, "Email only attribution", restricts
  credit to direct click-throughs **(excerpt)**. Drip's comparison page calls the two modes click-through and
  view-through (marketing copy, https://www.drip.com/comparisons/mailerlite, so lower weight).
- **Workflow vs single email.** The metrics object in the API covers Single-Email Campaigns and workflow emails
  together (https://developer.drip.com/, section Email Metrics), so the unit of attribution is the individual email,
  and a workflow or series figure is a roll-up of its emails (inference; the roll-up rule is not documented). A
  workflow's own revenue page is https://help.drip.com/hc/en-us/articles/4424695601933-Monitor-a-Workflow, which
  could not be read.
- **Share of store.** The "Data to Help Drive Your Marketing Strategy" article defines the percentage of revenue from
  Drip as Drip-attributed revenue divided by total e-commerce revenue across all channels, and calls 20 to 40 percent
  healthy **(excerpt)**.
- **Refunds.** The Single Email Campaign Metrics article says the revenue figures leave out returned and cancelled
  orders **(excerpt)**. Whether the Drip-attributed figure on the Revenue dashboard does the same is not documented.
  How a partial refund is treated is not documented.
- **Currency.** Not documented for attributed revenue. The API metrics are "in cents" with no currency named
  (https://developer.drip.com/, section Email Metrics). Order-level `currency` exists (section 1) but how reports
  combine orders in different currencies is not documented.

## 3. Drip: where revenue and orders appear

- **Per email, API.** The email metrics object has `orders` (orders attributed to the email), `order_rate`
  (`orders` / `sends`, four decimals), `revenue_in_cents`, `revenue_per_person_in_cents` (revenue / `sends`) and
  `average_order_value_in_cents` (revenue / `orders`). Values are in cents; covers Single-Email Campaigns and
  workflow emails; confirmation emails are excluded; dates use the account owner's time zone
  (https://developer.drip.com/, section Email Metrics).
- **Single email campaign dashboard.** Overall revenue, order count, revenue per subscriber, average order value;
  revenue figures appear only for accounts that use revenue attribution
  (https://help.drip.com/hc/en-us/articles/4424702693901-Track-Single-Email-Campaign-Revenue, **(excerpt)**).
- **Denominator drift.** The help center calls the per-recipient figure revenue divided by recipients, the API divides
  by sends, and a marketing page says delivered emails **(excerpt)**. One metric has three denominators across Drip's
  own surfaces.
- **Email metrics export.** Per email: orders, revenue, revenue per person
  (https://help.drip.com/hc/en-us/articles/4424702638605-Email-Metrics, **(excerpt)**). That page says it shows only
  email-attributed revenue and points to Analytics > Revenue for total store revenue.
- **Workflow report and Insights dashboard.** Existence is covered in drip-com-feature-analysis.md; their exact metric
  definitions were not readable (not documented in the readable sources).

## 4. RudderStack: the E-Commerce Events spec

Sources: https://www.rudderstack.com/docs/event-spec/ecommerce-events-spec/ and its Ordering sub-page
https://www.rudderstack.com/docs/event-spec/ecommerce-events-spec/ordering/.

- The spec has 26 events in seven groups (browsing, promotions, ordering, coupons, wishlist, sharing, reviewing).
  The ordering group holds Product Clicked, Product Viewed, Product Added, Product Removed, Cart Viewed, Checkout
  Started, Checkout Step Viewed, Checkout Step Completed, Payment Info Entered, Order Completed, Order Updated, Order
  Refunded and Order Cancelled. **Product Purchased does not exist**; Order Completed is the purchase event.
- Order event properties (Ordering page):

| Property                                  | Completed | Updated | Refunded | Cancelled |
| ----------------------------------------- | --------- | ------- | -------- | --------- |
| `order_id`                                | yes       | yes     | yes      | yes       |
| `total`                                   | yes       | yes     | yes      | yes       |
| `currency`                                | yes       | yes     | yes      | yes       |
| `products` (array)                        | yes       | yes     | yes      | yes       |
| `discount`                                | yes       | yes     | yes      | yes       |
| `revenue`, `shipping`, `tax`              | yes       | yes     | no       | yes       |
| `affiliation`, `coupon`                   | yes       | yes     | no       | yes       |
| `discounts` (array of code, amount, type) | yes       | no      | yes      | yes       |
| `subtotal`, `checkout_id`, `cart_id`      | yes       | no      | no       | no        |
| `cancel_reason`                           | no        | no      | no       | yes       |

- Product fields (same on all four): `product_id`, `sku`, `category`, `name`, `brand`, `variant`, `price`, `quantity`,
  `coupon`, `position`, `url`, `image_url`; `price`, `quantity` and `position` are numbers.
- Definitions: `revenue` excludes shipping and tax; `total` is revenue with discounts and coupons applied; `subtotal`
  is the total after discounts without taxes and shipping.
- **Required vs optional:** the page marks no property as required. `value` is not on these four events (only on
  Checkout Started). Units and decimal format are not specified; examples use plain numbers such as 52.0. A currency
  code is shown (USD in examples) and no allowed list is stated.
- **Refund shape:** the prose says a refund should carry all cart items like Order Completed, yet its property table is
  a subset (no `revenue`, `shipping`, `tax`). There is no `refund_amount` in the spec; a refund event carries `total`
  and `products`, so whether `total` means the order total or the refunded amount is ambiguous (inference from the
  table; the page does not say).
- The SDKs do not compute `revenue`, `total` or `subtotal`; you must send them, and destinations map them to their own
  fields so meaning can vary.

## 5. RudderStack: attribution and destinations

- RudderStack lists destinations that "do not adhere to the ecommerce spec strictly", Drip among them, along with
  Klaviyo, Google Analytics 4 and others (https://www.rudderstack.com/docs/event-spec/ecommerce-events-spec/, FAQ).
- The Drip cloud-mode destination (https://www.rudderstack.com/docs/destinations/streaming-destinations/drip/drip-cloud-mode/)
  creates or updates a Drip order when a track event is named order updated, order completed, order refunded, order
  cancelled, checkout started or fulfilled. Mapping: `affiliation` to `provider`, `total` to `grand_total`, `tax` to
  `total_taxes`, `shipping` to `total_shipping`, `discount` to `discounts`, `products` to `items` (product `url` to
  `product_url`), plus `order_id`, `currency`, `occurred_at`, `refund_amount`. The mapping page does not say how Drip
  processes refunds, and does not state price units. Items lacking `name` or `price` are dropped while the call still
  succeeds. A call needs a Drip id or an email.
- Attribution: RudderStack's docs show attribution as a destination category (an event is forwarded to a third-party
  attribution product), and the Attribution integration page describes single and multi-touch modelling as that
  vendor's job (https://www.rudderstack.com/docs/destinations/streaming-destinations/attribution/,
  https://www.rudderstack.com/integration/attribution/). I found no RudderStack feature that attributes revenue to a
  message or campaign; that conclusion is an inference from absence, not a stated claim. In practice the revenue event
  is a fact RudderStack carries, and the email tool (Drip) decides credit by its own rule.

## 6. Comparison

| Dimension                      | Drip                                                                                          | RudderStack                                                                  |
| ------------------------------ | --------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------- |
| Source of revenue              | Order records pushed by store integrations or the Shopper Activity API                        | Track events (Order Completed and others) from SDKs or sources               |
| Order identity                 | `provider` + `order_id` (v3), `provider` + `upstream_id` (v2)                                 | `order_id` only; no provider concept in the spec                             |
| Money fields                   | `grand_total` plus discounts, taxes, fees, shipping; v3 decimals, v2 cents; API metrics cents | `total`, `revenue`, `subtotal`, `tax`, `shipping`, `discount`; plain numbers |
| Currency                       | Optional per order; mixed-currency reporting not documented                                   | Optional string per event; nothing on conversion                             |
| Refunds                        | `refunded` / `canceled` actions with `refund_amount`, or a v2 refund record; lowers LTV       | Order Refunded and Order Cancelled events; no refund amount field            |
| Attribution rule               | Last-touch, click or on-site visit, default 5 days, editable, click-only toggle               | None; delegated to destinations                                              |
| Opens count?                   | Not named as a trigger (excerpt)                                                              | Not applicable                                                               |
| Per-message / per-journey view | Per email (orders, revenue, revenue per person, AOV), plus campaign and workflow reports      | Not applicable                                                               |
| Idempotency                    | Update key documented, dedup not documented                                                   | Not documented in the spec                                                   |
| Required fields                | `provider`, `action`, `order_id`, item `name`, person                                         | None marked                                                                  |

## 7. Implications for Sphericon

Facts are cited above. The statements below are **inference** unless a section number is given.

1. **Money representation.** The two sources disagree among themselves (Drip cents in v2 and decimals in v3; RudderStack
   plain numbers, sections 1 and 4). Integer minor units plus a currency code avoids float drift; Drip's own metrics
   API already uses cents. Decide whether to accept decimal input at the API edge and convert.
2. **One currency per workspace or many.** Neither source documents conversion or mixed-currency reporting (sections
   2 and 4). The cheapest honest rule is one reporting currency per workspace, with an order in any other currency
   rejected or stored but excluded from sums; summing across currencies without a rate would be wrong.
3. **Attribution rule and window.** Drip's default is last-touch, 5 days, click-based, workspace-editable, with the
   losing case between "clicked" and "received" under-specified (section 2). Sphericon already tracks clicks per
   broadcast, so a click-only rule needs no new tracking; an open is unreliable as a signal (inference, Apple Mail
   proxy opens) and Drip does not list it. The window should be a workspace setting with a default, and the rule must
   be stated in the UI so two figures never disagree.
4. **Refunds.** Drip keeps `grand_total` and records a separate refund amount (section 1); RudderStack has no refund
   amount (section 4). Storing the order once and refunds as separate rows (or an accumulating refunded amount)
   lets reports compute net revenue. Open choice: whether attributed revenue is shown gross, net, or both; Drip's
   single-email report excludes returned and cancelled orders (section 2).
5. **Idempotency.** Neither source documents dedup (sections 1 and 4). Sphericon should define it itself: a unique
   key such as (workspace, source, external order id), with `placed` as an upsert and repeat events as no-ops.
   Drip's `provider` + `order_id` is a usable model, and a mapped RudderStack spec would lose the provider.
6. **Entities.** Revenue attaches to a Contact (the buyer) and an Order; attribution then links the order to a
   Broadcast or an Automation through the delivery (the sent email) that won the rule. Drip attributes per email and
   rolls up (section 3), so the attribution link should point at the delivery or message, with broadcast and
   automation totals derived by sum. This reuses existing delivery tracking.
7. **Ingestion shape.** The Events concept may carry the order as an event type initially (the mindbox note already
   suggests this); if so, accept RudderStack's Order Completed, Refunded and Cancelled names as an optional ingestion
   dialect, remembering that `total` is ambiguous on refunds and that nothing is marked required.
8. **Denominators.** Drip's revenue-per-recipient has three denominators across its surfaces (section 3). Pick one
   (delivered is the least ambiguous) and name it in the UI.
9. **Workspace overview.** Total store revenue and attributed revenue are different numbers in Drip, and the
   attributed share of the total is a headline figure (section 2). If Sphericon only sees orders that were pushed to
   it, "total revenue" is total orders ingested, which should be labelled so.

## 8. Drip's predefined Events (read in a browser)

Source: https://help.drip.com/hc/en-us/articles/4424695565453-Predefined-Drip-Events (read in full, not an
excerpt). Drip records engagement as ordinary Events: `Opened an email`, `Clicked a link`, `Clicked a
trigger link`, `Received an email`, `Bounced` and `Issued a spam complaint` sit in the same list as
`Started a workflow` and `Subscribed to an email series`. The page lists no order event: orders come through
the Shopper Activity API (section 1), not through the predefined list. Implication (inference): Sphericon's
Events can carry order Events beside its engagement records, and a click is already a stored fact that
attribution can read.

## Gaps and unverified

- help.drip.com and drip.com cannot be fetched; every help-center claim is from search excerpts and may omit caveats.
- The Drip workflow report, Insights dashboard and Revenue dashboard definitions were not readable.
- Drip's attribution tie-break when both a click and a later received email exist, its maximum window, partial-refund
  handling, multi-currency handling and v3 money units are not documented in anything I could read.
- Drip idempotency on resend is not documented; the API reference summary said so explicitly for the Order Activity
  section.
- RudderStack: whether any product feature performs attribution was inferred from the docs showing it only as a
  destination category.
- The Drip API reference is about 270,000 characters; I read it in slices through a summariser, so field lists may
  miss rarely used properties.
