---
status: accepted
---

# An Automation starts on a date and waits until a local time

> **Amended by [[0135-a-broadcast-series-makes-an-ordinary-broadcast-at-each-occurrence]]:**
> scheduled sends are decided as a Broadcast series.

> **Amended by [[0134-marketing-sms-waits-for-the-workspaces-sending-window-in-the-recipients-time]]:**
> quiet hours for SMS are decided, as the Workspace's sending window in Outbound send.

> **Amended by [[0127-an-automation-may-enroll-a-contact-again-one-enrollment-at-a-time]]:** repeat
> Enrollment on Events is decided; a date Trigger keeps its occurrence key and has no entry policy.

An Automation's Trigger is one Event action, and its only wait is a delay. Three things marketers
schedule by the calendar have no place in that model: a message on a date the Contact holds (a
birthday, the anniversary of a sign-up, three days before a trial ends), a pause until a time that
suits the Contact ("the next weekday at 9:00"), and a send that repeats on a schedule (a weekly
digest).

A Contact's `birthday` is a calendar date with no time or zone. A datetime Custom field is a
moment, and a date written without an offset is placed in the Workspace time zone. A Contact's own
time zone is optional. An Enrollment is once ever, and a birthday comes back every year.

## Decision

### Scope

- **This decision covers a date Trigger and a Wait until step.** A send repeated on a schedule is a
  Broadcast concern, a series that makes an ordinary Broadcast at each occurrence, and is decided
  on its own: through an Automation it would enroll the whole audience every week. Quiet hours for
  SMS, which law sets in some countries, are a guard of Outbound send shared by Broadcasts and
  Automations, not a Step an author may forget, and are decided on their own too. Repeat Enrollment
  in general stays its own decision; this one adds only the repeat a date needs.

### A Trigger is an Event action or a date

- **An Automation version's Trigger is of one of two kinds**: an Event action, as today, or a date.
  A date is not turned into an Event such as `contact.date_reached`: each Automation reads its own
  offset, so the app would record Events for other Automations' settings, by the million, and an
  Event is what a customer or a provider reports (ADR 0125). A Rule has no date Trigger: it reacts
  to Events.
- **A date Trigger names a field, a recurrence, an offset and a local time.** The field is
  `birthday` or a datetime Custom field. The recurrence is _every year_ (a birthday, a sign-up anniversary) or _once_ (`trial_ends_at`). The
  offset is a whole number of days before or after. The local time is a time of day, such as 10:00.
  The field is in Definition use while a kept Automation's live version or draft names it, so the
  Removal check refuses to remove or retype it.
- **The field gives the date, the Contact gives the time of day.** A datetime field's calendar date
  is read in the Workspace time zone, where a date written without an offset is placed, so a
  sign-up date stored as midnight is the same day for every Contact. The offset and the local time
  then apply in the Contact's time zone (below). A `birthday` is a date already.

### Once per occurrence

- **A yearly date enrolls a Contact at most once per anniversary year, a once date at most once
  ever.** The year is the anniversary's, not the offset moment's: "3 days before" a birthday on
  2 January runs on 30 December and counts for the new year. An occurrence that comes while the
  previous year's Enrollment is still active is skipped. A date corrected after the Contact was
  enrolled for a year enrolls no one again for that year; a correction across New Year can so skip
  a year, which is rarer and milder than a second birthday message weeks after the first. The key is
  the occurrence, so cooldowns and Event deduplication, the open questions of repeat Enrollment, do
  not arise.
- **An Automation with a date Trigger cannot be the target of an Enroll**, a Step or a Rule Action
  (ADR 0125): an Enrollment it did not begin would have no occurrence to count once. Both directions
  are refused: saving an Enroll that names such an Automation, and publishing a date Trigger on an
  Automation that an active Rule or the live version of an active Automation enrolls in, the refusal
  naming them.
- **A Split in a date-triggered Automation draws again each year.** Its hash takes the anniversary
  year beside the Contact and the Step (ADR 0124), so a Contact in a holdout one year is not left
  without a birthday message for good.

