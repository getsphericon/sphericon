# Email attribution and store refund conventions: Klaviyo, Segment, Shopify, WooCommerce, Stripe

Researched October 2026 from primary sources: Klaviyo help center and developer docs (help.klaviyo.com,
developers.klaviyo.com), the Segment ecommerce spec (published under twilio.com/docs/segment), Shopify developer docs
(shopify.dev), the WooCommerce REST docs and source code (woocommerce.github.io, github.com/woocommerce), and the Stripe
docs (docs.stripe.com). It settles open questions of
[ADR 0064](../adr/0064-revenue-is-read-from-order-events-and-credited-to-the-last-clicked-message.md) and extends
[revenue-and-attribution-prior-art.md](revenue-and-attribution-prior-art.md) (Drip and RudderStack), which it does not
repeat. GA4, PostHog and Amplitude are in a separate note.

**Source quality caveat.** Every page was read through the WebFetch tool, which returns a model-written answer to a
question about the page, not the raw page. Facts below are those answers; where a page was too long to read in full this
is said. segment.com returned HTTP 403, so the Segment spec was read at its twilio.com mirror. Nothing here is from a
search excerpt unless marked **(excerpt)**. Quotes are kept under 15 words.

## Question

Three things the ADR leaves open: how an email tool with attribution (Klaviyo) sets its defaults and treats refunds;
whether the Segment spec, from which RudderStack's is derived, defines `total` on `Order Refunded` and partial refunds;
and how the shops that will send Events (Shopify, WooCommerce, Stripe) represent a refund: an amount or a whole order,
one or many per order, in which unit.

## Summary of findings

- Klaviyo defaults to 5 days for email opens and 5 days for email clicks, last touch across email, SMS and push; opens
  count unless disabled, and Apple Mail Privacy Protection (MPP) opens count unless excluded in settings.
- Klaviyo's revenue is not reduced by refunds or cancellations: its Shopify data reference says so, and its
  `Refunded Order` metric fires only for full Shopify refunds.
- The Segment spec does not say what `total` means on `Order Refunded`, does not mention partial refunds, and tells the
  sender to include all cart items.
- Shopify and Stripe: a refund is an amount against an order or charge, and there can be several. WooCommerce: the same,
  and a refund is a negative order whose amount is positive in the REST `amount` and negative in the order's `refunds[].total`.
- Money units differ: Stripe integer minor units; Shopify and WooCommerce decimal strings in the shop currency.

## 1. Klaviyo

### Events and `$value`

Source: https://developers.klaviyo.com/en/docs/guide_to_integrating_a_platform_without_a_pre_built_klaviyo_integration
and https://developers.klaviyo.com/en/reference/create_event.

- `Placed Order`: one event per order, plus one `Ordered Product` event per line item.
- `Refunded Order` and `Cancelled Order` (the guide spells it "Canceled"): a payload like `Placed Order` with another
  metric name, timestamp and a reason; for partial refunds or cancellations the guide says the items affected and not
  affected can be listed.
- `value` is the property that lets Klaviyo track revenue: the total monetary value of the event, a number not a string.
  The Create Event reference adds `value_currency` (ISO 4217). The guide does not say what the total includes.
- Klaviyo's Shopify data reference (https://help.klaviyo.com/hc/en-us/articles/115005080447) defines the Shopify
  `Placed Order` value as the order total Shopify reports, with discounts applied, including shipping, taxes and tips.
  This is for the Shopify integration only; for custom integrations it is not documented.
- The guide says `Refunded Order` and `Cancelled Order` events need a `unique_id` matching the earlier `Placed Order`
  event to count in customer lifetime value (CLV).

### `unique_id` (idempotency)

Source: https://developers.klaviyo.com/en/reference/create_event.

- A repeated `unique_id` for the same profile and metric records only the first processed event.
- If omitted, the event time to the second is used, which limits a profile to one event of a metric per second.
- The Create Event call answers 202: validated and submitted, not yet processed.
- The key is scoped to profile plus metric, so a refund sharing the order's `unique_id` is a different metric and does
  not collide with it. What happens to two refunds that share one `unique_id` follows from the rule above: the second is
  dropped (inference, from the stated rule). The guide does not discuss several refunds on one order.

