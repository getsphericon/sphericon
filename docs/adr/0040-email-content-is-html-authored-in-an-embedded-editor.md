---
status: proposed
---

# Email content is HTML, authored in an embedded editor

Proposed, pending the prototype below: Decisions 2, 6, 7 and 8 depend on what it finds.

Message content is a subject plus an MJML body today (ADR 0003, ADR 0005, the glossary). The body field
is a plain `Textarea`, an agent that drafts through MCP gets an MJML string and nothing else (no brand,
no vocabulary, no feedback), and a malformed body fails only when a send is planned. The research in
`docs/research/ai-email-design-landscape.md` and `docs/research/embeddable-email-editors.md` shows that
the vendors whose agents produce good-looking mail narrow what the agent writes, hold the brand as data
and give the agent a lint. It also shows that the one actively maintained MIT editor that fits React 19
and Mantine without a hosted service, `@react-email/editor`, stores its own JSON and emits HTML, not MJML.

## Decisions

1. **One format on every surface.** Templates, Broadcasts, Automation steps, Transactional sends, the
   external API and MCP all carry the same HTML body. There is no second input that accepts raw MJML, and
   `gomjml` leaves the build. The project is greenfield: no conversion of existing bodies, no guard for
   them, fixtures and callers are rewritten. The `body` column stays a string, so there is no migration.
2. **The stored source of truth is the HTML.** One `body` field holds email-ready HTML with Liquid inside.
   No editor JSON is stored beside it: an agent and an API client have no browser to render a JSON
   document, and a renderer for the editor's schema in Go would be hand-rolled work that follows the
   library's schema. The editor loads this HTML and saves the same HTML. The send path stays Liquid over
   the HTML, then the derived text part; Go renders nothing else, and there is no Node sidecar.
3. **The editor is `@react-email/editor`, in the core.** It is MIT and needs no key or host. Mainly is the
   fallback if the prototype rules the first out.
4. **The brand kit is data of the Workspace**: colors, fonts, logo, button style, footer. It sets the
   editor's palette and presets, an MCP tool (`brand_get`) returns it to an agent, and its values are
   written into `body` when the message is authored. No token resolves at send time, so editing the brand
   never changes a message that is already written (ADR 0003 holds).
5. **A lint, in Go, over the HTML body**, called by an MCP tool (`email_lint`), by the save path and by
   the send planner (where `emailrender.Validate` is today). It blocks only what cannot render: a Liquid
   syntax error, HTML that does not parse. Everything else is a warning returned to the author: a missing
   alt text, low contrast, weight, a value that is not in the brand kit, an unknown construct.
6. **The HTML is sanitized on save** with an established library and an allowlist, on every entry point
   (site, external API, MCP). Script, iframe, form, `on*` handlers and `javascript:` URLs go; tables,
   inline styles, `<style>` and Outlook conditional comments stay. This replaces the normalization the MJML
   compiler did.
7. **Liquid.** Variables (`{{ first_name }}`) are a first-class editor feature. `{% if %}` and `{% for %}`
   keep running, because Liquid runs over the whole HTML, but the editor has no block for them yet (nothing
   in the repo uses them); it must not break them when it loads and saves.
8. **An agent writes HTML**, starting from a copy of one of the Workspace's starter Templates and checking
   it with the lint; there is no separate block library.

## Prototype first

A throwaway prototype runs in a worktree (no branch, no code kept in the repo, `AGENTS.md`), and its
findings are recorded here and in `docs/research/`. It answers, in this order:

1. Does the editor load HTML and export it again without loss (its own output first, then fixtures and
   HTML an LLM writes), and does `composeReactEmail` or its equivalent run in the browser?
2. Do Liquid tags survive between table rows and around table sections, where an HTML parser moves text?
3. Which sanitizer policy keeps Outlook conditional comments and `<style>` and still strips the dangerous
   constructs?
4. Does the editor follow Mantine's light and dark schemes with no CSS of ours?

If the first fails, Decision 2 is reopened, not patched. An option not yet weighed then is a JavaScript
runtime inside the Go binary that renders the editor's JSON to HTML once, at save time.

## Open

- The starter Templates are created with the Workspace, before a brand kit exists, and Decision 4 never
  updates written content. A copy of a starter Template may carry stale brand values. Resolving the brand
  when a Template is copied, not when it is sent, would fit ADR 0003.

## Consequences

- Supersedes the MJML parts of ADR 0003, ADR 0005 and the glossary entry of Message content (subject plus
  an HTML body, no compile step). The copy-versus-reference rules of those ADRs are unchanged.
- `emailrender` loses the MJML step; tracking and the unsubscribe footer still layer on the HTML after it.
- The e2e helper `e2e.MJML`, the TypeSpec descriptions (`MJML body`), the playbooks, `docs/guide/sending.md`
  and the fixtures change with it.
