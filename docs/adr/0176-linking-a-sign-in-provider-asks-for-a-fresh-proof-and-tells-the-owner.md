---
status: accepted
---

# Linking a Sign-in provider asks for a fresh proof and tells the owner

> Amends [[0048-sign-in-providers-prove-an-email-and-replace-only-the-password]] (linking while
> signed in).

## Context

A signed-in User links another Sign-in provider account from the account security page (ADR 0048):
the link step drew a flow naming the User, and the callback added the provider's subject to that
User while the browser still held a current session of it. A live session was all it asked for, and
the link was recorded as `user.identity_link` and mailed nobody.

Every other step that adds or removes a way to sign in asks more. Adding a Passkey asks for a fresh
proof of identity (the password, or for a User without one a Sign-in provider login or a Passkey
check of the last few minutes, ADR 0052) and mails the owner. A link the callback makes at sign-in,
to a Verified User holding the provider's trusted email, mails the owner the Identity linked notice,
whose "this was not me" link unlinks the identity and ends every session.

A Linked identity outlives what ends a session: a password change and signing out everywhere end
the sessions and keep the Linked identities, as they keep the Passkeys. So whoever held a session of
the User, for as long as it lived, could add a provider account of their own and keep signing in with
it afterwards, and the owner was never told.

## Decisions

- **An explicit link asks for the same fresh proof as adding a Passkey.** The link step is a POST
  that carries the proof (`/site/me/identities/link`, the provider in its body) and checks it before
  it draws the flow: the password of a User who has one, or a Sign-in provider login or Passkey check
  inside the freshness window of the session (`users.IdentityProof`) for a User who has none. A proof
  that does not hold is refused as any step that asks for the password refuses it (403, a wrong
  password feeding the Login throttle), and no flow is drawn. The step answers the provider's
  authorization URL and the flow cookie; the browser follows the URL. The signup module owns the
  step (`signup.Module.StartLink`), beside the callback that completes it.
- **Every link tells the owner.** A link that adds a Linked identity publishes `IdentityLinked` in its
  transaction, whether the callback made it at sign-in or the User asked for it, so the owner is
  mailed the Identity linked notice with its "this was not me" link. The notice says a provider
  account was linked and can now be used to sign in, which holds for both: an explicit link may add an
  account whose address is not the User's. A link of a provider account the User already holds adds
  nothing and mails nothing.
- **A password change and signing out everywhere keep Linked identities**, as they keep Passkeys:
  they end sessions, not ways to sign in.

One rule now holds for every new way to sign in instead of two: it needs a fresh proof and it tells
the owner, as OWASP ASVS asks of a change to authentication factors (re-authenticate before it,
notify after it).

## Considered options

- **The notice alone.** Rejected: the mail may be read by whoever holds the session, and a notice
  only reports the link after it is made. The proof addresses the cause.

ADR 0048 rejected a "confirm with your password to link" step for the link a sign-in makes: a person
who signed up with a provider has no password, and one who forgot theirs is the one who needs the
provider. Neither reaches an explicit link: a User without a password proves a fresh provider login
or Passkey check instead, and one who forgot theirs resets it or signs in with a provider they hold.
The link a sign-in makes still asks no password.

## Consequences

- The account security page asks for the password before it sends the browser to the provider, or,
  for a User with no password whose proof is stale, offers the fresh Passkey check or provider
  sign-in first, as the Passkey section does.
- The "this was not me" link of an explicit link removes that identity and ends every session of the
  User. Unlike an unlink it is not refused when the identity is the last way in: an explicit link
  is made from a session, so the User had another way in when it was made.
- A link flow of a provider account the User already holds still counts as a fresh provider login
  (ADR 0048), but it can only start on a fresh proof, so it extends one rather than creating it.
