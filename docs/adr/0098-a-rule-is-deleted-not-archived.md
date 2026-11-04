---
status: accepted
---

# A Rule is deleted, not archived

A Rule had both Archive and Delete (ADR 0049, ADR 0054): Delete purged only a Rule that had never
fired and answered 409 for one with Firings, which had to be archived instead, and an archived view
of the Rules list restored them. The maintainer chose to drop Archive for Rules and keep Delete as
the one way to remove one, as ADR 0097 did for an Integration and a Sending domain.

## Decisions

- **One verb, Delete.** The `/site` and `/api` contracts drop `archive`, `unarchive`, the
  `archived` list filter, `archivedAt` and the Delete's 409; the `firing` kind of `UsedByKind`
  goes with it. The Rules list has no archived view and its rows no Archive.
- **Delete keeps the history.** A Rule with no Firings is purged, with its versions. One with
  Firings is discarded (`discarded_at`, the mechanism of ADR 0049) under the lock Publish takes, so
  it is made inactive in the same move and never fires again; its Firings stay until retention or
  erasure removes them (ADR 0019, ADR 0021). The Audit entry is `rule.discard`, labelled as a delete
  that keeps the Firings.
- **A deleted Rule is read, never changed.** As for an Integration (ADR 0097), Get still answers for
  it, with `deletedAt` set, and so does the list of its Firings, so its page shows what it did. The
  list leaves it out. Update, Publish, Revert, Deactivate and the dry run answer 404: the lifecycle
  checks the row under its lock (`Editable` in `internal/drafts`). A second Delete changes nothing.
  Nothing restores it: no surface calls `Undiscard` for a Rule. A Firing already running finishes,
  since the job entry points read discarded rows (ADR 0049).
- **What stays with Archive.** The Automation, the Broadcast and the Segment keep their archive (an
  Integration and a Sending domain are deleted, ADR 0097). The Rule calls the shared lifecycle's
  Archive as the second half of Delete.
