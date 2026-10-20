---
status: accepted
---

# Rate limiting: in-binary, in-memory flood limits, Postgres per-account counters

Public endpoints (login, password recovery, signup, invitation accept, consent, `/e/`,
`/collect/*`) and the authenticated `/api` and `/mcp` had no abuse protection. The limiter lives
in the Go binary, not only at the edge, because an open-core self-host may run without our
ingress. The edge may add its own limits on top.

## Decisions

- **Two stores, by what each protects.** Flood limits (per IP, per Workspace, tracking record guard) are
  in-memory sliding windows (`go-chi/httprate`): no new infrastructure, and with N replicas the
  effective limit is N× the configured one, which is acceptable for flooding. Failed-login and
  password-reset counters per account are an `auth_attempt` row in Postgres (in
  `internal/accounts`), because OWASP says the counter must follow the account, not the source
  IP, and it has to be exact across replicas. Redis was rejected: the stack is Postgres-only.
- **Login: exponential delay, no hard lockout.** After 5 failures in 15 minutes the delay doubles
  (1 s, 2 s, … capped at 15 min) and login answers 429 with `Retry-After` even for a correct
  password. A hard lockout would let anyone lock any account (a DoS); `forgot-password` always
  works. `auth_attempt` rows are also created for unknown emails so the table cannot enumerate
  accounts; a river job purges stale rows.
  The login module (`internal/login`, ADR 0020, used by `/site` and `/staff`) consults `auth_attempt` before it checks
  the password, answers 429 itself, and records the failure or success. The check and the count
  are two steps, so wrong passwords sent in parallel before the first failure is recorded all pass
  the delay check. The excess is bounded by the in-flight requests of one client (the per-IP login
  cap applies in front, and each attempt is a slow password hash), and the delay applies from the
  next request on. Closing it would mean claiming an attempt before the credential is checked;
  accepted.
- **Tracking never refuses a recipient.** Opens and clicks come mostly from mailbox-provider
  proxies (Gmail image proxy, Apple MPP), link scanners and RFC 8058 one-click POSTs, which share
  few IPs, so a per-IP 429 would break links and the ADR 0012 unsubscribe guarantee. A click
  always redirects and an open always returns the pixel; over the limit only the event recording
  is skipped. One-click unsubscribe is not rate limited at all: its signed token is the
  protection. Signup, invitation accept and consent confirm use a per-IP limit (60/min) and may
  answer 429.
- **`forgot-password` never reveals the account.** Over the per-address limit (3 mails/hour) the
  response is still a silent 202 with no mail sent; only the per-IP limit (10/hour) answers 429.
  The early return for an unknown email is removed so response time does not leak existence:
  every request takes a slot of the address budget in one atomic upsert (`Attempts.Take`, so
  parallel requests cannot send a further mail) and enqueues a mail job; for an unknown or
  over-limit address the job is a no-send one that builds the mail and drops it.
- **`/api` and `/mcp` are limited per Workspace**, not per token (a Workspace has many tokens, and
  the load is the Workspace's), with Klaviyo-style burst (1 s) and steady (1 min) windows and one
  shared budget for `/api` and `/mcp`. `/collect` is limited per Workspace too, plus per IP, plus
  size caps (32 KB per event, 500 KB per batch, 413; the per-event cap inside a batch is checked
  on the exact serialized bytes of each element before the ogen decoder runs). The Workspace is only known inside the
  ogen security handlers, which receive only a `ctx`. An outer middleware therefore puts the
  `ResponseWriter` and `*http.Request` into the context (as `clientip` does), the handler calls
  `httprate`'s `OnLimit` through them, and a typed error is rendered by `problemErrorHandler` as 429. Headers set on the shared writer reach successful responses too. Failed token / collect-key authentications are limited
  per IP in the same handlers, since guessing a credential is brute force too.
- **Contract and headers.** 429 is a common error model in TypeSpec, so the generated client types
  it. Responses carry `X-RateLimit-Limit/Remaining/Reset` and `Retry-After` (Segment, Drip), not
  the IETF `RateLimit`/`RateLimit-Policy` of `draft-ietf-httpapi-ratelimit-headers`: that is still
  a draft and `httprate` cannot emit its format. Revisit when it becomes an RFC.
- **Order and addresses.** recoverer → requestID → CORS → clientip → rate limit → timeout. CORS
  comes first so a 429 on `/collect` still reaches the browser. The client address always comes
  from `internal/clientip`, never from raw headers.
- **Config, off at zero.** Every limit is configuration with a default and `0` disables it (tests,
  dev). Limits are not gated by the EE licence: brute-force protection belongs in core.
- **Abuse protection only.** Per-period allowances (events or contacts per month) are not part of
  this and have a different data model. Out of scope here.
- Every rejection increments `ratelimit_rejected_total{policy}` and logs at warn, with no email in
  clear text.