### Attribution

Source: https://help.klaviyo.com/hc/en-us/articles/115005248128.

- Defaults for new accounts: 5 days for email opens, 5 days for email clicks (also 5 days for SMS clicks, 12 hours for
  SMS deliveries, 24 hours for push opens). Changed in Settings > Attribution, limited to 3 changes per 30 days. The
  maximum window is not documented.
- Opens count as touchpoints if open tracking is on. The page does not say whether a click outranks an open from the
  same message.
- Default rule is last touch: the most recent eligible touchpoint before the conversion, inside its channel's window,
  across email, SMS and push. A Linear model exists for some plans and splits credit.
- MPP: opens by MPP can count unless the account excludes them in its attribution settings, and the page warns that MPP
  may record opens the subscriber never made. The default state of that exclusion is not stated; the wording implies
  included by default (inference).
- The article does not say how campaigns and flows share credit, and the windows are described per account, not per
  campaign or flow.

### Refunds and cancellations

- Source https://help.klaviyo.com/hc/en-us/articles/115005080447 (Shopify data reference): Klaviyo's revenue does not
  drop cancelled or refunded orders, while Shopify's own does, so the two can differ. `Refunded Order` is tracked only
  when an order is refunded in full; partial refunds do not trigger it. `Cancelled Order` is tracked when a customer
  cancels before fulfilment, and is not available for every integration (the glossary,
  https://help.klaviyo.com/hc/en-us/articles/115005251588, was named by a search result and not read in full).
- The attribution article (115005248128) says nothing on refunds.
- Third-party and community statements that attributed revenue is gross of refunds exist (community.klaviyo.com, 2022;
  docs.vortexiq.ai) and are secondary, so they are not relied on beyond the primary statement above.

## 2. Segment ecommerce spec

Source: https://www.twilio.com/docs/segment/connections/spec/ecommerce/v2 (segment.com gave HTTP 403; the section was
read twice through the fetch tool, so exact table cells may be summarised).

- `Order Refunded` fires when an order or transaction was refunded. Its property table lists only `order_id`.
- The sample call sends `total` 30, `currency` USD and a `products` array of two items. The table does not define
  `total` for this event, and no prose says whether it is the refunded amount or the order total.
- The prose says to include all items in the cart as properties, as on `Order Completed`. It does not mention partial
  refunds, and nothing says to list only the refunded products. (Not documented.)
- `Order Completed` defines `subtotal` (after discounts, before tax and shipping), `total` (subtotal plus shipping and
  tax), and `revenue` (with discounts, excluding shipping and tax). On this page `total` is defined by arithmetic,
  whereas the RudderStack page read for the earlier note defines it as revenue with discounts applied; the Segment
  worked example (22.50 plus 3 plus 2 is 27.50) makes `total` include shipping and tax.
- `Order Cancelled` has `order_id`, `affiliation`, `total`, `revenue`, `shipping`, `tax`, `discount`, `coupon`,
  `currency`, `products`.
- Destination guidance: not every destination accepts array properties, so check each destination's docs; Google
  Analytics Ecommerce accepts `total` or `revenue`, not both. There is no refund-specific guidance for destinations.
- Money units are not specified; the examples use plain numbers.

## 3. How stores represent refunds

### Shopify

Sources: https://shopify.dev/docs/api/admin-rest/latest/resources/refund,
https://shopify.dev/docs/api/admin-graphql/latest/objects/Refund,
https://shopify.dev/docs/api/admin-graphql/latest/objects/Order,
https://shopify.dev/docs/api/admin-rest/latest/resources/order,
https://shopify.dev/docs/api/admin-graphql/latest/enums/WebhookSubscriptionTopic.

- A Refund is a record of money returned from an order, and its existence does not prove the money moved; the
  transactions say that. It is always tied to one order.
