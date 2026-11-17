---
status: accepted
---

# A Second factor reset needs the right in every Workspace of the member

> Amends [[0020-session-expiry-epoch-and-two-step-login]] (who may reset a member's Second factor,
> where the reset is recorded, and that the User is told).

## Context

The Second factor reset (ADR 0020) lets an Owner or Admin clear a member's Second factor so that
they can enroll again after losing both their authenticator and their Recovery codes. What it clears
belongs to the User, not to a Workspace: the factor, the Recovery codes and every Passkey (ADR 0052),
and the session epoch it bumps ends every session of the User in every Workspace.

The right to make it was read in one Workspace only: the caller's Role there, and owners:manage for
an Owner's. The Audit entry `user.second_factor_reset` was recorded in that Workspace alone, and the
User was not told. A User who belongs to several Workspaces is guarded in each of them by the same
factor, and a Workspace that requires one (the Two-factor requirement) relies on it. The Owner or
Admin of any one of them could remove it for all of them, and neither the User nor the other
Workspaces learned of it. The reset a Staff member makes from the command line
(`sphericon user reset-second-factor`) already recorded its entry in every Workspace of the User.

An account-wide credential belongs to the account, not to one tenant of it. GitHub likewise leaves
a member's two-factor authentication to the account rather than to an organization's owners.

## Decisions

- **A reset by an Owner or Admin needs the right in every Workspace of the member.** In each live
  Workspace the User holds a Membership in, the caller holds one too, whose Role passes the rule the
  reset already applied in one Workspace: second_factor:manage, and owners:manage for an Owner's. A
  Workspace in its deletion window does not count: nobody acts in it until it is restored. The rule
  is read in the reset's transaction. The refusals of the Workspace the reset is made in stand as they were: a Membership
  outside it is not found, a missing Permission is a 403, and the caller's own factor is a 422.
- **Otherwise the reset is refused on the member** (422 under `member`), in words that name no
  Workspace: the member also belongs to other Workspaces, so an Owner there or the administrator of
  the Instance resets it. The refusal tells the caller that the member belongs elsewhere, never
  where; we accept that.
- **Every reset is recorded in every Workspace of the User.** Whether an Owner or Admin or a Staff
  member made it, `user.second_factor_reset` is recorded by its actor in the log of each Workspace
  the User holds a Membership in. An Owner or Admin who made it is a member of each of them, by the
  first decision.
- **Every reset tells the User.** The reset publishes `user.second_factor_reset` (an account event,
  ADR 0105) in its transaction, and Notice delivery tells the User what is gone, that every session
  ended, that a Workspace which requires a Second factor asks them to enroll again, and to contact an
  administrator if they did not expect it. The mail carries no link. An Instance with no System
  sender sends no Notice, this one included.
- **The first proof of an address stays silent.** The drop of a factor and Passkeys at the first
  proof of an address (ADR 0170) is no reset: it is neither recorded nor mailed, as before.

## Consequences

- An Owner or Admin can no longer reset, alone, the Second factor of a member who also belongs to a
  Workspace they do not administer. An Owner there, or a Staff member with
  `sphericon user reset-second-factor`, makes the reset; the member page shows the refusal as it
  shows any refusal of the member.
- The Workspaces of the member see every reset in their Audit log, and the member learns of every
  reset from a mail, whoever made it, where the Instance sends mail.

## Rejected

- **Refusing the reset whenever the member holds another Membership**: it refuses the caller who
  administers every Workspace of the member, who may reset it as well as anyone.
- **Recording the reset everywhere and mailing the User, with the right read in one Workspace**:
  the other Workspaces and the User learn of it only after the factor that guarded them is gone.
