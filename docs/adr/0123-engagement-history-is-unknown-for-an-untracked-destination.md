---
status: accepted
---

# Engagement history is unknown for an untracked Destination

A Condition counts Events: "did `email.opened` at most 0 times in the last 90 days" is how a
Workspace finds the people its mail no longer reaches, and drops them. Since ADR 0120, a
Destination whose Engagement tracking consent is `denied`, or that falls under a `dont_track`
default, has no opens recorded, and a `denied` deletes the ones it had. Such a person matches "never
opens" and is the first to be dropped, for refusing to be tracked. ADR 0122 made a Branch's test of
its own message three-valued for this reason and left history tests two-valued; the same harm
reaches every Segment, Rule and Branch that counts engagement.

## Decision

- **A test of engagement history is unknown for a Contact whose Destination on that Channel is not
  tracked now**: a count of `email.opened` or `email.clicked` against the Contact's email, of
  `sms.clicked` against its phone, of `telegram.clicked` against its Bindings, in either direction
  ("at least" and "at most"). "Not tracked" is the Destination's effective state today: its
  Engagement tracking consent, or the Workspace's default where none is recorded. It does not look
  at which days of the window were tracked: someone tracked for 80 of 90 days and not today is
  unknown, which errs on the side of the person. A Contact with no Destination on that Channel keeps
  the two-valued test: nothing was sent there to be tracked.
- **Unknown passes through the tree by Kleene's rules**, as in ADR 0122: `AND` is false if any side
  is false, `OR` true if any side is true, `NOT` keeps unknown. A Segment and a Rule read an unknown
  result as no match, as `WHERE` does in SQL, so the person is neither "engaged" nor "unengaged". A
  Branch whose Condition holds any engagement test, of its own messages or of history, names the
  path unknown takes.
- **Every other test stays two-valued**: delivery Events, the customer's Events, On-site form
  Events, fields and Tags.
- **The Segment's preview and count say how many Contacts it left out** because their engagement is
  unknown, so an author sees who is missing and why.
- **No extra Condition field.** The Engagement tracking consent field (ADR 0120) already finds the
  people to ask for consent; this decision is what keeps the rest from being dropped.

## Consequences

- The Condition compiler emits a nullable leaf for an engagement count: `NULL` where the Contact's
  Destination is not tracked, so Postgres applies the three-valued logic of `AND`, `OR`, `NOT` and
  `WHERE` itself, and a Branch reads `NULL` as its unknown path.
- A Segment's figures and a Broadcast's audience shrink when a Workspace turns to `dont_track`, by
  the people whose engagement became unknown; the preview says so.
- A Rule with an engagement count does not fire for an untracked Contact.

## Considered options

- **A "tracked" field and a warning in the editor** when a Condition counts engagement without it.
  It leaves the protection to each author, and a sunset Segment is where it is most easily missed.
- **Excluding untracked Contacts from "at most" tests only.** `NOT` over an "at least" test would put
  them back in.
- **Tracking state per day of the window.** It needs the history of every Destination's consent and
  default, for a difference that only ever moves people from unknown to known.
