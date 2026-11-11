---
status: accepted
---

# A User deletes themself at once, and the Audit log keeps their name

A User could not leave a Workspace or delete themself: only an Owner removed a Membership, and the
only deleted Users were Unverified ones. A person has to be able to take themself out of the
system, in the core as well as in a hosted install (ADR 0014), without the core learning anything
about a hosted offering.

## Decisions

- **Leaving and User deletion are two actions.** Leaving removes the caller's own Membership in one
  Workspace; the last Owner is refused with the same `ErrLastOwner` rule as any Owner change
  (ADR 0004), and the removal runs under the same lock on the Workspace's Owner rows.
- **User deletion is refused while the User is the only Owner of any Workspace**, and the refusal
  lists those Workspaces. Nothing is deleted on the User's behalf: handing the Workspace over or
  deleting it (ADR 0043) is a decision of its own, taken first.
- **User deletion is immediate and irreversible, with no window.** Workspace deletion has 30 days
  because a Workspace holds other people's data and the Owner may regret it; a User holds only
  their own access. A window would need a lifecycle table (ADR 0046), a "deleted but still a Taken
  address" state and a rule for a registration on that address. The guard against a slip is the
  confirmation: the password (and the Second factor if enrolled) plus the User's own email, or,
  for a User with no password, a one-time link to the Verified email plus the email typed.
- **Audit entries keep the actor id and the name snapshot.** The Audit log is immutable
  (ADR 0022) and exists to answer who changed what; rewriting it to blank a name would break both.
  This is the one place a deleted User's name survives, and the GLOSSARY says so.
- **Nothing keeps a person's address or client past need.** An Audit entry names a person by the
  name they gave: a User or Membership target by the User's name, an Invitation target by nothing,
  since its only name is the address (`events.AuditEntry.TargetName`). Acceptance clears an
  Invitation's address with the `accepted_at` latch, and a CHECK keeps an accepted row without one:
  the Membership and the Audit entry are what remain of the offer. An entry's client address and user
  agent are cleared once older than 90 days (`auditlogretention.ClientWindow`), licensed or not and
  whatever the Workspace's retention: the one change an entry takes after it is written. Before
  this, the address survived a deleted User in three Audit actions and every accepted Invitation, and
  the client details stayed for ever in a Workspace with no retention window; the migration that
  brought the rule cleared the addresses those entries and Invitations already held.
- **A pending Invitation outlives its sender.** `invited_by` is cleared, and any Owner can still
  revoke it.
- **The scope ends at the User.** Contacts keep their own Erasure (ADR 0021) and Staff members are
  a separate identity (ADR 0008).

## Considered Options

- **A grace period like Workspace deletion**: forgiving, but costs the state and the registration
  rule above for a user who has already proven their password.
- **Delete a sole-Owner Workspace together with the User**: one click, but an irreversible removal
  of a Workspace and its Contacts' data as a side effect of leaving.
- **Blank the name in Audit entries**: tidier for privacy, but the immutable log would have to be
  rewritten and "who did this" would become unreadable.
- **Keep the address in Invitation entries**: an Owner reads who was invited at a glance, but the
  address then outlives the person it names; the pending Invitation still shows it while it is
  pending.
- **Clear the client details only when the User is deleted**: they are no less personal for a User
  who stays, and a sign-in older than 90 days is past looking into.
