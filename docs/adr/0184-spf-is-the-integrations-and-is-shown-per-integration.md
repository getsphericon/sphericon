---
status: accepted
build: [1733]
---

# SPF belongs to the Integration and is shown and checked per Integration

> **Revised before it was built:** SPF is the envelope's, which only the provider's own setting changes:
> the app reads SES's custom MAIL FROM instead of suggesting a record on the Sending domain, says
> nothing for an SMTP relay it does not know, and makes an item only of an envelope set and broken.

> Amends [[0010-sending-domains-native-dkim]] (where the suggested SPF record comes from, and what
> "status-checked" means).

## Context

ADR 0010 gates sending on DKIM alone and says SPF and DMARC are "generated, shown, and status-checked".
SPF is not checked anywhere, and the record shown is `v=spf1 include:amazonses.com ~all` on the domain
itself whatever carries the mail. It does nothing in any case: SPF is checked on the envelope domain
(`Return-Path`), which is the provider's (ADR 0131), not the Sending domain.

An envelope aligned with the Sending domain exists only where the provider is told to use one: Amazon
SES's custom MAIL FROM is a setting of the SES identity (a subdomain, with an MX record to SES and an
SPF record), and DNS records alone change nothing; SES checks the MX record continuously, and when it
breaks it either falls back to its own domain or rejects the send, as the identity says. Postmark's
custom Return-Path and Mailgun's sending domain work alike, but they reach the app only as SMTP
servers it does not know. Gmail asks bulk senders for "SPF and DKIM email authentication for your
domain" with DMARC aligned through either; Microsoft asks that SPF "must pass for the sending domain"
without defining it. SPF passing unaligned on the provider's domain meets the letter of neither for
certain; an aligned envelope meets both.

## Decision

- **The Sending domain shows no SPF record of its own.** The apex include goes; a record the domain
  already publishes is left alone.
- **SPF is shown per email Integration of the Workspace**, as each Integration's envelope:
  - **Amazon SES**: the app reads the SES identity of the Sending domain with the Integration's
    credentials (`GetEmailIdentity`): its custom MAIL FROM domain, its status and its behaviour on an
    MX failure. With none, the page recommends setting one at SES and shows the two records it will
    need on a subdomain (the MX to SES's feedback host of the Integration's region and the SPF record),
    suggesting a name that is none of the domain's reply and tracking hosts. With one, it shows that
    domain, SES's status and the two records, each checked against DNS. Credentials that cannot read
    the identity leave the envelope "unknown", as the check that an SES configuration set exists is
    skipped when they cannot ask.
  - **SMTP**: the app does not know which provider relays, so it shows that the envelope and its SPF
    are set at that provider, and no record.
  - **The Test inbox** sends nowhere and shows nothing.
- **Needs attention shows an envelope that is set and broken**: a custom MAIL FROM whose status is
  failed, or whose records DNS does not have; blocked when SES rejects the send on an MX failure,
  degraded when it falls back. An envelope never set is a recommendation on the page, not an item.
- **The checks run with the daily re-check of ADR 0010.** DKIM stays the one gate: SPF never holds or
  refuses a send.
- **The Return-Path stays the provider's (ADR 0131)**: an aligned envelope is a subdomain the sender
  delegates to the provider, which still receives the bounces.

## Considered options

- **No SPF guidance**, as the provider's business: Gmail and Outlook.com still ask for SPF, and the
  sender learns it only from rejections.
- **SPF as a gate**: halts a domain over a record that depends on a third party's servers.
- **A suggested record on the Sending domain**: the envelope is not the Sending domain, so it does
  nothing; and DNS alone never moves SES to a custom MAIL FROM.
- **Setting the custom MAIL FROM at SES from the app**: it would need a write permission on the
  sender's AWS account; reading the identity needs only `ses:GetEmailIdentity`.

## Consequences

- ADR 0010's "status-checked" becomes true; the apex-wide Amazon include goes, and the contract's
  single `spfRecord` gives way to the envelopes of the Integrations.
