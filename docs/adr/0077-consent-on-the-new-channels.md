---
status: accepted
---

# Consent on the new Channels

Send-eligibility (ADR 0013) has four layers keyed by (Channel, Destination): Suppression, an
`everything` Unsubscribe, a source Unsubscribe, and a Confirmation that only a Workspace requiring
confirmed opt-in asks for. That fits email. The research on the map issue
(getsphericon/sphericon#916, `research/channel-consent-rules`) found that every new Channel needs a
positive opt-in, that a STOP revokes all of a sender's marketing on the Channel (FCC 26-67, 38-FZ,
GDPR, WhatsApp policy), and that providers report both a person's choice and a dead address.

## Decisions

- **Where consent comes from.** On SMS and WhatsApp a Confirmation is always required, whatever the
  Workspace's confirmed-opt-in setting, which stays an email setting. On Telegram and web push the
  Binding is the opt-in: it exists only because the person started the bot or allowed
  notifications. On mobile push the external push service holds it (ADR 0074).
- **How an SMS or WhatsApp Confirmation is recorded.** Through the API or an import, as `imported`,
  carrying the consent wording, its source and the brand in the `marketing.confirmed` Event; or
  through an on-site form, if that decision keeps forms. sphericon runs no SMS double opt-in of its
  own: a customer who needs a code or an identifying action (Russia's FAS reading) uses their own
  app or a verification service and reports the result.
- **An opt-out made in the Channel is Channel-wide.** A STOP keyword (in the languages sphericon
  supports), `/stop` and a blocked bot on Telegram, WhatsApp's marketing stop and an unsubscribe
  button become an Unsubscribe with the `everything` scope of that Channel, not of the message's
  Sending source. WhatsApp's split between marketing and utility is not modelled: everything
  sphericon sends there is marketing. A source-scoped opt-out exists on these Channels only as a
  link to the unsubscribe page that the author puts in the message.
- **A reversal made in the Channel lifts it.** START or UNSTOP, WhatsApp's resume and an unblocked
  bot lift the Unsubscribe; on SMS and WhatsApp a START also records a Confirmation with the
  provenance `keyword`, since the person asked to receive again.
- **A dead address is not an opt-out.** A web push 404 or 410 and a deactivated Telegram user remove
  the Binding (`<channel>.unbound`). An SMS number the provider reports as not existing is
  suppressed like a hard bounce. Ambiguous failures (WhatsApp `131026`, a Russian operator refusing
  mass SMS, an unknown-state report) record nothing beyond the failed message. A mobile push
  service saying the person is not subscribed makes the message Skipped and is not recorded.
- **sphericon sends no compliance replies.** The confirmation of a STOP or a START is a provider's
  auto-reply where it has one (Twilio) and otherwise the customer's: the FCC rule allows one and
  does not require it. So no non-marketing message is sent on these Channels and Transactional
  stays email-only.
- **An opt-out that arrives outside the Channel** (an email saying "no more SMS") is recorded by
  hand in the app or through the API, for any Channel, as for email today.

## Consequences

- An SMS or WhatsApp audience starts empty until the customer asserts Confirmations.
- A STOP on one Sending source's message silences every Broadcast and Automation on that Channel.
- Eligibility is checked per message at send time, as it is for email, which meets Russia's
  "immediately".
