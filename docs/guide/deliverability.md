# Deliverability and consent

sphericon treats staying out of the spam folder, and out of trouble, as part of the product rather
than an add-on. This page describes what happens for you and what you set up once.

## Sending domains

You can only send from a domain you control and have verified. Add the domain in **Settings →
Sending domains**; sphericon generates a DKIM keypair, you publish one TXT record at
`selector._domainkey`, and verification flips the domain on. From then on every message is
signed by sphericon itself, whichever transport carries it. Sending from an unverified domain is
rejected on all three surfaces: broadcasts, automations and transactional mail. The one exception is
a broadcast's test send to your own address, which falls back to the platform's sender until a
domain is verified ([Quickstart](/guide/quickstart#_3-see-an-email-before-dns)). DNS can take time
and nothing else in setup waits for it.

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

If the row stays **Pending** with a red note that another workspace has already verified the host,
the same `track.<domain>` is held by a different workspace on this instance (a host belongs to one
workspace). Your CNAME may be perfectly right, but the host cannot be yours until that workspace
releases it, so use a sending domain of your own. The note is cleared by the next check that no longer
loses the host, or finds the CNAME gone. The same fact is the `reason` (`host_taken`) of the
`trackingRecord` in the API.

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

Bounces and complaints reach the suppression list only through an Integration's **hook**: the
provider posts a callback to the Integration's hook URL, and a **Preset** verifies the request and
turns it into a hard bounce or a spam complaint for one address. Only permanent bounces and
complaints suppress an address; a soft bounce still counts in the deliverability rates. The design is
recorded in
[ADR 0037](https://github.com/getsphericon/sphericon/blob/main/docs/adr/0037-inbound-provider-callbacks-are-presets-on-an-integration.md).

::: warning
Plain SMTP with no provider callback gets no Suppression: sphericon never learns that a message
bounced. SMTP alone is for tests and small volumes; for real sending use a provider below, connected
through its hook.
:::

Each Integration has its own hook URL (`hookUrl`, shown on every read of the integration). To connect
a provider, open **Settings → Integrations**, choose **Inbound** on the Integration, copy the **Hook
URL**, choose the **Preset**, fill its params as JSON and **Save**. A secret is stored encrypted and
never shown again. **Regenerate URL** replaces the hook URL at once; paste the new one into the
provider. Changing a secret keeps the old one valid until you choose **Retire old secrets**, so you
can rotate without dropping callbacks. The Workspace also tracks bounce and complaint rates per
sending domain; read them from `GET /api/sending-domains/rates`.

### Amazon SES (`ses`)

Also for SES-compatible services that publish to SNS.

1. In SES, publish bounce and complaint notifications to one SNS topic.
2. Subscribe an HTTPS endpoint on that topic with the Integration's hook URL. Sphericon confirms the
   subscription itself once the request is verified.
3. On the Integration, choose the `ses` Preset and set `topic_arn` to that topic's ARN. A signed
   notification from any other topic is refused, and without a topic every notification is refused.

### Postmark (`postmark`)

Postmark does not sign webhooks; the Preset checks HTTP basic auth.

1. On the Integration, choose the `postmark` Preset and set `user` and `secret` (the basic auth user
   name and password you pick).
2. In Postmark, for each message stream add a webhook with the URL
   `https://user:secret@host/hooks/key` (the hook URL with the user and password in front of the
   host), enabling the **Bounce** and **Spam complaint** triggers.

Hard bounces and bad addresses are permanent; soft, transient and blocked bounces are not and never
suppress. Other record types are ignored.

### Mailgun (`mailgun`)

1. In Mailgun, copy the account-wide **HTTP webhook signing key** (Settings, API Security; it is not
   an API key).
2. On the Integration, choose the `mailgun` Preset and set `signing_key`.
3. In Mailgun, add the hook URL as the webhook for the **permanent failure**, **temporary failure**
   and **spam complaint** events (`failed` and `complained`).

A `failed` event with severity `permanent` is a permanent bounce; every other event is ignored.

### Any other sender (`generic`)

For a source that can post JSON and sign it with
[Standard Webhooks](https://www.standardwebhooks.com/). Set `secret` to the `whsec_` signing secret.
The body is one failure or an array of them, each `{id, destination, kind, permanent, sending_domain?}`
with `kind` `bounce` or `complaint`.

### A provider with no Preset

On a self-hosted Instance, put your own Preset file in the directory named by `SPHERICON_PRESETS_DIR`
and choose it by name on the Integration. See [Presets](/guide/presets) for the format and
`sphericon preset test`.
