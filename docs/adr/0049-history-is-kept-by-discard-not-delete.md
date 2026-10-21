---
status: accepted
---

# History is kept by Discard, not by delete

Only a Workspace was removed softly (ADR 0043); every other row was removed with `DeleteOneID`.
That broke history in three places: deleting an Automation cascaded away its Enrollments (the
Outbound messages stayed, the record of who was enrolled and why a run failed did not), deleting a
Broadcast removed its Recipients, from which its figures are derived, and deleting a Segment left a
Broadcast pointing at a row that no longer existed, so its plan failed at send time. Editing an
active Automation's steps also moved the step an in-flight Enrollment stood on, because an
Enrollment holds only an index into the live definition.

## Decisions

- **Three classes of row.** _Configuration without a history trail_ (Tag, Template, Brand kit,
  Webhook endpoint, Membership, Invitation) is deleted physically; the Audit log is its trace
  (Membership and Invitation removals already record their own Audit entries by hand). _Configuration
  that history or live work depends on_ (Segment, Integration, Sending domain, Automation,
  Broadcast) is never removed while something live uses it, and is otherwise discarded. _Facts_
  (Event, Outbound message, Audit entry, Suppression) are never discarded, only removed by
  Retention or Erasure. A Suppression is lifted, not deleted: `lifted_at` is stamped, the row
  stays, the send path reads only rows with `lifted_at IS NULL`, and the unique index on
  (workspace_id, channel, destination) is partial on it. Contact erasure only detaches the
  Contact, as today.
- **Discard is one mechanism**, a `DiscardMixin` (`discarded_at`, nullable; no `discarded_by`, the
  Audit entry names the actor) and a generated part of the scoped client (ADR 0017). An entity that
  embeds the mixin gets `Discard`, `Undiscard` and `Purge` on `*ent.Scoped` and **no `Delete`**.
  Only **list queries** hide discarded rows: `Scoped.Query()` sees kept rows and
  `QueryWithDiscarded` sees all. A by-id `Get` and `UpdateOneID` see every row, because a
  discarded row is still addressable: it is viewed (a sent Broadcast's report), restored, and
  receives tracking, unsubscribe and engagement. The readers that always include discarded rows
  are Undiscard, Contact erasure (ADR 0021), Workspace erasure, `sphericon encryption-key
  reencrypt`, the Export streams, the bus subscribers and the job entry points, and the analytics
  reads that must not change with a discard (ADR 0011). `Purge` is the physical removal and is
  called only for a draft Broadcast, an Automation with no Enrollments, and Workspace erasure.
  Discard and Undiscard publish Audit entries (`discard`, `restore`).
- **Workspace takes the field, not the generated methods.** It is the tenant root with no scoped
  wrapper, so `deleted_at` becomes `discarded_at` through the mixin and `erased_at` stays; its
  delete, restore and erase stay the compare-and-swap lifecycle of ADR 0046. ADR 0043 is amended
  only in that column name.
- **The contract says `archived`.** The `/site` and `/api` surfaces name the state `archived`
  (a filter `archived=true`, `archive` and `unarchive` operations) for Broadcasts and
  Automations; `discarded_at` is the column and the glossary term for the mechanism.
- **Live dependencies block, they do not cascade.** A Segment used by a draft or scheduled
  Broadcast, an Integration or Sending domain used by a scheduled or sending Broadcast or an active
  Automation, a Tag used by a Segment rule or an Automation step, and an Automation with active
  Enrollments each answer 409 naming what uses them. References from sent or failed Broadcasts are
  history and do not block. **Use is not only an id column**: a Broadcast with no Integration uses
  the Workspace's default Integration, a Sending domain is used through the domain of a Broadcast's
  or an Automation step's From address, and a Segment rule names a Tag by its name.
- **Broadcasts and Automations are discarded, not deleted.** Only a sent or failed Broadcast is
  discarded (a draft is purged). Discarded rows are hidden from lists, listed by `archived=true`,
  restorable, kept for ever, and still counted by reports and the analytics dashboard.
- **An Automation's steps are versioned.** `automation_versions` rows are immutable once an
  Enrollment points at them; `AutomationRun.version_id` is required and the worker reads steps from
  it. Saving steps creates a new version unless the latest one has no Enrollment, in which case it
  is rewritten in place. Name and trigger stay on the Automation. Versions hold sealed HTTP secrets,
  so `internal/rekey` and Workspace erasure cover them.
- **Segment, Integration and Sending domain take the mixin too**, with the same blocking rules and
  `archived` in the contract. An archived Integration loses `is_default` and `Undiscard` does not
  restore it; the default index is partial on `is_default AND discarded_at IS NULL`. An archived
  Sending domain serves no Tracking host (the tracking host gate, `tls/ask` and the change log
  ignore it, and the Instance-wide domain index excludes it); `Undiscard` starts a new DNS check.
  A Segment's definition is not snapshotted into a sent Broadcast: its Recipients are the record
  of who received it. Only Workspace erasure calls `Purge` on them.
- **Custom fields are never deleted**, only renamed, so a Segment rule can never name a missing key.
- **A unique index on a discardable entity is partial** (`WHERE discarded_at IS NULL`), or a
  discarded row holds the name for ever.

## Considered Options

- **ent interceptor and hook (the entgo.io soft-delete example, theopenlane, eidng8/go-ent)**:
  hides rows from every query and turns `Delete` into an update. Rejected: it is global and
  implicit, and a physical delete needs a context flag. Rails `discard` (explicit scopes, no
  `default_scope`) and GORM's `Unscoped` show both ends; ours hides only in the generated scoped
  client and only for the mixin, and makes a hard delete a different method.
- **A `status` value `archived`**: rejected, a sent Broadcast must stay `sent` while discarded.
- **Auto-delete of old archives**: rejected, it adds a third way to lose history beside Erasure
  and Workspace deletion.
- **Block editing steps while an Automation is active**: rejected, it does not fix an Enrollment
  that stands on a step the edit moves, and it stops the owner fixing a live Automation.

## Consequences

- The migrations are destructive and follow ADR 0033 and 0047: renaming `deleted_at` and dropping
  `automations.definition` wait for a breaking change with an `Upgrade:` line and a guard that the
  data has moved. `automation_runs.version_id` is added nullable, version 1 of every Automation and
  its runs are backfilled by a river job, and the constraint is added `NOT VALID`.
