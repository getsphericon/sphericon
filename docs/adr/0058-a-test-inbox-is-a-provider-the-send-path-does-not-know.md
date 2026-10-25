---
status: accepted
---

# A Test inbox is a provider the send path does not know

A Workspace cannot see what its mail will look like and do until a real provider and its DNS exist, and
the development stack and the end-to-end suite lean on an external Mailpit for the same need (ADR 0024).
We add a **Test inbox**: a third email provider of an Integration, beside `smtp` and `ses`, that sends
nothing outside and keeps each message as a **Captured message** the Workspace reads in the app and
through the API. It is chosen, set as default and rate-limited like any other Integration, and the
Outbound send path (ADR 0015) has no case of its own for it: no comparison of a provider's name, only
what its descriptor in the catalog says.

- **The gate follows a property, not a name.** A descriptor says whether the provider delivers outside.
  The Sending-domain gate (ADR 0010) and DKIM signing apply to providers that do, so a Workspace can send
  a Broadcast, an Automation step and a Transactional message through a Test inbox before any DNS exists.
  Nothing leaves, so a domain the Workspace does not own cannot be put in a From that reaches anyone.
- **The ceiling is the provider's own.** A Test inbox reports 200 messages per rolling 24 hours, the
  mechanism SES already uses (ADR 0023), equal to what it keeps; the rest are Deferred. A Workspace has at
  most one.
- **Content lives in one new place.** A Captured message holds the headers, parts and links, owned by the
  Workspace and the Integration, for 7 days and at most 200 per Workspace. An Outbound message still holds
  no rendered content. Erasure deletes the Captured messages of the erased Destinations and a Privacy
  request counts them.
- **It does what a delivery does.** A send through it yields a `delivered` Delivery notice. A message
  opened in the app is a real open and its links are the real tracker's, so open and click tracking can
  be tried end to end. No bounce or complaint is simulated.
- **It is never chosen for anyone.** It is created by the Workspace on purpose, never becomes the default
  by itself, and every report of a send through it says "captured, not delivered". Any member reads and
  deletes Captured messages.
- **The System sender gets the same mode** (`inbox`), kept per Instance and shown to the Instance's
  owners; it is refused in production unless the Self-hoster asks for it, as the local relay is.

## Considered options

- **A shared Mailpit as part of the hosted offering.** One inbox for every customer leaks their mail to
  each other, and SMTP does not pass the HTTP load balancer.
- **A mode of the Workspace that overrides every Integration.** It would put a case in the send path and
  make "which Integration sent this" unanswerable.
- **Keep the Sending-domain gate for the Test inbox.** The first email would wait on DNS again, which is
  what the Test inbox exists to avoid.

## Consequences

- Mailpit leaves the end-to-end suite and the development stack once the Test inbox covers them, in
  three steps: the provider, the Captured message and the Workspace screens first; the System sender's
  mode and the Instance screen next; the end-to-end suite and the dev stack last, with a decision that
  replaces ADR 0024's choice of Mailpit.
- The end-to-end suite stays black-box through the public API: it reads Captured messages from the
  contract instead of an SMTP inbox.
