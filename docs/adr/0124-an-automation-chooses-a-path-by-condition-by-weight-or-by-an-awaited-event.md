---
status: accepted
---

# An Automation chooses a path by Condition, by weight or by an awaited Event

> **Amended by [[0126-an-automation-starts-on-a-date-and-waits-until-a-local-time]]:** in a
> date-triggered Automation a Split's hash also takes the anniversary year, so it draws again each
> year.

> **Amended by [[0127-an-automation-may-enroll-a-contact-again-one-enrollment-at-a-time]]:** when Contacts
> are enrolled again, a Split's hash also takes the Enrollment's ordinal.

ADR 0122 gave an Automation one way to fork: a Branch with a Condition and two paths, `yes` and
`no`, that rejoin. Three more forks are common in marketing automation and were left out of it. One
is a choice among more than two paths: new, active and lapsed customers each get their own path. One
is a random split that tests one path against another. One is a wait that ends early when something
happens: "wait up to three days for a purchase, then remind".

Notifuse's journeys have the first two as nodes of a free graph: `branch` with named paths and a
default, and `ab_test` with weights and a hash of the contact and the node. They have no wait for an
Event, only a fixed `delay`. This decision keeps ADR 0122's structured tree: every new fork is a Step
whose nested paths rejoin after it.

## Decision

### A Branch has ordered paths

- **A Branch holds an ordered list of paths, each with a Condition and its Steps, and a last path,
  `otherwise`, with no Condition.** The Enrollment takes the first path whose Condition is true, and
  `otherwise` when none is. ADR 0122's `yes` and `no` are the case of one path and `otherwise`. A
  Branch with no path besides `otherwise` cannot be saved.
- **Unknown follows from Kleene's rules, with no rule of its own.** A path further down the list
  means "none of the paths above, and its own Condition": `NOT c1 AND … AND ck`. So the first path
  whose Condition is not false decides. If that Condition is true, the Enrollment takes the path. If
  it is unknown, the Enrollment takes the unknown path, even when a later path's Condition is true.
  Otherwise a Contact who refused tracking would skip "did not open" and land on a path written for
  "opened but did not click".
- **The unknown path is a path of its own**, beside the listed ones, with its own Steps; it may be
  empty, meaning "go on", and it rejoins with the others. This settles ADR 0122's "names the path
  unknown takes": the author gives the unknown path, never points it at another path, since no test
  can tell unknown apart and a Contact who refused tracking could get no handling of their own
  otherwise. A Branch whose Conditions hold an engagement test (ADR 0122, ADR 0123) cannot be
  published without it; one without such a test has none.
- **Every path has an id**, given when it is created and kept by every version, like a Step's. The
  Enrollment records the path it took by that id, and the report counts it across versions.

### A Split chooses a path by weight

- **A Split is a Step with two or more paths, each with a weight.** Weights are whole numbers of
  at least 1 that sum to 100. The paths rejoin after the Split, and a path may be empty: that is a
  holdout group, sent nothing.
- **The path is chosen by a hash of the Contact's id and the Split's Step id, mod 100, against the
  running total of the weights.** The choice is deterministic: the same Contact at the same Split
  lands on the same path, with no random state. The path taken is recorded on the Enrollment, as a
  Branch's is. A new version with other weights changes only the Enrollments that begin on it.
- **A content variant is a Split with one send Step in each path.** A send Step has no variants of
  its own.
- **No winner is picked automatically.** The report shows each path's figures, and the author
  publishes a version without the losing paths.

### A Wait for event waits for an Event, with a timeout

- **A Wait for event is a Step with two paths that rejoin after it: `arrived` and `timed_out`**,
  and an unknown path when it waits for an open or click (below). It
  holds a timeout, required and greater than zero, in the same unit as a wait's delay: an Enrollment
  never waits forever, and an Automation is archived only once none of its Enrollments is active. The
  path is part of the Step, not of a Branch after it: whether the Event arrived is decided once and
  recorded on the Enrollment, never evaluated again.
- **It waits for one of two things.** Either an Event of one of the actions it names, with an
  optional Condition that reads the Event's properties as a Rule reads its triggering Event's. Or an
  open or click of the message a named send Step sent to this Enrollment, the test of ADR 0122, with
  the same limit: the send Step must be able to come before the Wait for event.
