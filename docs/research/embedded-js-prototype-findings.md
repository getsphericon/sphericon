# Embedded JS runtime for @react-email/editor HTML: prototype findings

Worktree: `/Users/kirillmokevnin/projects/sphericon/.claude/worktrees/agent-a0e975d51b9ee6428`
Code and raw evidence: `prototype/embedded-js/` (JS side in the folder root, Go module in `go/` with its own `go.mod`, so the repo's go.mod is untouched; outputs in `out/`, documents in `docs/`).
Tested: `@react-email/editor@1.7.14` (`@tiptap/core` 3.32.0, react/react-dom 19.3.1, `@react-email/render` browser build), esbuild 0.28, `github.com/dop251/goja` (master, 2026-10-19), bluemonday v1.0.27, osteele/liquid v1.9.2, Go 1.27.2, Node 26.10. Machine: 10 cores, shared with other agents (load 4 to 11 during the runs), so every timing below is noisy; ranges are given where I saw them.

Tests 1 and 2 PASS. goja is enough; QuickJS was not tried (not needed). No cgo: the Go tests also pass with `CGO_ENABLED=0`.

## Verdicts

| #   | Test                                         | Verdict                                                                                                                                                                                                                                                                                               |
| --- | -------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | Bundle the render path with esbuild          | **Pass with caveats.** One 2.69 MB script (842 KB gzip), no DOM needed. Needs 1 build-time rewrite and 6 small runtime shims.                                                                                                                                                                         |
| 2   | Run it in goja, compare to the editor export | **Pass with caveats.** Byte-identical HTML for both documents. Cold start about 3 s per VM; warm render 80 to 160 ms (prettier is nearly all of it). One VM per goroutine (pool).                                                                                                                     |
| 3   | JSON -> HTML -> bluemonday -> Liquid         | **Pass with caveats.** Tags survive. The export HTML-escapes `"` and `'` inside `{{ }}`, which broke Liquid filters with string args until the tags were unescaped. Hostile variable values rendered raw gave live `onerror` and `javascript:`; a second bluemonday pass removed them.                |
| 4   | Schema for agents                            | **Pass with caveats.** A precise node/mark listing (2 KB) and a JSON Schema (25 KB) derive headlessly, in the same goja runtime. Attribute types and enums cannot be derived. The editor's own loader silently drops or keeps invalid content; the server validator rejects with one error at a time. |
| 5   | Hostile documents                            | **Pass with caveats.** No panic, no hang. Time limit works (goja Interrupt, lands within ms), memory limit works only through a Go watchdog, and an interrupted VM did not recover in 7 of 8 tries. Cost grows super-linearly with output size.                                                       |

## 1. Bundle

Entry `entry.mjs`, build `build.mjs` (esbuild, IIFE, browser platform, minified, target es2020, `process.env.NODE_ENV="production"`).

- What it bundles: `composeReactEmail` from `@react-email/editor/core` (unchanged), the editor's `StarterKit` and `EmailTheming` (public exports), the image extension (not a public export: imported from the package's internal chunk `dist/extension-Dw-hZZaA.mjs` by relative path), `@tiptap/core` `resolveExtensions` + `getSchemaByResolvedExtensions`.
- Size: **2,687,316 bytes, 841,353 gzip** (the first prototype's browser bundle was 2.45 MB / 777 KB gzip). From an earlier esbuild metafile, the weight is mostly unused code pulled in by the `react-email` barrel and the editor's browser chunks: react-email 714 KB (prism 566 KB for the code-block component, tailwind 287 KB and css-tree 203 KB for its `Tailwind` component), prettier 253 KB (used), react-dom 208 KB (used), parse5 126 KB, prosemirror-view 98 KB, @tiptap/core 77 KB. Tree shaking did not remove them; aliasing them away was not tried, so a smaller bundle is likely possible.
- Real DOM: **not needed.** The editor object that `composeReactEmail` reads is faked with 4 members (`getJSON`, `extensionManager.extensions`, `schema`, `state.doc`); the document is parsed with ProseMirror's `schema.nodeFromJSON` (no DOM). jsdom was used only as a _reference_ (real TipTap `Editor` in jsdom).

Shims, all of them:

Build time:

1. `import()` with a template string (the lazy Prism language loader in the editor's code-block node view; never reached by a server render) is rewritten to a rejected promise. goja has no dynamic `import()` (a syntax error). Other `import()` calls (react-email's `import("prettier/...")`, `import("react-dom/server")`) are inlined by esbuild itself.
2. Target es2020 (default esnext produced syntax goja rejected).

Runtime (`go/prelude.js` plus `go/rt.go`): 3. `String.prototype.split` override for one regex, `/\s*(?=\b|\W|$)/`, which is how prosemirror-model tokenizes content expressions (`"(tableCell | tableHeader)+"`). **goja bug**: it returns extra empty strings (`["(","tableCell","","|",...]`), so schema creation fails with `Missing closing paren`. Replaced by an equivalent `match(/\w+|[^\s\w]/g)`. 4. `TextEncoder` / `TextDecoder` (UTF-8). The first version was in JS and dominated big-input runtime; now thin wrappers over two Go functions. 5. `MessageChannel` (react-dom/server.browser builds one at module init; backed by `setTimeout`). 6. Minimal `ReadableStream` / `WritableStream` (just `pipeTo`, `enqueue`, `close`), because `@react-email/render`'s browser build calls `renderToReadableStream` and `pipeTo`. Using the real streaming renderer matters: a first attempt that substituted `renderToString` produced different HTML (no `<style>` dark-mode rule, no `<!--html-->` markers). 7. `setTimeout` / `setImmediate` / `clearTimeout` / `console` in Go, with a timer queue the Go side drains while awaiting the promise. The render leaves one inert timer per call; left in the host's queue, every render's closures stay alive (leak of 0.2 MB per render found and fixed, retained heap is flat at 68 MB after 300 renders). 8. Not a JS shim but a behaviour copy: the editor's `containerEnforcer` plugin wraps a doc that has no `container` in one (`extensions/container.tsx`), so `entry.mjs` does the same wrap before rendering. Without it the output differs from the editor's (no max-width container table). `TrailingNode` is not replicated; it never changes the export (the composer drops a trailing empty paragraph). 9. `window`, `self`, `globalThis` aliases were tried first and are **not** needed (removed; output unchanged).

## 2. goja runtime

Documents: `docs/newsletter.json` (h1 and h2, paragraph with bold, italic and a link with `href="https://x.dev/u?id={{ contact.id }}&t={{ token | url_encode }}"`, bullet list, button with `href="{{ cta_url }}"`, image, hr, centered paragraph with `{{ unsubscribe_url }}` link) and `docs/tables.json` (table with header and body rows, twoColumns, ordered list, blockquote, `{{ item.name }}` in cells). The first prototype's folder was not in this worktree, so the documents were recreated; the reference is not Chromium either (see "not tested").

Reference: real TipTap `Editor` under jsdom + the package's own `composeReactEmail` (`ref.mjs`, `out/ref/*.real.html`).

- **Output: byte-identical** in three runtimes (the jsdom reference ran react-email's Node render build, the bundle runs its browser build on the stream shims, so this compares two different renderers, not the same code twice): real editor (jsdom), bundle in Node V8, bundle in goja. newsletter 6618 bytes, tables 6542 bytes (Go test `TestCompareToReference`). Full XHTML doctype document, head with meta tags and the dark-mode `li::marker` rule, `<!--$-->` and `<!--html-->` markers, theme inline styles, Prettier-formatted.
- Cold start: compile of the 2.7 MB script once per process: 0.69 to 0.80 s. Each new VM from the compiled program (prelude + run the bundle's top level, i.e. module init and schema build): **2.9 to 3.8 s** (up to 9 s in one heavily loaded run). First render in a fresh VM: about 0.13 to 0.15 s. So "process start to first render" is about 3.7 s once, then 3 s for each extra VM.
- Warm render, 100 renders per document: newsletter **127 to 140 ms each**, tables **78 to 80 ms**; up to 164 ms under heavy load. Prettier alone on the 6.6 KB output took 135 ms against a 121 ms whole render in the same run: **prettier is nearly the entire cost**. `composeReactEmail` always runs it (it returns `html`, `text` and `unformattedHtml`, but formats before returning), so skipping it needs a different entry than `composeReactEmail`.
- Memory: compiled program + first VM +65 MB heap; each extra VM about +16 MB. After the leak fix the retained heap is flat over 300 renders (68.1 MB total for one VM).
- Concurrency: **a `goja.Runtime` is not goroutine safe.** I used a pool: one compiled `*goja.Program` shared (it is read-only), one VM per pooled slot, a channel as the pool. 8 goroutines x 25 renders over a pool of 4 VMs produced identical HTML each time (`TestPool`): 200 renders in 10.6 s while loaded. Pool of 4 took 11.8 s to create under load (about 3 s each). A VM interrupted mid-render did not recover (test 5).

## 3. JSON -> HTML -> sanitize -> Liquid

The first prototype's Go code was not in this worktree, so `go/sanitize.go` re-creates the policy from its findings: mask every `{{ }}` and `{% %}` with a per-call random nonce token, strip comments and `<style>`, run bluemonday (element and attribute allowlist, `AllowStyles` with a value denylist, schemes http/https/mailto), unmask. It is a compact stand-in, not the first prototype's policy (no VML, no conditional comments, no `<style>` kept).

- Editor export contains all six tags byte-for-byte (`{{ first_name }}`, `{{ contact.id }}`, `{{ token | url_encode }}`, `{{ cta_url }}`, `{{ unsubscribe_url }}`, `{{ company }}`), in text and in hrefs.
- After the masked sanitize: all six tags still intact; table, h1, img, anchors and inline styles retained; no script. Size 6618 -> 4495 bytes (the head, markers and the style block are dropped by the stand-in policy).
- After osteele/liquid: `Welcome, Ada`, `href="https://x.dev/u?id=42&amp;t=a+b%26c"`, the CTA and unsubscribe hrefs and `Acme` all rendered correctly.
- **Caveat 1, escaping in the export.** React escapes `"` and `'` in text and attributes, so `{{ first_name | default: "friend" | upcase }}` is exported as `{{ first_name | default: &quot;friend&quot; | upcase }}` and `{% if plan == 'pro' %}` as `{% if plan == &#x27;pro&#x27; %}`. Liquid then fails to parse: `syntax error in "first_name | default: &quot;friend&quot; | upcase"`. Unescaping the HTML entities inside each tag (before the Liquid step) fixes it (`Hi FRIEND and PRO`, link `?n=a&p=pro`). (The first prototype checked Liquid only through the document JSON and the browser export, not through a Liquid engine.)
- **Caveat 2, sanitize after Liquid.** Values go into the HTML raw. With hostile variable values (`first_name = <img src=x onerror=alert(1)>`, `cta_url = javascript:alert(1)`, `unsubscribe_url = https://x" onmouseover="alert(1)`) the rendered HTML contained `onerror`, a `javascript:` href and a live attribute break. A second bluemonday pass over the rendered output removed all three (`onerror`, `javascript:`, `onmouseover` absent). Same conclusion as the first prototype, now verified end to end on this export.

## 4. Schema for agents

Derived inside the same goja runtime (`globalThis.dumpSchema()` reads the ProseMirror schema built from the resolved TipTap extensions, `out/schema-dump.json`), then two pure transforms in `mkschema.mjs`:

- `out/agent-schema.txt`: compact listing, **2,033 bytes**: 25 node types, incl. `text` (content expression and attributes each) and 9 marks (`link(href,target,rel,title,...)`, `code`, `italic`, `bold`, `strike`, `sup`, `underline`, `uppercase`, `preservedStyle`). Example lines: `listItem: content "paragraph block*"`, `twoColumns: content "columnsColumn columnsColumn"`, `button: content "inline*"; attrs alignment="left", href="#"`, `heading: content "inline*"; attrs level=1`, `image: attrs alignment="center", src="", alt="", width="auto", height="auto", href=null`.
- `out/agent-schema.json`: JSON Schema 2020-12, **25.5 KB compact**, one `$def` per node and mark; child node types expanded from groups (`block`, `list`, ...); the content expression is kept as a `description`. It is a structural approximation: order and counts (`paragraph block*`, `columnsColumn{4}`, exactly two columns) cannot be expressed per child with `anyOf`.
- What cannot be derived from the editor's extensions headlessly: **attribute types and enumerations.** The ProseMirror spec holds a default per attribute and nothing else. `alignment` (left/center/right), heading `level` (1 to 6), `width` as number or string, ordered-list `type`: all absent. The JSON Schema therefore types every attribute as "any scalar". The top-level `doc` content is `block+`; a doc with no `container` is accepted and wrapped by the editor (and by the server entry).

Three documents written the way an LLM would from that listing (`docs/agent/`), run through (a) the JSON Schema (ajv), (b) the server validator (`schema.nodeFromJSON(...).check()` inside goja, same result in V8), (c) the real editor loading it in jsdom, default and with `enableContentCheck`:

| Document                                                                                                                                                                                                                 | JSON Schema                  | Server validator (goja)                                           | Real editor, default load                                                                                          | Real editor, `enableContentCheck`            |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------- | ----------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ | -------------------------------------------- |
| `a-clean` (h1, bold, list, button, image, centered link paragraph, Liquid)                                                                                                                                               | ok                           | ok, renders 5688 B, 3 `{{ }}` kept                                | all 149 chars and all nodes kept                                                                                   | no error                                     |
| `b-plausible-mistakes` (heading `level:"2"`, `align` for `alignment`, `marks:["bold"]`, empty text node, text directly in `listItem`, unknown `callout` node, button with `url`/`label` attrs, image inside a paragraph) | rejects (unknown attr)       | rejects: `There is no mark type undefined in this schema`         | **whole document silently replaced by one empty paragraph: 0 of 145 chars kept, no event, a console warning only** | `contentError` event: `Invalid JSON content` |
| `c-layout` (tableCell with raw text, `threeColumns` with 2 columns)                                                                                                                                                      | rejects (raw text in a cell) | rejects: `Invalid content for node tableCell: <"{{ item.qty }}">` | loads **unchecked**: invalid structure kept in the editor state, all text kept                                     | `contentError` event                         |

What is invalid in plausible agent output, and what each layer does (15 single-defect micro cases, `out/agent-matrix.json`):

- **Server validator rejects, with a message and no content dropped:** unknown node type, unknown mark, marks given as strings, empty text node, text directly in `doc` / `listItem` / `tableCell`, image inside a paragraph, `twoColumns` with 3 columns, empty or missing doc content. It reports only the first error, so an agent needs one round trip per error.
- **Real editor default load:** unknown node, unknown mark, string marks, empty text node and missing content all end with the **whole document replaced by an empty paragraph**, silently. Structural violations (text in `listItem`, image in `paragraph`, 3 columns) are loaded **unchecked** with nothing repaired. Text directly in `doc` **throws** (`Called contentMatchAt on a node with invalid content`).
- **Accepted by every layer, silently wrong:** unknown attribute names are dropped (`align:"center"` is ignored, no error: `paragraph.alignment` stays null); `level:"2"` and **`level:9`** pass (and `level:9` renders `<h9>`); a `button` with `url`/`label` attributes is valid and renders an empty button with `href="#"`; a `link` mark with no `href` is valid; the JSON Schema also accepts `twoColumns` with 3 columns and the 9-level heading.
- Net: only the server-side `nodeFromJSON` + `check` returned an error message for these; the editor's default loader dropped the whole document or kept the invalid structure without one, and its `enableContentCheck` mode reported only `Invalid JSON content`.

## 5. Hostile documents

Cases in `go/hostile_test.go`, `go/snippet_test.go`, `go/memory_test.go`. Through the goja render (before any sanitizer):

- Text containing `<script>` or `<img onerror>`: escaped by React (`&lt;script&gt;`), inert.
- `javascript:` href on a link mark or button, and in image `src`/`href`: React 19 rewrites it to `javascript:throw new Error('React has blocked a javascript: URL...')` (still a `javascript:` URL, harmless but present). `data:text/html,...` href passes unchanged (script text escaped). Quote-breakout hrefs and `width="100%\" onload=\"..."` are written as a properly quoted attribute value (single-quoted), not a breakout.
- `style` attribute: passes through as given into the element's inline style (`background:url(javascript:alert(1))` stays; the `</style><script>` tail is dropped by the style parser). Unknown attrs (`onclick`, `dangerouslySetInnerHTML`, `__proto__`, `constructor`) are ignored by ProseMirror and never reach React.
- Unknown node or mark types, a non-object or `null` JSON root, invalid JSON, a raw NUL byte in the JSON: rejected with an error string, no panic. `level:"script src=x"` makes React throw error 65, rejected. `level:9` renders `<h9>`.
- Through the bluemonday stand-in, every dangerous case above (`javascript:`, `data:text/html`, `url(`, `<h9`, `<script`) is gone.

Load (fresh VM per case, `TestHostileLoad`; after the TextEncoder moved to Go):

| Input                                | Result                                                                        |
| ------------------------------------ | ----------------------------------------------------------------------------- |
| 100 KB text                          | OK, 0.7 s                                                                     |
| 1 MB text                            | OK, 6.7 s                                                                     |
| 5 MB text (JSON 5 MB)                | OK, 33 s (54 s in the final full run), 10 MB of output                        |
| 1,000 paragraphs (60 KB JSON)        | OK, 8 s (190 KB output)                                                       |
| 10,000 paragraphs (600 KB JSON)      | not finished in 60 s, stopped by the interrupt                                |
| nesting 50 / 200 / 1,000 blockquotes | OK, 0.23 s / 0.85 s / 9.6 s (3.4 MB output at 1,000)                          |
| nesting 5,000 and 100,000            | clean `SyntaxError: exceeded max depth` from goja's `JSON.parse` in 4 to 8 ms |

No panic and no hang in any case. Cost grows faster than linearly with output size (prettier on a big document); 1,000 nested levels took about 10 s and 1,000 paragraphs about 8 s.

Limits:

- **Time: enforceable.** `vm.Interrupt` called from another goroutine stopped a 5 MB render at 301 ms for a 300 ms limit (`deadline at parse (bundle.js...)`), and at 5 to 400 ms in the sweep.
- **An interrupted VM did not recover.** The next render on the same VM failed (`promise pending, no timers`: the promise never settles) at **7 of 8** interrupt points; only the earliest (5 ms, which landed in synchronous code) was reusable. A warm-up render before the interrupted one made no difference (`TestInterruptReuseWarm`, still 7 of 8), so lazy first-use initialisation is not the cause. After the interrupt even prettier alone on `<p>x</p>` never settled, and a trivial `Promise.resolve().then(...)` job never ran (`__t` stayed 0). That points at goja's promise job queue being left stuck when an interrupt lands inside a microtask; I did not confirm it in goja's source and did not look for a way to reset it. Building a fresh VM took about 3 s.
- **Memory: no limit in goja.** A Go watchdog goroutine that reads `/memory/classes/heap/objects:bytes` every 20 ms and calls `Interrupt` worked: the 5 MB text case was stopped at 403 MB growth under a 400 MB limit; a normal render peaks at about 40 MB; 1,000 paragraphs peaks at about 94 MB; nesting 1,000 at 386 MB. It is a process-wide heap counter, so with several VMs in flight it cannot attribute growth to one of them. Input-size, depth and node-count caps before entering the VM were not implemented or tested.
- A VM per request would cost about 3 s each (cold start above).

Also: text with an escaped NUL (`\u0000`), a lone surrogate (`\ud800`) and an RTL override (`‮`) renders OK in 45 ms, no dangerous substring. Several load and memory figures above vary between runs because the machine was shared (a final full `go test -count=1` run, 501 s, passed; log in `out/final-go-test.log`; its numbers: 1,000 paragraphs 9.8 s, nesting 1,000 6.8 s, pool of 200 renders 14.6 s, watchdog peaks 422 / 134 / 355 MB).

## Not tested / caveats

- The first prototype's folder is not in this worktree: its documents and its Go sanitizer were re-created from its findings, not copied. The "browser-built export" reference is the real editor in **jsdom**, not Chromium (no Playwright run here), so byte-identity is shown against the package's own `composeReactEmail` on a real TipTap `Editor`, not against a browser.
- QuickJS (`fastschema/qjs`, `modernc.org/quickjs`): not tried because goja passed.
- Not measured: goja performance on a quiet machine (all runs shared the machine); a goja VM used from two goroutines at once (documented as unsafe, not exercised); a bundle shrunk by aliasing out `tailwindcss`, `prismjs`, `css-tree`, `prosemirror-view`; a render entry that skips prettier (the largest per-render cost) or caches the schema build; custom `EmailNode` extensions, themes other than `basic`, the code-block extension rendering (its lazy language loader is stubbed, so highlighted code blocks were not exercised), `previewText`, `globalContent` (theme/style data in the document JSON), collaboration extensions.
- The image extension is not a public export of `@react-email/editor`; it was imported from an internal chunk path, which would break on a package update.
- Sanitizer is the compact stand-in described in test 3, with a regex denylist for `style` values.
- Only the Liquid features listed were exercised (filters with string arguments, `{% if %}`, `{{ }}` in href and text); `{% for %}` around table rows is a known loss of the editor's HTML parse (first prototype) and does not occur here because rendering starts from JSON, but it was not retested.
