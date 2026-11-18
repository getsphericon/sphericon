---
status: accepted
build: none
---

# SPF belongs to the Integration and is shown and checked per Integration

> Amends [[0010-sending-domains-native-dkim]] (where the suggested SPF record comes from, and what
> "status-checked" means).

## Context

ADR 0010 gates sending on DKIM alone and says SPF and DMARC are "generated, shown, and status-checked".
SPF is not checked anywhere, and the record shown is `v=spf1 include:amazonses.com ~all` on the domain's
apex whatever carries the mail: a Workspace sending through an SMTP server, Postmark or Mailgun is told
to publish Amazon's include, which does nothing for it.

SPF is checked on the envelope sender (`Return-Path`), which stays the provider's (ADR 0131). It passes
on the provider's domain, unaligned with the From; DMARC passes through the aligned DKIM signature. Gmail
asks bulk senders for SPF and DKIM, and Outlook.com refuses bulk mail that fails SPF, DKIM or aligned
DMARC. Most providers offer a custom envelope domain on a subdomain of the sender's (Amazon SES's custom
MAIL FROM, Postmark's Return-Path, Mailgun's sending domain), which makes SPF pass aligned too.

## Decision

- **The provider says what SPF it needs.** Each provider of the messaging catalog declares, for a
  Sending domain, the DNS records of its envelope domain: a custom envelope subdomain with its records
  where the provider offers one, else the include its servers need, else nothing (a generic SMTP server,
  whose servers the app does not know). The app keeps no provider's include outside its descriptor.
- **A Sending domain shows the records of every Integration that sends from it**, and checks each one
  with the daily re-check of ADR 0010; a record missing or wrong is an item of Needs attention, as a
  missing DMARC record is (ADR 0012).
- **DKIM stays the one gate.** SPF never holds or refuses a send: the app does not control the
  provider's servers, and the aligned DKIM signature already passes DMARC.
- **An aligned envelope subdomain is the recommended setup**, shown first where the provider has one.

## Considered options

- **No SPF guidance**, as the provider's business: Gmail and Outlook.com still ask for SPF, and the
  sender learns it only from rejections.
- **SPF as a gate**: halts a domain over a record that depends on a third party's servers.

## Consequences

- ADR 0010's "status-checked" becomes true; the apex-wide Amazon include goes.
