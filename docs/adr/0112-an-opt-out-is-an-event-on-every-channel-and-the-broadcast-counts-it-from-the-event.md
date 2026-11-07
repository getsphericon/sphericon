---
status: accepted
---

# An opt-out is an event on every Channel, and the Broadcast counts it from the event

> Amends [[0111-every-entry-point-is-a-driver-that-calls-one-operation-of-the-core]]: it decided that
> a Telegram opt-out is recorded through `unsubscriberecording`, with the same effects in the same
> transaction as any other Unsubscribe. This reverses that decision for the Broadcast's count.
>
> Amends [[0102-the-core-is-layered-and-an-operation-across-modules-is-a-use-case]]: its rejected
> option "Domain events between modules" no longer holds for a counter.
>
> Amends [[0077-consent-on-the-new-channels]]: an SMS opt-out is recorded with an `sms.unsubscribed`
> Event, as a Telegram one is with `telegram.unsubscribed`.

## Context

A Broadcast counts the opt-out of its recipient (`broadcasts.RecordUnsubscribe`, a mark on the
recipient row set once). Two paths set that mark today:

- **Synchronously in `unsubscriberecording`**, for the unsubscribe link of an email, a text or a
  Telegram message, in the transaction of the opt-out.
- **Asynchronously in the broadcasts consumer** (`event_broadcast_unsubscribe`), from a
  `telegram.unsubscribed` that names a Broadcast. `channelstop.StopTelegram` publishes it for a
  button press or a blocked bot read in the Ingest hook, and for a bot found blocked at send time
  inside `outboundmessages`.

A Telegram unsubscribe link runs both paths. That is harmless only because the mark is set once.

Several facts make the synchronous path the wrong one to keep:

- **It cannot reach every opt-out.** A blocked bot or a number that texted STOP to the provider is
  found by the Outbound send, in `outboundmessages`. That module is in the sending context, below
  orchestration, so it may not call `broadcasts`. ADR 0111 wanted every opt-out in one transaction,
  which works only where the opt-out is recorded by a use case.
- **SMS has no opt-out event.** `channelstop.StopSMS` takes no publisher. A send Twilio refuses with
  21610 records the opt-out and counts on no Broadcast, although the refused text belongs to one.
  An Automation cannot start or end on an SMS opt-out, and no webhook tells the customer about it.
- **The consumer no longer runs after the commit.** ADR 0102 rejected events between modules because
  "an event consumer runs after the commit". Since ADR 0101, publishing inserts the consumer's job
  in the producer's transaction. The count is not lost if the process stops: the job commits with
  the opt-out or not at all.

## Decisions

- **The Broadcast counts an opt-out only from its event, on every Channel and every path.** The
  broadcasts consumer wants any opt-out event that names a Broadcast and a Contact:
  `email.unsubscribed`, `telegram.unsubscribed` and `sms.unsubscribed`. It counts the opt-out at the
  event's occurred time.
  - An email engagement shares its Go type with an open and a click, so the consumer tells an
    opt-out by its bus name (`events.IsUnsubscribed`), not by its type.
  - `unsubscriberecording` no longer calls `broadcasts.RecordUnsubscribe`.
- **A cascade that must commit with the change stays synchronous.** Leaving an Automation when its
  unsubscribe link is clicked (`automations.Exit`) is still called by `unsubscriberecording`, in the
  opt-out's transaction. An Enrollment that went on after its Contact opted out would send its next
  step, while a count that lags changes only a figure.
- **An SMS opt-out is the `sms.unsubscribed` Event**, which mirrors `telegram.unsubscribed`:
  - `channelstop.StopSMS` takes the publisher and publishes it on every new SMS opt-out, whatever its
    source: a STOP keyword, the unsubscribe link of a text, a send the provider refused, or one
    recorded by hand. A number already opted out records no second one.
  - It is recorded for the Contact whose phone the number is (at most one: the phone is unique in a
    Workspace), and for the Contact of the text the stop was made from, when that is another one.
  - It carries the Contact, its email, the number (`phone`), how the person stopped (`via`: keyword,
    link, refused_send, manual), the sender it came through, and the Broadcast and Automation of
    the text, on that text's Contact only.
  - Its source id is the new opt-out row's id with the Contact's. A START deletes the row, so a later
    stop records a new Event. The number is not part of the id, so an Erasure that anonymizes the
    Event leaves no number behind.
  - It is a System event under the reserved `sms.` prefix. An Automation may start or end on it,
    and a Webhook endpoint receives it like any other Event of a Contact. An Erasure anonymizes it
    like the other opt-outs. Like `telegram.unsubscribed` it is not evidentiary: the opt-out row is
    the record that outlives the Event.
- **Which opt-outs count on a Broadcast:**
  - an unsubscribe link: the Broadcast the link names, as before;
  - a Telegram button press or a bot found blocked at send time: the Broadcast of the message, as
    before;
  - a refused send of a Broadcast's text (Twilio's 21610): that Broadcast. This is new.
  - a STOP keyword on SMS: none, since no text identifies the Broadcast it answers.
- **Adding an event only adds** (ADR 0033): a new bus name and a new payload are not a breaking
  change, and a receiver that does not know `sms.unsubscribed` ignores it.

## Considered options

- **Keep the synchronous count and add one for the send-time paths.** `outboundmessages` cannot call
  `broadcasts`, so it would need a use case around the Outbound send. That use case would exist
  only to set a mark the event already carries.
- **Count in both places and rely on the mark being set once.** Two writers of one fact, with
  nothing that says which counts. The next change to one path would miss the other.
- **Name the Broadcast on the opt-out row and count from the row.** It mixes the scope of an opt-out
  (what the person left) with its attribution (which message provoked it), which the glossary keeps
  apart. A row that outlives its Contact would also keep a link to a Broadcast it no longer explains.

## Consequences

- A Broadcast's unsubscribed figure now lags by the latency of the consumer's job. A job discarded
  after its attempts leaves the opt-out uncounted until an operator retries it from River UI. The
  opt-out itself, and Send-eligibility, take effect at once.
- A Telegram unsubscribe link is counted once, by one path.
- A test of a count works the consumer's job (`env.PerformEnqueuedJobs`) before it reads the mark.
- `docs/architecture.md`, `docs/design/domain-events.md` and the glossary's Unsubscribe and
  Unsubscribe recording entries change with the code.
