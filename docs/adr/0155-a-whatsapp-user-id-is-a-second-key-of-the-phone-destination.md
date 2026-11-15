---
status: accepted
---

# A WhatsApp user id is a second key of the phone Destination

> Amends [[0075-destinations-are-contact-attributes-or-bindings]] (the business-scoped user id it left
> unmodelled) and [[0077-consent-on-the-new-channels]] (WhatsApp's own marketing opt-out is a Channel
> stop).

## Context

sphericon sends WhatsApp templates to people who opted in off WhatsApp with their number (ADR 0075),
and WhatsApp consent is keyed by that number (ADR 0001, ADR 0077). WhatsApp now gives each user, per
business portfolio, a business-scoped user id that every webhook carries, and with usernames a webhook
may carry that id and no phone number. An inbound STOP with only the id binds to nobody today, so the
opt-out is lost; WhatsApp's policy requires honouring a request to stop made on or off WhatsApp, and
Meta requires supporting the id.

What Meta documents: a webhook carries the number when that business number exchanged messages or calls
with the person in the last 30 days, or the person is in the portfolio's contact book, which Meta fills
on its own once the business messages the number (it can be switched off; no API adds to it). A status
webhook for a message sent to a number carries the user id, except a `failed` one. The id is regenerated
when the person changes number, announced by a `user_changed_number` message carrying the previous id.
A send may address the number, the id or both, and Meta recommends the number so that webhooks keep
carrying it. A `user_preferences` webhook reports a person turning marketing messages from the business
off or on inside WhatsApp.

## Decisions

- **The WhatsApp Destination stays the phone number; the WhatsApp user id is its second key**, one per
  business portfolio, learned from every webhook that carries both (the status of a message sent to the
  number, an inbound message with the number) and kept current. It is keyed by the Destination, as an
  opt-out is, not stored on the Contact, and it is not a Binding: it belongs to the portfolio, not to one
  Integration, and nothing sphericon issues makes it.
- **An observation that carries only the id is resolved through the key.** A STOP, a button, a block or
  a marketing preference with only the id is recorded on the number when the key is known. When it is
  not, the observation is kept under the id, as a Telegram stop from an unknown Destination is, and is
  applied to the number when the key becomes known, in the order ADR 0154 sets. The remaining gap is
  accepted: a first message to a number that was never seen with that id goes out before an earlier stop
  under the id finds it, which needs a person who wrote STOP to a business that never messaged their
  number.
- **Sends address the number**, never the id, so the contact book and the webhooks keep the pair coming.
  The WhatsApp Integration's setup states that the portfolio's contact book must stay on, and Needs
  attention reports inbound messages that lose the number for numbers already known.
- **A number change moves the key, not consent and not the Contact.** `user_changed_number` points the
  new id at the new number and records `whatsapp.number_changed` (old and new number) on the Contact of
  the old one. Consent stays with the number it was given for: the new number has no Confirmation and
  receives no WhatsApp marketing until the person opts in with it. `Contact.phone` is not changed, since
  it is also the SMS address and the customer's data; a Rule or an Automation can act on the Event.
- **Nobody known only by the id is sent to.** An inbound message with only the id binds to nobody and
  matters only as a consent observation. Replies inside the customer service window are a separate
  decision.
- **Turning marketing off inside WhatsApp is a Channel stop**: an Unsubscribe with WhatsApp's
  `everything` scope; turning it on again lifts it, ordered by the webhook's time.
- **Erasure**: the key is personal data. It is deleted with the Destination unless an opt-out stands; then
  it stays with the number's hash in place of the number, as an SMS opt-out does (ADR 0021), so a later
  stop under the id and a send to the number still meet.

## Consequences

- A WhatsApp key table (portfolio, user id, number or its hash) and the System event
  `whatsapp.number_changed`.
- Pending observations under an unknown id, applied when the key resolves.
- Glossary: **WhatsApp user id**; the Destination and Channel stop entries.

## Rejected

- **The id as the WhatsApp Destination**, as Telegram's user id is: consent is given off WhatsApp for a
  number, the first send knows only the number, and the id changes with the number.
- **The id as a Binding**: a Binding is one Integration's and is made by a token sphericon issued; the id
  is the portfolio's and arrives on its own.
- **Moving consent or `Contact.phone` with a number change**: it would assert an opt-in no one gave the
  new number, and rewrite the SMS address from a WhatsApp message.
