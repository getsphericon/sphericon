---
status: accepted
---

# End-to-end tests: in-process application, Mailpit, driven through the public API

Handler tests drive handlers through an in-memory recorder, inside a rolled-back transaction, with mail captured in
memory. Nothing exercises the real chain: HTTP API, event bus, job queue, MIME build, SMTP delivery, the message a
recipient gets and the link they follow. We add a black-box end-to-end suite that boots the whole application,
sends through a real SMTP server and observes the result only in that server's inbox.

## Decisions

- **One seam: the public HTTP API (`/api`, `/collect`) in, a Mailpit inbox out.** Tests never read the database, the
  event outbox or an in-memory sender.
- **The application runs in the test process**, with the real event router, job queue and HTTP server, started on a
  caller-provided listener so the public URL (the base of unsubscribe, confirm and tracking links) is the test
  server's own address. In-process beats driving the compiled binary: it is faster, debuggable, and reuses
  the application's own lifecycle, the same one the binary uses. Binary and reverse-proxy routing are
  left to a separate smoke test.
- **Lifecycle: `App.Start` brings everything up, `App.Close` takes everything down.** `Start(ctx)` returns without
  blocking once the metrics listener, the event router and the job workers run and the public server is serving; an
  App starts once (a second `Start` returns `ErrAlreadyStarted`). A `Start` that fails partway unwinds what it had
  started, so nothing is left running. `Close(ctx)` cancels the router and the workers, waits for both to stop (also
  after a partial failure to start the workers), shuts the HTTP servers down and then the container; it is
  idempotent and returns the first call's result. `Done()` delivers, once, how the public server ended: an error if
  serving failed, nil after a graceful `Close`.
- **A dedicated database, rebuilt on every run, and a fresh Workspace per test.** The per-test transaction rollback
  used elsewhere (go-txdb) cannot work here: the job queue and bus workers use their own connections and would never
  see uncommitted rows. Everything is Workspace-scoped (ADR 0017), so tests need no cleanup and may run in parallel.
- **Mailpit is started by the suite on free ports**, not shared with the dev stack, so the suite never collides with a
  running `mise run dev` and needs no Docker. Mailpit's own API is read through a thin typed helper: it publishes
  only Swagger 2.0, which the project's OpenAPI generator does not accept. The helper sits behind each Workspace's
  Inbox, so scenarios never call it.
- **Mail is observed only through the Workspace's Inbox.** A `Match` selects mail by exact recipient (compared
  case-insensitively; Mailpit's own search is a substring match, so the Inbox insists on the whole address) and,
  optionally, an exact subject. The Inbox offers one wait, `Wait(Match)`, polled up to a bounded timeout, which on a
  miss fails with what the inbox held instead, and one absence check, `RequireNone(Match)`, which watches for a short
  window. An absence check cannot be awaited, so a scenario first waits for a sibling delivery of the same send, which
  proves the pipeline has caught up. A failing Mailpit request fails the test; it is never read as absence. Every
  message the Inbox observed, by a wait or by an absence check that found a match, is deleted when the test ends, and
  only those, so parallel tests sharing one Mailpit stay independent. A recipient unique to the test keeps matches
  from colliding.
- **What leaves the app is tested as delivered.** A link, a header, a page reached from a link or a hook's body is
  taken where its recipient gets it (the Inbox, the test's own Hook) and used as the recipient uses it: a link is
  followed through its redirects, a page's action is the call the page makes, a signature is checked with the secret
  the app shows its User. The harness never builds such an artefact or its expected value with the server's own
  helpers (URL building, token signing). Added after a competitor shipped a broken `List-Unsubscribe` behind tests
  that rebuilt links with the server's helpers.
- **Only the arrange step is outside the API:** creating the User, Workspace and first API token with the product's
  own domain functions. Everything else is an API call. The bootstrap token is not used; it addresses only the
  oldest Workspace.
- **Sending domain verification uses a harness-only DNS lookup.** The send path requires a live DKIM check
  (ADR 0010); the suite injects the existing development lookup, which echoes a domain's own stored key, through an
  e2e-only app option (`app.WithE2EDNS`, refused outside the e2e profile) rather than the development flag. Because that lookup resolves by domain name alone
  while uniqueness is per (Workspace, domain), every test uses a unique Sending domain.
- **Asynchronous steps are polled with a bounded timeout.** The job queue clock is not controlled, Automation wait
  steps are not used and Deferral (ADR 0023) is out of scope.
- **The external API gains what the scenarios need** (Integration and Sending domain), because the API is meant to
  expose every capability and the MCP surface is a projection of it (ADR 0016). Only what the scenarios require is
  added; the rest of the parity gap is tracked separately.
- **Scenarios are written as domain steps** (import contacts, send a Broadcast, wait for an email), with the
  transport inside the harness. An MCP-driven run can then replay the same scenarios on a second transport. No
  common driver interface is introduced until that second implementation exists.
- **The suite is its own package behind a build tag, with its own task and CI job.** The default test task does not
  run it.

## Exception to "tests build on committed fixtures"

Tests normally start from committed YAML fixtures. End-to-end tests deliberately create their own User, Workspace
and data through the API, because fixtures rely on the transaction rollback that this suite cannot use. The
exception covers the end-to-end package only; every other test keeps the fixtures rule.

## Considered options

- **Compiled binary behind the reverse proxy:** truest to production, but slow, hard to debug and unable to choose
  its own public URL. Kept for a separate smoke test.
- **Shared Mailpit daemon from the dev stack:** simpler, but collides with a running dev stack and shares one inbox.
- **testcontainers:** adds a Docker dependency the toolchain otherwise avoids.
- **Provisioning through the site API with a cookie login:** needs a login flow the suite does not otherwise test and
  hides the missing external resources instead of fixing them.