- **Its Condition reads unknown as no match, as a Rule's does.** A test of engagement history that is
  unknown for an untracked Destination (ADR 0123) and makes the Condition unknown leaves that Event
  uncounted, so the wait goes on. The test of the Enrollment's own messages is refused in it, as in a
  Rule: the second form above is the way to wait for one.
- **An Event is matched as an exit or goal Event is**: by Contact, never a machine open or click
  (ADR 0053) and never loaded history. An Event by action counts when it occurred once the Enrollment
  reached the Step, by the time it occurred. Any open or click of the awaited message counts,
  including one before the Enrollment reached the Step: a message is often opened in the seconds
  between the send and the Step that waits for it.
- **An exit or goal Event of the version cannot be awaited.** It ends the Enrollment first, so
  `arrived` would never be taken; a save that awaits one is refused.
- **The arrival and the timeout race, and the first to commit wins.** Each moves the Enrollment off
  the Step only if it still stands there. When the timeout fires, it first looks for a matching Event
  already recorded that occurred within the window; one found takes `arrived`. An Event recorded
  after the Enrollment left the Step changes nothing. The deadline is stored on the Enrollment, so a
  restart neither loses it nor waits twice.
- **Waiting for an open or click has an unknown path of its own, as a Branch's is, and it cannot
  be published without it.** The Step takes it
  at once, without waiting, when the awaited message went out untracked (ADR 0120), when the send
  Step was skipped, or when the Enrollment never passed it. It takes it at the timeout, instead of
  `timed_out`, when the message lost its `tracked` stamp during the wait because the Contact denied
  tracking.

### Shared by all three

- **A later `denied` leaves the recorded paths and the per-path figures as they are.** ADR 0120
  deletes the Contact's opens and clicks; a path taken because of one stays, because it records what
  the Automation did, as the Outbound messages it sent on that path do, and it is the guard that keeps
  a restarted worker from choosing again. Nothing reads it as engagement: no Condition tests a path
  taken, and the Engagement rates count opens and clicks, not paths.

- **The report shows, for each path of a Branch, a Split or a Wait for event, how many Enrollments
  took it and how many of them reached the goal**, beside each Step's own figures, whose rates count
  tracked messages only (ADR 0053).
- **Exit and goal Events end an Enrollment wherever it stands**, inside a path or in a Wait for
  event; an Enrollment stays on the version it began on (ADR 0065).
- **Limits count the Steps of every path**, as in ADR 0122; nesting has no limit of its own.

## Consequences

- The version's Step tree gains two Step kinds and paths with ids; the contract, MCP, the editor and
  the canvas follow. A Branch's `yes` and `no` become a list of paths.
- The Enrollment records the path id taken at each Branch, Split and Wait for event, and stores the
  deadline of a Wait for event.
- An Event recorded for a Contact also wakes that Contact's Enrollments that wait for its action,
  beside ending the ones it exits or completes.
- The Removal check reads the Conditions of a Branch's paths and of a Wait for event as Definition
  use.
- Not in this decision, each for its own: an exit on a reply, which needs inbound mail (ADR 0012);
  repeat Enrollment; rendering the awaited Event's properties in later Steps; and waits until a
  calendar time.

## Considered options

- **A free graph** (Notifuse's journeys). It is rejected for the reasons in ADR 0122.
- **N-way as its own Step kind beside a two-path Branch.** It would give two Step kinds with the
  same chooser and differing only in how many paths they have.
- **Skipping a path whose Condition is unknown and testing the next one.** A later path's meaning
  includes "not the paths above", so it would route a Contact on a guess.
- **Variants on a send Step.** They test content only; a Split tests timing, Channel and sequence as
  well, and covers content with one send in each path.
- **A random draw stored on the Enrollment.** It works too, but the hash needs no random source,
  can be checked in a test and gives the same answer to the same Contact.
- **Picking an A/B winner automatically.** It needs a stopping rule and a significance test, and it
  would change what a live version sends without a Publish (ADR 0065).
- **A Wait for event followed by a Branch on whether it arrived.** The wait ends knowing the answer;
  asking it again in a Branch would evaluate it a second time and split one decision into two Steps.
- **Waiting for a Condition to become true.** It needs the Condition evaluated again on every change
  of the Contact or on a schedule. Waiting for an Event wakes only on what is recorded, and the
  Condition on the awaited Event covers cases such as "a purchase over 100".
- **Reading "the awaited message went out untracked" as `timed_out`.** That would send the reminder
  to everyone who refused tracking, the mistake ADR 0122 avoids.
