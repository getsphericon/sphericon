---
status: accepted
---

# Enforced session expiry, session epoch and two-step login on go-pkgz

Site sessions are stateless JWT cookies. go-pkgz's `Parse` ignores an expired `exp` (it expects its
own middleware to refresh), and the site security handler only calls `Parse`, so the 1h token TTL
was never enforced: a stolen cookie stayed valid until the signing secret changed. We keep go-pkgz
for what it does well (JWT signing and parsing) and add: (1) `exp` enforced in the site security
handler, with one `SPHERICON_SESSION_TTL` (default 24h) for token and cookie and no refresh; (2) a
`session_epoch` on the User, written into the token when `auth.Sessions.Issue` mints it and checked
on every request; a password change or reset, an email change, enrolling or disabling a Second
factor, regenerating Recovery codes, a Second factor reset and "sign out everywhere" bump it
(tokens without an epoch are rejected once); (3) a two-step login:
the direct provider cannot see the request (no IP) or return a "second factor required" outcome,
so a thin handler replaces it, checks the password, answers with a short-lived single-use
challenge, and `/site/auth/second-factor` verifies a TOTP (`pquerna/otp`) or Recovery code before
issuing the cookie. The handler is the `login` operation of the `/site` contract; an ogen handler
has no response writer, so the cookie comes from one issuer (`auth.Sessions`, a go-pkgz token
service) as the operation's declared `Set-Cookie` header, not through `TokenService().Set`. `/auth/` is no longer mounted, so no second path
mints a session around the Second factor. Failed password, Second factor and Recovery code
attempts all feed the existing per-account counter of ADR 0025 (rate limiting): the Login throttle
(GLOSSARY) is that mechanism, not a new one. So do the password checks a signed-in User passes to
enroll, disable or regenerate Recovery codes (enrolling needs the password too, so a hijacked
session cannot enroll a factor of its own and sign the real User out elsewhere).

## Considered options

- **Hard lock until an admin unlocks (Cal.com, Mailchimp's 24h lock)**: rejected, it lets an
  attacker lock a victim out by failing on purpose.
- **A server-side session table**: rejected, it adds a store lookup and cleanup for a need the
  epoch covers; the User row is already read on every `/w/{slug}` request.
- **Redis or in-memory counters**: rejected, a second datastore for self-hosters, and in-memory
  state is lost across restarts and instances.

## Consequences

- Revocation is all-or-nothing per User; per-device revocation would need a session table later.
- A bump logs out the acting User's other sessions too; the acting session is reissued.
- The challenge is an `authtoken` JWT (purpose `login_challenge`, 5 minutes) whose signing key is
  derived from the User's session epoch, password hash, last accepted TOTP step and count of unused
  Recovery codes. Every successful second step moves one of them, so the challenge works once
  without a store of spent challenges and holds across instances. The second step re-checks that
  binding inside its transaction with the User row locked (`SELECT … FOR UPDATE`), so two steps
  racing on one challenge (one TOTP, one Recovery code) cannot both start a session. Only the second step resets the
  Login throttle counter: resetting it on the password step would let a password holder guess codes
  without end.
- SMS and trusted-device ("remember me") factors are deliberately absent.
