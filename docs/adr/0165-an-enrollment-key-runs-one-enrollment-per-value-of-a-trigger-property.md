---
status: accepted
build: [1555, 1617]
---

# An Enrollment key runs one Enrollment per value of a Trigger property

> Amends [[0127-an-automation-may-enroll-a-contact-again-one-enrollment-at-a-time]] (one active
> Enrollment per Contact and Automation).

## Context

A Contact has at most one active Enrollment in an Automation (ADR 0127), so a post-purchase series for
each of two orders a day apart runs once. ADR 0127 kept it so because parallel Enrollments left open
which one an exit or goal Event ends, which one a Wait for event wakes and which one an unsubscribe
exits. Marketing platforms let a person go through an event-triggered flow once per triggering purchase,
in parallel, and a series per order is the common case.

What makes the order's Enrollment identifiable is a value the Events already carry: the `order_id` of
the Trigger, of the order's later Events and of its cancellation.

## Decisions

- **A version with an Event Trigger may name an Enrollment key**: a property of the Trigger Event, such
  as `order_id`. One active Enrollment is then allowed per Contact, Automation and value of the key.
  Without a key nothing changes. A date Trigger has none: its occurrence is its key (ADR 0126). A
  triggering Event without the property enrolls no one, and the entry is skipped and counted.
- **An exit or goal Event and a Wait for event match by the key when the Event carries it.** An Event
  with the property acts only on the Enrollment of the same value: `Order Cancelled` for X ends the
  series of X. An Event without it acts on every active Enrollment of the Contact in the Automation, as a
  general exit should.
- **An unsubscribe ends every Enrollment of the Contact in the Automation**: an opt-out is from the
  Sending source, not from one order.
- **Entry policy and cooldown count per value**: under `once` the same order never starts the series
  again, and the cooldown runs from the beginning of that value's previous Enrollment.
- **A ceiling per Contact replaces the cooldown's floor as the loop guard.** One hour between Enrollments
  bounds a loop through an outside system only while every entry has the same key; a loop that mints new
  keys would not meet it. So a keyed Automation allows a Contact at most 10 active Enrollments and at most
  10 begun in 24 hours, and an entry past either is skipped and counted.
- **Everything else is unchanged**: two orders make two Enrollments and two messages within what the
  Frequency cap allows (ADR 0137); the reports count Enrollments as before; the Enrollment timeline and
  the Path preview (ADR 0159) show the key's value.

## Consequences

- A version gains `enrollmentKey`; an Enrollment records its key's value; the uniqueness of an active
  Enrollment includes it; exits, goals and waits match by it.
- Glossary: **Enrollment key**; the Entry policy and Enrollment entries.

## Rejected

- **Parallel Enrollments without a key**: nothing would say which one an Event belongs to, the reason
  ADR 0127 refused them.
- **A key from the Contact's data**: what tells two runs apart is the Event that began each, not the
  person.
