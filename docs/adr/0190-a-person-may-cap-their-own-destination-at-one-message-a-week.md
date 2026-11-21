---
status: accepted
build: [1793, 1794, 1795, 1796]
terms: [Personal frequency cap]
---

# A person may cap their own Destination at one marketing message a week

> **Amended by [[0121-a-preference-page-per-destination-and-only-the-person-lifts-a-refusal]]:**
> clearing the cap takes the proof value only the page's verification constructs; a Merge records the
> cap again on the Destination the survivor keeps, with the provenance `merge`, as it records a
> refusal.

> Amends [[0121-a-preference-page-per-destination-and-only-the-person-lifts-a-refusal]] (a choice on
> the page besides opting out) and [[0137-a-frequency-cap-limits-marketing-messages-per-destination]]
> (a cap the person sets).

## Context

The Preference page offers a person two ways to hear less: opt out of a source, or of everything. A
person who wants fewer messages, not none, has only the second, and mail that stops for good is lost
for the sender too. Most platforms store a frequency answer and leave it to the sender to honour
by hand, or offer a topic or digest the sender builds; few enforce a frequency the person sets. No
regulator or mailbox provider requires the choice, and every one that allows a menu requires the
full opt-out beside it; the M3AAWG sender practices recommend offering a frequency, and the manuals
agents load say to offer "fewer" and "snooze" before "leave". The app already has the one enforcement point a frequency needs: the Frequency cap's check
and claim of an Outbound message (ADR 0137, 0178).

## Decision

- **A Personal frequency cap**: the Preference page of an email or Telegram Destination (the
  Channels where the page acts, ADR 0121) offers "at most one a week": one marketing message per
  rolling seven days. It is held per (Workspace, Channel, Destination), a closed value (`weekly`), so a
  later choice is a new value, not a free number.
- **It is read where the Frequency cap is read**, in the same check and claim and under the same
  lock (ADR 0137, 0178): it counts the same marketing messages to that Destination, and the stricter of
  it and the Workspace's cap holds. Over it a Broadcast's Recipient is Skipped and an Automation's send
  is deferred, for one period of the cap that holds it at most, then Skipped. The reason is
  `personal_frequency_cap`, apart from `frequency_cap`, so a report says the person asked for it.
  Transactional messages, confirmations and the page's fresh links are outside it, as they are outside
  the Frequency cap.
- **Setting it narrows, clearing it widens** (ADR 0121): anyone may set it (the person from any link
  however old, a member, the API, MCP), and only the person clears it, on fresh proof of the address.
  The contract has no operation that clears it. Each change is an evidentiary Event
  (`<channel>.frequency_reduced`, `<channel>.frequency_restored`).
- **It goes with the Destination**: an Erasure clears it (ADR 0021), and a Merge keeps it, since it is
  the Destination's, not the Contact's (ADR 0128); being a narrowing, it is also recorded again on the
  Destination the survivor keeps on that Channel, with the provenance `merge`, as a refusal is.

## Considered options

- **A snooze that ends by itself** (pause for 30 or 90 days): its end widens reach with no fresh proof
  of the address, which ADR 0121 forbids; no comparable platform enforces one, and no regulator or
  mailbox provider asks for one. A person who wants a pause opts
  out of the source and returns when they choose.
- **A stored answer the sender honours by hand**: the page would promise what the app does not keep.
- **A free number per period**: a choice the person has to compute, and a cap that can be looser than
  the Workspace's for nothing.
- **A digest series kept when the person leaves `broadcasts`**: leaving `broadcasts` leaves every
  series (ADR 0135), and the digest is the sender's to build.

## Consequences

- A Skipped reason and a Deferral cause gain `personal_frequency_cap`.
- Glossary: **Personal frequency cap**, **Frequency cap**, **Preference page**.
