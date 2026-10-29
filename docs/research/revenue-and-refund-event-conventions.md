# Revenue and refund event conventions: GA4, PostHog and Amplitude

Researched October 2026 from primary sources: the Google Analytics developer docs and help center, the PostHog docs
(posthog.com/docs) and the Amplitude docs (amplitude.com/docs). This note settles the questions
[ADR 0064](../adr/0064-revenue-is-read-from-order-events-and-credited-to-the-last-clicked-message.md) left open and
extends [revenue-and-attribution-prior-art.md](revenue-and-attribution-prior-art.md), which covers Drip and the
RudderStack spec and is not repeated here.

**Source quality caveat.** Every page below was fetched directly, but the fetch tool returns a model summary of the page,
not the raw text, so field lists and wording were extracted by summarisation. Claims taken from search-engine excerpts
only are marked **(excerpt)**. Pages that returned 404 or no usable content are listed under "Gaps".

## Question

ADR 0064 reads `total` on an `Order Refunded` Event as the amount refunded, because the RudderStack spec has no refund
amount field and does not say what `total` means on a refund. Is that the common convention? Should Sphericon also
accept another field? How should repeated refund Events be handled?

## Summary of findings

- GA4 expresses a refund as a `refund` event that names the original `transaction_id` and carries `value` for the
  refunded items; a partial refund is expressed by sending only the refunded items (or a `value` for them).
- Amplitude expresses a refund as a negative `price` or `revenue` on an ordinary revenue event, with an optional
  `revenueType` of refund. There is no link to the original order.
- PostHog's own e-commerce spec page says `total` on Order Refunded is the refunded portion, not the whole order. This
  directly supports ADR 0064. (RudderStack's own page does not say it; see the existing note.)
- PostHog revenue analytics documents no refund handling for custom revenue events at all.
- Idempotency is explicit only in GA4 (same `transaction_id` keeps the first event) and Amplitude (`insert_id` plus
  `device_id`, 7 days). Neither documents a rule for two refunds against one order.

## 1. Google Analytics 4

Sources: https://developers.google.com/analytics/devguides/collection/ga4/ecommerce (section "Make a purchase or issue
a refund"), https://developers.google.com/analytics/devguides/collection/protocol/ga4/reference/events (Measurement
Protocol events reference, `purchase` and `refund`), https://developers.google.com/analytics/devguides/collection/protocol/ga4/sending-events,
https://developers.google.com/analytics/devguides/collection/ga4/validate-ecommerce.

**A. Refund shape.**

- A refund is a `refund` event, not a negative `purchase`. `transaction_id` is required and names the transaction being
  refunded (Measurement Protocol events reference, `refund`).
- Partial refund: `items` is optional but recommended; the page says to list the refunded items with their quantities
  so item-level refund metrics work. It does not define "full" or "partial" refunds. Listing only the refunded items
  with `value` matching them is the pattern in the example, which is my reading, not a documented rule
  (inference).
- Meaning of `value` on a refund: the sum of price times quantity across the refunded items, excluding shipping and
  tax (Measurement Protocol events reference, `refund`). So it is the amount returned, not the order total. This is
  the closest match to the "amount refunded" reading. The ecommerce guide's sample (value 30.03 for one item at 10.01
  times 3) is consistent with it.
- Shipping and tax can be sent on a refund as separate optional parameters, recorded as transaction costs; whether
  they are added to `value` is not documented.

**B. Revenue figure.** `value` is the sum of price times quantity of the items, excluding shipping and tax, which are
separate parameters (Measurement Protocol events reference, `purchase`). Item `price` is the unit price after any item
discount (reference, `refund`; assumed the same for `purchase`, inference).

**C. Currency.** One per event, an ISO 4217 code, required whenever `value` is set; the page says metrics may not
compute accurately without it (events reference). The ecommerce guide says the event-level currency is the
transaction's primary currency. Conversion to a property currency and the behavior when currency is absent are not
documented on the pages read.

