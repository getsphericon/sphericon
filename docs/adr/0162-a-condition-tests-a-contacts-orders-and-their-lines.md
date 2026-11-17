---
status: accepted
build: [1465]
---

# A Condition tests a Contact's Orders and their lines

> Amends [[0064-revenue-is-read-from-order-events-and-credited-to-the-last-clicked-message]] (an Order
> keeps its lines) and completes what
> [[0153-a-conditions-event-test-filters-the-events-it-counts-by-their-properties]] left to Orders.

## Context

A Workspace's shop reports purchases through order Events (ADR 0064), and an Order is the read model of
them: its purchase amount, currency, refunds and cancellation, kept as long as the reports need it,
independently of Event retention. No Condition reads it. "Bought in the last 90 days", "spent more than
500", "has not ordered since spring" and "bought product Y" can only be approximated by counting order
Events, which expire with the analytical Events (ADR 0019), and an Order keeps no lines at all: the
`products` of the spec live only in the Event's properties. Email platforms that take shop data segment
by purchase activity (total spent, number of orders, product purchased) as conditions of their own.

## Decisions

- **A Condition has an `order` test that reads the Contact's Orders**, not their Events: the number of
  Orders, their net revenue, or the time of the last one, each optionally within the last N days, against
  a bound (`atLeast`, `atMost`, or a moment for the last one). "No order in 90 days" is a count of at
  most zero within 90 days. A cancelled Order counts in none of them. It is valid in every Condition, as a
  test of the Contact.
- **An Order keeps its lines**: copies of the `products` of its latest `Order Completed` or
  `Order Updated` in source order, each with `product_id`, `sku`, `name`, `category`, `quantity` and
  `price`. An `order` test may narrow the Orders it reads to those with a line matching a filter on these
  keys (`eq`, `contains`, `beginsWith`). "Bought Y" is an Order not cancelled with a line for Y; a refund
  does not take a line away, since a refund is not split by line. There is no product catalogue.
- **Sums are in the reporting currency only.** An Order in another currency counts in the number of
  Orders and the time of the last one, never in a sum; nothing is converted.
- **RFM is composed, not built in.** Recency, frequency and value are three `order` tests in one Segment;
  a scoring of them is not part of this decision.
- **Erasure is unchanged.** An erased Contact's Orders stay anonymous for the revenue reports, and their
  lines, which hold no personal data, stay with them.

## Consequences

- The Order read model gains its lines, filled from the order Events already stored; the Condition
  grammar gains the `order` leaf in the contract.
- Glossary: the Order entry.

## Rejected

- **Counting order Events in an `event` test**: they expire with the analytical Events, so "ever bought"
  would shrink without a word, and an Update or a cancellation would have to be replayed in every query.
- **A product catalogue**: an entity of its own to sync from the shop, for tests a copy of the line
  already answers.
