---
status: accepted
build: [1444]
---

# A Broadcast tests its Variants on a share and sends the Winner to the rest

> Amends [[0080-a-broadcast-has-one-channel]] (a Broadcast's content is its Variants) and
> [[0149-a-broadcasts-audience-is-chosen-explicitly-and-counted-before-it-is-armed]] (the count names
> the test share).

## Context

A Broadcast has one Message content (ADR 0080). ADR 0124 gave Automations a weighted Split, where a
content variant is a Split with a send Step on each path and no winner is picked: the author publishes
a version without the losing paths, and Contacts keep arriving. A Broadcast has no such stream. Its
audience is fixed when it is armed, so testing content means trying the variants on part of it and
sending the best one to everyone else, and the rest waits while the test runs.

Email platforms do this the same way: the variants live in one campaign, a share of the audience is
split between them, and after a set time the winner goes to the remainder, chosen by a metric or by
hand. An agent driving an email platform is expected to pick a winner on clicks or conversions, never
on opens, to fix the sample size before the test and not to stop at the first significant result, and
on small lists to test only large differences (the sender's name, the offer, the format).

## Decisions

### A Broadcast's content is its Variants

- **A Broadcast holds one to N Variants**, each a Message content of the Broadcast's Channel, and on
  email also its own `from_name`. A Broadcast with one Variant is an ordinary send. Content has one
  model, so nothing asks whether a Broadcast is tested before it reads its content. Channel,
  Integration, audience and send time are the Broadcast's and never vary.
- **More than one Variant makes a Split test**, with three settings fixed before the Broadcast is
  armed: the **test share** of the audience (10 to 100%, 20 by default), the **winner metric** and the **test
  duration**.
- **Only a hand-made Broadcast can be tested.** A Broadcast series issue (its content comes from the
  series, ADR 0135) and a Retry (ADR 0151) have one Variant. A Retry of a tested Broadcast copies the
  Winner, or the first Variant if none was picked.

### The test share, then the Winner to the rest

- **Recipients are frozen once, at plan, the rest included.** Whether a Recipient is in the test share,
  and which Variant it gets there, follow from a hash of the Broadcast's and the Contact's ids, as
  ADR 0124's Split does, and are recorded on the Recipient. The test share is split evenly between the
  Variants.
- **The rest go out after the test with the Winner**, each Recipient passing Send-eligibility and the
  Frequency cap when it is sent, so someone who unsubscribed during the test is Skipped. At a test
  share of 100% there is no rest: the test is for learning, and the report marks the Winner.
- **The test lasts a fixed time, from 1 to 72 hours**, by default 4 on the click rate and 24 on
  revenue, which needs a full day of Orders to settle, counted from when the last
  message of the test share went out, so a Deferral does not shorten any Variant's window. The cap
  keeps the rest close to the Broadcast's moment, the reason ADR 0137 does not defer Broadcast
  Recipients.
- **The Winner is picked automatically when the test ends, or by hand before.** A pick by hand is
  final and releases the rest at once. Interim figures are shown, but the automatic pick happens only
  at the end, never on an early lead.
- **The winner metric is clicks or revenue, never opens**: the click rate (unique human clicks), or
  the revenue per Recipient credited to the Broadcast within the Attribution window, each over the
  Variant's tracked messages. A message sent untracked (ADR 0120) counts in neither the numerator nor
  the denominator. The click rate is the default. Every Channel with tracked links can be tested.
- **The leading Variant goes to the rest even when the difference is not significant**: something has
  to be sent, and the leader is no worse on average. Below 95% confidence the report names no Winner
  but "no significant winner", so no one learns a lesson from noise. An exact tie goes to the first
  Variant.
- **The sample size is a warning, not a gate.** Before Schedule the console shows the group size per
  Variant against the size needed to see a 20% lift on the Workspace's recent click rate (two-sided
  alpha 0.05, power 0.8), and below it advises testing large differences only. More than two Variants on an audience under
  100,000 is warned against too: each added Variant thins every group.

### The rest of the Broadcast stays as it is

- **No new status**: a tested Broadcast is `sending` until the rest has gone, and the report shows the
  test and when it ends. Cancel during the test cancels the rest, counted as cancelled (ADR 0150). A
  Hold does not move the pick; the rest waits for the Hold to lift as any pending Recipient does.
- **Picking the Winner by hand is authoring** (`broadcasts:write`, listed over MCP without `mcp:send`):
  the Send already allowed every Variant to go, and the pick only chooses among them.
- **Figures and attribution are per Variant**, beside the Broadcast's totals. An Order is credited to
  the Broadcast and to the Variant of the message that was clicked. ADR 0149's count gives the total
  and the size of the test share.
- **There is no holdout inside a Broadcast.** Attribution credits clicks, and a group sent nothing has
  none, so a holdout measures only when whole groups' Orders are compared. That belongs to a
  Workspace-wide control group, decided apart.

## Consequences

- The contract moves a Broadcast's content into `variants`, adds `testShare`, `winnerMetric`,
  `testDuration` and the Winner, an operation to pick the Winner on `/api`, `/site` and MCP, and per
  Variant figures in the report.
- A Recipient records its Variant and whether it was in the test share.
- Glossary: **Split test**, **Variant**, **Winner**; the Broadcast and Split entries.

## Rejected

- **A test Broadcast and a remainder Broadcast**: two audiences to keep apart, two Cancels, two
  counts, and figures that have to be joined to read one send.
- **A winner by hand only, as in an Automation**: the rest would wait for someone to click, and go out
  stale if no one did.
- **The open rate as a metric**: opens are unreliable both ways (machine opens, untracked messages).
- **A test that stops once a Variant leads significantly**: stopping at the first significant result
  makes false winners far more often than the stated error rate.
