---
status: accepted
build: [1824, 1825, 1826, 1827, 1828]
terms: [Revenue per recipient]
---

# Revenue per recipient is attributed revenue over the same tracked messages

> Amends [[0064-revenue-is-read-from-order-events-and-credited-to-the-last-clicked-message]] (the
> denominator of a per-recipient figure) and
> [[0152-a-broadcast-tests-its-variants-on-a-share-and-sends-the-winner-to-the-rest]] (the revenue
> Winner metric is this figure).

## Context

A Broadcast and an Automation report their orders and attributed revenue (ADR 0064), but nothing
says what a message earned on average, the figure the manuals agents load judge a welcome or an
abandoned-cart flow by. The two ADRs that name it disagree: ADR 0064 divides by delivered messages,
ADR 0152 by tracked ones. The vendors disagree with themselves: Klaviyo's help divides by successful
deliveries and its blog by sends; Drip's help, API and marketing pages use three denominators for one
figure. The external `/api` and MCP reports of a Broadcast carry no revenue at all, so an agent cannot
read what a Broadcast earned.

## Decision

- **Revenue per recipient is a Broadcast's or an Automation's attributed revenue in the Reporting
  currency over its tracked messages**: a message sent untracked, or whose tracking was withdrawn
  since (ADR 0120), counts in neither, since its links were never rewritten and no click of it can
  take a credit. An Order in another currency counts in no sum, as everywhere. Delivered is not the
  denominator: Telegram reports no delivery, and a message that bounced takes no credit either way.
- **The numerator and the denominator are always the same messages.** For a Broadcast and for an
  Automation's whole history, the revenue is all of its attributed revenue. For a period of an
  Automation's report it is the revenue credited to clicks of the messages sent in that period (the
  send cohort), not the revenue whose movements fall in the period, which the period's revenue keeps
  showing beside it; a ratio of two different sets of messages means nothing.
- **Per Step**: an Automation's Step figures, keyed as Step statistics are (the version and the
  Step's stable id), carry it too, over that Step's messages.
- **It is a model of its own in the contract**, on `/api`, `/site` and MCP: the Reporting currency,
  the revenue, the tracked messages and the value in the currency's minor unit, unrounded, so a
  client can check the value against the two figures it came from. The value is null with no
  Reporting currency or no tracked message. A check over the contract refuses a property named for a
  per-recipient figure that is not of that model.
- **One computation**: the Orders module gives the figure, and the revenue Winner of a Split test is
  read from it.
- **Where**: a Broadcast's report and list, an Automation's report (whole history, each period, each
  Step) and list. The external Broadcast report also gains its orders and revenue. The overview of a
  Workspace shows none: it sums every send, and a per-message average there would judge nothing.
- **Only this figure.** The average order value follows from the orders and revenue already shown;
  an order rate waits for an ask.

## Consequences

- Message retention keeps the denominator: a frozen Broadcast figure and an Automation figure keep
  their untracked count from the freeze (ADR 0143), as they keep what they sent.
- A period's and a Step's figures need an Order to name the message whose click it took; until it
  does, only a Broadcast's and an Automation's whole-history figures can be given.
- Until Engagement tracking consent is built, every Broadcast and Automation message is tracked.
- The console draws the model through one component, formatted in the Reporting currency, and shows
  a dash for null.

## Considered options

- **Delivered messages** (ADR 0064, Klaviyo's help): no delivery on Telegram, and untracked messages,
  which cannot earn a credit, would lower the figure as more people refuse tracking.
- **Sent messages** (Drip's API): the same dilution, and bounces with it.
- **Per Contact**: a Contact receives several messages of an Automation; revenue per Contact is the
  Control group's comparison (ADR 0163), a different question.
- **A period's revenue over the period's messages**: the revenue of last month's sends would be
  divided by this month's.
- **Fields on the existing revenue model**: a period's figure has its own revenue, the cohort's, which
  the model's revenue is not.
