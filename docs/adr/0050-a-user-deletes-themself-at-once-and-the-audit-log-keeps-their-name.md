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
