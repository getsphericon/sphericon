---
status: accepted
---

# A test send falls back to the System sender while a Workspace has no verified Sending domain

ADR 0010 makes a verified Sending domain the only way to send and ADR 0015 puts the Test send behind
that gate, so a new Workspace cannot see one email until its DNS has propagated, which no one in
sphericon controls. Resend lets a new account send from a shared test address to its owner first, and
Loops makes the domain step one and the user waits. We take the first: while a Workspace has no
verified Sending domain, a Test send goes from the System sender (the Instance's own `SPHERICON_SYSTEM_EMAIL_*`
sender, the one that sends verification and reset mail), and only to the address of the User who asks
(a Verified User, ADR 0030). With a verified domain the Test send is the ordinary one, through the
Integration and the Workspace's domain. Nothing else changes: Broadcast, Automation and Transactional
still need a verified domain, and the gate stays one place in Outbound send.

The System sender is not DKIM-signed by sphericon (platform mail has no workspace Sending domain to
sign with); it authenticates through its provider, as the SaaS's SES identity does.
So this Test send is not the mail the Workspace will send, and the editor says so: it shows the From it
used.

(Amended by ADR 0058: the fallback applies while the default Integration delivers outside, or there is none.
Through a Test inbox, which sends nothing outside, the Test send goes through the Integration and is captured.)

## Considered options

- **Keep the gate on Test send (ADR 0015 as written).** No exception, but the first email waits on DNS
  and the Setup checklist cannot show the product working before it.
- **Let any From through for a Test send.** Breaks "From is a verified Sending domain plus a local
  part", so a Workspace could put a domain it does not own in the From.
- **Seed a trusted dev domain (the open point (2) in ADR 0010).** Fixes local Mailpit only, not a real
  Workspace.

## Consequences

- A Test send from a Workspace with no verified domain needs no Integration; the Setup checklist's
  first two steps gate sending to Contacts, not seeing a mail.
- It is a new way for a Workspace to cause mail from the platform's own domain, so it is capped per
  User and per Workspace (the spec sets the numbers), not by the Integration's limit (ADR 0023).
  A suspended Workspace still sends nothing, test or not (ADR 0015).
- An Instance with no outbound mail has no System sender, so there the Test send is refused with
  that reason, never silently dropped.
- This answers the onboarding path left open in ADR 0010 (point 1) for a new Workspace. It does not
  close point (2): a Broadcast in local dev still needs a verified domain.
