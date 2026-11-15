---
status: accepted
---

# Wait until may wait for a window, and go on at once inside it

> Amends [[0126-an-automation-starts-on-a-date-and-waits-until-a-local-time]] (the window it queued).

## Context

Wait until holds an Enrollment until the next local time of day, on chosen weekdays, or until a date a
Contact holds (ADR 0126). Both forms always wait, so "go on now if it is between 9:00 and 18:00 on a
weekday, else wait for 9:00" cannot be said: an Enrollment that arrives at 11:00 would wait until the
next day. Automation tools offer this as a time window step.

## Decisions

- **Wait until has a third form, a window**: chosen weekdays and a time span within one day, from before
  to. An Enrollment that reaches it inside the window goes on at once; outside, it waits for the next
  opening. The moment is computed and stored when the Enrollment arrives, as for the other forms.
- **In the Contact's time zone, or else the Workspace's**, by the local clock: a span that starts in an hour
  a clock change skips starts at the first moment after it.
- **No span across midnight.** From must be before to; nothing asks for "22:00 to 06:00" yet, and allowing
  it later changes no stored window.
- **The Sending window of marketing SMS is apart**: that is the Workspace's legal floor, checked at send as
  a Deferral (ADR 0134); this is the author's choice in a flow. Both apply.

## Consequences

- The Wait until Step gains the `window` form in the contract, MCP and the editor.
- Glossary: the Wait until entry.
