---
status: accepted
build: [1746, 1747, 1748]
---

# A run of soft bounces suppresses a Destination

> **Revised before it was built:** a run is at least five counting soft bounces whose first and latest
> are seven days apart (five daily sends span four days); only address and mailbox codes count and a
> bounce with no code does not (a provider's block of the sender suppressed every recipient); only
> Presets that report deliveries; the run restarts after a lift.

> Amends [[0011-deliverability-rate-metrics]] (a soft bounce is still no reputation signal, but a run
> of them is a Suppression) and [[0001-send-eligibility-model]] (a reason of Suppression).

## Context

A soft bounce never suppresses (ADR 0011): a Destination whose mailbox is full or abandoned for good is
mailed on every send, indefinitely. Most marketing platforms suppress after a run, of five to seven
soft bounces in a row with a delivery breaking it, or three of the last five; some count only failures
on the recipient's side. Amazon SES's own suppression list never takes a soft bounce. A full mailbox at Gmail is often
the account's shared quota: the practice is to wait a week or two before giving up on it.

## Decision

- **A Soft bounce run suppresses the Destination**, with the reason `soft_bounce_run`: at least five
  soft bounces that count, with no delivery between them, the first and the latest at least seven days
  apart. A delivery ends the run, and so does a person's lifting of the Suppression: only what comes
  after the lift counts again. The Suppression is a Suppression like any other: it blocks every
  surface, Transactional included (ADR 0005), and only a Member lifts it (ADR 0061, ADR 0154), never
  a notice, an agent or the person's Preference page.
- **A soft bounce counts when its enhanced status code (RFC 3463) is about the address or the
  mailbox**: subject 1 (addressing, such as `4.1.1`) or 2 (mailbox, such as `4.2.2` mailbox full). A
  code of the mail system, routing, the protocol, the content or the security policy (subjects 3 to 7:
  a message too large, a DNS failure, content refused, a sender blocked) is the message's, the
  network's or the sender's, and does not count; nor does a soft bounce with no code, so a provider
  that blocks the sender for a week suppresses no one.
- **The code comes with the Delivery notice**: a Preset passes the provider's code where it has one
  (Amazon SES's `status` of a bounced recipient), or maps the provider's own subtype to one (SES's
  `MailboxFull` to `4.2.2`); with neither, the core reads the first enhanced status code in the
  notice's reason (the provider's SMTP diagnostic). The field is optional, so a Preset without it
  still loads.
- **The run is read only where deliveries are reported**: a Preset declares which kinds of Delivery
  notice it reports, and its tests prove each; a bounce that came through a Preset that reports no
  delivery never counts, as "no delivery between" would mean nothing there.
- **The run is read from the Destination's delivery Events**, which exist for an address that is no
  Contact too, in the order of the messages' sending where a notice names its Outbound message, else
  in the order of reporting. A delivery reported late for an earlier message ends no run already
  ended by a Suppression: a Suppression is never lifted by a notice. A Test send's notices do not
  count.
- **Email only**; the other Channels have their own unreachable verdicts.
- **A soft bounce still counts in no rate** (ADR 0011).

## Considered options

- **Never**: the list keeps an address that will not receive, and every send
  to it is a soft bounce more.
- **A count with no time span**: one outage of a day turns into a Suppression.
- **Per-Preset rules of what counts**: every Preset decides the same thing differently; the enhanced
  code is the standard every provider already speaks.

## Consequences

- A Delivery notice gains the optional enhanced status code; a Preset declares the kinds it reports.
- Glossary: **Soft bounce**, **Soft bounce run**, **Suppression**.
