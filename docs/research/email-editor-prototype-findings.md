# @react-email/editor as the human editor for a stored email-HTML body: prototype findings

Worktree: `/Users/kirillmokevnin/projects/sphericon/.claude/worktrees/agent-af50d14fe8516120e`
Code and raw evidence: `prototype/email-editor/` (inputs `fixtures/`, every output in `out/`, screenshots in `out/shots/`).
Tested: `@react-email/editor@1.7.14` (TipTap 3.x, React 19.3, Vite 8, real Chromium via Playwright), `@maily-to/core@0.3.7` + `@maily-to/render@0.2.3`, bluemonday v1.0.27.

## Verdicts

| #   | Check                                       | Verdict                                                                                                                                                        |
| --- | ------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | Self round-trip (export, re-import, export) | **FAIL.** HTML import and browser export both work, but the editor cannot re-import its own export: every cycle nests another shell and grows the document.    |
| 2   | External HTML round-trip                    | **FAIL** for LLM-written or MJML-compiled HTML. Head, `<style>`, all comments, `<img>`, `bgcolor`, classes and layout are dropped or rewritten.                |
| 3   | Liquid                                      | **Pass with caveats.** Passes in text, in hrefs and inline. **Fails** for `{% %}` between `<tr>` rows or around a table: the tags are pulled out of the table. |
| 4   | Sanitizer (bluemonday)                      | **Pass with caveats.** A working policy needs placeholder extraction for comments, `<style>` and Liquid. Stock bluemonday cannot do it alone.                  |
| 5   | Mantine + dark                              | **Pass.** Light/dark follow Mantine with zero custom CSS via `cssVariablesResolver`.                                                                           |
| 6   | Maily fallback, checks 1-3                  | **Fail on tables.** HTML import and browser render work, but it flattens every table into paragraphs. Not idempotent either.                                   |

## Check 1 FAIL: stated plainly

The editor cannot be used for "stored string is the one source, human edits it, string comes back" as it stands. Re-importing the editor's own export wraps another shell around the content each time. Details:

### What works

- HTML import: `EmailEditor content="<html string>"` and `editor.commands.setContent(html)` both work. The docs say "HTML string, parsed automatically into the editor's document model".
- `composeReactEmail({editor, preview})` runs entirely in the browser. It was exercised in headless Chromium with no Node server (`src/harness.tsx`, `EmailEditorRef.getEmailHTML()`). The export bundle is large: the Vite build produced one `client` chunk of 2.45 MB (777 KB gzip), plus a React server renderer (`server.browser`, 62 KB gzip).
- The export is a full document: XHTML 1.0 doctype, `<head>` with meta tags and a `@media (prefers-color-scheme: dark){li::marker{...}}` style, and `<body>` wrapped as `<table role=presentation> > td > <table style="max-width:600px;..."> > td > content`. Styles are inlined by the theme (`basic`). Output is Prettier-formatted, with `<!--$-->`, `<!--html-->`, `<!--head-->`, `<!--body-->` and `<!--/$-->` marker comments. React also adds `<!-- -->` between adjacent text nodes (for example `Hello<!-- -->{{ first_name }}`).

### What fails

`run-cycles.mjs`: load HTML, export, feed the export back in, 5 times. The input was a small editor-native document: h1, p with bold, italic and a link, ul, hr, centered p.

```
cycle 1 bytes 3833  jsonDepth 5   container:1
cycle 2 bytes 6341  jsonDepth 9   container:2 table:1 tableRow:1 tableCell:1
cycle 3 bytes 9750  jsonDepth 13  container:3 table:2 ...
cycle 4 bytes 14237 jsonDepth 17  container:4 table:3 ...
cycle 5 bytes 18900 jsonDepth 21  container:5 table:4 ...
```

The outer shell table has no `max-width`, so it is imported as a plain `table` node, and the inner container table is imported as a nested `container`. Each cycle also adds one empty trailing paragraph (from the `trailingNode` extension) and, on the second cycle, an extra `<u>` inside links (`<a ...><u>link</u></a>`).

Diff of the first export against the second (`out/native.out1.html` vs `out/native.out2.html`) is mostly the new nested table:

