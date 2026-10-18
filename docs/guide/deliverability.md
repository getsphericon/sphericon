# Deliverability and consent

sphericon treats staying out of the spam folder, and out of trouble, as part of the product rather
than an add-on. This page describes what happens for you and what you set up once.

## Sending domains

You can only send from a domain you control and have verified. Add the domain in **Settings →
Sending domains**; sphericon generates a DKIM keypair, you publish one TXT record at
`selector._domainkey`, and verification flips the domain on. From then on every message is
signed by sphericon itself, whichever transport carries it. Sending from an unverified domain is
rejected on all three surfaces: broadcasts, automations and transactional mail.

## Who may be mailed

There is no "subscribed" status on a contact. Whether a message may reach an address is decided
in layers, for each channel and destination:

1. **Suppressed** addresses are never mailed. Hard bounces, spam complaints and manual bans end
   up here and are not cleared automatically.
2. An address that **unsubscribed from everything** is never mailed.
3. An address that **unsubscribed from this sending source** is never mailed.
4. If your workspace requires **confirmed opt-in**, an address that has not confirmed is not
   mailed.
5. Otherwise it is sent.

Transactional mail skips layers 2 to 4 but still respects suppression.

Opt-outs are keyed by address rather than by contact, so they survive deleting and re-importing a
contact. That is the minimum data kept to honor a refusal.

## Unsubscribing

Every marketing email carries an unsubscribe link and a one-click `List-Unsubscribe` header
(RFC 8058), which mailbox providers expect from bulk senders. Both opt the person out of the **sending
source** that sent the mail: all broadcasts share one source, and each automation is its own, so
leaving one drip leaves the others untouched. "Unsubscribe from everything" is a separate,
deliberate action.

Opening the link in a browser only shows a confirmation page. The opt-out happens on submit, so
link scanners that visit every URL in an email cannot unsubscribe your readers by accident.

## Confirmed opt-in

Confirmed opt-in is a workspace policy, off by default. When it is on, an address has to confirm
before marketing mail reaches it. The confirmation is recorded as an immutable
`marketing.confirmed` event, which gives you proof of consent and can also trigger a welcome
automation.

## Bounces and complaints

Bounce and complaint notifications from an SES-compatible service over SNS are ingested and written to the suppression list, so you
stop mailing an address after it bounces. Only notifications from the SNS topic named on the SES
integration are accepted; set its **SNS topic ARN** or no bounce is ingested. Each integration has its
own hook URL (`hookUrl`, shown on every read of the integration and in the console under **Inbound**):
subscribe SNS to that URL, not to a workspace-wide one. The workspace also tracks bounce and complaint rates
per sending domain; read them from `GET /api/sending-domains/rates`.
