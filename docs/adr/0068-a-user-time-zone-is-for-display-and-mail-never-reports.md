---
status: accepted
---

# A User time zone is for display and mail, never for reports

The app showed every time in the browser's zone, and the one email to a User that carries a time,
the deadline of a Two-factor requirement, printed it in UTC: the server has no browser to ask. A
member who works from a machine set to another zone, or reads mail on the phone, had no way to say
where they are.

## Decisions

- **User time zone** (GLOSSARY): an IANA name on the User, empty until they choose one on their
  profile, checked against the zone database like the Workspace time zone.
- **The app shows times in it**, and in the browser's zone while it is empty. The app reads it from
  the cached profile, so a public page and the Staff console, which have none, keep the browser's
  zone.
- **A User's mail prints its dates in it**, in UTC while it is empty, followed by the zone's IANA
  name: an abbreviation such as `CET` does not exist for every zone.
- **Reports never read it.** A Report day stays a day in the Workspace time zone
  ([[0057-a-report-day-is-a-day-in-the-workspace-time-zone]]), which rejected a zone per viewer so
  that two members see the same totals. The User time zone moves where a timestamp is shown, never
  which day it is counted in.

## Considered options

- **The browser's zone only.** Rejected: mail has no browser, and a User whose machine is set to
  another zone could not correct it.
- **The Workspace time zone for display.** Rejected: one User belongs to several Workspaces, which
  may sit in different zones, and their members do not.
