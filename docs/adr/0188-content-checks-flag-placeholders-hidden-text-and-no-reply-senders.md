---
status: accepted
build: none
---

# Content checks flag placeholders, hidden text and no-reply senders, and not weight

> Amends [[0130-deliverability-is-observed-through-feedback-ids-gmails-postmaster-data-and-content-checks]]
> (the list of Content checks) and [[0040-email-content-is-html-authored-in-an-embedded-editor]] (the
> lint's warnings).

## Context

The Content checks of ADR 0130 include "rendered HTML over 102 KB, which Gmail clips", and ADR 0040
lists "weight" among the lint's warnings, while the lint itself does not check weight: no first-party
source gives a size at which a mail client clips or penalizes a message. Mistakes that do reach
recipients are not checked: the starter Templates link their buttons to `https://example.com`, an
element hidden by inline style stays in the HTML and is read by filters and by AI summaries, invisible
characters ride along in a pasted subject, a `noreply@` sender refuses the replies that signal a wanted
message, and a preview text that repeats the subject wastes the line.

## Decision

- **Added Content checks**, warnings like the others, in the editor, the lint, at Schedule and Publish
  and on save of a Template:
  - a link or an image at a name reserved for examples and tests (RFC 2606 and RFC 6761: `example`,
    `example.com`, `example.net`, `example.org` and their subdomains, `.test`, `.invalid`,
    `.localhost`);
  - text hidden from the reader by inline style, other than one preheader before the visible content;
  - invisible characters in the subject or the text: format characters (Unicode category Cf) and the
    tag block (U+E0000–U+E007F);
  - a From or a Reply-To whose local part is a no-reply word (`noreply`, `no-reply`, `donotreply`,
    `do-not-reply`);
  - a preview text equal to the subject.
- **Weight is not checked**, and the 102 KB rule goes: a rule needs a first-party source.
- **Not checked either**: dark-mode colours, the size of tap targets, image weight and the number of
  columns. Each depends on the client that renders the message, and none has a first-party threshold.
- **The Brand kit gains the business's website**, and the starter Templates link their buttons to it.
  A Brand kit with none leaves them at `https://example.com`, which the first check above flags: a
  button needs a link to be saved (ADR 0040), so the placeholder stays, and it no longer passes
  unnoticed.

## Considered options

- **A check that a Transactional message carries no promotion**: whether a message's primary purpose
  is commercial is the sender's legal judgement, and no rule decides it from the HTML; where a platform
  checks it, a person reviews each message.
- **Stripping invisible characters**: the app does not rewrite what the author wrote; it says so.
