---
status: accepted
---

# Email content is authored in an embedded editor; the server builds the HTML

Message content is a subject plus an MJML body today (ADR 0003, ADR 0005, the glossary). The body field
is a plain `Textarea`, an agent that drafts through MCP gets an MJML string and nothing else (no brand,
no vocabulary, no feedback), and a malformed body fails only when a send is planned. The research in
`docs/research/ai-email-design-landscape.md` and `docs/research/embeddable-email-editors.md` shows that
the vendors whose agents produce good-looking mail narrow what the agent writes, hold the brand as data
and give the agent a lint. The one actively maintained MIT editor that fits React 19 and Mantine without
a hosted service is `@react-email/editor`: it stores its own JSON document and emits HTML, not MJML.

Two throwaway prototypes (in worktrees, not kept) tested it; their findings are in
`docs/research/email-editor-prototype-findings.md` and `docs/research/embedded-js-prototype-findings.md`.
What shaped this decision:

- The editor cannot load its own HTML export back without growing it, and drops what it does not model
  in HTML written elsewhere (Outlook conditional comments, `<head>`, `<style>`, `@media`, `bgcolor`,
  classes). A body written outside the editor cannot be opened in it without damage.
- Liquid `{{ }}` survives in text and in `href`; `{% for %}` and `{% if %}` placed between table rows or
  around a table are moved out of it by the HTML parser.
- Stock bluemonday mangles `href="{{ x }}"` and drops comments and `<style>`; a policy with workarounds
  neutralised all 26 attack vectors and kept the email constructs.
- The editor follows Mantine's schemes once `--re-*` is mapped to `--mantine-color-*` in the provider's
  `cssVariablesResolver`, with no CSS of ours. Maily failed the same checks on tables.
- The editor's own export code runs inside the Go binary in an embedded JavaScript runtime (goja): no
  Node, no browser, no cgo, and the HTML is byte-identical to the editor's own export on the two test
  documents. One script is 2.69 MB (842 KB gzip) and needs six runtime shims and one build-time rewrite.
  A cold start is 3 to 3.8 s per runtime plus 0.7 to 1 s of compiling once per process; a warm render is
  80 to 140 ms; the first runtime takes 68 MB and each further one about 16 MB. A runtime is not
  goroutine-safe, and after a time limit stops it, it did not recover in 7 of 8 tries.
- A schema for agents can be derived from the editor's extensions (a 2 KB listing of 25 node types and 9
  marks, a 25 KB JSON Schema), but not the attribute types and allowed values. The editor's own loader
  replaces an invalid document with an empty paragraph without a word, and accepts a heading of level 9,
  an empty button and a link with no `href`; a server check (`nodeFromJSON` and `check`) rejects the
  structurally invalid ones with a message.
- Rendered unescaped, a hostile variable value produced a live `onerror` and `javascript:` URL; a second
  sanitizer pass over the rendered HTML removed them. Rendering a 5 MB text took 33 to 54 s, and 10,000
  paragraphs did not finish in 60 s, so inputs need limits before the runtime.

## Decisions

1. **One format on every surface.** Templates, Broadcasts, Automation steps, Transactional sends, the
   external API and MCP all carry the same Message content. There is no input that accepts MJML, and
   `gomjml` leaves the build. The project is greenfield: no conversion of existing bodies, no guard for
   them; fixtures and callers are rewritten.
