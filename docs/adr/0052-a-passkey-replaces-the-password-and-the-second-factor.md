---
status: accepted
---

# A passkey replaces the password and the Second factor

A User may register **Passkeys** (WebAuthn, `go-webauthn/webauthn`) and log in with one without typing an
email or a password. Unlike a Sign-in provider ([[0048-sign-in-providers-prove-an-email-and-replace-only-the-password]]),
which replaces only the password step, a Passkey login requires user verification (biometric or PIN), so
possession and the person are both proven and the login skips the Second factor (ADR 0020). A Passkey also
meets a Workspace's Two-factor requirement. The requirement is met by how the session was obtained, not by
what the User has enrolled: the session token records its method, a Passkey login or a TOTP or Recovery
code step is strong, and a password-only or provider-only session of a User without a Second factor falls
under the grace period and is then withheld. Otherwise the password login that stays would bypass the
requirement for a User who holds a Passkey. A credential the authenticator cannot verify the user for is
refused.

## Decisions

- **Usernameless.** Credentials are discoverable; `/login` offers a Passkey button and conditional UI. The
  User is found by credential id, never by email, and the Login throttle (ADR 0025) counts against that User.
- **Several per User**, each named, with its last use. Adding and removing one passes `Prove` (the password
  a fresh provider login (ADR 0048) or a fresh Passkey check), bumps `session_epoch` and mails the owner.
  There is no Audit entry: a Passkey belongs to the User and an Audit entry to a Workspace (ADR 0022), as
  for the self-service Second factor. A User with no password and no Linked identity cannot remove the
  last Passkey.
- **A fresh Passkey check is the proof of a User with no password.** `Prove` accepts it beside the
  provider login of ADR 0048, and the same way: the session token carries the time of the check
  (a Passkey login sets it, and so does a step-up ceremony of its own, `POST /me/passkeys/check`),
  it holds for five minutes and a reissue keeps the original time. The step-up is a Passkey
  assertion of the session's own User, with user verification, under a signed ceremony bound to
  that User and their session epoch, and it spends its challenge like a login. It is not a login:
  it takes no part in the Login throttle and issues no new session method. A User with a password
  still proves the password.
- **Recovery is the password reset.** There are no Recovery codes for Passkeys. The Second factor reset
  (an Owner or Admin) also removes the User's Passkeys.
- **Ceremony state.** The challenge travels in a signed cookie and `begin` writes nothing (it is anonymous
  and would let anyone fill a table). Single use comes from a unique row of the spent challenge, inserted
  at `finish` and pruned by the daily cleanup, because a usernameless login has no User yet to bind a
  stateless challenge to. An assertion with an unknown credential id gets the one generic failure and no
  counter: without the private key a guess cannot succeed, and there is no User to throttle.
- **Ownership.** Passkeys and spent challenges belong to the User and live in `internal/secondfactor`, which
  already holds the raw client for the User's Second factor. The login runs through `internal/login` and
  `auth.Sessions`, so the verification gate, the throttle and suspension apply as before.
- **Scope.** `/site` only; the Staff console keeps password plus TOTP (ADR 0026). Registration is unchanged:
  a Passkey is added after the account exists. The relying party id is the host of `SPHERICON_APP_URL`, with
  no variable of its own.

## Considered options

- **Passkey as a password replacement only (the provider shape)**: rejected, it asks a User who just proved
  device and person to also type a TOTP code, which is what makes passkeys unattractive.
- **Passkey as a second factor beside the password**: rejected, it keeps the phishable password in the
  login.
- **A stateless signed cookie alone for the ceremony**: rejected, it leaves a replay window the
  specification forbids; **a row written at `begin`**: rejected, anonymous writes.
- **Only a Second factor meets the requirement**: rejected, it undoes the point of a Passkey; **a Passkey
  step after the password**: rejected, a password reset would strand the User at a challenge they cannot pass.

## Consequences

- Changing the host of an Instance invalidates every Passkey; the Instance configuration docs say so.
- The session token carries its sign-in method, so the Two-factor check reads the session as well as the
  User.
- An attacker who controls the mailbox can still reset the password and log in with it; a
  "passkey only" mode needs its own recovery rules and is a later decision.
