---
status: accepted
build: [1801, 1802, 1803]
---

# A Broadcast may be scheduled for a local time in each Recipient's zone

> **Revised before it was built:** the zones that bound the send are the Earth's, not the
> audience's: it runs from the time in UTC+14 to the time in UTC−12, Schedule refuses it once the time
> has come in UTC+14, and Recipients are frozen at that start. Each Recipient's due moment is the
> earliest its send is paced at, a Recipient a Hold or a Deferral keeps past it is sent when released,
> a Hold at the start delays the freeze, and Schedule takes a union of the two forms.

## Context

A Broadcast goes out at one moment, so "10:00" is 10:00 in one zone and the middle of the night in
another. A Contact has a time zone; a Wait until already reaches a local time in the Contact's zone or
else the Workspace's (ADR 0126); marketing SMS already waits for a Sending window in the recipient's
time (ADR 0134).

Sending at the same local hour in each recipient's zone is a basic feature of email platforms, the one
to have before anything smarter. Some start it when the time comes in the Earth's first zone and ask
for it to be scheduled about a day ahead; others start from the audience's first zone. Choosing a moment
per recipient from their past engagement (send-time optimisation) is offered too, but its gain is
reported on opens, which are unreliable, and it can only be proven against a group held out of it; it
does not help time-sensitive content or small lists.

## Decisions

### The schedule

- **A Broadcast may have a Local-time schedule**: a date and a time of day, reached in each Recipient's
  time zone, the Contact's or else the Workspace's (UTC while it has none), as a Wait until reaches it
  (ADR 0126): the same zone rule, and the same rules for a time a change to daylight saving time skips
  or repeats. A number's prefix is not used to time a send; it only decides where a Sending window
  must be open.
- **The Earth's zones bound it, not the audience's.** The send runs from the moment the time comes in
  UTC+14, the first offset any zone has, to the moment it comes in UTC−12, the last, a span of 26 hours
  fixed by the date and the time alone. Every zone a Contact or a Workspace can hold falls inside it,
  so no Recipient, whatever zone it holds when it is frozen, is due before the send starts.
- **It cannot be armed once the time has come in UTC+14.** Schedule refuses it then: the time has
  already come somewhere on Earth. No Recipient is moved to the next day, and none is armed late.
- **The Broadcast is `sending` from the start of the span** until every Recipient's send has ended,
  as for any Broadcast.

### Recipients and their due moments

- **Recipients are frozen once, at the start of the span**, from the Condition copied at Schedule
  (ADR 0065), each with its **due moment**: the time on the date in its zone then. A change of zone
  after that does not move it, as a Wait until's moment does not move (ADR 0126).
- **A due moment is the earliest a Recipient is sent.** The pace of the Provider limit (ADR 0023)
  runs one timeline for the whole Broadcast: each send goes at its due moment or one step after the
  send before it, whichever is later, so a zone's backlog may delay the next zone's and the limit holds.
- **A Recipient past its due moment is sent when it is released**, as any pending Recipient is: a Hold
  of the Workspace or the Integration, a Deferral of the Send cap, the Provider limit or the Sending
  window keeps it, and it goes out when that lifts. It is not skipped for being late.
- **A Hold at the start of the span delays the freeze**, as it delays any Broadcast's plan: the
  Recipients are frozen when it lifts, each with its due moment as above, and those whose due moment
  has already passed go out at once.
- **Everything else is checked when it is sent**, as for any Recipient: Send-eligibility, the
  Frequency cap, the Sending window, the Send cap and the Provider limit. Cancel cancels those not yet
  sent, the ones not yet due among them (ADR 0150). ADR 0149's count at Schedule is the same.

### Where it is not offered

- **Not with a Split test.** A Split test needs one moment for its test share before the rest (ADR
  0152), so Schedule refuses a Local-time schedule for a Broadcast with several Variants.
- **Not for a Broadcast series.** A series makes its issues in the Workspace time zone (ADR 0135), and
  an issue is armed for a moment, until a local time is asked for there.
- **A Retry takes its own schedule** (ADR 0151): it is a Broadcast like any other, scheduled in either
  form, and inherits nothing from its source's.
- **Unschedule returns it to draft** as any scheduled Broadcast, and a new Schedule may choose either
  form.

### No send-time optimisation

- **Choosing a moment per Recipient from engagement is left out**: its gain is measured on opens and
  cannot be shown without a control group the app does not have yet.

## Consequences

- Schedule takes one of two forms, a moment or a Local-time schedule (a date and a time of day), as a
  tagged union, and the Broadcast shows its schedule in the same union in place of a bare moment.
- A Recipient records its due moment under a Local-time schedule; the pace takes the Recipients' due
  moments.
- The report gives the first and last due moment of the Recipients, and while it sends how many are not
  yet due. The console's Schedule dialog offers both forms and shows when the send starts and ends; it
  disables the local form for a Broadcast with several Variants and says why.
- The share outside the Sending window before Schedule (ADR 0134) evaluates each Recipient at its due
  moment. The Send cap warning reads no moment: it counts the audience against the Send cap.
- Glossary: **Local-time schedule**; the **Broadcast recipient** entry gains its due moment.

## Rejected

- **Bounding the send by the audience's zones**: Schedule would resolve the audience to find its first
  zone, and a Contact who moved to an earlier zone, or arrived in one, before the freeze would be due
  before the send began and would have to be skipped. The Earth's bounds hold every zone, and the cost
  is scheduling up to 26 hours ahead.
- **Sending at once to the zones already past**: those Recipients get the Broadcast hours late, the
  failure a Broadcast's moment exists to avoid (ADR 0137).
- **Moving the zones already past to the next day**: a Broadcast that runs over two days, and content
  dated for one.
- **A pace per zone**: zones that overlap would together exceed the Provider limit.
- **Skipping a Recipient a Hold kept past its due moment**: a Hold makes any Broadcast late, and a lift
  minutes after a zone's hour would drop that zone whole.
- **Timing by the phone prefix's zone**: a prefix spans several zones and says where a number was issued,
  not where its holder is.
