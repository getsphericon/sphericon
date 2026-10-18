---
status: accepted
---

# Inbound provider callbacks are Presets on an Integration

Suppression was fed by one hand-written SES-over-SNS adapter at `/hooks/{ingest_key}/ses`, with the
key on the Workspace. A Self-hoster on an SMTP relay (Postmark, Mailgun) never learned of a bounce,
and the SES hook had a hole: an SNS signature proves a message came from AWS, not from this
account's topic, so anyone holding the key could post a forged bounce through a topic of their own
and suppress any address for good (Suppression does not auto-clear).

## Decisions

- **A provider is a Preset, not code.** A Preset is YAML with a `verify` block and a `transform`
  block. `verify` picks one of a closed set implemented in Go (HMAC over a header, Standard
  Webhooks, HMAC over body fields, shared token or basic auth, SNS). `transform` is a gojq program
  that returns an array of Delivery failures. Built-in Presets (generic, ses, postmark, mailgun)
  ship in the binary; the Self-hoster adds more from a directory named by an environment variable.
  A Workspace never supplies a Preset: the program runs in our process, so it is as trusted as the
  Instance's configuration. The decision that no plugin mechanism exists: a Go binary has no safe
  dynamic loading, and a mapping from JSON to three fields needs none.
- **The Integration owns its inbound side.** The Integration holds its Preset and the Preset's
  secret or parameters (for SES the topic), encrypted with its credentials, and has its own ingest
  key: `POST /hooks/{key}`. The Workspace's `ingest_key` and the `{provider}` path segment go. One
  URL is one Integration, one Preset and one secret, so a forged source never matches.
- **The transform is a pure function with a fixed output.** No environment, no `input`, no modules;
  a per-run timeout; limits on body size, program size and event count; exactly one result. Go
  validates what comes out (a closed type, a parsed address, bounded strings) and never trusts the
  program's shape. `generic` verifies with Standard Webhooks.
- **Verify the raw bytes first, write before answering.** An unknown event type answers 2xx, a bad
  signature 4xx, a failure on our side 5xx so the provider retries; the Delivery failure is in the
  outbox before the 2xx. Redeliveries dedupe on the provider's event id plus the Destination.
- **Out of scope.** A bare MTA with no provider (VERP return paths and an inbound mailbox parsing
  DSNs) is rejected: it is a subsystem of its own and production mail goes through a provider.
  Correlating a failure with a message we sent is not required yet; the provider's message id is
  already stored on the send, so it can be added without a schema change.

## Consequences

Presets are a published format that Self-hosters depend on, so it carries a `version` and changes
only between Majors (ADR 0033). Each built-in Preset ships sample payloads with the events they
must produce, and `sphericon preset test` runs the same samples against a Self-hoster's own file.
