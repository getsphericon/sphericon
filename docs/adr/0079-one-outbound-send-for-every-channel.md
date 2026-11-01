---
status: accepted
---

# One Outbound send for every Channel

The Outbound send (ADR 0015) is email's: eligibility, the Sending-domain gate, rendering, the
unsubscribe footer and headers, DKIM, the provider call and the Outbound message, whose delivery
result is stamped by Delivery notices (ADR 0053). The new Channels (ADR 0074) send through other
providers, to Bindings that may be several per Contact (ADR 0075), and report back differently:
SMS has delivery reports, WhatsApp also reads, Telegram and web push nothing after acceptance.

## Decisions

- **One pipeline, two parts per Channel.** Every Channel goes through the same Outbound send:
  Send-eligibility, the Workspace freeze check, the Send route, rendering, the provider call and the
  record. Only the renderer and the provider adapter differ. The Sending-domain gate, DKIM, the
  unsubscribe footer and headers and the open pixel stay email's. No gate is added for the new
  Channels (10DLC, sender name registration): those are the customer's at their provider.
- **One Outbound message per Contact and send.** On Telegram and web push the message goes to every
  Binding the Contact has on the chosen Integration, each passing Send-eligibility on its own; it is
  Sent when one of them was accepted, and dead Bindings are removed on the way. A per-Binding
  provider id is not kept, as neither Channel reports delivery. A Broadcast recipient still ends in
  exactly one Outbound message.
- **The route names one Integration.** A send uses the Integration it names or the Channel's
  default. On a Channel of Bindings it reaches only that Integration's Bindings; a Contact with
  none, with no phone on SMS or WhatsApp, or with no subject_id on mobile push is Skipped
  (`no_destination`).
- **Delivery results per Channel.** System events `<channel>.sent` everywhere; `.delivered` and
  `.undelivered` on SMS and WhatsApp, `whatsapp.read`, and `mobile_push.delivered` when the push
  service reports it. A Delivery notice gains the kinds `read` and `undelivered`; the Outbound
  message gains `read_at` and `undelivered_at` beside `delivered_at`, and `bounced_at` and
  `complained_at` stay email's. A report of unknown state stamps nothing.
- **Errors sort by whom they concern.** An account or credential error (Telegram 401, a rejected
  SMS key, a blocked WhatsApp number) holds the Integration, like SES's `shutdown`. A rate signal
  (429, `retry_after`, throttled) is a Deferral. An error about one Destination is that message's
  failure or skip.
- **An unavailable WhatsApp template holds its source.** When Meta pauses or disables the template
  a Broadcast or an Automation step references, the source is held with the reason
  `template_unavailable` rather than failing every recipient; the send retries one message about
  an hour later (Meta's pauses last 3 and 6 hours) and resumes once it is accepted. A template
  disabled for good keeps the hold until the author picks another.

## Consequences

- Reports of Telegram and web push can show sends, Bindings reached and clicks, never deliveries.
- A Contact with three subscribed browsers counts once in a Broadcast's figures.