2. **A message written in the editor is its document, and the server builds the HTML.** Each place that
   holds a `body` gains a nullable `doc` (the editor's JSON). A client (the editor, an agent, the API)
   writes the `doc` only; the server renders it to the `body` in the same save and stores both. A `doc`
   that cannot be rendered is refused, so a stored `doc` always has its HTML and they never disagree.
   The browser needs the editor UI only, not the export code. A message that arrived as HTML (the external
   API) has no `doc`; a write of `body` clears any `doc`. Such a body opens in the editor as source,
   editable as text, and is never loaded into the editor.
3. **The render runs in an embedded JavaScript runtime in the Go binary** (goja), the editor's own
   export code, with no Node sidecar and no renderer for the document's schema written in Go. One or two
   runtimes, started in the background at boot (a start takes longer than a request may run on a small
   machine, so the first save must not wait for it), behind a queue: saving is rare and the send path
   never renders a document. A runtime that hit its time or memory limit is discarded and a new one
   started. Inputs are capped (size, depth, node count) before they reach it. The editor version is stored
   with the `doc`, and a test renders the fixtures against a known output on every dependency change.
4. **An agent writes the editor's document**, from the schema MCP returns (derived from the editor's
   extensions, with the attribute types and allowed values written by hand) and starts from a copy of a
   starter Template. The server validates a document by the schema (`check`) and by rules of ours (heading
   levels, a button's required attributes, a link's `href`), and answers an invalid one with a `422` and
   the reason: it never repairs or drops anything in silence.
5. **The editor is `@react-email/editor`, in the core.** It is MIT and needs no key or host. Its code
   is loaded only on the pages that edit a message.
6. **The Brand kit is data of the Workspace** (glossary): it sets the editor's palette and presets, an MCP
   tool (`brand_get`) returns it to an agent, and its values are written into the message when it is
   authored. No token resolves at send time, so editing the Brand kit never changes a message already
   written (ADR 0003 holds).
7. **A lint, in Go, over the rendered HTML**, called by an MCP tool (`email_lint`), by the save path and
   by the send planner (where `emailrender.Validate` is today). It blocks only what cannot render: a
   Liquid syntax error, HTML that does not parse. Everything else is a warning returned to the author: a
   missing alt text, low contrast, weight, a value outside the Brand kit.
8. **The HTML is sanitized twice.** On save, on every entry point (site, external API, MCP), with an
   established library and an allowlist that keeps tables, inline styles, `<style>` and Outlook
   conditional comments and strips script, iframe, form, `on*` handlers and `javascript:` URLs. And again
   in full on the rendered message for each recipient, after Liquid, because a variable can carry markup
   or a URL that the first pass never saw. The values of variables in the body are HTML-escaped by the Liquid
   engine's auto-escape as they are written (not pre-escaped in the bindings); `{{ value | safe }}` is the
   explicit raw output for a Transactional variable that carries HTML on purpose, and the second pass still
   sanitizes it. The subject is plain text, rendered by a separate engine with no escaping. The sanitizer is
   bluemonday behind `emailrender.SanitizeHTML`: Liquid tags are masked while it runs, comments, `<style>`
   blocks and the doctype are lifted out and checked on their own, and the HTML hidden by a conditional
   comment is sanitized with the VML elements allowed.
9. **Liquid.** Variables (`{{ first_name }}`) are a first-class feature of the editor. `{% if %}` and
   `{% for %}` still run (Liquid runs over the whole HTML) but the editor offers no block for them, and a
   tag placed between table rows or around a table is not supported inside it. Nothing in the repo uses
   them today.
10. **Starter Templates are made from the Brand kit**, when it is first saved, not when the Workspace is
    created, so a copy never carries brand values that are not the Workspace's. Editing the Brand kit
    offers an explicit "recreate the starter Templates"; it never touches a message already written.
11. **An automation's email step holds `doc` beside `body`** in the JSON string of its `definition`
    (`{"type":"email","subject":…,"body":…,"doc":…}`): an addition, no migration.

## Consequences

- Costs accepted: about 2.7 MB more in the binary, 70 to 120 MB of memory with the runtimes started, and
  3 to 4 s on the first save after a start. The shims and the regex workaround tie the render to the
  editor's version, which is why it is stored and tested.
- Supersedes the MJML parts of ADR 0003 and ADR 0005 and the glossary entry of Message content (done).
  The copy-versus-reference rules of those ADRs are unchanged.
- `emailrender` loses the MJML step; tracking and the unsubscribe footer still layer on the HTML after it.
- The e2e helper `e2e.MJML`, the TypeSpec descriptions (`MJML body`), the playbooks, `docs/guide/sending.md`
  and the fixtures change with it.
