# Deliverability and consent

sphericon treats staying out of the spam folder, and out of trouble, as part of the product rather
than an add-on. This page describes what happens for you and what you set up once.

## Sending domains

You can only send from a domain you control and have verified. Add the domain in **Settings →
Sending domains**; sphericon generates a DKIM keypair, you publish one TXT record at
`selector._domainkey`, and verification flips the domain on. From then on every message is
signed by sphericon itself, whichever transport carries it. Sending from an unverified domain is
rejected on all three surfaces: broadcasts, automations and transactional mail.

### Tracking domain

By default the open pixel and click links in your mail point at your sphericon instance's own host.
To put them on your own hostname, open **Settings → Sending domains**, expand the domain and add the
**Tracking domain (optional)** row next to the DKIM, SPF and DMARC records. The row appears only when
your instance has an edge set up for it (`SPHERICON_TRACKING_CNAME_TARGET`, see
[Self-hosting](../self-hosting#tracking-domains)); on Sphericon Cloud it is always there.

1. At your DNS provider, add a `CNAME` record. The host is `track.<your sending domain>` (a fixed
   name, you cannot pick another label) and the value is the edge host shown in the row.
2. Click **Verify** (it checks the tracking CNAME and HTTPS along with the DKIM record), or wait: a
   domain that is DNS verified or Serving is rechecked every 15 minutes. A **Pending** domain is
   only checked when you click **Verify**.
3. The row shows where the domain stands:
   - **Pending**: the CNAME is missing or does not point at the edge host yet.
   - **DNS verified**: the CNAME is right, so the edge may now get a certificate for the host. Mail
     still uses the instance's host.
   - **Serving**: sphericon fetched `https://track.<domain>/.well-known/sphericon-tracking` and got
     the expected answer, so HTTPS works. New mail from this sending domain now builds its links on
     the tracking domain, so the link host matches your From domain.

A certificate takes time to issue, so a domain stays in **DNS verified** until HTTPS answers. Until the
domain is **Serving**, no message carries a link that could fail TLS. If a later check finds the CNAME
gone or HTTPS broken, the domain drops back and new mail goes out on the instance's host again; the
workspace Owners are told.

The tracking domain serves only link paths: clicks, opens and the one-click unsubscribe. The
unsubscribe and confirm pages redirect to the instance's own host, and everything else on it answers 404.

**Removing a domain.** If you delete the sending domain, rename it or remove its CNAME, links already
in recipients' inboxes keep working through the instance's own host, but the same links on
`track.<domain>` stop reaching sphericon. Keep the CNAME in place for as long as old mail may still
be opened or clicked. The design is in [ADR 0038](../adr/0038-tracking-domains-are-core-and-the-edge-is-not.md)
and [ADR 0036](../adr/0036-paths-embedded-in-sent-mail-and-customer-sites-outlive-a-major.md).

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
stop mailing an address after it bounces. The workspace also tracks bounce and complaint rates
per sending domain; read them from `GET /api/sending-domains/rates`.
