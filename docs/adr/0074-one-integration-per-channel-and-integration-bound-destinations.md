---
status: accepted
---

# One Integration per Channel, and Destinations bound to their Integration

sphericon sends email only. The Integration already carries a `channel` with `sms` reserved, and
Send-eligibility is keyed by (channel, Destination), but nothing beyond email was designed. Adding
SMS, WhatsApp, Telegram, web push and mobile push showed two things the email model never had to
face: one provider account may serve several channels (Twilio, Bird and Vonage send SMS and
WhatsApp), and on most new channels an address only works through the connection that obtained it.
The research behind this is on the `research/*` branches linked from the map issue
(getsphericon/sphericon#916).

## Decisions

- **Six Channels:** `email`, `sms`, `whatsapp`, `telegram`, `web_push`, `mobile_push`. WhatsApp is
  its own Channel although its Destination is usually a phone number: its consent, its content (a
  template approved by Meta) and its address (a business-scoped user id may stand in for the
  number) all differ from SMS. Web push and mobile push are separate: a browser subscription and a
  user of an app are different addresses with different credentials.
- **An Integration serves exactly one Channel.** A provider account used for SMS and WhatsApp is two
  Integrations, and the credentials are entered twice. The default per (Workspace, Channel), the
  Send route, the Provider limit and the Ingest hook all stay one per Integration, unchanged. A
  multi-channel Integration would split each of them inside one row.
- **A Destination may be bound to its Integration.** A Telegram chat is reachable only by the bot
  the person wrote to, a WhatsApp business-scoped user id only within one business portfolio, a web
  push subscription only with the key it was made for, a mobile push user only in one app. Such a
  Destination is reached through the Integration it was obtained through and no other; the
  Workspace's default Integration on such a Channel decides only where new Destinations are obtained.
  An email address and a phone number on SMS stay unbound.
- **Web push is a built-in provider.** There is no third party: the push services belong to the
  browsers and sphericon signs with a VAPID key pair it generates for the Integration. Replacing that
  key loses every subscription made with it.
- **Mobile push goes through an external push service** (OneSignal, Pusher Beams, WonderPush and the
  like), addressed by the Contact's `subject_id`, which the customer's app passes to the service's
  SDK as its external user id. sphericon stores no device token and ships no SDK; a Contact with no
  `subject_id` is skipped on the Channel. Sending to APNs or FCM directly is rejected for now: it
  makes sphericon the device registry, and its delivered and opened signals exist only if the app
  reports them back.
- **Inbound stays declarative; sending stays code.** A Preset (ADR 0037) keeps its closed set of
  verification schemes, which grows by the schemes the new providers use, and its output grows from
  Delivery notices to a closed set of inbound facts: opt-outs, inbound messages and bindings of a
  Destination to a Contact, on the channels that have them. The shape of each is decided with the
  inbound model. How a provider is sent through stays a Go adapter in the catalogue; a declarative
  send description is not built.
- **Prefer what an external service already does.** Where a provider or service already does a job
  (delivery, device registration, template review, retries), sphericon integrates with it rather
  than repeating it. Send-eligibility is the exception: providers block a STOP only per sender, the
  Russian ones not at all, and the law puts the proof of consent on the sender, so consent and
  opt-outs stay sphericon's.

## Consequences

- A switch of the default Integration on a bound Channel does not move existing Destinations; a
  Workspace that replaces its Telegram bot or its VAPID key starts that audience over.
- Mobile push reaches only Contacts the customer's app has logged in to the push service with the
  same `subject_id`; delivered and opened depend on what that service reports.
- Self-hosters can add an inbound Preset for a provider but not a new provider to send through.