```
>   <table align="center" width="100%" ... style="margin-top:0;margin-right:auto;...">
>     <tbody><tr style="margin:0;padding:0">
>       <td data-id="__react-email-column" style="margin:0;padding:0;font-family:-apple-system,...">
>         <table ... style="max-width:600px;align:center;width:100%;...">   <- container again
...
>   ><u>link</u></a>
```

### Workaround probe (`run-workaround.mjs`)

If only the exported container `<table style="max-width:600px...">` is extracted (DOM-parse the export, take that table) and fed back, it converges after 2 cycles (3833 → 3840 → 3840 bytes, identical thereafter). So an unwrap step before import would make it idempotent for editor-native content. This is a workaround the editor does not provide: the product would have to ship its own unwrap and keep the head/shell separately.

## Check 2 FAIL: external HTML

Inputs: 3 MJML fixtures compiled with the repo's gomjml (`fixtures/mjml-*.html`), plus two hand-written LLM-style emails (`fixtures/llm-promo.html`, `fixtures/llm-receipt.html`: table layout, inline styles, `<!--[if mso]>` including VML button, `<style>` with `@media`/dark mode, preheader, Liquid).

Counts of features in the input versus the editor's export (`out/*.out1.html`, default `EmailEditor`):

| Feature                             | llm-promo in → out                                                             | llm-receipt in → out | mjml-welcome in → out |
| ----------------------------------- | ------------------------------------------------------------------------------ | -------------------- | --------------------- |
| `<img>`                             | 1 → **0**                                                                      | 0                    | 0                     |
| `bgcolor`                           | 2 → **0**                                                                      | 0                    | 1 → **0**             |
| `<!--[if ...` conditional comments  | 5 → **0**                                                                      | 1 → **0**            | 8 → **0**             |
| VML `v:roundrect`                   | 2 → **0**                                                                      |                      |                       |
| `<title>`                           | 1 → **0**                                                                      | 1 → **0**            | 1 → **0**             |
| user `<style>` / `@media`           | 2 → 1 (only the editor's own li::marker rule)                                  | 1 → 1 (same)         | 1 → 1 (same)          |
| `class="container"` / `class="btn"` | 1/1 → **0/0**                                                                  |                      |                       |
| `data-track` attribute              |                                                                                | 1 → **0**            |                       |
| `xmlns*`                            | 4 → **0**                                                                      |                      | 3 → **0**             |
| `<thead>`                           |                                                                                | 2 → **0**            |                       |
| `:hover` rules                      | 1 → **0**                                                                      |                      |                       |
| preheader `display:none` div/span   | 1 → 1 (kept, but the receipt's `visibility:hidden` span is now inside a `<p>`) |                      |                       |

Other observed rewrites:

- Every text node is wrapped in a `<p>` with theme margins; `<th>`, `<td>` content becomes `<p>`; `<br>` kept. Cells become `data-id="__react-email-column"`.
- Image import only works when `onUploadImage` is passed to `EmailEditor` (it adds the image extension). Without it, `<img>` is silently dropped. With it, the tag survives but its style is replaced: `display:block;border:0` becomes `display:block;outline:none;border:none;text-decoration:none;max-width:100%;border-radius:8px`.
- Links get `target="_blank" rel="noopener noreferrer nofollow"` and `text-decoration-line:none;text-decoration:underline` added.
- A `<table align=center width=560 ...>` with `class="wrap"`: kept as a `table` node but `class` is lost.
- Visual diff (`out/shots/llm-promo-in.png` vs `llm-promo-out.png`): the logo is gone, the **CTA button disappears completely** (its cell `bgcolor`/blue background was dropped, leaving white link text on white), the card background, padding and radius are lost, and the grey page background moves into a padded table. The two-column block loses its widths.
- MJML-compiled fixtures: all MJML conditional comments and its `<style>` blocks, including the `mj-column-per-100` responsive rules, are lost. The two-column/responsive structure collapses to one cell.

`<style>` blocks, comments, `<head>` and unknown attributes are not parsed at all, so they cannot be reconstructed on export. Only nodes the editor has an extension for survive: p, h1-6, lists, blockquote, code block, hr, table family, div, link, button (a specific `<a>` pattern), image (with the upload option), columns container, preview text.

## Check 3 Liquid (pass with caveats)

`run-liquid.mjs`, input to editor model (`out/liquid.json`):

| Case                                                                                                                        | Result                                                                                                                                                                                                                                                                                                                                                                                                                              |
| --------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `{{ first_name }}` in a `<p>`                                                                                               | Survives load, JSON and export exactly.                                                                                                                                                                                                                                                                                                                                                                                             |
| `{{ ... }}` in href (`https://x.dev/u?id={{ contact.id }}&amp;t={{ token \| url_encode }}`, `href="{{ unsubscribe_url }}"`) | Survives; the Link extension accepts it (no URI rewrite).                                                                                                                                                                                                                                                                                                                                                                           |
| Filter syntax `{{ first_name \| default: "friend" \| upcase }}`                                                             | Survives as text.                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `{% if %}...{% endif %}` inline inside a paragraph                                                                          | Survives.                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `<p>a</p>{% if premium %}<p>Premium</p>{% endif %}<p>b</p>` (between blocks)                                                | Survives and stays balanced, but each tag becomes its own paragraph: `<p>{% if premium %}</p><p>Premium</p><p>{% endif %}</p>`. When false it renders an empty `<p></p>`.                                                                                                                                                                                                                                                           |
| `{% for %}` / `{% if %}` as text between `<tr>` rows                                                                        | **Broken.** Foster-parenting moves the tags out of the table: `<table><tbody>{% for i in items %}<tr>...</tr>{% endfor %}...` becomes `<p>{% for item in items %}{% endfor %}</p><table>...rows...</table>` (tag pair empty, before the table). Same for `{% if %}` around a row and around a whole table (`{% if show %}` → `<p>{% if show %}</p><table>..</table><p>{% endif %}</p>`). The tags still exist; their scope changed. |
| `{% if %}` inside a `<td>` around inline content                                                                            | Survives, wrapped in a `<p>` inside the cell.                                                                                                                                                                                                                                                                                                                                                                                       |
| Liquid inside HTML comments (`<!-- {% if x %} -->` between rows)                                                            | Comments are dropped, so the tags are lost (2 → 0).                                                                                                                                                                                                                                                                                                                                                                                 |

The browser's HTML parser is the cause (this is HTML5 foster-parenting, not editor-specific), so any pipeline that parses with a DOM would do the same. Template logic around table rows would have to be expressed inside cells or as full-block paragraphs, or avoided.

Editor's own variable support: **none in the package.** `@tiptap/extension-mention` is a declared dependency, but no merge-tag/variable node is exported or used (grep of `dist/`: no Mention references; the extension list has no variable node). `{{ }}` is just text. I built a custom `EmailNode` (`liquidVar`: inline atom, chip in the editor, `renderToReactEmail` outputs `{{ expr }}`) in `src/harness.tsx`. It works for authoring: the chip exports as `Hello<!-- -->{{ first_name }}` (React text separator comment between nodes). On re-import the chip becomes plain text (the export has no `data-liquid` marker), so chips do not survive a round trip unless the export is customised.

## Check 4 Sanitizer (pass with caveats)

Code: `go/sanitize.go`, `go/sanitize_test.go` (run with `cd go && go test ./... -v`; reports in `out/sanitizer-report.txt`, sanitized samples in `out/sanitized/`). The module is separate (`prototype/email-editor/go/go.mod`) so the repo's go.mod is untouched.

What bluemonday does not do by itself, so the policy works around it:

1. Strips all comments, so `<!--[if mso]>` and `<!--[if !mso]><!-->...<!--<![endif]-->` vanish. Workaround: lift them out into placeholder elements before sanitizing, sanitize the inner HTML of hidden conditional comments with a policy that adds VML elements, restore afterwards. The condition text is checked against `^[A-Za-z0-9 !&|()<>=.]{1,60}$`.
2. `<style>` is refused by default. Workaround: extract blocks first, reject any whose CSS matches `@import|expression(|javascript:|vbscript:|behavior:|-moz-binding|url(javascript|...` or contains `<`, keep only a safe `media` attribute, put them back. This is a denylist, not a CSS parse; a real CSS parser would be needed for production.
3. Inline `style=""` is not sanitized by bluemonday unless `AllowStyles` is used; the policy drops any `style` attribute that matches the same denylist.
4. **Liquid in URLs breaks bluemonday's URL handling.** Stock policy on `href`: `{{unsub}}` becomes `%7B%7Bunsub%7D%7D`; `{{ unsubscribe_url }}`, `/x/{{ id }}` and `https://x/{{ id }}` are dropped (the anchor loses its href, or the whole anchor goes). Workaround: mask every `{{...}}` / `{%...%}` with a per-call random nonce token before sanitizing and restore afterwards. Masked, they are plain relative URLs. With masking, Liquid in text, attributes and between `<tr>` rows (bluemonday is tokenizer-based, no foster-parenting) survives byte for byte.
5. `AllowAttrs("href")` on namespaced VML elements bypasses bluemonday's scheme check (`v:roundrect href="javascript:..."` survived in the first attempt). Fixed with an explicit value regex for those elements.

Real bugs the attack tests caught in my own policy while building it: the placeholder indexes could be forged by input (`<sph-hc i="999">` panicked with index out of range); fixed by stripping any `<sph-*>` from input and bounds-checking. The `<!--[if !mso]><!-->` / `<!--<![endif]-->` forms must be handled before the hidden-conditional regex, because `<![endif]-->` also occurs inside `<!--<![endif]-->`; the `<!-- -->` spelling of the opener (LLM output) is normalised to `<!-->`.

Results (26 attack vectors, all neutralised): `<script>`, `<iframe>`, `<form>`/`<input>`, `<svg onload>`, `<object>/<embed>`, `<base>`, `<link rel=stylesheet>`, `on*` handlers, `javascript:`/`data:text/html` hrefs (also with entity-split whitespace), CSS `expression()`, `url(javascript:)`, `@import`, `</style><script>` breakout, scripts inside conditional comments, forged conditional-comment openers, forged placeholders, VML `onclick`/`javascript:` href. `<meta http-equiv="refresh">` loses `http-equiv` and is inert.

Survival on the fixtures (`out/sanitizer-report.txt`): everything the policy allows is kept byte-identical in count: tables, inline styles, `class`, `bgcolor`, `role`, `data-track`, `<img>`, `<head>`/`<meta>`/`<title>`, `<!--[if mso]>`, VML button fallback, `@media` style blocks, preheader, all Liquid tokens, `href="{{ unsubscribe_url }}"`. Differences: one `<style>` block per MJML fixture (the Google Fonts `@import`, correctly denied) and `<link href=fonts.googleapis...>` (links not allowed); `<!--[if !mso]><!-- -->` normalised.

Also: sanitizing the stored template cannot see what Liquid renders into URLs. A contact attribute rendered into `href="{{ x }}"` can become `javascript:...`; bluemonday run on the rendered output strips it (verified: `javascript:` href is dropped when it is literal in the markup). A policy applied only at save time would not cover that.

Editor output through this policy (`out/sanitized/*.out1.html`): passes unchanged apart from nothing lost (only the Prettier-formatted export with its markers is retained).

## Check 5 Mantine + dark (pass)

`src/main.tsx` (served at `/`), screenshots `out/shots/os-{light,dark}-mantine-{light,dark}.png`.

- CSS shipped: one opt-in file, `import '@react-email/editor/themes/default.css'` (23 KB; also `style.css` 18 KB, plus separate `styles/bubble-menu.css`, `slash-command.css`, `inspector.css`). Ordinary global stylesheet, no CSS-in-JS. The repo rule allows only library stylesheets imported in `src/main.tsx`, so this fits the pattern of `@mantine/core/styles.css`. Selectors are `[data-re-*]` hooks, `.tiptap ...` content classes, and the variable definitions; it contains no global resets (unlike the workflow-builder SDK noted in `src/main.tsx`).
- Theme variables: `--re-bg`, `--re-border`, `--re-text`, `--re-text-muted`, `--re-hover/active/pressed`, `--re-separator`, `--re-danger`, `--re-radius`, `--re-shadow`. Defined on `:root`; dark via `@media (prefers-color-scheme: dark)` on `:root` and via a `.dark` class. Neither is Mantine's `data-mantine-color-scheme` attribute, so out of the box the editor UI follows the OS, not the Mantine scheme.
- Fix with Mantine primitives only: `MantineProvider cssVariablesResolver` mapping `--re-*` to `var(--mantine-color-*)` in the `light` and `dark` sections. Mantine emits these under `:root[data-mantine-color-scheme=...]`, which outranks the editor's `:root` and `.dark`. Verified in all four combinations of OS scheme × Mantine scheme (computed `--re-bg`: `#fff` for Mantine light regardless of OS dark; `#2e2e2e` for Mantine dark regardless of OS light). No `.css`, no inline `style`, no hardcoded colours.
- Screenshots show the bubble menu and the editor chrome in Mantine's palette in both schemes. The email canvas itself stays white in dark mode: it is the email's own inline `background-color:#ffffff` from the `basic` theme, which is correct for an email preview. A theme object (`extendTheme('basic', {...})`) can change the canvas colours; this was not tested.
- Not tested: slash command menu and inspector in dark mode (only the bubble menu was opened); focus-ring and selection colours in the editor body.

## Check 6 Maily fallback (`@maily-to/core` 0.3.7)

Separate install in `prototype/email-editor/maily/` (it pins TipTap 2.27, conflicting with the React Email editor's TipTap 3; the two cannot share a dependency tree). Needed `yjs` added manually (undeclared; Vite failed with `Failed to resolve import "yjs"` until installed).

- HTML import: yes, `contentHtml` prop and `editor.commands.setContent(html)`. Browser render: `@maily-to/render` `render(json)` ran in Chromium with no server.
- Maily's model is JSON-first (`contentJson`); the rendered output pulls the Inter font from `https://rsms.me/inter/...` (an external request at view time; the page logged `ERR_NAME_NOT_RESOLVED` offline).
- Self round-trip: **not idempotent, with drift.** `native` 3882 → 3916 bytes: the second export wraps headings and links in `<strong>` (`<h1>…<strong>Welcome {{ first_name }}</strong></h1>`, `<a><strong>link</strong></a>`). Convergence beyond 2 cycles not measured.
- External HTML: **tables are flattened.** MJML fixtures: output smaller than the input (2.4 KB vs 3.4 KB), all conditional comments and style blocks gone. LLM promo: the table layout and the card background are gone; the two columns become one paragraph with `<br>`; the button becomes a bold link in a paragraph; `<img>` and preheader text survive as an image and a paragraph (visible text, not hidden). Receipt table `<thead>/<tbody>` rows become paragraphs.
- Liquid: text, hrefs and filters survive (12/12 and 4/4 tokens in the promo and receipt). `{% for %}` around table rows: the loop is empty and the rows become paragraphs (`<p>{% for item in items %}{% endfor %}</p><p>head{{ item.name }}</p>`), the same failure as the other editor, plus the table is gone. Between blocks, `{% if %}` becomes its own paragraphs (as in the React Email editor). Maily has its own variable and conditional blocks in JSON (`{{var}}`, showIf), not tested.

## Things I could not test or did not do

- `docs/research/embeddable-email-editors.md` does not exist in this worktree (only `ai-email-design-landscape.md`), so I could not read it.
- Rendering in real mail clients (Outlook, Gmail): not done; the check is structural and visual in Chromium only.
- The editor's "HTML code view" mentioned for Resend's hosted editor (changelog 2026-03-12) was not found in the npm package; `getEmailHTML` is export-only. I did not look for a raw-HTML editing mode.
- Editing round trips after real user edits (typing, bubble menu, slash commands): only programmatic `setContent` and `insertContent`; no scripted typing sessions.
- Custom `EmailNode` extensions to preserve unknown blocks (a raw-HTML passthrough node for tables/style/comments) were not built, so I cannot say if one would fix check 2. The docs describe the `EmailNode` API and `parseHTML`, but it does not give access to `<head>` or comments, which are dropped by ProseMirror's parser.
- Maily's own variable/conditional blocks and its idempotency beyond 2 cycles.
- The sanitizer CSS check is a regex denylist; I did not test a CSS parser, `@font-face`/`@media` abuse, or very large inputs.
- Slash command and inspector UI in dark mode, mobile width, accessibility.
