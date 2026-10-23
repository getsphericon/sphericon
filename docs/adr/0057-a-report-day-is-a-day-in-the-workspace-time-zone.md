---
status: accepted
---

# A report day is a day in the Workspace time zone, computed live

Every analytics report counts by **Report day**, a calendar day in the Workspace time zone (UTC until
an Owner sets it), and the hour-of-day report reads the hour in the same zone. Nothing is stored per
day: reports query the Events, Broadcast recipients and Contacts at read time, so a change of the
zone re-buckets the whole history at once and the dashboard, the engagement rates and every report
agree on where a day ends.

UTC days were simpler, but an "opens by hour" report in UTC puts one local morning peak in different
buckets for different Workspaces and means nothing to the reader. A Contact's own time zone was not
considered: a report is one Workspace's view, not one person's.

## Considered options

- **Rollup tables per day.** Rejected for now: a stored bucket freezes the zone it was cut in, so a
  zone change would need a rebuild job. Revisit when a report's volume (Site traffic) makes the live
  query measurably slow.
- **A zone per report or per viewer.** Rejected: two members would see different totals for the same
  window.
