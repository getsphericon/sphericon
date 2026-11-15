---
status: accepted
---

# Marketing email is armed only with a postal address

> Amends [[0056-marketing-email-is-always-html-with-a-derived-text-part]] (the footer's postal address
> is never empty).

## Context

The footer of a marketing email carries the Workspace's postal address beside the unsubscribe link
(ADR 0012, ADR 0056). When the address is empty the footer prints nothing and the email goes out
anyway: no check warns or stops a Broadcast or an Automation, and the external `GET /workspace` does
not return the address, so an agent preparing a send cannot see it missing.

CAN-SPAM requires a valid physical postal address in every commercial email; a PO box or a virtual
office address satisfies it. Email platforms require the address before a business can send (when its
audience or account is set up) and put it in every footer. An agent driving an email platform is
expected to block a send without the address, as a gate it cannot edit its way around.

DMARC is a warning and not a gate (ADR 0012) because it is a readiness signal about the domain. A
missing address is a fault in the email itself.

## Decisions

- **Arming marketing email needs a postal address.** Schedule and Send of an email Broadcast, and
  Publish and Activate of an Automation with an email send Step, are refused with 409
  `postal_address_required` while the Workspace has none. The check is made where the send is armed, so
  nothing stops halfway through a send and no Hold reason is added.
- **The address can be changed but not cleared while marketing email is armed.** Saving an empty address
  is refused with 409, naming them, while an email Broadcast is `scheduled` or `sending` or an Automation
  with an email send Step is active. With nothing armed it may be cleared.
- **It holds on every Instance and for every Workspace**, whatever the recipients' country: the core
  picks no rule by jurisdiction (ADR 0062), and it does not know the recipients'. It concerns marketing
  email only, not Transactional email and not the other Channels.
- **An address is non-empty text after trimming**, several lines in free form. A PO box or a virtual
  office is fine; nothing is geocoded or checked further.
- **The earlier signs help, they do not decide**: a Content check (ADR 0130) shows the missing address in
  the editor before Schedule or Publish; Needs attention (ADR 0073) shows it while the Workspace has an
  email Integration and no address; the external `GET /workspace` returns `postalAddress`, read-only, so
  an agent sees the gap before it asks for a send. Only a person sets the address, in the console: an
  agent has no address of the business to give.
- **A Test send needs none.** It goes to the sender (ADR 0132), not to recipients, and its Content check
  still shows the gap.

## Consequences

- The refusal `postal_address_required` on the arming operations of `/api`, `/site` and MCP; the
  Workspace settings refuse an empty address while marketing email is armed.
- `postalAddress` on the external Workspace resource.
- Glossary: **Postal address**.

## Rejected

- **A warning only**: an agent or a hurried person sends past it, and the mail goes out unlawful in the
  one place the law names.
- **A Hold reason**: it would stop a send already going out and make the problem known after the fact,
  and add a reason to the contract for a fault that can be refused before anything is armed.
- **A rule by recipients' country**: the app does not know it, and the address costs a sender nothing to
  give.
