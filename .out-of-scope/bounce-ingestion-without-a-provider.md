# Bounce ingestion without a provider (VERP and DSN parsing)

This project does not ingest bounces from a bare MTA (Postfix, Exim, a corporate relay) that has no
provider callback.

## Why this is out of scope

Such a sender reports a bounce only as a DSN mail to the return path. Handling it means VERP return
paths, an inbound mailbox (IMAP or SMTP reception), a DSN parser per MTA dialect and SPF/DKIM alignment
for the return-path domain: a subsystem of its own. Production mail is expected to go through a
provider that calls back (SES-compatible services, Postmark, Mailgun, or any sender that can post a
Delivery failure to the `generic` Preset). See ADR 0037.

## Prior requests

- #325: "Ingest bounces and complaints for plain SMTP"
