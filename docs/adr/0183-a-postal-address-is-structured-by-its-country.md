---
status: accepted
build: none
---

# A postal address is structured by its country

> Amends [[0156-marketing-email-is-armed-only-with-a-postal-address]] (what counts as an address, and
> how the footer writes it).

## Context

ADR 0156 takes any non-empty text and says that "a PO box or a virtual office address" satisfies
CAN-SPAM. The FTC asks for "your current street address, a post office box you've registered with the
U.S. Postal Service, or a private mailbox you've registered with a commercial mail receiving agency
established under Postal Service regulations": a city and a country are not an address, and a virtual
office counts only as such a mailbox. Free text arms a send with "Berlin, Germany".

Email platforms do not verify an address against a postal service; those that require one ask for it
in fields (a first line, a city, a country, or the fields a country's format names).

## Decision

- **The Postal address is fields, not text**: a country and the lines its format names (address lines,
  locality, region, postal code), stored and returned as such on `/site` and the external Workspace.
- **A country's format decides what is required and how it is written**, from
  `github.com/bojanz/address` (Google's libaddressinput data, CLDR country list): it validates the
  required fields and the postal code's pattern on save, formats the footer line, and serves the formats
  to the console's form. The app keeps no table of its own.
- **A PO box or a commercial mail receiving agency's mailbox is an address line.** Nothing is geocoded
  or checked against a postal service.
- ADR 0156's arming rule, its "changed but not cleared" rule and its earlier signs stand: arming needs a
  Postal address that its country's format accepts.

## Considered options

- **Free text with a heuristic warning** (no digit, one line): a guess about every country's addresses,
  and still a send armed with "Berlin, Germany".
- **Verification with a postal service**: no service covers every country, and email platforms do not
  do it.

## Consequences

- The Workspace setting, the contract's `postalAddress` and the footer change shape; the earlier text
  form is not kept (no Instance holds one).
- Glossary: **Postal address**.
