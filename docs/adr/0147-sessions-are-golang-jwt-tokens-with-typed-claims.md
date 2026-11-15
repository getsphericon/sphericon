---
status: accepted
---

# Sessions are golang-jwt tokens with typed claims

> Amends [[0020-session-expiry-epoch-and-two-step-login]] (the library a session is signed and
> parsed with). Its session rules stand: an expiry that is enforced, the session epoch, the
> two-step login and one issuer of the cookie, `auth.Sessions`.

## Context

ADR 0020 kept go-pkgz/auth for signing and parsing the session JWT. Of that library the app used
only its token service: not its middleware, its refresh or its providers, which ADR 0020 and ADR
0048 rule out. Its `Parse` skips the validation of the registered claims, so each of the four
places that read a session (the site security handler, `auth.Sessions.Holder` and `SessionOf`, the
Staff member's `Sessions.Verify`) checked `exp` against the Instance clock by hand, and the Staff
member's checked the issuer and the audience by hand too. The User id, the session epoch and the
times of a provider login and of a Passkey check rode its user attributes as decimal strings, parsed
back at every read.

golang-jwt v5, which go-pkgz wraps, is already a direct dependency: the account flow tokens
(`internal/kernel/authtoken`), the sign-in flow cookie and the Passkey ceremony cookie are its
tokens with typed claims. Its parser validates the registered claims itself, on a clock it is
given (`WithTimeFunc`).

## Decisions

- **A session is a golang-jwt token with typed claims.** The site's claims are
  `auth.SessionClaims`: the registered claims, the User id, the session epoch, the Method, and the
  times of the provider login and of the Passkey check as `jwt.NumericDate`. The Staff member's
  carry its id and epoch. The email no longer rides the token: nothing read it.
- **The parser enforces the registered claims.** Each surface builds one parser: HS256 only, its
  issuer and audience, an expiry that is required (`WithExpirationRequired`) and checked on the
  Instance clock (`WithTimeFunc(app.Clock)`), with no leeway. No code reads `exp` itself.
- **The session's own claims are validated by the parser too.** The claims implement
  `jwt.ClaimsValidator`: a token without a User id, an epoch or a known Method is refused there, so
  every reader of a session refuses the same tokens.
- **A JWT secret rotation is a key set.** The site parser verifies against
  `jwt.VerificationKeySet`, the primary secret and then each secondary one, instead of a parser per
  secret.
- **One constructor of the claims.** `auth.NewSessionClaims` is what `auth.Sessions.Issue` signs and
  what the test harness mints its sessions from, so the harness cannot drift from a login.
- **A purpose key is HKDF.** The keys of the Passkey ceremony and of the sign-in flow cookie are
  `crypto/hkdf` of the instance secret with the purpose as its info, as `authtoken` derives its
  keys, not an HMAC of the label.

## Considered options

- **Keep go-pkgz/auth.** Rejected: of a library built around its middleware and providers the app
  used one service, and worked around it at every read of a session.
- **A session helper of our own in the kernel.** Rejected: two surfaces, each a few lines of the
  library's own API; a helper would be a wrapper again.

## Consequences

- A cookie signed before this change no longer verifies: every User and Staff member signs in once
  more. A Passkey ceremony or a sign-in flow in flight at the deploy fails and is started again.
- A site session of another issuer or audience is refused, as a Staff member's already was.
