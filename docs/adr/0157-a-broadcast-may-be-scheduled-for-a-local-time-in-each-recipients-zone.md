---
status: accepted
build: none
---

# A Broadcast may be scheduled for a local time in each Recipient's zone

## Context

A Broadcast goes out at one moment, so "10:00" is 10:00 in one zone and the middle of the night in
another. A Contact has a time zone; a Wait until already reaches a local time in the Contact's zone or
else the Workspace's (ADR 0126); marketing SMS already waits for a Sending window in the recipient's
time (ADR 0134).

Sending at the same local hour in each recipient's zone is a basic feature of email platforms, the one
to have before anything smarter. Choosing a moment per recipient from their past engagement (send-time
optimisation) is offered too, but its gain is reported on opens, which are unreliable, and it can only
be proven against a group held out of it; it does not help time-sensitive content or small lists.

## Decisions

- **A Broadcast may have a Local-time schedule**: a date and a time of day, reached in each Recipient's
  time zone, the Contact's or else the Workspace's, as a Wait until reaches it. A number's prefix is not
  used to time a send; it only decides where a Sending window must be open.
- **Sending runs from the first zone to the last.** The Broadcast is `sending` from the moment the
  earliest zone reaches the time until the latest has.
- **It cannot be armed once the moment has passed in any zone.** Schedule refuses a Local-time schedule
  whose moment has already come in the earliest zone. No Recipient is sent late, and none is moved to
  the next day.
- **Recipients are frozen once, when the first zone is due**, each with the moment it is due, and each
  is sent at that moment. Send-eligibility, the Frequency cap, the Sending window, the Send cap and the
  Provider limit are checked when it is sent, as for any Recipient. Cancel cancels those not yet due
  (ADR 0150). ADR 0149's count is the same.
- **Not with a Split test, not for a series.** A Split test needs one moment for its test share before
  the rest (ADR 0152), so a Broadcast with several Variants cannot have a Local-time schedule. A
  Broadcast series makes its issues in the Workspace time zone (ADR 0135) and has none, until it is
  asked for.
- **No send-time optimisation.** Choosing a moment per Recipient from engagement is left out: its gain
  is measured on opens and cannot be shown without a control group the app does not have yet.

## Consequences

- Schedule takes either a moment or a Local-time schedule (`localDate`, `localTime`); the Broadcast
  shows which, and its report the span of zones.
- A Recipient records the moment it is due.
- Glossary: **Local-time schedule**.

## Rejected

- **Sending at once to the zones already past**: those Recipients get the Broadcast hours late, the
  failure a Broadcast's moment exists to avoid (ADR 0137).
- **Moving the zones already past to the next day**: a Broadcast that runs over two days, and content
  dated for one.
- **Timing by the phone prefix's zone**: a prefix spans several zones and says where a number was issued,
  not where its holder is.
