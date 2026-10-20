---
status: accepted
---

# Sign-in providers prove an email and replace only the password

A person who wants to try sphericon should not have to invent a password and wait for a verify
link. The open core lets a visitor register or log in with a **Sign-in provider** (Google first,
GitHub next) from `/register` and `/login`. The provider's proof of an address counts as proof
(a Verified email), and the provider replaces only the password step: an enrolled Second factor,
the Login throttle, a Workspace's Two-factor requirement and the registration gates keep working
as they do for email and password (ADR 0020, ADR 0030, ADR 0042). Per-Workspace federation
(SAML or OIDC of an organization, SCIM) stays Enterprise (ADR 0014); "SSO" is on the GLOSSARY's
Avoid list for this concept.

## Decisions

- **Libraries.** `golang.org/x/oauth2` for the flow and `coreos/go-oidc` for verifying an
  id_token. Google is OIDC; GitHub is plain OAuth2 and reads its primary verified email from its
  emails endpoint. One session issuer stays (`auth.Sessions`, ADR 0020): a library that mints
  sessions would be a second path around the Second factor.
- **The registry is data.** `internal/signin` has one entry per provider: name, endpoints,
  scopes, how the profile is read and how far its email is trusted. A provider is live only when
  `SPHERICON_SIGNIN_<PROVIDER>_CLIENT_ID` and `_CLIENT_SECRET` are both set; one without the other
  stops the boot with a message naming the missing variable, neither leaves the provider off. The
  redirect URI derives from `SPHERICON_APP_URL` (`<app url>/site/auth/signin/<provider>/callback`).
  The variables avoid the `OAUTH` prefix, which the OAuth server for MCP clients uses (ADR 0016).
- **Linked identity.** `user_identities` holds provider, the provider's immutable subject and the
  User, unique on provider and subject. A sign-in finds the User by that pair, never by email.
- **`email_proven_at`.** A nullable `users` column set when an address is proven (a provider
  sign-in, a verify link, a reset link (mailed to the address too), an accepted Invitation and a
  confirmed email change). It differs from `email_verified_at`, which an install that turned verification off sets
  at registration with no proof, and it is what keeps silent linking by email from being a
  pre-registration takeover.
- **One decision.** `accounts.DecideSignin` is a pure function of the facts (known subject, trust
  of the email, the User holding the address, whether sign-up is open) with the outcomes known,
  link, take over, create and refuse. Each branch is tested on its own, and the apply step is
  separate.
- **Linking to an existing address.** A trusted email held by a Verified User links the identity
  silently and signs that User in; the visitor's response is the same as for a new address. When
  the holder's `email_proven_at` is empty, the same transaction also clears its password hash, so
  a pre-registration's password dies with the link. Every silent link bumps `session_epoch` (the
  other sessions end; the session the sign-in starts carries the new epoch). Every silent link mails the
  owner (the Taken address pattern: a system mail, never a reply to the visitor) a notice whose
  "this was not me" link unlinks that identity and bumps `session_epoch`. The link is a signed
  token naming the identity, with no binding: it works once, because a link whose identity is gone
  answers 400 and changes nothing.
- **Taking over a registration.** A trusted email held by an Unverified User makes that User
  Verified through the provider's proof: its pending password is cleared, its sessions end, it
  takes the provider's name, gets the identity and its first Workspace, and a pending Invitation to
  the address stays. No notice is mailed: the person signing in is the address's owner.
- **Trust of the email.** GitHub: the primary email with `verified = true`. Google:
  `email_verified` is accepted only for a Gmail address or when the `hd` claim is present (Google
  is then authoritative for the address); any other Google address is refused, because Google's own
  guidance treats the claim as weak evidence for addresses it does not own.
- **Flow state** (`state`, PKCE verifier, `nonce`, the validated `return_to`) lives in a signed,
  httpOnly, SameSite=Lax cookie scoped to `/site/auth/signin` and valid for ten minutes. It is Lax,
  not Strict like the Registration browser's, because the provider sends the browser back with a
  cross-site redirect. `return_to` is honoured only as a relative in-app path.
