---
status: accepted
build: none
terms: [Soft bounce run]
---

# A run of soft bounces suppresses a Destination

> Amends [[0011-deliverability-rate-metrics]] (a soft bounce is still no reputation signal, but a run
> of them is a Suppression) and [[0001-send-eligibility-model]] (a reason of Suppression).

## Context

A soft bounce never suppresses (ADR 0011): a Destination whose mailbox is full or abandoned for good is
mailed on every send, indefinitely. Most marketing platforms suppress after a run, of five to seven
soft bounces in a row with a delivery breaking it, or three of the last five; some count only failures
on the recipient's side. Amazon SES's own suppression list never takes a soft bounce. A full mailbox at Gmail is often
the account's shared quota: the practice is to wait a week or two before giving up on it.

## Decision

- **Five soft bounces in a row on the recipient's side, with no delivery between them and at least
  seven days from the first to the last, suppress the Destination**, with the reason `soft_bounce_run`.
  A delivery ends the run. The Suppression is a Suppression like any other: only a person lifts it
  (ADR 0121).
- **The recipient's side is read from the enhanced status code (RFC 3463)** a Delivery notice carries:
  a Preset passes the provider's code, or maps the provider's own bounce subtype to one. Codes of the
  mail system, the protocol, the content and the security policy (subjects 3, 5, 6 and 7: a message too
  large, content refused, a policy block) are the message's or the sender's fault and do not count. A
  soft bounce with no code counts.
- **A soft bounce still counts in no rate** (ADR 0011).

## Considered options

- **Never**: the list keeps an address that will not receive, and every send
  to it is a soft bounce more.
- **A count with no time span**: one outage of a day turns into a Suppression.
- **Per-Preset rules of what counts**: every Preset decides the same thing differently; the enhanced
  code is the standard every provider already speaks.

## Consequences

- A Delivery notice gains the enhanced status code; Presets that know the provider's subtype map it.
- Glossary: **Soft bounce**, **Soft bounce run**, **Suppression**.
