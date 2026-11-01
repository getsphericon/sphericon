---
status: accepted
---

# Presets parse what comes in, and the core reads what it means

ADR 0037 made a provider's callbacks a Preset whose transform returns Delivery notices only. The new
Channels (ADR 0074) send more back: a person's message (an SMS reply, a Telegram `/start`, a
WhatsApp button), and a provider's own word that someone is out of reach (a blocked bot, WhatsApp's
marketing stop, a provider-side suppression). Bindings are made only from such input (ADR 0075).

## Decisions

- **A Preset produces three kinds of fact:** a Delivery notice (its kinds per Channel are decided
  with the Outbound send), an **Inbound message** (Channel, the Destination it came from, its text,
  the payload of a pressed button, the provider's message id) and a **Reachability change** (the
  provider says a Destination became unreachable or reachable again). The closed set grows only by
  a decision like this one.
- **The Preset parses; the core interprets.** What a message means on its Channel is the same for
  every provider and lives in the core: `/start <token>` binds a chat, a STOP keyword in any
  language sphericon supports opts out (Russian SMS providers do not recognise one). Putting that in
  every Preset would repeat it per provider and let two providers disagree.
- **System events.** From a known Contact an Inbound message is recorded as `<channel>.received`,
  its text cut at 1 KiB, so later triggers and the customer's Webhook endpoints can read it. A
  Binding made or removed is `<channel>.bound` / `<channel>.unbound`, so a welcome Automation can
  start on `telegram.bound`.
- **An unknown sender creates nothing.** A message from a Destination no Contact has, a bare
  `/start`, a WhatsApp message with only a business-scoped id: no Contact, no Event, no Binding, and
  the bot does not answer. The one exception is an opt-out, which is kept by Destination because an
  Unsubscribe outlives any Contact.
- **Start tokens.** sphericon issues a single-use opaque token (at most 64 characters of
  `[A-Za-z0-9_-]`, Telegram's limit) for `t.me/<bot>?start=<token>`, from a merge tag in a message
  of any Channel, from the tracker for the current Visitor, or through the API; it lapses after 30
  days, since a link waits in an inbox for weeks. The chat binds to whoever the token was issued to,
  a Contact or a Visitor; a Visitor's Binding follows it through Identify (amends ADR 0075).
- **sphericon connects the bot.** Saving a Telegram Integration calls `setWebhook` with the
  Integration's hook URL and a secret token, replacing any webhook the bot had, which the UI says
  first. ADR 0037's rules hold for every Channel: verify before parsing, write before answering 2xx,
  deduplicate on the provider's event id.
- **A pressed button is an Inbound message with a payload.** sphericon answers a Telegram callback
  at once so the person's client stops waiting; what the button does (an opt-out, a tracked click)
  belongs to the consent and click-tracking decisions.

## Consequences

- Inbound text of known Contacts is stored in Events, bounded, and erased with them.
- A Self-hoster's Preset for a new SMS provider maps its payload to these three facts and gets STOP
  handling and bindings for free.
