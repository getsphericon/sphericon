---
status: accepted
---

# Outbound calls to third-party hooks

Sphericon calls other services' URLs from two places: a Webhook endpoint (domain events to a URL)
and the HTTP step of an Automation. Both go through the one hardened client (`webhook.NewClient`);
a second caller never builds its own, because the one prior-art project whose campaign action did
(Mautic) skipped the SSRF check. Everything is core, not `ee/`, and works with no configuration.

## Decisions

- **Internal targets are an Instance setting, never a Workspace's.** Private ranges stay blocked
  unconditionally. A Self-hoster names the hosts and ports that may be reached (an n8n or Node-RED
  container on the same network) in one environment variable, off by default and unset in SaaS. A
  Workspace must not open the Instance's network to itself, so the allowlist has no per-Workspace
  form. It also lets a test run the real client against `httptest`.
- **A Webhook endpoint disables itself, by a rule that survives a restart.** It stores its
  consecutive failures, last error and reason for being disabled. A 410, or persistent failure over
  a window of time (a count alone would let a restart in a burst retire a healthy endpoint),
  disables it and mails its owners; a 429 is not a failure and honours `Retry-After`. The retry
  horizon is days, not river's default of about four hours. An HTTP step and a Rule's HTTP Action
  retry only what another attempt can fix (`webhook.Retryable`: a timeout, a transport error, a 5xx,
  a 408); a blocked address or any other 4xx fails at once. Threshold, window and horizon are
  constants, not settings. There is no delivery table and no replay: the endpoint's state is the
  record, and a lost event is not redelivered. The secret can be regenerated (the old one stops
  at once) and a sample event of a chosen type can be sent to check the URL and the signature.
- **An HTTP step holds its own URL and does not reference a Webhook endpoint.** A Webhook endpoint
  is a subscription with a secret and failure state; an HTTP step is a one-off call for one
  Contact, so the Workspace's copy-at-author-time rule applies (URL, header values and body live in
  the step; the URL and header values are secrets, since a Zapier or Slack hook URL is itself the
  credential). The body is a Liquid template rendered by the email engine (no file tags), checked as
  JSON when the step is saved. Third-party hooks do not verify signatures, so the request is signed
  only when the author sets a secret. A failed call fails the Enrollment; there is no
  continue-on-error mode.
- **Out of scope.** Methods other than POST, response data feeding later steps, per-endpoint
  concurrency limits and secret rotation with two live signatures.

## Considered options

An HTTP step that picks one of the Workspace's Webhook endpoints was rejected: it would couple a
Contact-level call to an event subscription's secret, failure state and event filter, none of which
apply. A per-Workspace internal allowlist was rejected for the reason above.
