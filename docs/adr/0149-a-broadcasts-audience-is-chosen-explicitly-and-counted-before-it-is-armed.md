---
status: accepted
---

# A Broadcast's audience is chosen explicitly and counted before it is armed

> Amends [[0065-a-live-automation-or-rule-changes-only-by-publish]] (what Schedule answers) and
> [[0135-a-broadcast-series-makes-an-ordinary-broadcast-at-each-occurrence]] (a series' audience).

> **Amended by [[0151-a-broadcasts-unreached-recipients-are-reached-by-a-retry]]:** a Retry's
> audience is its source's unreached Recipients, named by its link, beside a Segment and all Contacts.

> **Amended by [[0152-a-broadcast-tests-its-variants-on-a-share-and-sends-the-winner-to-the-rest]]:**
> the count of a Split test also gives the size of its test share.

## Context

A Broadcast's audience is a Segment or all Contacts. On create, `segmentId` is optional and its
absence means all Contacts (`typespec/external/resources/broadcasts.tsp`), so the widest audience is
the one a caller gets by saying nothing. `PUT /broadcasts/{id}/segment` already requires the field
and takes `null` for all Contacts.

Nothing in the external contract says how many people a Broadcast will reach before it sends.
`segments_preview` counts the Contacts a Condition matches, before Suppression and Unsubscribe.
`segments.Module.Preview` also counts the Recipients a Broadcast on a Channel and Integration would
take now, and `/site` exposes that as `recipients`; `/api` does not, and Schedule answers with the
Broadcast alone. The report's `recipientsTotal` is known only once the Broadcast is planned, that is,
once it is sending. The console shows the count at Schedule only inside the Send cap warning, so
only on an Instance with a Send cap.

An agent driving the app is expected to read back the audience and its size before it asks a person
to approve a send; a person approving in the console needs the same.

## Decisions

- **The audience is always named.** Creating a Broadcast requires `segmentId`; `null` is all Contacts,
  chosen as such. The same holds for a Broadcast series (ADR 0135) and anywhere else a send's audience
  is chosen. In the console the audience picker has no default: the author picks a Segment or "All
  contacts", and the form cannot be saved without a choice.
- **The count is the Recipients a send would take now**: the Contacts its audience matches who have a
  Destination on its Channel and pass Send-eligibility, the figure `segments.Module.Preview` already
  computes.
- **The external preview gives it.** The Segment preview of `/api` (and so MCP) takes an optional
  Channel and Integration and then also answers `recipients`, as `/site` does.
- **Schedule answers it.** The Broadcast Schedule returns carries `recipients`, counted when it was
  scheduled. Recipients are still resolved at send time from the Condition's copy (ADR 0065), so the
  figure is said to be the count at Schedule, not a promise.
- **The console's Schedule dialog always shows it**, beside the authors and link hosts it shows
  already (ADR 0132), whether or not a Send cap applies.

## Considered options

- **An explicit audience object** (`audience: all | segment`) instead of a required nullable
  `segmentId`. The same guarantee with a reshaped model, console and series; a required field, with
  request bodies closed (ADR 0148), already turns a misspelt key into a refusal.
- **Schedule takes the expected count and refuses a mismatch.** The console would fill it in from the
  figure it shows, so it would check nothing there; and the audience legitimately grows between
  Schedule and send (ADR 0065), so the margin would be invented. The count is given; asserting it is
  the caller's.
- **Keep all Contacts as the absent value** and rely on closed bodies. A caller who forgets the field,
  rather than misspelling it, still mails everyone.
