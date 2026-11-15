---
status: accepted
---

# An Invitation link verifies an address but proves nothing

> Amends [[0030-email-verification-gates-access]] (what the Invitation link stands for) and
> [[0051-a-workspace-admits-users-by-a-verified-join-domain]] (the Invitation accept is no longer a
> proof point).

## Context

ADR 0030 lets the holder of an Invitation link set up the account of the invited address: the link
"proves the address", so accepting it creates a Verified User, or completes an Unverified one, with
the name and password the holder gives. ADR 0051 then counts that acceptance among the moments an
address becomes a Proven email, the one event that makes a Domain join.

The Invitation's token is not only mailed to the address. The inviter gets the same link back to
copy (`SiteInvitationsCreate` answers `inviteUrl`), so that an Instance without outbound mail can
still invite. Holding the link therefore shows that someone was invited, not that they read the
mailbox. Any Owner or Admin of any Workspace (sign-up is always open, ADR 0063, so anyone can be
one) could invite an address on another Workspace's verified Join domain, accept the link
themselves, and be joined to that Workspace as a member without controlling any mailbox on its
domain.

## Decisions

- **Accepting an Invitation link stamps the Verified email, never the Proven email.** A User the
  link creates or completes is Verified (ADR 0030: it can sign in and holds the Membership the
  Invitation names), and its address stays unproven until a link only the address receives is
  followed: the verify link, a reset link, a confirmed email change, or a Sign-in provider's proof
  (ADR 0048). A Verified User who accepts with their own session proves nothing either.
- **The Invitation accept is not a Domain join step.** It makes the Membership the Invitation names
  and nothing more. A person invited to Workspace B whose address is on Workspace A's Join domain
  joins A when they first prove the address, as any other User does.
- **The copy link stays.** Keeping the token from the inviter would take away the only way to invite
  on an Instance that sends no mail; what changes is what the token is taken to show.

## Consequences

- ADR 0051's accepted cost, "an Invitation to Workspace B for a not-yet-proven address on Workspace
  A's domain also joins that person to A", is gone: that person joins A only by a later proof.
- An invited User who set their password from the link is unproven, like a Sign-up on an Instance
  with verification off. Their first proof follows ADR 0048: a provider sign-in that links the
  address clears that password, so whoever proves the address is the one who controls it.
- The inviter can still set up an account for an address no Verified User holds, which is the copy
  link's purpose, including completing that address's pending Sign-up (ADR 0030). The account holds
  only the inviting Workspace's Membership and no proof, and the address's owner takes it back by a
  reset link, which proves the address and sets a new password.
