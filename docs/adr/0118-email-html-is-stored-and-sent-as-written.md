---
status: accepted
---

# Email HTML is stored and sent as written

> Replaces point 8 of [[0040-email-content-is-html-authored-in-an-embedded-editor]] ("The HTML is
> sanitized twice").

ADR 0040 sanitized an email body twice: where it was saved, and again on each recipient's rendered
message. The sanitizer was bluemonday with an allowlist of email markup, plus our own handling of
what bluemonday drops or cannot read: Liquid tags masked so stored HTML survived byte for byte,
comments, `<style>` blocks and the doctype lifted out and checked on their own, the HTML inside an
Outlook conditional comment sanitized with the VML elements allowed, and checks of the CSS of a
style attribute and of a style block.

None of it protects anyone:

- **The authors are the Workspace's own members**, writing to the Workspace's own audience. What a
  body can do to a recipient, its author can do by writing it, with or without the sanitizer.
- **A mail client runs no script.** Gmail, Outlook and Apple Mail drop scripts, event handlers,
  forms and `javascript:` URLs themselves; the sanitizer removed what never runs.
- **The app shows a stored body in one place**, the rendered tab of a Captured message, inside an
  `<iframe sandbox="">`: no script runs and the frame has no access to the app's origin. The
  editor opens the document (JSON), never a body's HTML. A future view of a message in the app is
  held to the same rule.
- **A Contact's data is escaped by Liquid**, where the body writes it, not by the sanitizer.

## Decision

- **A body is stored as it was written**, on every entry point (site, external API, MCP), for a
  Template, a Broadcast and an Automation's email step. A save refuses only what cannot render: a
  Liquid syntax error or HTML that does not parse (`emailrender.CheckFields`).
- **A message is sent as its body renders**: Liquid runs, values are HTML-escaped as they are
  written, and the result is the HTML part. `{{ value | safe }}` writes a variable's markup as it is.
  An attribute that takes a URL (`href`, `src`) is written as the variable gives it: Liquid escapes
  its characters, not its scheme.
- **Any view of a message's HTML in the app is a sandboxed iframe** (`sandbox=""`), never markup
  inserted into the app's page.
- **The lint no longer names an element the sanitizer removed** (`unknown_construct` is gone): a
  save removes nothing. It keeps its checks of the message's quality: alt text, contrast, the Brand
  kit.
- The editor document's checks stay where they are rules of the document (an absolute link, a style
  that is a string), not a guard against its author.

## Consequences

- The body an author saved is the body they read back and the body that is sent, byte for byte; the
  masking and lifting the sanitizer needed are gone, with bluemonday.
- A Telegram text is still cut to Telegram's HTML subset: that is the Bot API's format, not a
  sanitizer.