**D. Idempotency.** `transaction_id` "helps you avoid getting duplicate events for a purchase" (events reference). The
validation guide says that when an ecommerce event is sent twice with the same transaction ID, only the first is kept,
even if the second has different values (validate-ecommerce page; it does not single out `purchase` or `refund`). A
help page adds that an empty transaction ID is itself deduplicated and that this works for web streams only
(https://support.google.com/analytics/answer/12313109, **(excerpt)**). Whether a second refund with the same
`transaction_id` is dropped is not documented; the `refund` event reuses the purchase's `transaction_id`, which
suggests a second refund of one order could collide (inference).

**E. Units.** Decimal major units (examples such as 72.05). No rounding or rejection rule documented on the pages read.

## 2. PostHog

Sources: https://posthog.com/docs/revenue-analytics/capture-revenue-events,
https://posthog.com/docs/revenue-analytics/troubleshooting,
https://posthog.com/docs/data/event-spec/ecommerce-events.

**A. Refund shape.**

- Revenue analytics on custom events: refunds and negative revenue are not documented on the capture or the
  troubleshooting pages. A search excerpt also found refunds discussed only for the Stripe-based billing views
  **(excerpt)**. So there is no documented way to send a refund as a custom revenue event.
- PostHog's ecommerce events specification (https://posthog.com/docs/data/event-spec/ecommerce-events) defines
  `Order Completed` with `total` (subtotal plus shipping and taxes), `subtotal`, `revenue` (discounts applied, shipping
  and taxes excluded), `shipping`, `tax`, `discount` and `currency`. It defines `Order Refunded` with only `total`,
  `currency` and `products`, and says `total` there is the refunded portion, not the whole order. It has no
  refund-specific breakdown. Its Order Refunded example is internally inconsistent: `total` is 30 while the listed
  products add up to 90, and the page does not say whether `products` lists only the refunded items.
- This is a statement in a primary source, but a summariser read it; confirm it on the page before relying on it.

**B. Revenue figure.** For revenue analytics the revenue figure is whatever event property the project maps in the
Revenue tab of data management, with optional product, coupon and subscription properties
(capture-revenue-events). Whether tax or shipping is included is up to the sender: not documented. In PostHog's
ecommerce spec, `total` includes shipping and tax and `revenue` excludes them (spec page above). The spec's
`Order Completed` `total` therefore differs from the revenue the spec calls `revenue`; ADR 0064 reads `total`.

**C. Currency.** A currency property is optional but recommended; if absent a static project currency can be set
(capture-revenue-events). Revenue is converted to the reporting currency using Open Exchange Rates, the rate being the
one for the event's date, and this happens automatically only in the managed views; a "Sum of Revenue" trend sums raw
values (troubleshooting page). An unsupported currency code records zero revenue silently (troubleshooting page).

**D. Idempotency.** Not documented on the pages read.

**E. Units.** The revenue property may be minor units (recommended, to avoid float drift) or a decimal amount, selected
by a setting. With minor units on, most currencies are two-decimal and a list of zero-decimal currencies (JPY, KRW and
others) is whole units; ISK is treated as two-decimal for Stripe compatibility (capture-revenue-events). No rejection
or rounding rule is documented.

## 3. Amplitude

Sources: https://amplitude.com/docs/data/sources/instrument-track-revenue,
https://amplitude.com/docs/apis/analytics/http-v2 (HTTP V2 API),
https://amplitude.com/docs/data/currency-conversion.

**A. Refund shape.** A refund is an ordinary revenue event with a negative `$price` or `$revenue`; `$revenueType` is a
free label whose examples include refund (instrument-track-revenue). The HTTP API says the same for `price` and
`revenue`. There is no field linking a refund to an original order, and no refund-amount field. A partial refund is
simply a smaller negative number (inference; the pages do not discuss partial refunds). The amount means the amount
returned, never the order total, since the event is its own revenue line.

**B. Revenue figure.** `$revenue` or `$price` times `$quantity` (quantity defaults to 1). If price, quantity and
revenue are all sent, the HTTP API doc says Amplitude uses price times quantity (HTTP V2 API). The same page's example
request sends all three with inconsistent values, so the example contradicts the stated rule. A Revenue LTV chart needs
`$revenue` (instrument-track-revenue). Tax and shipping handling is not documented, except that `$revenueType` may
label a tax line, which means tax would be its own event (inference).

**C. Currency.** `$currency` is one ISO 4217 code (three uppercase letters) per event. Conversion to a project target
currency happens through derived properties and only for events that carry both `$currency` and `$revenue` or
`$price`; events without `$currency` are not converted and no other fallback is described. Rates come from ExchangeRate
API, updated daily, using the previous day's rate; user timelines show the unconverted value
(https://amplitude.com/docs/data/currency-conversion). A search excerpt says the target currency defaults to USD when
unset, in event streaming **(excerpt)**.

**D. Idempotency.** `insert_id`: later events with the same `insert_id` and `device_id` as an earlier one within the
past 7 days are ignored, per app. Amplitude recommends sending one with every event, which also guards against
retries (HTTP V2 API). The behavior without a `device_id` is not stated. This works at the event level, so it
dedups a retried refund, but it also means two real refunds with the same `insert_id` collapse to one.

**E. Units.** `price` and `revenue` are floats in major units. No rejection or rounding rule is documented.

## 4. Comparison

| Dimension              | GA4                                                                | PostHog                                                                                          | Amplitude                                                     | RudderStack (existing note)                                  |
| ---------------------- | ------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------ | ------------------------------------------------------------- | ------------------------------------------------------------ |
| Refund expressed as    | `refund` event naming `transaction_id`                             | Custom revenue: not documented. Ecommerce spec: `Order Refunded` event                           | Negative `$price` or `$revenue` on a revenue event            | `Order Refunded` event                                       |
| Link to original order | Required `transaction_id`                                          | `order_id` in the spec, not stated                                                               | None                                                          | `order_id`                                                   |
| Partial refund         | Refunded items only, `value` matching them (inferred, not defined) | Smaller `total`; products ambiguous (spec example inconsistent)                                  | Smaller negative number (inferred)                            | Not defined                                                  |
| Refund amount field    | `value` = refunded items, excluding tax and shipping               | `total` = refunded portion (spec page)                                                           | The negative amount itself                                    | None; `total` meaning not stated                             |
| Revenue field          | `value`, items sum, excludes tax and shipping                      | Mapped property; spec `total` includes shipping and tax                                          | `$revenue`, or `$price` times `$quantity`                     | `total` (discounts applied); `revenue` excludes ship and tax |
| Currency               | One per event, required with `value`; conversion not documented    | Per event or static; converted at the event date in managed views only; unsupported records zero | One per event; converted to a project currency only when sent | Optional string, no conversion                               |
| Missing currency       | Metrics may be inaccurate; not documented further                  | Static project currency, else not documented                                                     | Event not converted                                           | Not documented                                               |
| Dedup                  | Same `transaction_id`: first kept; refund case not documented      | Not documented                                                                                   | `insert_id` plus `device_id`, 7 days                          | Not documented                                               |
| Units                  | Decimal                                                            | Minor units or decimal, a setting                                                                | Float decimal                                                 | Plain numbers                                                |
| Rejection or rounding  | Not documented                                                     | Not documented; unsupported currency becomes zero                                                | Not documented                                                | Not documented                                               |

## 5. What this means for Sphericon

Facts are cited above. The rest of this section is **inference**.

**Is "total on Order Refunded = amount refunded" the common convention?** It is the reading that fits every source
that says anything. PostHog's own ecommerce spec states it outright (section 2). GA4 and Amplitude do not use `total`,
but in both a refund's amount field means the amount returned: GA4 `value` is the sum of the refunded items
(section 1), and Amplitude's refund is a negative amount by itself (section 3). No source reads a refund amount as the
whole order total. So the ADR's choice matches the convention; it is not yet an explicit one in the RudderStack text,
and the PostHog example that contradicts itself (30 against products summing to 90) shows senders do not always
follow it. A caution: the spec's Order Refunded `products` list may hold the whole order, so `products` should not be
used to compute the refund.

**Should Sphericon also accept another field?** Reasonable, with a fixed precedence and no guessing:

- GA4 uses `value` and Amplitude uses `revenue`, while Drip uses `refund_amount` (existing note). A sender bridging
  from those tools may send any of them. Accepting `refund_amount` when present, ahead of `total`, costs little and
  removes the ambiguity for senders who can state it. This follows Drip's clear field (prior-art note, section 1).
- Accepting `value` on a refund is weaker: in the RudderStack spec `value` is not on order events at all (prior-art
  note, section 4), and in GA4 it is the items' sum excluding tax and shipping, which is not the same figure as
  `total`. If both `value` and `total` arrive and differ, there is no documented rule to pick one. Leaving `value`
  out keeps ADR 0064's "the spec is the contract" intact. This is a judgement call for the maintainers.
- Whatever is chosen, name it in the ADR and keep one stored meaning: refund amount.

**How should repeated or duplicate refund Events be handled?** No source documents a rule for two refunds of one
order, so Sphericon defines it:

- GA4 and Amplitude both dedup by an explicit identifier (`transaction_id`; `insert_id`). A refund Event has no
  identifier of its own in the RudderStack spec, only the order's `order_id`, so keying on `order_id` would collapse
  two genuine partial refunds, as GA4's same-`transaction_id` rule may do (section 1, inference).
- ADR 0064 says refunds "are only added". With no refund id, a plain retry of the same Event cannot be told apart
  from a second partial refund of the same amount. Two options follow. (1) Dedupe by the Event's own identity (the
  message id the Event already has at ingest, if the ingest path keeps it), which treats a re-delivery as a duplicate
  and two separate Events as two refunds. (2) Accept an optional `refund_id` (not in the spec; Drip v2 has an
  `upstream_id` for refunds) as a dedup key when present. I would use (1) by default and (2) as an optional addition.
- Guard against over-refund: if the sum of refunds would exceed the order total, store the Event but do not count the
  excess in net revenue, and surface it. This is my suggestion; no source describes it.
- A refund for an unknown `order_id` has no documented analog in these tools. The ADR may decide whether it is kept
  pending or ignored.

## Gaps and unverified

- PostHog revenue analytics: refunds and negative revenue, idempotency, and the `revenue-events` and `event-ecommerce`
  pages (both URLs tried returned 404; the actual pages are `capture-revenue-events` and `data/event-spec/ecommerce-events`).
- PostHog's Order Refunded statement and Amplitude/GA4 field details were read through a summariser; a human should
  confirm the load-bearing sentences (PostHog `total` on refund; GA4 `value` on refund) on the pages.
- GA4: the exact behavior of a repeated `transaction_id` on `refund` events, behavior when `currency` is missing,
  and Measure ecommerce help center definitions of revenue metrics (support.google.com/analytics/answer/9267568 turned
  out to be about key events and had no ecommerce content).
- Amplitude: the SDK reference pages (the browser SDK 2 page had no Revenue section; the TypeScript SDK URL returned
  404), insert_id behavior without `device_id`, and the default target currency (only a search excerpt).
- Rounding or rejection rules for money are not documented for any of the three products.
