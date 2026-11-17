---
status: accepted
build: none
---

# A frequency cap counts each Destination a message is claimed for, after Send-eligibility

> Amends [[0137-a-frequency-cap-limits-marketing-messages-per-destination]] (what is counted, the
> order of the checks, the takeover of a claim, who decides between Skipped and Deferral, and the
> record of a wait), [[0134-marketing-sms-waits-for-the-workspaces-sending-window-in-the-recipients-time]]
> (the record of a wait and the shape of the Workspace's settings),
> [[0015-outbound-send-single-chokepoint]] (the order of Outbound send) and
> [[0159-an-enrollment-explains-its-path-and-a-path-preview-shows-one-without-effects]] (a send the
> Path preview would defer).

## Context

ADR 0137 counts marketing messages by Destination and checks the cap with the claim of the Outbound
message, in one transaction under an advisory lock. Outbound send as it stands does not fit that
sentence as written:

- One message may reach several Destinations. On a Channel of Bindings a Contact is reached at each of
  its Bindings on the Integration (a Telegram chat, a browser on web push), and the Outbound message
  keeps one Destination, the first that accepted it.
- The claim's time is not a fact. A claim handed back after a provider failure has its time reset, and
  a takeover writes a new one, so a count over that column would drop a message that still holds its
  place.
- The claim comes before the targets and Send-eligibility. A cap checked with the claim would defer an
  Automation send to a Contact who has unsubscribed, which then waits instead of exiting.
- A Deferral leaves nothing behind: the claim is dropped. The one-period bound of ADR 0137 has no
  moment to count from, the reports have no deferral to count, and the Enrollment's timeline of ADR 0159
  has no wait to show.

ADR 0159's Path preview names "Skipped" as the verdict of a send over the Frequency cap or outside the
Sending window; an Automation's send is deferred in both cases first.

## Decision

### Order of Outbound send

- **Outbound send runs: replay, the Erasure check, holds (the Workspace freeze first), the targets,
  Send-eligibility of each target, the Sending window, then under the lock the frequency cap of each
  target and the claim; then render, the Provider limit and the Send cap, the provider call.** A final
  Outcome outranks a temporary one: an unsubscribed Contact's Automation exits at once. The cap is the
  last check before the claim, and the holds stay first.
- **A send Skipped by Send-eligibility is recorded as a decided Outbound message**, as today, without a
  claim: nothing goes to the provider.

### What is counted

- **The count is a record of its own: one row per marketing Outbound message and each Destination it
  is claimed for, with the moment it was counted, which nothing rewrites.** A row counts while its
  message is pending or sent; a message that failed or was skipped frees its places, and a target the
  provider drops (a Binding gone, an opt-out at the provider) frees its own. A Deferral of the Send cap
  or the Provider limit drops the claim, and its rows with it.
- **It is written for every marketing message, whether or not a cap applies**, so a cap set later sees
  the period behind it, and is kept seven days, the longest period. An Erasure deletes the rows of the
  Contact's messages (ADR 0021), so the count restarts, as ADR 0137 has it.
- **Each target is capped on its own.** A target over its cap is dropped from the message as an
  ineligible target is; when no target remains, the cap decides the Outcome.
- **Marketing is a Broadcast's and an Automation's message.** A new kind of Outbound message decides
  whether it counts before it can be sent: the code that sorts the kinds has no default.

### The lock and the claim

- **The lock is per Workspace, Channel and Destination, taken only when a cap applies there**; a
  message to several Destinations takes their locks in one fixed order, so two sends never wait for
  each other in a cycle. It is held from the count to the claim's commit, and joins no lock order with
  the Contact's (ADR 0137).
- **Only a new claim takes a place.** The takeover of a lapsed claim does not count again: the claim's
  rows have held its places since it was made. The code that writes a claim takes the lock's proof, so
  no claim is written without it.

### Skipped or deferred

- **Outbound send decides, in one place.** A Broadcast's Recipient over the cap is Skipped. An
  Automation's send carries when its wait at the step began, and is deferred until a place frees: the
  moment the oldest counted message in its way leaves the period, never later than one period after
  the wait began. At that bound it is Skipped with the cap's reason and the Enrollment moves on (ADR
  0137).

### The record of a wait

- **A wait is recorded where it waits.** An Enrollment carries since when and why it waits at its step,
  cleared when it moves on; a Broadcast Recipient carries why it is deferred beside until when; an
  Outbound message keeps since when and why it waited once it is decided. The causes are one list: the
  Provider limit, the Send cap, the provider, the Sending window, the Frequency cap. The reports count
  what waits now and what the cap skipped from these, and the Enrollment's timeline shows them.
- **The Path preview reads the cap without a lock or a claim** and shows a send it would defer as
  deferred, with the cause and until when, and as Skipped only when it would be: a send at the
  bound of its wait, or one for a number whose Sending window never opens.

### The Workspace's settings

- **The caps and the sending windows are rows per Channel and country**: a row with no country is the
  Channel's, and a country's row outranks it for a number of that country. A cap row with no maximum
  removes the cap there; a period runs from one minute to seven days. A new Workspace gets the SMS
  default as a row, and a migration gives one to every Workspace (ADR 0137).

## Consequences

- Outbound send changes its order: the targets and Send-eligibility move before the claim, the claim
  and the count run in one transaction under the Destination's lock.
- A count table with its Erasure and a seven-day cleanup; the caps and windows as tables of the
  Workspace; the Enrollment, the Broadcast Recipient and the Outbound message gain the record of a wait.
- The Skipped reasons and the Deferral causes become closed lists in the code and the contracts.
- The Path preview's verdict gains "deferred".

## Considered options

- **Counting the Outbound messages by their Destination.** A message holds one Destination while it
  may reach several, and the claim's time is rewritten.
- **A token bucket, as the Send cap has.** A bucket cannot hand back the place of a message that
  failed or was skipped.
- **The cap before Send-eligibility.** A temporary Outcome would hide a final one: an unsubscribed
  Contact's Automation would wait a period before it exits.
- **Counting the takeover of a claim again.** The claim held its place all along; counting it again
  could Skip a message that was already counted.
- **The caller deciding between Skipped and Deferral.** Each surface would carry the rule, and the
  copies drift (the reason ADR 0015 has one Outbound send).
- **Counting a message once on a Channel of Bindings, whatever its Destinations.** ADR 0137 counts by
  Destination; a Binding moved to another Contact would carry the wrong count.