### Time zones and the calendar

- **The time zone is the Contact's when it has a valid one, and the Workspace time zone
  otherwise.** A change of either moves only occurrences not yet reached.
- **A local time that does not exist**, skipped by a change to daylight saving time, is the first
  moment after it; **one that occurs twice** is the first of the two.
- **29 February is 28 February in a year without it**, so the occurrence stays in its month.

### Running it

- **Only forward from the moment the Automation became active.** An occurrence before it enrolls no
  one, even one a few hours before: this year's birthdays already past are not sent at Publish.
  Deactivating and publishing again starts a new active span; occurrences while it was inactive are
  never enrolled.
- **An occurrence is enrolled at most 24 hours late.** An occurrence within the active span that the
  scheduler reached late, after an outage, is enrolled if it is less than 24 hours old; an older one
  is dropped and counted on the Automation's report as missed. A birthday message three days late is
  worse than none. The same 24 hours enroll a Contact whose date was set, or who was created, just
  after its occurrence; a date that arrives later than that has no occurrence this year and is not
  counted as missed.
- **The Enrollment begins at the occurrence's moment**, which is where its exit and goal Events are
  counted from, and its `event` is empty: no Event began it.
- **A date Trigger has no entry Condition.** A Branch with an End at the start filters who goes on;
  with one Enrollment per occurrence that costs nothing. Sends are filtered by Send eligibility as
  always.

### Wait until

- **Wait until is a Step that holds the Enrollment until a calendar moment**, in one of two forms:
  the next local time of day, optionally on chosen weekdays ("the next weekday at 9:00", "Tuesday at
  10:00"); or a date read as a date Trigger reads one, a field, a recurrence, an offset and a local
  time ("3 days before `trial_ends_at`, at 10:00"; for a yearly field, its next occurrence).
- **The moment is computed when the Enrollment reaches the Step**, in the time zone and with the
  calendar rules above, and stored, so a restart neither loses it nor computes it again. A change of
  time zone during the wait does not move it.
- **A moment already past, or a field the Contact does not hold, moves the Enrollment on at once**,
  and the report counts it. A field a Wait until names is in Definition use.
- **A moment more than 366 days ahead moves the Enrollment on at once too**, counted apart. A date
  field comes from ingest, and a far-future value would hold an Enrollment for decades and keep its
  Automation from being archived; a yearly date's next occurrence is always within that bound.
- **A window is not expressible**: "go on now if it is between 9:00 and 18:00, else wait for 9:00"
  needs a Wait until that may not wait. It is queued on its own.

## Consequences

- The Trigger becomes a choice of two kinds in the contract, MCP and the editor; the version's Step
  tree gains Wait until.
- An Enrollment's uniqueness gains the occurrence for a date Trigger: once per year, or once.
- A scheduler finds the due occurrences of every active date-triggered Automation, in each
  Contact's time zone, and enrolls them; it records the missed ones.
- The Automation report counts missed occurrences and Wait until steps passed at once.
- Queued: a Broadcast series for scheduled sends, quiet hours as a guard of Outbound send, and a
  window for Wait until.

## Considered options

- **A synthetic Event per date.** It records Events nobody reported, by the million, shaped by
  Automations' settings.
- **Settling repeat Enrollment first.** A date needs only the repeat its occurrence keys; repeat
  entry on Events, with cooldowns and concurrency, is a larger question it does not wait for.
- **Enrolling every missed occurrence however late.** It sends birthday messages days after the
  birthday after an outage.
- **Enrolling past occurrences at Publish.** Publishing a birthday Automation in December would send
  a year's birthday messages at once.
- **1 March for 29 February.** It moves the occurrence out of its month; 28 February keeps a
  birthday in February.
- **A sending window on the Automation instead of a Wait until step.** It hides a delay that
  applies to every send in a setting; a Step shows it where it happens, and the legal quiet hours it
  would also serve belong to Outbound send.
- **An entry Condition on a date Trigger.** A Branch at the start does the same, and keeps entry
  conditions out of the Automation, as ADR 0122 decided for Event Triggers.
