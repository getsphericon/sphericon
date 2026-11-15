---
status: accepted
---

# The first proof of an address drops what was set up before it

> Amends [[0048-sign-in-providers-prove-an-email-and-replace-only-the-password]] (what the first
> proof clears) and [[0052-a-passkey-replaces-the-password-and-the-second-factor]] (Passkeys set up
> before the proof go with it).

## Context

A User can hold an address nobody has proven: a Sign-up on an Instance with verification off is
Verified at once (ADR 0063), and so is an account an Invitation link set up (ADR 0158). Whoever set
the password could then add every other way in that the password unlocks: a Passkey or a TOTP
Second factor (ADR 0052, ADR 0020), a Personal token or a Connection (ADR 0061), a Linked identity
of their own provider account.

ADR 0048 already distrusts what was set before the proof: when a provider first proves the address,
the same transaction clears the password, "so whoever proves the address is the one who controls
it". The password was the only thing it cleared. The other ways in stayed: the person who set them
kept signing in with their Passkey or provider account, kept calling `/api` with their token, and a
Second factor they enrolled stood between the address's owner and the account, before and after the
owner's first proof. A reset link, the other first proof of a Verified User's address, cleared
nothing at all but set the new password.

## Decisions

- **The first proof of a Verified User's address drops every credential set up before it.** At the
  moment `email_proven_at` is stamped from empty by a provider link (ADR 0048) or a reset link (ADR
  0030), in the transaction that stamps it: the password (the provider link; the reset link sets
  the new one), every Linked identity but the one the provider link adds, the TOTP Second factor and
  its Recovery codes, every Passkey, and every Personal token and Connection of the User in every
  Workspace. Every session ends, as both steps already end them.
- **Each owner clears its own.** `signup` runs both steps' transactions and, when the step reports a
  first proof, calls `secondfactors.ClearFactorAndPasskeysIn` (the factor, the codes and the
  Passkeys, as a reset does, with nothing to clear being no error) and `apitokens.RevokeOfUser` in each Workspace of the
  User; `users` drops the Linked identities with the password.
- **Only the first proof.** An address already proven keeps everything: a later reset link is a
  plain password change, and a provider link of a proven address adds an identity and drops
  nothing. The verify link of an Unverified User and a provider's take-over of one are first proofs
  too, but such a User could not sign in, so nothing was set up to drop. A confirmed email change
  stamps the new address from empty as well, but it proves an address the signed-in User chose for
  their own account, so it drops nothing.

## Consequences

- A User on an Instance with verification off, or one an Invitation set up, loses their Passkeys,
  Second factor, Personal tokens and Connections at their first reset link or provider link, and
  enrolls them again. Their scripts stop with a 401 until a new token is minted. This is the price
  of telling the address's owner apart from whoever typed the address first.
- A Workspace that requires a Second factor withholds such a User after the proof until they enroll
  again (ADR 0020), as after a reset.
- No Audit entry is added for the drop: the link or the reset that caused it is recorded already.
- The drop covers what is stored when the proof commits. A Personal token, Passkey or Linked
  identity whose own transaction commits during the proof's, or an OAuth code approved before the
  proof and exchanged in its five minutes afterwards, can outlive it: the first three only from a
  request already in flight when the proof ended its session, the code within its lifetime.
