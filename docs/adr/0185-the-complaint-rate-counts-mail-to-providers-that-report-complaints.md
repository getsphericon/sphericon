---
status: accepted
build: [1729]
---

# The Complaint rate counts only mail to mailbox providers that report complaints

> **Revised before it was built:** the reporting providers are named (Microsoft's consumer mail and
> the Yahoo family), not "every provider but Gmail"; complaints and deliveries come from the same
> providers; the provider is read when the rate is read, so a domain not yet looked up heals; the
> floor and the detector are stated.

> Amends [[0053-delivery-and-engagement-events-cover-every-send]] (the Complaint rate's denominator),
> [[0011-deliverability-rate-metrics]] and
> [[0130-deliverability-is-observed-through-feedback-ids-gmails-postmaster-data-and-content-checks]]
> (Gmail's spam rate is shown beside the Workspace's own).

## Context

The Complaint rate is `complaints / delivered` over all delivered mail (ADR 0053). Gmail sends no
per-message complaint (ADR 0130 says so): a complaint at Gmail never reaches the numerator, while mail
delivered to Gmail fills the denominator. The more of a list is at Gmail, the lower the rate reads, and
the EE abuse detector's 0.1% and 0.3% tiers (ADR 0026) read that rate. Amazon SES computes its own
complaint rate "on mail sent to domains that send complaint feedback"; marketing platforms that divide
by all delivered mail tell their users that Gmail is missing from it.

## Decision

- **A recipient domain has a Mailbox provider**, read from its MX hosts against a fixed table of the
  providers' mail-exchanger names: Gmail and Google Workspace, Microsoft's consumer mail (Outlook.com,
  Hotmail), Microsoft 365, the Yahoo family (Yahoo, AOL and the domains Yahoo hosts), Apple iCloud, and
  "other". The table records, for each, whether it sends per-message complaints to senders' providers.
- **Only Microsoft's consumer mail and the Yahoo family count as reporting.** They are the two
  feedback loops every email provider the app sends through receives; Gmail and Google Workspace send
  aggregates only, Apple sends none, Microsoft 365 tenants are outside Microsoft's loop, and the
  smaller loops depend on each provider's own enrollment, which the app cannot see.
- **The Complaint rate is complaints / deliveries, both to reporting providers**: the same mail on
  both sides, so the rate stays in [0, 1]. A complaint from elsewhere still suppresses its address; it
  is just not in the rate.
- **The provider is read when the rate is read**, from an Instance-wide record per recipient domain
  that no Workspace owns, filled by a job that looks up the domains that delivery Events name and
  refreshes them. A domain not yet looked up is not counted, and is counted once it is, so a new list
  of custom domains heals within the job's period. The record holds a domain and its provider only,
  with no link to a person or a Workspace, and a domain no delivery named for the Event retention
  window is deleted.
- **The rate shows how much mail it covers**: the share of the window's deliveries that went to
  reporting providers, beside it. Where the Workspace has connected Postmaster Tools, Gmail's own spam
  rate (ADR 0130) is shown beside too, labelled as Gmail's.
- **The complaint rate's volume floor counts the reporting deliveries** (1,000 in the window, as ADR
  0026's floor); the bounce rate keeps its floor on sent mail. A Workspace whose mail is mostly at
  Gmail or in Microsoft 365 may not reach it: its complaint rate is then undefined, the EE detector's
  complaint tiers do not fire on it, and Gmail's own rate is the signal there. The tiers' thresholds
  (0.1% and 0.3%) do not change: they are the reporting providers' own bulk-sender thresholds, now
  read on the mail they are about.

## Considered options

- **Keep all delivered mail**, with a warning: the number automated
  decisions read stays wrong in the direction that hides a problem.
- **The recipient address's domain alone** (`gmail.com`): misses every business domain whose mail
  Google hosts.
- **Every provider but Gmail as reporting**: Apple and Microsoft 365 send nothing, and the rest
  depend on enrollments the app cannot see, so the rate would still read low.
- **The provider stamped on the delivery Event when it is recorded**: a domain the job has not looked
  up yet would stay unknown for good.

## Consequences

- The detector's tiers read the corrected rate; their thresholds do not change.
- The same Mailbox provider later splits the other rates by provider.
- A Workspace sending mostly to Google-hosted or Microsoft 365 business mail sees its complaint rate
  undefined until Gmail's own rate is connected.
- Glossary: **Mailbox provider**, **Complaint rate**.
