---
status: accepted
build: [1480]
---

# Definition documents are typed `jsonb` values

## Context

An Automation version's steps, a Rule's Condition and Actions, and the Conditions of a Segment and of an
armed Broadcast are stored as text with an `IS JSON` check. Because they are text, the code compares
text: the audit diff and the unchanged-write check compare the stored string with the written one,
secret rotation swaps a definition only where its exact text still stands, and the Automation version
comparison decodes and re-encodes both sides to compare them. An empty Condition is `''`, meaning every
Contact, and a missing one is nil, meaning none. Nothing queries inside these columns.

## Decisions

- **Each column is `jsonb` holding its Go type** (`field.JSON`): ent encodes and decodes it, so the audit
  diff, the unchanged-write check and a version's comparison compare values, and the canonical
  re-encoding goes.
- **An empty Condition is an empty group, never `''`.** A Condition with no tests, the group
  `condition.Parse` already returns for `''`, matches every Contact; nil means there is no Condition, as
  on a Broadcast to all Contacts (ADR 0149). The check becomes the document's JSON type (an object or an
  array).
- **Secret rotation is a compare-and-swap on the revision.** It reads the row, re-encrypts the secrets in
  the value and writes it on condition that the revision it read still stands (ADR 0119), trying again
  on a miss. The columns stay `Sensitive`.

## Consequences

- A migration converts the columns and replaces `''` with the empty group; the code that reads `''` and
  compares text goes.

## Rejected

- **Text with a Go type that encodes canonically**: it keeps the exact-text comparisons alive, which is
  what this removes.
- **Keeping `''`**: two spellings of "every Contact" that every reader has to know.
