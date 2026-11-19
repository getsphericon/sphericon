---
status: accepted
build: [1731]
---

# A postal address is structured by its country

> **Revised before it was built:** the first line is required in every country (four formats leave it
> optional), a postal code off its pattern warns, arming checks presence only, the formats come
> through the contract, and the footer's layout and language are stated.

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

- **The Postal address is fields, not text**: a country (ISO 3166-1 alpha-2) and the lines its
  format names (three address lines, sublocality, locality, region, postal code). "No address" is
  none at all, and clearing it makes it none; a field the country's format does not use is refused.
- **A country's format decides what is required and how it is written**, from
  `github.com/bojanz/address` (Google's libaddressinput data, the CLDR country list): its required
  fields and its region list are checked on save, and a missing one is refused on its own field. The
  first address line is required in every country, over the format, so a city and a country alone
  never pass. A postal code that does not match the country's pattern is a warning, not a refusal:
  the data may lag a real code, and a sender must never be left unable to send.
- **Arming checks only that an address is there** (ADR 0156), never re-validates it: a newer release
  of the format data does not stop a send.
- **The console reads the formats through a typed `/site` operation**, part of the contract like any
  other, and renders the form of the chosen country from it.
- **The footer writes the address in its country's layout**, one line per line of the format, the
  country's name in English like the rest of the footer, and the same lines in the text part.
- **A PO box or a commercial mail receiving agency's mailbox is an address line.** Nothing is geocoded
  or checked against a postal service.
- ADR 0156's arming rule, its "changed but not cleared" rule and its earlier signs stand.

## Considered options

- **Free text with a heuristic warning** (no digit, one line): a guess about every country's addresses,
  and still a send armed with "Berlin, Germany".
- **Verification with a postal service**: no service covers every country, and email platforms do not
  do it.

## Consequences

- The Workspace setting, the contract's `postalAddress` and the footer change shape; the earlier text
  form is not kept (no Instance holds one).
- Glossary: **Postal address**.