- **A callback always redirects.** To the page the flow began from with the session cookie, or to
  `/login?error=<code>` (the `SiteSigninError` enum of the `/site` contract), which the SPA renders
  through i18n. A person who denies consent, a bad `state` and a provider error all end there.
- **A created User has a Verified email, no password, its first Workspace and no mail.** The
  provider proved the address, so there is no link to send; a password is set later through the
  reset flow.
- **A User with a Second factor gets the `login_challenge` of ADR 0020 from the callback**, not
  a session: the callback redirects to `/login?challenge=<token>` (plus `redirect` when the flow
  had a `return_to`), and the login page finishes it through the same second step, so wrong
  codes feed the Login throttle exactly as after a password. The provider's own MFA does not
  count, and a Workspace's Two-factor requirement is enforced as for any session.

- **Gates and Invitations.** A provider sign-up of a new address obeys the registration gate
  (`SPHERICON_SIGNUP_OPEN`, ADR 0042: `signup_closed`); a Linked identity or a matching Verified
  User still signs in. The Instance has no other registration gate, so provider sign-up has
  none either. A Workspace may admit by domain (ADR 0051) once an address is proven, but that
  never opens or closes registration: it adds a Membership, not a User. An Invitation link carries its token into the flow
  (`start?invitation=`, kept in the flow cookie): the provider's trusted email must equal the
  invited address (case-insensitive) or the callback refuses with `invitation_email_mismatch`,
  before anything is created, and lands back on the Invitation page. A match opens a closed
  sign-up for that address (as accepting by password does), signs the person in as in any
  provider sign-in (link, take over or create) and accepts the Invitation through
  `AcceptInvitation`, the provider's proof standing in for the session it asks of a Verified
  User: the Membership gets the invited Role, beside the first Workspace a new User gets. A
  Linked identity of an account whose address is another one is refused the same way. An
  Invitation that is not pending is `invitation_invalid`.
- **Linking and unlinking while signed in.** `/site/me/identities/{provider}/link` runs the same
  flow with the User's id in the signed flow cookie; the shared callback then adds the subject to
  that User (whatever email the provider reports) only when the browser still holds a current
  session of that User, and refuses a subject that belongs to another User (`identity_in_use`).
  Unlinking is refused (409) when the User would have no password and no other identity. Both
  bump `session_epoch` and reissue the acting session.
  The reissued session keeps the time of its provider login when the link added a new subject: a
  stolen session could otherwise add its own provider account and pass the fresh-login proof
  below. Only a link flow whose subject was already one of the User's identities is a fresh
  provider login.

- **A User with no password proves a fresh provider login instead of the password.** The steps
  that ask for the password (enrolling or confirming a Second factor, regenerating Recovery codes,
  disabling it) accept, for a User without one, a Sign-in provider login of the last five minutes.
  The proof is the time of that login, a claim of the session token itself (the challenge carries
  it across the second step, and a reissued session keeps the original time): stateless, no
  session table, ADR 0020's spirit. A User with a password still proves the password, whatever the
  provider login; a missing proof answers 403 and the SPA offers the provider buttons. A password
  can be set later through the reset flow.

## Considered options

- **go-pkgz providers.** Rejected: ADR 0020 keeps one session issuer, and a library that issues
  its own sessions is a second path around the Second factor.
- **Linking by email without verification.** Rejected: an unproven address would let anyone who
  controls a provider account take over the account registered with that address.
- **A "confirm with your password to link" step.** Rejected: a person who signed up with a
  provider has no password, and a person who forgot theirs is the one who needs the provider.
- **Treating the provider's own multi-factor sign-in as a Second factor.** Rejected: the Instance
  cannot tell whether the provider asked for it, and a Workspace's Two-factor requirement is about
  our own factor.

## Consequences

- No provider token or raw subject is logged or written to an Audit entry. A provider login is
  `user.login` with the provider as the method (`password` for a password login), and link and unlink
  are `user.identity_link` and `user.identity_unlink` naming the provider, each recorded in the log of
  every Workspace the User belongs to.
- The callback needs the session cookie to be SameSite=Lax, which it already is.
- Adding a provider is one registry entry and the two variables its credentials are read from.
