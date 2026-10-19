---
status: proposed
---

# Email content is HTML, authored in an embedded editor

Message content is a subject plus an MJML body today (ADR 0003, ADR 0005, the glossary). The body field
is a plain `Textarea`, an agent that drafts through MCP gets an MJML string and nothing else (no brand,
no vocabulary, no feedback), and a malformed body fails only when a send is planned. The research in
`docs/research/ai-email-design-landscape.md` and `docs/research/embeddable-email-editors.md` shows that
the vendors whose agents produce good-looking mail narrow what the agent writes, hold the brand as data
and give the agent a lint. The one actively maintained MIT editor that fits React 19 and Mantine without
a hosted service is `@react-email/editor`: it stores its own JSON document and emits HTML, not MJML.

A prototype (throwaway, in a worktree, not kept) tested it. Findings that shape this decision:

- It cannot load its own HTML export back without growing it: each export, import, export cycle nests
  another shell table (3.8 KB, 6.3, 9.8, 14.2, 18.9 KB over five cycles).
- It drops what it does not model when HTML written elsewhere is loaded: comments (the Outlook
  conditionals), `<head>`, every `<style>` block and `@media`, `bgcolor`, classes, `data-*`. A body
  written outside the editor cannot be opened in it without damage.
- `composeReactEmail` runs in the browser with no Node server; the export bundle is 2.45 MB (777 KB gzip).
- Liquid `{{ }}` survives in text and in `href`; `{% for %}` and `{% if %}` placed between table rows or
  around a table are moved out of it by the HTML parser.
- Stock bluemonday mangles `href="{{ x }}"` and drops comments and `<style>`; a policy with three
  workarounds neutralised all 26 attack vectors and kept the email constructs.
- It follows Mantine's schemes once `--re-*` is mapped to `--mantine-color-*` in the provider's
  `cssVariablesResolver`, with no CSS of ours.
- Maily failed the same checks on tables (it flattens them) and pins TipTap 2.

## Decisions

1. **One format on every surface.** Templates, Broadcasts, Automation steps, Transactional sends, the
   external API and MCP all carry the same Message content, a subject and an HTML body. There is no input
   that accepts MJML, and `gomjml` leaves the build. The project is greenfield: no conversion of existing
   bodies, no guard for them; fixtures and callers are rewritten.
2. **A body is HTML and, when it was written in the editor, the editor's document beside it.** Each place
   that holds a `body` gains a nullable `doc` (the editor's JSON). The editor builds the HTML in the
   browser when it saves and writes both; the send path takes the HTML, runs Liquid over it, and derives
   the text part. Go renders nothing else, there is no Node sidecar and no renderer for the document's
   schema in Go. A body that arrived as HTML (the external API) has no `doc` and opens in the editor as
   source, never loaded into it.
3. **An agent writes the editor's document**, using the schema MCP returns, and starts from a copy of a
   starter Template. A draft with a `doc` and no HTML cannot be sent or activated until a person has
   opened it in the editor and saved it: nothing but a browser makes the HTML. Drafts are already
   human-activated.
4. **The editor is `@react-email/editor`, in the core.** It is MIT and needs no key or host. Its bundle
   is loaded only on the pages that edit a message.
5. **The Brand kit is data of the Workspace** (glossary): it sets the editor's palette and presets, an MCP
   tool (`brand_get`) returns it to an agent, and its values are written into the message when it is
   authored. No token resolves at send time, so editing the Brand kit never changes a message already
   written (ADR 0003 holds).
6. **A lint, in Go, over the HTML**, called by an MCP tool (`email_lint`), by the save path and by the
   send planner (where `emailrender.Validate` is today). It blocks only what cannot render: a Liquid
   syntax error, HTML that does not parse. Everything else is a warning returned to the author: a missing
   alt text, low contrast, weight, a value outside the Brand kit.
7. **The HTML is sanitized on save**, on every entry point (site, external API, MCP), with an established
   library and an allowlist that keeps tables, inline styles, `<style>` and Outlook conditional comments
   and strips script, iframe, form, `on*` handlers and `javascript:` URLs. A value Liquid renders into a
   URL is invisible to that pass, so the URLs of a rendered message are checked again after Liquid.
8. **Liquid.** Variables (`{{ first_name }}`) are a first-class feature of the editor. `{% if %}` and
   `{% for %}` still run (Liquid runs over the whole HTML) but the editor offers no block for them, and a
   tag placed between table rows or around a table is not supported inside it. Nothing in the repo uses
   them today.

9. **Starter Templates are made from the Brand kit**, when it is first saved, not when the Workspace is
   created, so a copy never carries brand values that are not the Workspace's. Editing the Brand kit
   offers an explicit "recreate the starter Templates"; it never touches a message already written.
10. **An automation's email step holds `doc` beside `body`** in the JSON string of its `definition`
    (`{"type":"email","subject":…,"body":…,"doc":…}`): an addition, no migration.
11. **A body with no `doc` opens as source**, editable as text, and is never loaded into the editor, which
    would drop parts of it. A message in the editor is written in the editor.
12. **`body` and `doc` never disagree.** A message with a `doc` and no HTML cannot be scheduled, activated
    or sent as a Transactional send; the error says to open it in the editor and save. A write of `body`
    through the API or MCP clears `doc`; a write of `doc` clears the HTML. The last write wins and the
    other part is gone.
13. **Live content is never left without HTML.** A write of `doc` alone is refused (the error says to open
    the message in the editor and save) for a Template, which a Transactional send renders live (ADR 0005),
    and for a step of an active Automation, which `Update` lets change. A draft (a Broadcast not yet
    scheduled, an inactive Automation) accepts it.

## Second prototype, before the spec

Decision 3 rests on two things nobody has tested: the package exports no document schema that sphericon
could serve to an agent (none in `@react-email/editor` 1.7.14's exports), and the prototype loaded HTML,
never JSON an agent wrote. A second throwaway prototype runs the editor's rendering in an embedded
JavaScript runtime inside the Go binary (goja or QuickJS): can it build the HTML from a document at save
time, how long does one render take with a 2.45 MB bundle, and can a schema for agents be derived from the
editor's extensions. If it can, the HTML is built on the server for a draft written by an agent, the
"a person must save it first" part of Decision 3 and the refusal of Decision 13 shrink, and the lint of
Decision 6 sees agent drafts. If not, an agent draft stays unchecked until a person saves it, or a second
lint walks the document. This ADR stays proposed until it reports.

## Consequences

- Supersedes the MJML parts of ADR 0003 and ADR 0005 and the glossary entry of Message content (done).
  The copy-versus-reference rules of those ADRs are unchanged.
- `emailrender` loses the MJML step; tracking and the unsubscribe footer still layer on the HTML after it.
- The e2e helper `e2e.MJML`, the TypeSpec descriptions (`MJML body`), the playbooks, `docs/guide/sending.md`
  and the fixtures change with it.
