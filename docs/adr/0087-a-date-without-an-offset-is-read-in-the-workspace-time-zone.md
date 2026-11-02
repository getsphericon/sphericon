---
status: accepted
---

# A date without an offset is read in the Workspace time zone

A datetime Custom field (#817) receives values in three ISO forms: a date-time with an offset
(`2026-01-15T10:00:00+03:00`), one without (`2026-01-15T10:00:00`) and a bare date (`2026-01-15`).
The last two name no moment by themselves. We read them in the **Workspace time zone**: the
date-time at that wall-clock time there, the bare date at the start of that day there. The date
Condition leaves (before, after, within N days) and anything else that compares such a value to a
moment read it this way.

Most tools read them as UTC, a bare date as midnight UTC (Braze, Klaviyo, Iterable, HubSpot's API,
Mixpanel), and Klaviyo documents the cost: date-triggered flows fire a day early or late, and it
advises storing a date without time as text. A `trial_ends` of `2026-01-15` means that day where
the business is, and the Workspace time zone is already where its Report days begin and end
([[0057-a-report-day-is-a-day-in-the-workspace-time-zone]]).

## Considered options

- **UTC.** Rejected: the off-by-one day above for every Workspace west or east of UTC.
- **A separate `date` type.** Rejected: one more type in the leaves and the builder for what "the
  start of the day in the Workspace time zone" already says.
- **The Contact's own time zone.** Rejected: most Contacts have none, and two Contacts with the same
  value would fall into a Segment at different moments.

## Consequences

- Changing the Workspace time zone moves the moment of every value without an offset, and with it
  Segment membership for those leaves. Values with an offset do not move.
- Ingest stays ISO only: no Workspace setting reads `MM/DD/YYYY` or other slash forms, which tools
  read month-first or day-first inconsistently. A JS `Date` sent through an SDK arrives as UTC with
  `Z`, so a date without time is sent as a `YYYY-MM-DD` string; the docs say so.
