# No endpoint confirms that an address is taken

Whether an email address has a User must not be learnable by someone who does not own the
address. With verification gating access (ADR 0030) the proof of ownership is a mailed link, so
every endpoint that would answer "taken" answers the same whether or not it is, and the existing
owner is told by mail instead.

- **Registration** (gate on) answers 202 for a new, an Unverified and a Verified address. A new
  address gets its User and the verify link; an Unverified one gets a fresh verify link on its
  existing row (name untouched); a Verified one gets a tokenless notice ("someone tried to register
  with this address", links to sign in and to reset the password). Every request does the same
  work, as `forgot-password` does: it takes a slot of the address's budget (its own `Kind`), mints a
  token and queues a mail job that delivers only when a slot was granted and the mail is due, so
  neither the answer nor its timing says which case it was.
- **Email change** answers 202 on request. A taken target gets the same tokenless notice instead of
  a confirm link; the confirm step never says "in use" (the unique index remains only as a race
  guard and fails as an invalid link).
- **The invitation lookup** no longer returns `hasAccount`. The accept page offers both paths
  (sign in, create account) and the server enforces ADR 0030's rule: a Verified User's address is
  accepted only with that User's own session.
- **The accept step still refuses** a Verified address without that User's own session (401, "sign in")
  and a session of another account (403): the link holder can learn, one submit later, that the
  address has an account. That cannot be avoided, since joining a Verified User's address needs its
  session, and the holder of the link is the one invited.
- **Budgets:** the notice and the verify mail share the forgot-password per-address budget
  (`RATE_LIMIT_FORGOT_PASSWORD_PER_ADDRESS_PER_HOUR`), taken under a Kind of its own for
  registration and for email change, so a stranger cannot mail-bomb an owner through either.
- **The exception** is `REQUIRE_EMAIL_VERIFICATION=false`: the password arrives with the request and
  no mail is sent, so registration keeps answering 409. An operator who turns the gate off accepts
  that enumeration.

## Considered Options

- **Keep the 409 and rely on rate limits**: limits slow enumeration, they do not stop it; rejected.
- **Keep `hasAccount` because the lookup is behind a token**: the token holder is not always the
  invitee (a forwarded or leaked mail), and the page can branch without it; rejected.
- **Reveal "taken" only to an authenticated session on email change**: a session holder is not the
  owner of the target address; rejected for one invariant without exceptions.

## Consequences

- A mistyped address is discovered only when no mail arrives; the registration page offers resend
  and sign-in beside its "check your email" message.
- The 409 and `errors.email_exists` leave the register operation; `hasAccount` leaves the
  invitation lookup contract and its callers.
