---
status: accepted
build: none
---

# What needs attention reaches a Webhook endpoint that asks for it

> Amends [[0073-the-overview-leads-with-what-needs-attention]] (the job that remembers which items
> are open) and [[0105-a-notice-is-a-reaction-to-a-domain-event-and-an-event-need-not-belong-to-a-workspace]]
> (what leaves through a Webhook endpoint).

## Context

What stops a Workspace's mail (a Sending domain that lost its DKIM record, a provider that stopped the
account, a held Broadcast, an Automation that cannot send, a suspension) is shown in the console, in
Needs attention and the banners, and some of it is mailed to the owners. None of it reaches a machine:
only Events and Audit entries leave through a Webhook endpoint (ADR 0105), and these changes are
neither. An agent or a monitoring system that drives a Workspace learns that sending stopped only by
polling. Email platforms tell owners by email or in the app; Amazon SES's Virtual Deliverability Manager is
the machine-readable precedent: it publishes an advisor recommendation as "open" and "resolved".

## Decision

- **Needs attention publishes its items' changes**: `needs_attention.opened` when an item appears and
  `needs_attention.resolved` when it goes, each with the item's kind, severity and the object it is
  about. Every kind of ADR 0073 is covered, and a kind added later is covered with no further work.
- **Needs attention stays a live read.** A job per Workspace, run every few minutes, reads it and
  compares it with the set of items it saw open last time, which is the only thing it stores; the
  difference is the events. An item that comes and goes between two runs is not reported.
- **A suspension of the Workspace and its lifting, and a Staff hold and its lifting** are published the
  same way, though they are banners and not items.
- **A Webhook endpoint receives them only when its filter names them**, as an Audit entry (ADR 0105):
  an endpoint with no filter keeps receiving the Events alone.

## Considered options

- **An event from each module at the moment of its change**: a new event per kind, and a kind that
  forgets to publish one is silently missing; Needs attention already says in one place what needs a
  person.
- **Owners' email and the console only**: the agent that started a send
  cannot learn that it stopped.

## Consequences

- Two event types for a Webhook endpoint's filter, described in the contract.