- Several refunds per order: the REST page says an order can have several, each its own record, listed by endpoint;
  the GraphQL page does not say it directly. The refund calculation returns `maximum_refundable` amounts for what remains.
- REST refund fields: `id`, `created_at`, `processed_at`, `note`, `refund_line_items` (each with a line item id,
  quantity and restock type), `refund_shipping_lines`, `order_adjustments`, `duties` and `transactions`. There is no
  single amount field on the REST refund; the money is in `transactions[].amount` (kind `refund`) (inference from the
  field list). In GraphQL, a refund has `totalRefundedSet`: the total across all its transactions.
- Partial refunds: a refund may return less than calculated by setting a custom amount, and the shop then adds an
  order adjustment to reconcile reports.
- Order level: `totalPriceSet` is the price before any returns; `currentTotalPriceSet` is the price after returns;
  `totalRefundedSet` is the cumulative refund on the order; `netPaymentSet` is received minus refunded. REST has
  `total_price` and `current_total_price` (strings); the REST page text read here did not define the second.
  `financial_status` can be `partially_refunded` or `refunded`.
- Unit: decimal strings in `*_set` objects pairing `amount` with an ISO 4217 `currency_code`, in shop money and
  presentment (customer) money; multi-currency requests need `currency`. The REST sample mixes a numeric `tax_lines[].price`
  with strings.
- Webhook: topic `refunds/create` fires whenever a new refund is created without errors on an order, whether or not
  money has moved; `orders/updated` fires on any order change. The refund webhook payload was not read (the page text
  was truncated). Delivery can repeat, and `X-Shopify-Webhook-Id` identifies one delivery for de-duplication
  (https://shopify.dev/docs/apps/build/webhooks/ignore-duplicates).

### WooCommerce

Sources: https://woocommerce.github.io/woocommerce-rest-api-docs/ (the Orders part; the refunds table was not in the
parts that could be read), and WooCommerce source on GitHub
(`includes/wc-order-functions.php`, `includes/class-wc-webhook.php`, `includes/class-wc-order-refund.php`,
`includes/rest-api/Controllers/Version2/class-wc-rest-orders-v2-controller.php`).

- REST: `POST /wc/v3/orders/{order_id}/refunds` takes `amount` (string), `reason`, `line_items` and `api_refund`
  (default true: the gateway returns the money). `GET /wc/v3/orders/{order_id}/refunds` lists them, so an order can have
  several.
- Decimal monetary values come back as strings with two decimals by default (a `dp` parameter changes it).
- Source: a refund is an order-like record (the class docblock calls it a negative order). `wc_create_refund` takes an
  `amount`, rejects a negative one or one above the remaining refundable balance ("Invalid refund amount"), stores the
  amount positive but the refund's total negative, and gives refunded line items negative quantities.
- The order's REST `refunds` array has `id`, `reason` and `total`; the code builds `total` with a minus sign in front of
  the positive amount, so it is negative; the schema describes it as "Refund total, including tax".
- Partial or full: `woocommerce_order_partially_refunded` or `woocommerce_order_fully_refunded` fire; a full refund
  moves the order to status `refunded` by default.
- Webhooks: there is no refund topic. `order.updated` is triggered by the hooks `woocommerce_update_order`,
  `woocommerce_order_refunded` and `woocommerce_order_refund_deleted`; other order topics are created, deleted, restored.
  A refund can also be deleted, which the REST API allows.
