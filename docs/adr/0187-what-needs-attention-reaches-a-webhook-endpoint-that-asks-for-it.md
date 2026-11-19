---
status: accepted
build: [1739]
---

# What needs attention reaches a Webhook endpoint that asks for it

> **Revised before it was built:** only blocked items are published (the others open and close with
> normal sending or by the clock), the first run records without publishing, the events and the set
> are written together, and `/api` reads Needs attention for an agent that receives no webhook.

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

- **What stops mail is published as it starts and as it ends**: `needs_attention.opened` when an
  item of severity blocked appears in Needs attention and `needs_attention.resolved` when it leaves,
  each with the item's kind, the object it is about, when it started, and an id stable for that item's
  opening, so a receiver can drop a redelivery. Items that only slow mail or record a past failure
  (the Send cap and Provider limit spent, a failed Broadcast or runs, a disabled Webhook endpoint) are
  not published: they open and close in the course of normal sending, or leave the list by the clock.
  A blocked kind added later is covered with no further work.
- **A suspension of the Workspace and a Staff hold** are published the same way, as two more blocked
  kinds of these events, though the console shows them as banners and not as items.
- **Needs attention stays a live read.** A job per Workspace, run every few minutes and only for a
  Workspace with a Webhook endpoint whose filter names these events, reads it and compares it with the
  set of blocked items it saw open last time; it writes the events and the new set in one transaction.
  Its first run for a Workspace, and its first after no endpoint named them, only records the set: a
  receiver learns the state at that moment by reading Needs attention. An item that comes and goes
  between two runs is not reported.
- **A Webhook endpoint receives them only when its filter names them**, as an Audit entry (ADR 0105):
  an endpoint with no filter keeps receiving the Events alone.
- **`/api` reads Needs attention**, as `/site` does, so an agent over MCP, which receives no webhook,
  can ask what stops the Workspace's mail.

## Considered options

- **An event from each module at the moment of its change**: a new event per kind, and a kind that
  forgets to publish one is silently missing; Needs attention already says in one place what needs a
  person.
- **Owners' email and the console only**: the agent that started a send
  cannot learn that it stopped.

## Consequences

- Two event types for a Webhook endpoint's filter, described in the contract; the kinds of Needs
  attention move to the contract's common part, as `/api` shows them too.
