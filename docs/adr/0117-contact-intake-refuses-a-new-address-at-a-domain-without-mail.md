---
status: accepted
---

# Contact intake refuses a new address at a domain without mail

A list brought from an earlier system is the one way unchecked addresses enter an audience in bulk:
a form's address is proven by its Confirmation, an address typed by a member is one at a time. So
the Contact intake (`POST /contacts/batch`, ADR 0022) checks each row's email address before the
row is stored. Other paths are not checked.

**Refused:** a row that brings an address new to the Workspace at a domain that cannot receive
mail. DNS says so definitely: the domain does not exist (NXDOMAIN), it has neither an MX record nor
an address record that serves as its implicit MX (RFC 5321 section 5.1), or it publishes a null MX
(RFC 7505). The row fails with `ErrEmailDomainNoMail`; the others go on.

**Accepted with a warning:** an address at a disposable mailbox provider, or an address of a role
rather than a person (`info@`, `sales@`). Both are real mailboxes: in B2B a role address is often the
one the customer has. The row is stored and its result carries `disposable_email` or `role_email`.

**Accepted silently:** a domain DNS gives no definite answer for (a timeout, SERVFAIL, the lookup
budget spent). Nothing proves it wrong, and a resolver's outage must not empty an import.

## How

- The domains are read once per batch, outside every row's transaction: one lookup per distinct
  domain, at most 16 at a time and 5 seconds for all of them, so a row's transaction never waits
  on DNS.
- The DNS is the host's resolver, not `SPHERICON_DNS_ADDR`: that server publishes only the
  Instance's own proof records (Sending and Join domains), and in the development stack it answers
  NXDOMAIN to everything else.
- The disposable and role lists come from `github.com/AfterShip/email-verifier`, compiled in. Its
  other checks are not used: its MX check takes no context and knows neither the implicit nor the
  null MX, its SMTP probe would connect to the recipient's server from the Instance, and its list
  auto-update would fetch from the internet. Nothing of it touches the network.
- The rule lives in `internal/kernel/addresscheck`; the contacts module applies it in
  `UpsertBatch`. The e2e suite and the test harness give every domain a mail exchanger, as their
  addresses are at no real domain (`app.WithE2EDNS`, formerly `WithE2EDKIMLookup`;
  `testhelper.WithMailResolver`).

## Consequences

- **An import stays repeatable.** An address a Contact already holds is past the check, so sending
  the same row again updates the Contact whatever DNS answers now.
- **A self-hoster's resolver decides.** An Instance whose resolver answers NXDOMAIN for names
  outside its own network refuses every new address. There is no switch to turn the check off;
  one is added when such an Instance appears.
- The disposable list ages with the library's release; it is refreshed by upgrading the dependency.
- The disposable list costs about 13 MB of memory in every process that loads the contacts module.
