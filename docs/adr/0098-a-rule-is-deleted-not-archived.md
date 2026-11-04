---
status: accepted
---

# A Rule is deleted, not archived

A Rule had both Archive and Delete (ADR 0049, ADR 0054): Delete purged only a Rule that had never
fired and answered 409 for one with Firings, which had to be archived instead, and an archived view
of the Rules list restored them. Nobody restores a reaction to an Event: a Rule is recreated in a
minute, while the second verb and the refused Delete made removing one a two-step puzzle.

## Decisions

- **One verb, Delete.** The `/site` and `/api` contracts drop `archive`, `unarchive`, the
  `archived` list filter, `archivedAt` and the Delete's 409; the `firing` kind of `UsedByKind`
  goes with it. The Rules list has no archived view and its rows no Archive.
- **Delete keeps the history.** A Rule with no Firings is purged, with its versions. One with
  Firings is discarded (`discarded_at`, the mechanism of ADR 0049) under the lock Publish takes, so
  it is made inactive in the same move and never fires again; its Firings stay until retention or
  erasure removes them (ADR 0019, ADR 0021). The Audit entry is `rule.discard`, labelled as a delete
  that keeps the Firings.
- **A deleted Rule is gone for every read and move.** Get, the Firings list, the dry run, Update,
  Publish, Revert, Deactivate and a second Delete answer 404. Nothing restores it: `Undiscard` is
  called for a Rule by no surface. A Firing already running finishes, since the job entry points
  read discarded rows (ADR 0049).
- **What stays with Archive.** The Automation, the Broadcast and the Segment keep their archive (an
  Integration and a Sending domain are deleted, ADR 0097); the shared lifecycle (`internal/drafts`) is unchanged, and the
  Rule calls its Archive as the second half of Delete.
