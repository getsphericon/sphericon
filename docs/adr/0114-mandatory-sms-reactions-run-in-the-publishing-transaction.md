---
status: accepted
---

# Mandatory SMS reactions run in the publishing transaction

> **Amended after #1227:** the stop's subscriber also publishes the customer-facing
> `sms.unsubscribed` when the opt-out is new (ADR 0115). It is a projection of that transition for
> the Contact; the facts below stay internal.

An SMS stop or a provider's verdict that a number is unreachable is an internal semantic fact.
Publishing it runs its mandatory subscriber inside the same transaction: the stop records the
Unsubscribe and withdraws Confirmation together, and the unreachable verdict records Suppression.
An error from the subscriber fails publication and rolls back that transaction, including queued
asynchronous reactions. The producer reports the fact without directly choosing the registry write.

This adds a synchronous dispatch mode to the event module. ADR 0101 continues to describe
asynchronous consumers, whose River jobs commit with publication and run afterwards. ADR 0102's
statement that an event consumer necessarily runs after commit no longer applies to every
subscriber. Mandatory changes retain their transaction and table ownership.

## Decisions

- **Start with these two SMS reactions.** The stop handler calls the owner of the atomic stop
  operation; it does not split Unsubscribe and Confirmation withdrawal across subscribers whose
  order a caller must know. The unreachable handler calls the Suppression owner. A subscriber
  never writes another module's rows itself.
- **Publish meaning, not a raw provider error.** The provider adapter interprets a refusal into
  a semantic verdict. A subscriber must not reload a mutable Integration merely to reinterpret
  its historical provider code. A transient or ambiguous refusal must not suppress a number.
- **Internal facts stay internal.** The new SMS facts have no customer-facing Event projection,
  Rules or Automations trigger, or Webhook payload. The existing `sms.confirmed`, `sms.received`
  and delivery Events retain their meanings. Unknown numbers remain subject to stop and
  suppression rules without creating a Contact.
- **The synchronous seam is transactional.** It receives the publisher's transaction scope and
  propagates refusals and failures. `OnStored` remains an after-commit callback and is not used
  for mandatory changes. Infrastructure knows no domain module: wiring registers subscribers.
- **Deduplicate external facts before any subscriber writes.** An external fact with a stable
  source id is claimed in the publishing transaction, including when it has no Event projection.
  A repeated fact runs no synchronous handler and queues no asynchronous work. Rolling back
  publication rolls back that claim, so a failed attempt can be handled again. Receipt identity
  is scoped to the originating Workspace and source, independent of the Contact or the current
  opt-out, and must distinguish separate semantic facts produced from one external callback.
  The Event source identity includes the semantic event name as well, so storage and downstream
  deduplication agree with the receipt rather than discarding a distinct projected fact.
- **Consumption survives later state changes.** STOP, then START, then redelivery of the first
  STOP leaves the restored consent intact. A new STOP with its own source id may stop the number
  again. Repeated START likewise cannot undo a later new STOP. Consumption must not depend on
  retaining a projected Event row or on whether a Contact exists.
- **An observation and a transition are distinct facts.** Synchronous handlers may find that an
  opt-out or Suppression already stands. Downstream reactions to a state change run only when
  the owner actually changed it; a new observation alone does not repeat those reactions. STOP
  followed by a new STOP has one transition, while STOP, START and a new STOP have two. Existing
  SMS final-state gating still prevents a repeated delivery callback from re-suppressing a
  number after its Suppression was lifted.
- **Deduplication is not a personal event archive.** Preserve only the receipt identity needed
  to reject redelivery. It must not retain the raw personal envelope, Contact id, Destination or
  provider message reference that Erasure removes. Consumer jobs retain their own existing
  Erasure rules, and anonymous accounting work follows ADRs 0113 and 0115.
- **The interface is the test surface.** Test publication with actual subscribers and database
  writes: a stop is effective before asynchronous work runs, a failure rolls every change back,
  and neither producer needs to know a particular handler. The existing database and River
  test-executor adapters are sufficient; no mock-only interface is introduced.
  Include simultaneous duplicate delivery, a failed first attempt, unknown numbers, distinct
  Workspaces/sources, STOP/START with redelivery in either direction, and a new real transition.

## Considered options

- **Only announce changes after direct writes.** This records facts but leaves each producer
  coupled to the mandatory registry operation. The chosen event seam concentrates reaction
  policy in its owner, providing locality and leverage across send and callback producers.
- **Use the existing asynchronous consumers.** A committed STOP would leave a window in which
  the number was still eligible, and handler failure would leave the mandatory change pending.
- **Use after-commit callbacks.** They cannot fail the already committed publication and offer
  neither transactional rollback nor River's durable retry.

The project is greenfield: build one publishing model with explicit synchronous and asynchronous
subscribers, without a compatibility route through the previous direct SMS writes.
