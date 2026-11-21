---
status: accepted
build: [1759, 1760, 1761, 1762, 1763]
---

# Content checks flag placeholders, hidden text and no-reply senders, and not weight

> **Revised before it was built:** Content checks are the lint's warnings; the ADR 0130 checks that
> read the message are folded in; the preheader and its padding, mobile-only blocks, emoji, scripts'
> joiners and flags are exempt; the sender and preview text rules apply where those exist.

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

- **Content checks are the email lint's warnings**: one mechanism, run in the editor, by the MCP tool
  `email_lint`, on save of a Broadcast, an Automation and a Template, and at Schedule and Publish. They
  never block. The lint takes the message's sender beside its subject and body.
- **The checks of ADR 0130 that read the message itself** are part of it: a message whose visible
  content is images with no text; a link or an image not on `https`; a link through a URL shortener; an
  image with no `alt` (already built).
- **Added checks**:
  - a link or an image at a name reserved for examples and tests (RFC 2606 and RFC 6761: `example`,
    `example.com`, `example.net`, `example.org` and their subdomains, `.test`, `.invalid`,
    `.localhost`); a host that comes from a merge tag is decided per recipient and is not checked;
  - text hidden from the reader by inline style, other than the preheader (the first hidden element
    before any visible content, with what it contains) and other than an element that a media query of
    the message's own stylesheet shows (a block for small screens only);
  - invisible characters used to hide text, in the subject or the visible text: Unicode tag characters
    (U+E0000–U+E007F) outside an emoji flag's tag sequence, and the zero-width space, the word joiner
    and the zero-width no-break space (U+200B, U+2060, U+FEFF) anywhere but in the preheader; the
    joiners of emoji and of scripts (U+200C, U+200D) and the direction marks are left alone;
  - a From whose local part is a no-reply word (`noreply`, `no-reply`, `donotreply`, `do-not-reply`),
    where the message has a From (a Broadcast, an Automation's email step); a Reply-To once there is
    one to read (ADR 0131);
  - a document's preview text equal to its subject; a raw HTML body has no preview text to compare.
- **Every starter Template and every export of the editor yields no finding**, its own preheader
  padding included: a check that fires on what the app itself produces is wrong.
- **Weight is not checked**, and the 102 KB rule goes: a rule needs a first-party source.
- **Not checked either**: dark-mode colours, the size of tap targets, image weight and the number of
  columns. Each depends on the client that renders the message, and none has a first-party threshold.
- **The Brand kit gains the business's website**, and the starter Templates link their buttons to it.
  A Brand kit with none leaves them at `https://example.com`, which the reserved-name check flags: a
  button needs a link to be saved (ADR 0040), so the placeholder stays, and it no longer passes
  unnoticed. The document rule's refusal of a button with no link names no example host.

## Considered options

- **A check that a Transactional message carries no promotion**: whether a message's primary purpose
  is commercial is the sender's legal judgement, and no rule decides it from the HTML; where a platform
  checks it, a person reviews each message.
- **Stripping invisible characters**: the app does not rewrite what the author wrote; it says so.
