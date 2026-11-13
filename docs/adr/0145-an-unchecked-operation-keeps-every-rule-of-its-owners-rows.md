---
status: accepted
---

# An unchecked operation keeps every rule of its owner's rows

> Refines [[0110-an-unchecked-operation-lives-where-only-a-use-case-can-import-it]]: it said an
> unchecked write "still checks its own input; only the rule across modules is the use case's". This
> says what its own input covers, and how the use case runs its rule on the row the write replaces.

## Context

The four writes behind the Removal check left rules of their owner's own rows to the use cases
that call them:

- `contactsunchecked.UpdateCustomFieldIn` checked the new name but not the revision (ADR 0119). The
  revision rule was `contacts.CustomFieldUpdateInput.Check`, exported for `customfieldretyping`
  alone, and whether an edit retypes the field was computed both there and in the write.
- "Deleting a deleted Integration or Sending domain changes nothing" lived in `integrationdeletion`
  and `sendingdomaindeletion` (`row.DiscardedAt != nil`), while `integrationsunchecked.DeleteIn`
  answered a deleted Integration with `integrations.ErrNotFound`.
- Releasing a deleted Telegram bot's webhook was `integrations.Module.ReleaseDeleted`, a duty of the
  owner after the commit, exported for `integrationdeletion` alone, which had to remember to call it.
  A crash between the commit and the call left the bot's webhook in place.
- Each use case read the subject through its owner (`tags.Get`, `contacts.CustomField`,
  `integrations.Module.Get`, `sendingdomains.Module.Get`) only to hand a field to the Removal check.

## Decisions

- **An unchecked operation keeps every rule of its owner's rows**: its input, the revision the
  edit was made on, the no-op of a repeated removal, and the owner's work after the commit. Only the
  rule across modules is the use case's.
- **Where that rule needs fields of the row between the read and the write, the operation is
  opened on its row.** The owner's subpackage opens it under the lock it is given and reads the row
  (`tagsunchecked.DeletionIn`, `integrationsunchecked.DeletionIn`,
  `sendingdomainsunchecked.DeletionIn`, `contactsunchecked.CustomFieldUpdateIn`). Opening refuses
  what the row's own rules refuse (an unknown id, a refused name, a stale revision) and reports a
  no-op (`Done`). The operation exposes what the Removal check reads (`Name`, `Domain`, `Key` with
  `Retype`), the use case runs that check, and `Apply` writes, taking the time and an
  `events.Enqueuer` where it needs them. `Apply` keeps the owner's rules itself: applied to a
  deletion that is `Done`, it writes and queues nothing.
- **The owner's work after the commit is a job queued in the same transaction.** A deletion of a
  Telegram Integration that held its webhook (enabled) queues `integrations.ReleaseWebhookArgs` in
  the discard's transaction (`WithinWorkspaceLockEnqueueTx`), so the release runs iff the discard
  commits, and survives a crash after it. Its worker is in `internal/jobs`, as the quota refresh's
  is, and calls `integrations.Module.ReleaseWebhook`, which releases only a deleted Telegram
  Integration. The release stays best effort, as on disabling a bot: a refusal of Telegram is
  logged and the job ends, since a revoked token never succeeds and a retry later could take the
  webhook of a bot connected again meanwhile. The release in `integrations.Module.Update` (a bot
  disabled or given a new token) stays where it is, after its own commit.
- **The four use cases stay four** (`tagdeletion`, `customfieldretyping`, `integrationdeletion`,
  `sendingdomaindeletion`), each reduced to lock, open, Removal check, apply:
  - a fold into one has no name in the glossary: a retype is no removal;
  - its four methods would share nothing but the lock;
  - each passes the deletion test, since a driver calls one operation of the core (ADR 0111).
- `DeleteIn` of the three removals, `UpdateCustomFieldIn`, `CustomFieldUpdateInput.Check` and
  `Retype`, and `integrations.Module.ReleaseDeleted` are gone. The use cases no longer take the
  Integrations or the Sending domains module.

## Considered options

- **Keep the write a function and move the checks into it.** The use case would still read the row
  through the owner to hand the Removal check its name or domain, and the write would read it again;
  the row checked and the row written would be two reads, joined only by the lock.
- **Release the webhook in the use case after the commit, through an exported method** (as before).
  Every caller of the removal has to remember it, and a crash after the commit loses it.

## Consequences

- The owner's rules are tested in each `<module>unchecked` package's own tests on Postgres, and the
  release job's worker in `internal/jobs`. The use cases keep their lock-race tests.
- A test that deletes a Telegram Integration and asserts the release works the queued job first
  (`env.PerformEnqueuedJobs`).
- A new unchecked operation whose use case's rule reads its row is opened on the row the same way.
