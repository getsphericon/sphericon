---
status: accepted
---

# Destinations are the Contact's own attributes, or Bindings

ADR 0074 fixed six Channels and said that on most new ones a Destination is reachable only through
the Integration that obtained it. It left open where each Destination lives. A Contact has three
alias keys (subject_id, email, phone), and the phone was only trimmed, so `+7 999 123-45-67` and
`89991234567` were two different Contacts. A Telegram chat comes from the person writing to a bot,
a web push subscription from a browser that may not be identified yet, and both can be many per
person. The research is linked from the map issue (getsphericon/sphericon#916).

## Decisions

- **A scalar address stays on the Contact.** Email is `Contact.email`; SMS and WhatsApp both send to
  `Contact.phone`; mobile push sends to `Contact.subject_id` through the external push service. No
  new field is added for them.
- **A phone is stored in E.164.** Every door (Identify, the API, import) normalizes it; a number
  without a country code is read with the Contact's country; a number that cannot be parsed is
  refused like an invalid email. One person, one key, so one consent and one opt-out.
- **WhatsApp is addressed by the number.** sphericon sends one-way template messages to people who
  opted in off WhatsApp with their number. The business-scoped user id WhatsApp introduces with
  usernames is not modelled yet; an inbound message that carries only that id binds to nobody.
- **Telegram chats and web push subscriptions are Bindings.** A Binding is (Integration,
  Destination) and its owner: a Contact for a Telegram chat, a Visitor for a web push subscription.
  One entity serves both, and a Workspace may have several Telegram bots or web push keys. Per-channel
  fields on the Contact (`telegram_id`) were weighed and rejected: a field cannot say which bot can
  reach the person, and a browser subscription exists before its Visitor has a Contact.
- **A Binding is made only by what the channel sends in.** A Telegram chat binds through `/start`
  with a single-use token from a link sphericon generated; a web push subscription is reported by
  the tracker for its Visitor. A chat id is not an alias key and the API cannot assert one: only the
  bot the person wrote to can reach them anyway.
- **The latest Binding wins.** A chat that starts again with another Contact's token moves to that
  Contact; a Visitor's Bindings follow it when Identify resolves it.
- **An anonymous subscription waits.** A web push Binding on a Visitor with no Contact reaches nobody
  (sends target Contacts through Segments); it starts working when Identify resolves the Visitor.
- **No cap per Contact.** Dead Bindings are removed on the channel's own signal (a 404 or 410 from a
  push service, a blocked bot), not by eviction.

## Consequences

- A Segment condition such as "has Telegram" and the Contact card read Bindings, not a field.
- Existing phone values are rewritten to E.164 by the change; values that do not parse are dropped.
- Whether one message goes to every Binding of a Contact or to one is decided with the Outbound send.
