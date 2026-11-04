---
status: accepted
---

# Archive is not Discard

ADR 0049 made Discard the one mechanism behind both what the UI calls Archive (a Broadcast, an
Automation, a Segment) and Delete (an Integration, a Sending domain, ADR 0097; a Rule, ADR 0098; a
Workspace), and purged a row instead when it had no history yet: a draft Broadcast, an Automation
with no Enrollments, a Rule with no Firings. The two are different things. Archive puts aside work a
person comes back to; Delete removes something for good, and keeps the row only because other rows
refer to it. One column for both left a Sign-up item "archived for good", a contract that called the
same state `deletedAt` on one resource and `discardedAt` on another, and a removal whose outcome
depended on the row's past.

## Decisions

- **Each kind has one way out, the same for every row of it.**
  - _Archived, never deleted_: a Broadcast (a draft too), an Automation, a Segment. Archive takes
    the row out of the working lists and new work, an archived view lists it, Unarchive brings it
    back. The column is `archived_at`; these kinds have no Delete and no `discarded_at`.
  - _Discarded_: an Integration, a Sending domain, a Rule, a Sign-up item, a Workspace. Delete always
    discards: the row leaves every list and every new work, is still read by id so what refers to it
    shows its name, and is never restored, except a Workspace within its window (ADR 0043). There is
    no purge for a row that happens to have no history yet.
  - _Deleted outright_: a kind nothing refers to (Tag, Template, Brand kit, Webhook endpoint,
    Membership, Invitation), as ADR 0049 already had it.
  - Facts (Event, Outbound message, Audit entry, Suppression) stay as ADR 0049 has them. Only
    Retention and erasure physically remove an archived or discarded row.
- **An archived or a discarded row is read-only.** Edits, Publish, Revert, Deactivate and the
  like answer 404 for a discarded row (as ADR 0097 and ADR 0098 have it for an Integration and a
  Rule) and 409 for an archived one until it is unarchived. Provider hooks, tracking and other
  history still reach both.
- **Live use blocks both** (ADR 0099). A draft is not Live use: archiving a Segment that only a
  draft Broadcast names is allowed, and scheduling that draft is then refused.
- **The contract names the state as the owner does.** An archivable resource has `archivedAt`, the
  `archived` list filter and `archive`/`unarchive`. A discardable one has `DELETE` and
  `discardedAt`, following the `discard` gem's vocabulary: an Integration's, a Sending domain's and
  a Rule's `deletedAt` and a Sign-up item's `archivedAt` become `discardedAt`, and the Workspace
  keeps it. A User's `deletedAt` stays: a User is removed at once (ADR 0050).
- **The scoped client follows the kinds.** An entity with the archive column gets `Archive`,
  `Unarchive` and no `Delete`; one with the discard column gets `Discard` and no `Delete` or
  `Undiscard`, the Workspace's window keeping its own lifecycle (ADR 0046). `Query()` hides archived
  and discarded rows; the readers ADR 0049 and ADR 0099 name see them.

## Considered Options

- **One mechanism for both** (ADR 0049): it cannot say that an Automation is never deleted while an
  Integration is never restored, and it named one state three ways in the contract.
- **Archive and Discard on the same row**, so an archived sent Broadcast could also be deleted: two
  states for what a person sees as one, for kinds whose rows are few.
- **Purge a row without history** (a draft, an Automation never enrolled, a Rule never fired): the
  outcome of the same action then depends on the row's past, and an Automation a Rule names can be
  purged from under it. The archive of these kinds stays small.
- **`deletedAt` in the contract** (ADR 0097): the owner keeps the `discard` vocabulary for the state,
  and Archive, the other state, has a name of its own.

## Consequences

- Broadcasts, Automations and Segments move from `discarded_at` to `archived_at`; the Broadcast's
  draft Delete, the Automation's Delete and the Rule's purge go away. Renaming the columns is a
  destructive migration and lands with a breaking commit (ADR 0033, ADR 0047).
- The renamed contract fields reach every client of `/site`, `/api` and the staff surface at once
  (the service feeds keep theirs); a consumer of a published contract picks them up at its next pinned tag.
