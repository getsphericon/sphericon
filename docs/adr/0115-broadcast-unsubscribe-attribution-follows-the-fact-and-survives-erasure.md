---
status: accepted
---

# Broadcast unsubscribe attribution follows the fact and survives Erasure

> **Extended by [[0116-the-event-journal-retains-explicit-anonymous-replay-inputs]]:** attribution
> retains the earliest accepted observation for a received message, so distinct source-specific
> and everything opt-outs and their replays converge regardless of handling order.
>
> **Amended by #1236:** an SMS opt-out has a customer-facing Event, as email's and Telegram's
> have. A new opt-out of a Contact's number records `sms.unsubscribed`, which Webhook endpoints,
> Rules and Automations receive; a repeated STOP records none. It names a Broadcast or an
> Automation only where this ADR attributes the opt-out: the link of a text the person received.
> It is a projection of the transition, not the attribution fact: the Broadcast still counts
> through the internal `message.unsubscribed` alone, and the internal SMS facts stay internal.

A Broadcast's unsubscribe count is a reaction to an accepted opt-out, independently retried,
and may lag behind it. The opt-out, the Confirmation it withdraws and the Enrollment it ends
still commit together (ADR 0102). This replaces the glossary's same-transaction count promise;
it does not defer the changes that stop sending.

Email, Telegram and SMS attribution follow the same rule through one consumer. Keep the existing
email and Telegram System events; the missing SMS attribution fact is internal, with no new
customer-facing SMS Event, Automation or Rule trigger, or Webhook payload in this change.

Only a person's action on a received message, through its unsubscribe link or button, attributes
the opt-out to that Broadcast. A standalone Channel stop names no Broadcast. A provider refusing
a new send because the Destination had already stopped it must block further sending, but does
not make that new Broadcast the cause of the opt-out.

## Consequences

- A retry counts nothing twice. Attribution keeps the time of the accepted opt-out, not the
  time its consumer ran. An already standing opt-out creates no new attribution merely because
  another link was clicked.
- An attribution accepted before Erasure must still reach the anonymous report if its consumer
  runs afterwards. Already counted attribution also survives Erasure. This must not depend on
  finding the erased Contact or on retaining the original event's personal fields.
- Ordinary event and Webhook jobs keep their Erasure rules (ADR 0021, ADR 0101). Preserving the
  anonymous accounting fact does not authorize retaining a Contact id, Destination, original
  personal deduplication key or a full personal event envelope for it.
- The projection's implementation must tolerate both orders of handling and Erasure, repeated
  handling, and a missing or foreign delivery record. An attribution job never recreates a
  Contact or an erased relationship. The River adapter and the application's River test executor
  cross the same event seam; no additional mock-only interface is needed.

## Considered options

- **Keep the direct count on links.** It leaves two implementations of attribution: links count
  in the opt-out's transaction, Telegram Channel stops through a consumer. One consumer gives
  the Broadcast module locality over its attribution rule and leverage across all Channels.
- **Retain every event job through Erasure.** A full envelope includes personal fields and may
  embed a Contact id in its deduplication key. Only anonymous accounting data may survive.
- **Let a purged job lose its attribution.** It makes the reported result depend on whether
  the consumer happened to run before Erasure, even though the opt-out was already accepted.
