---
status: accepted
build: none
terms: [Mailbox provider]
---

# The Complaint rate counts only mail to mailbox providers that report complaints

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
  large providers' mail-exchanger domains (Gmail and Google Workspace, Microsoft, Yahoo and AOL, Apple,
  and the rest as "other"). It is looked up off the send path, cached per domain and refreshed, and
  carried by the `email.delivered` Event of a message to that domain; a domain not yet looked up is
  "unknown".
- **The Complaint rate's denominator is mail delivered to mailbox providers that report complaints**:
  every provider but Gmail. Its numerator is unchanged. "Unknown" counts as reporting, so the rate never
  reads higher for a lookup not yet made.
- **Gmail's own spam rate (ADR 0130) is shown beside it**, labelled as Gmail's, where the Workspace has
  connected Postmaster Tools; where it has not, the rate says that the share of mail at Gmail is not in
  it, with that share.
- The rate stays a flow rate with its volume floor; the floor counts the reporting deliveries.

## Considered options

- **Keep all delivered mail**, with a warning: the number automated
  decisions read stays wrong in the direction that hides a problem.
- **The recipient address's domain alone** (`gmail.com`): misses every business domain whose mail
  Google hosts.

## Consequences

- The detector's tiers read the corrected rate; their thresholds do not change.
- The same Mailbox provider later splits the other rates by provider.
- Glossary: **Mailbox provider**, **Complaint rate**.