- The WooCommerce webhook docs page (https://woocommerce.com/document/webhooks/) names topic groups but no refund topic.
- Currency: an order has its own `currency` code. Unit is decimal, the shop's precision.

### Stripe

Sources: https://docs.stripe.com/api/refunds/object, https://docs.stripe.com/api/charges/object,
https://docs.stripe.com/refunds, https://docs.stripe.com/currencies,
https://docs.stripe.com/api/events/types (read in part; its length exceeded the tool's output).

- A Refund has `amount` (integer, smallest currency unit), `currency` (lowercase ISO code), `charge`, `payment_intent`,
  `status` (`pending`, `requires_action`, `succeeded`, `failed`, `canceled`) and `reason`.
- Partial refund: pass `amount`. More than one refund can be issued against a charge, but the total cannot exceed the
  original charge.
- A Charge has `amount_refunded` (integer, may be below `amount` after a partial refund) and `refunded` (boolean, true
  only when fully refunded; partial leaves it false). Refunds on a charge appear in an expandable `refunds` list.
- Units: all `amount` values are in the currency's minor unit; 1000 is 10 USD, 10 is 10 JPY (zero-decimal). ISK and UGX
  are two-decimal in the API with the decimals always 00. Three-decimal currencies are not covered in what was read
  (not documented here).
- Webhooks: `refund.created` (Stripe recommends listening to it), `refund.updated`, `refund.failed`, and
  `charge.refunded`, sent when a charge is refunded including partial refunds; the page says to read `refund.created`
  for the refund itself. A refund can fail later, moving the money back, so a created refund is not always a final one.
- A Stripe Refund is not an order. Stripe has no order concept in these objects; a shop must map a charge or
  PaymentIntent to its own order id (inference).

### Comparison

| Question                         | Shopify                                                                               | WooCommerce                                           | Stripe                                   |
| -------------------------------- | ------------------------------------------------------------------------------------- | ----------------------------------------------------- | ---------------------------------------- |
| Refund is an amount or an order? | Record with transactions and line items; amount is the sum of its refund transactions | Negative-order record with an `amount` and line items | Amount against a charge or PaymentIntent |
| Several per order?               | Yes (REST); each its own record                                                       | Yes                                                   | Yes, up to the charge amount             |
| Cap                              | `maximum_refundable` on calculate                                                     | Remaining refundable balance enforced                 | Cannot exceed original charge            |
| Refund identifier                | `id`                                                                                  | `id`                                                  | `id` (`re_...`)                          |
| Unit                             | Decimal string, shop and presentment money                                            | Decimal string, shop precision                        | Integer minor unit                       |
| Sign in the API                  | Not read                                                                              | `amount` positive; order `refunds[].total` negative   | Positive                                 |
| Webhook                          | `refunds/create`, `orders/updated`                                                    | `order.updated` only                                  | `refund.created`, `charge.refunded`      |
| Duplicate deliveries             | Possible; `X-Shopify-Webhook-Id`                                                      | Not read                                              | Not read                                 |

## What this means for Sphericon

Facts are cited above. Statements marked **Inference** are mine.

### Is Klaviyo's attribution default better or worse than ours (last click, 5 days, no opens)?

- Fact: the window is the same, 5 days. The difference is that Klaviyo counts opens (and, by default, MPP opens) as well
  as clicks, and credits across email, SMS and push; Sphericon credits clicks on its own messages only.
- Fact: Klaviyo's own help says MPP can record opens the subscriber did not make, and it needs a setting to exclude
  them.
- **Inference:** ours is the better default. It matches Klaviyo's click window, avoids the MPP problem Klaviyo documents
  without needing an exclusion switch, and the ADR's rule that software clicks do not count is the same idea applied to
  clicks. Counting opens is a later per-Workspace option (off by default) if customers who migrate from Klaviyo ask for
  matching numbers. Expect a Workspace's Klaviyo revenue figure to be larger than ours for two reasons: opens, and
  Klaviyo not subtracting refunds (next point).
- Fact: Klaviyo's revenue is gross of refunds and cancellations; ours is net. **Inference:** keep net, and label it,
  since the overview already says it counts orders sent in.

### Does the Segment spec settle what `total` means on `Order Refunded`?

No. Fact: the table lists only `order_id`, the example has `total` 30 with two products and no definition, the prose
says to send all cart items, and partial refunds are not mentioned. This agrees with what the earlier note found at
RudderStack. The ADR's reading (`total` is the amount refunded) is Sphericon's own rule, not the spec's.
**Inference:** the ADR should keep stating it as a Sphericon rule, and a sender that follows the spec's "all items in the
cart" advice literally may send the original order total on a partial refund. A refund `total` that equals the order total
cannot be told from a full refund, so a repeated or misread event over-counts the refund by the rest of the order. The
`products` list is not a safe source for the amount, since the spec allows the whole cart.

### If a store sends its own refund object, which field would a plain mapper take, and does "total = amount refunded" match what a shop owner expects?

- Fact: the field named `amount` is the refunded money on a Stripe Refund (integer minor units) and on a WooCommerce
  refund (positive decimal string in the REST create and read). Shopify's refund has no single amount field; the money is
  in its refund transactions, and GraphQL gives `totalRefundedSet` per refund.
- **Inference:** a plain mapper that copies `amount` to `total` works for Stripe (after the minor-unit conversion the
  ADR already requires, here from an integer to a decimal) and Woo (if it reads the refund's `amount`, not the order's
  `refunds[].total`, which is negative). For Shopify it must sum the refund transactions, and it must not take the order's
  `total_price` or `current_total_price`: those are order-level, the second is the price after returns, so on a refund
  event it would carry what is left, not what was returned.
- **Inference:** "total = amount refunded" is what a shop owner expects. All three platforms present a refund as money
  returned, and none presents it as a smaller copy of the order. The risk is not the meaning but the three traps above:
  the sign (Woo), the unit (Stripe), and the order-level field (Shopify). Worth stating in the tracker documentation, with
  one mapping example per platform.

### How should several partial refunds on one order be sent and de-duplicated?

- Fact: all three platforms allow several refunds per order, each with its own id (Shopify `id`, Woo `id`, Stripe
  `re_...`), and cap the sum at the order or charge amount. Klaviyo's `unique_id` keys an event per profile and metric,
  and its guide ties the refund's key to the order's, which leaves no room for a second refund (inference from the rule).
  Shopify can deliver the same webhook twice.
- **Inference:** send one `Order Refunded` Event per refund, with the amount of that refund in `total` (not a running
  sum) and the same `order_id` as the order. The spec has no refund id, and the ADR keeps unknown properties in the
  Event, so a sender-supplied refund identifier property (for example `refund_id`, a Sphericon extension and not in the
  spec) is the natural de-duplication key: a refund row is unique per (Workspace, `order_id`, `refund_id`), and a repeat
  is a no-op, as the ADR already says for `Order Completed`. Without it the only safe key left is the Event's own
  identity if Sphericon has one (not checked here), and two real refunds of the same amount on one day cannot be told
  from a retry.
- **Inference:** a sum of refunds above the order total should be capped or flagged, since every platform enforces that
  cap and an excess means a duplicate or a wrong amount; and a refund in a different currency from the order should be
  rejected, in line with the one-currency rule.
- **Inference:** sending a cumulative "refunded so far" total is the wrong shape, because a retry would then replace
  rather than repeat, and it is not what any of the three stores emits per refund.

## Gaps and unverified

- Segment: only the twilio.com mirror was read, via a summarising fetch tool; segment.com returned 403. The statement
  that nothing is said about partial refunds is from that read and may miss a footnote.
- Klaviyo: the default state of the MPP exclusion, the maximum attribution window, click-versus-open priority, how
  campaigns and flows share credit, and the `$value` content for custom integrations are not documented in what was read.
  The Klaviyo glossary and CLV pages were not read. Whether refunds reduce attributed revenue in the current UI is
  stated only through the Shopify data reference.
- Shopify: the `refunds/create` payload and the exact REST definition of `current_total_price` were not read (the pages
  were truncated); the GraphQL definitions were. The "no single amount field on a REST refund" point is an inference
  from the field list. Sign of amounts was not checked.
- WooCommerce: the REST docs page's order refund table could not be reached; the facts come from source code on the
  trunk branch (October 2026), read through the fetch tool. De-duplication of Woo webhook deliveries was not read.
- Stripe: three-decimal currencies, webhook retry and de-duplication, and the `refunds` list on a charge were not
  checked beyond the pages named.
