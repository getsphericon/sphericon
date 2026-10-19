# Embeddable Visual Email Editors (what could replace the body `<Textarea>`)

Researched 2026-10-18 from primary sources: GitHub repos and LICENSE files (read through the
`gh` API and a shallow clone of `GrapesJS/mjml` and `zalify/easy-email`), npm registry metadata
(`npm view`), and vendor docs fetched that day. Third-party roundups were used only as leads and
are listed in "Unverified". Docs pages were read through a summarising fetcher, so exact quotes
are marked as such; anything it said the page did not state is recorded as "not stated", not as
"absent". Nothing here was installed or run: **no round-trip, bundle or dark-mode behaviour was
tested**, only read from source and docs.

Scope: map the options and trade-offs. No pick: the owner decides in a grill. How vendors
constrain agents is in `docs/research/ai-email-design-landscape.md` and is not redone here.

## 1. Summary

- Only **two** open-source candidates speak MJML on both sides: **GrapesJS + `grapesjs-mjml`**
  (imports an `<mjml>` string, exports MJML) and **Easy Email** (`MjmlToJson` / `JsonToMjml`).
  Everything else (React Email editor, Maily, Unlayer, Beefree) stores its own JSON or emits
  HTML, which breaks the "Message content = subject + MJML" model of ADR 0003 and
  `internal/emailrender`.
- **Easy Email** is MIT and MJML-shaped but its open-source build is **React 18 only** (its own
  README: "React 19 Support: Not supported", "Only supports Chrome"), ships on MJML 4
  (`mjml-browser ^4.10.4`), its last npm release was 2026-06-10 and its UI is Arco Design, so it
  does not fit React 19 + Mantine without a fight. Easy Email Pro (React 19) is a paid product.
- **GrapesJS + grapesjs-mjml** is BSD-3-Clause, very active (core v0.23.8 on 2026-10-15, plugin
  last commits 2026-09-22), but it is a framework with its own UI chrome, themed by CSS
  variables set on `:root` (a conflict with "no custom CSS" unless Mantine's theme can emit them),
  its MJML plugin covers a fixed component list (no `mj-accordion`, `mj-carousel`, `mj-table`
  in the source tree), and the official React wrapper `@grapesjs/react` 2.0.0 (2026-03-06) peers
  on `grapesjs ^0.22.5` while core is 0.23.x.
- **React Email 6 editor (`@react-email/editor` 1.7.14, MIT, React 18/19)** is the cleanest fit
  for the React/Vite/TS stack and needs no hosted service, but it is a TipTap block editor that
  **exports HTML via React Email, not MJML**. Adopting it means changing the stored format.
- **Unlayer and Beefree** are proprietary hosted editors (Unlayer's editor loads from
  `editor.unlayer.com/embed.js`; Beefree needs `client_id`/`client_secret` and a login call to
  `auth.getbee.io`). Both break "self-hosted single binary, no phone-home, no key" and neither
  stores MJML. They are listed as references, with the one real upside: both ship an agent
  surface (MCP) that open-source editors lack.
- **No candidate documents an agent API for the editor's own document** apart from Resend's
  hosted MCP (TipTap JSON), Unlayer's MCP and Beefree's MCP (all vendor-hosted). For an MJML-only
  design the agent keeps writing MJML through `internal/mcpserver`; the editor is the human's
  view of the same string.
- The Liquid-before-MJML pipeline (`emailrender.RenderEmail`: "Liquid merge tags first (so a
  `{% if %}` can change MJML structure per recipient)") is the sharpest compatibility risk: a
  visual editor parses the template as MJML/XML, so a `{% if %}` wrapped around an `<mj-section>`
  may not survive import. Untested for every candidate; see open questions.

## 2. Comparison table

"MJML in/out" = reads and writes an MJML string for the same document. Dates are 2026.

|                                           | GrapesJS + grapesjs-mjml                             | Easy Email (OSS)                                                                                 | `@react-email/editor`                            | Maily (`@maily-to/core`)                        | Unlayer `react-email-editor`                                      | Beefree SDK                                                    |
| ----------------------------------------- | ---------------------------------------------------- | ------------------------------------------------------------------------------------------------ | ------------------------------------------------ | ----------------------------------------------- | ----------------------------------------------------------------- | -------------------------------------------------------------- |
| License                                   | BSD-3-Clause (core and plugin)                       | MIT                                                                                              | MIT                                              | MIT (repo `license` file; no npm license field) | wrapper MIT, editor proprietary                                   | npm wrapper Apache-2.0, service proprietary                    |
| AGPL + closed ee/ + self-host OK          | yes (permissive, self-contained)                     | yes (permissive)                                                                                 | yes (permissive)                                 | yes (permissive)                                | **no**: editor loaded from Unlayer CDN, project ID for production | **no**: client id/secret, vendor auth endpoint, per-user plans |
| Last release                              | core 0.23.8 (Oct 15); plugin 1.0.8 (Mar 13)          | npm 4.17.1 (Jun 10); last commit Aug 13 ("update demo")                                          | 1.7.14 (Oct 16)                                  | core 0.3.7 (Oct 14)                             | 2.1.3 (Oct 4)                                                     | 11.7.2 (Oct 13)                                                |
| React                                     | wrapper `@grapesjs/react` 2.0.0: `^18 \|\| ^19`      | **^18.2 only** (OSS)                                                                             | `^18 \|\| ^19`                                   | `^18 \|\| ^19`                                  | `>=16.8`                                                          | n/a (iframe SDK)                                               |
| Stored format                             | GrapesJS project JSON (own); MJML via export command | own block JSON; MJML via `JsonToMjml`                                                            | TipTap JSON / HTML                               | TipTap JSON                                     | Unlayer design JSON                                               | Beefree JSON                                                   |
| MJML in/out                               | in: `<mjml>` string; out: yes                        | in: `MjmlToJson`; out: `JsonToMjml`                                                              | **no** (HTML out via React Email)                | **no** (HTML via React Email render)            | **no** (HTML + JSON)                                              | **no** (HTML/JSON; HTML importer API is paid per import)       |
| Follows Mantine light/dark, no custom CSS | partial: needs `--gjs-*` vars on `:root`             | no: Arco theme CSS                                                                               | partial: CSS import plus theme objects           | Tailwind v4 in deps; `style.css`                | iframe, own appearance config                                     | iframe, own appearance config                                  |
| Brand lock via mj-attributes/theme        | head `mj-attributes` supported; no lock UI found     | `mj-attributes` read and written; fonts list prop                                                | theme objects (`createTheme`, `extendTheme`)     | not read                                        | design config / locked blocks (not read in detail)                | `brandRules` (agent only, Superpowers+)                        |
| Merge tags                                | none native; text `{{ }}` and `mj-raw`               | MergeTags attribute panel in source; Liquid `{% for %}`/`{% if %}` generated for advanced blocks | `Mention` extension in deps; docs: "not covered" | Variables, Repeat, Show If                      | `mergeTags` config option                                         | "Smart Merge Tags" listed, tier not stated                     |
| Agent surface                             | none found                                           | none found                                                                                       | hosted Resend MCP (TipTap JSON)                  | none found                                      | vendor MCP (OAuth)                                                | vendor MCP (live editor session)                               |

## 3. Per-candidate notes

### GrapesJS + grapesjs-mjml

- **License.** Core `LICENSE` is the BSD 3-clause text ("Copyright (c) 2017-current, Artur
  Arseniev"); GitHub's SPDX detector reports `NOASSERTION` but npm says `BSD-3-Clause`.
  `GrapesJS/mjml` is `BSD-3-Clause` in both repo and npm. Permissive: compatible with AGPL core
  and closed ee/. No key, no hosted dependency (it runs fully in the browser).
  <https://github.com/GrapesJS/grapesjs>, <https://github.com/GrapesJS/mjml> (accessed 2026-10-18)
- **Maintenance.** Core releases v0.23.3 (Jul 24) through v0.23.8 (Oct 15), 32 open issues,
  26.3k stars. Plugin: v1.0.8 (2026-03-13, "Update MJML version"), then fixes on 2026-09-12,
  09-20 and 09-22, 37 open issues, 719 stars. The plugin depends on `mjml-browser ^4.18.0` and
  `@types/mjml`; MJML 5.4.1 is current upstream and 6.0.0-alpha.1 exists
  (<https://github.com/mjmlio/mjml/releases>), so the plugin's compiler is a major behind.
  Bundle: npm unpacked size 12.5 MB (grapesjs) and 4.1 MB (grapesjs-mjml); these are unpacked
  tarball sizes, not gzipped bundle sizes. React: the official wrapper `@grapesjs/react` 2.0.0
  (2026-03-06) has peers `react ^18 || ^19` and `grapesjs ^0.22.5`, while core is 0.23.8: a peer
  mismatch to resolve. TypeScript types ship in core (the plugin is TS).
- **Format and round trip.** The plugin parses the MJML string through GrapesJS's component
  model and exports MJML again through its export command (`src/commands/openExportMjml.ts`; the README
  documents `overwriteExport`, `preMjml`, `postMjml`, `useXmlParser`). Components registered in
  `src/components/`: `mj-mjml`, `mj-head`, `mj-body`, `mj-wrapper`, `mj-group`, `mj-section`,
  `mj-column`, `mj-text`, `mj-image`, `mj-button`, `mj-social(-element)`, `mj-divider`,
  `mj-spacer`, `mj-style`, `mj-font`, `mj-hero`, `mj-navbar(-link)`, `mj-raw`; `mj-head` accepts
  `mj-preview`, `mj-attributes`, `mj-style`, `mj-font`, `mj-title`, `mj-raw`. Not present in the
  source tree: `mj-accordion`, `mj-carousel`, `mj-table`, `mj-class` handling, `mj-include`
  (grep of `src/` found none; `gomjml` implements `mj-accordion`, `mj-carousel`, `mj-table`).
  The README says `useXmlParser` is "Experimental: ... importing void MJML elements (without
  closing tags) like `<mj-image/>`", i.e. by default the import goes through an HTML parser.
  GrapesJS's own storage docs advise relying on the project JSON for persistence "since
  HTML/CSS can lose information" (accessed 2026-10-18). **Keeping MJML as the only stored format
  means: import on open, export on save, and accept that anything the model drops is lost on the
  first human save.** Round-trip for hand-written MJML is not verified.
  <https://grapesjs.com/docs/modules/Storage.html> (accessed 2026-10-18)
- **Theming.** `import 'grapesjs/dist/css/grapes.min.css'` (library stylesheet: fits the rule).
  The docs say there are CSS custom properties, shown with `--gjs-primary-color`,
  `--gjs-secondary-color`, `--gjs-tertiary-color`, `--gjs-quaternary-color` set on `:root`, and
  say to put custom rules after the GrapesJS styles. No dark mode is mentioned. Hypothesis, not
  tested: Mantine's `cssVariablesResolver` could emit those variables per color scheme, which
  would keep the repo free of `.css` files; otherwise it is custom CSS.
  <https://grapesjs.com/docs/getting-started.html> (accessed 2026-10-18)
- **Brand kit.** Head `mj-attributes` and `mj-font` are known components, so a brand block can
  be injected into the imported MJML as text before `loadProjectData`/import; locking blocks and
  styles is possible through GrapesJS component properties (`draggable`, `droppable`,
  `removable`, `editable`), which are core API not read in detail here. The plugin's
  `customComponents`, `blocks`, `block` options and `mjmlParser` hook (a custom `mjml-browser`
  instance "to extend MJML functionality or add custom MJML components") are the extension
  points.
- **Agent fit.** No MCP, no JSON schema for the editor document found. The agent's contract is
  the MJML string, unchanged.
- **Gaps.** Merge tags: none native (the plugin's only mention of templating is in `mj-raw`
  blocks); `{{ }}` in `mj-text` is plain text and `{% %}` outside text nodes is the risk.
  Image upload: GrapesJS Asset Manager (core; not read here). Preview: the canvas is a live
  render; the plugin "resets devices and sets a new one for mobile". Dark-mode preview: not
  found.

### Easy Email (zalify/easy-email, open-source build)

- **License.** MIT (`License` file read). Open source build vs "Easy Email Pro" (paid): the
  README's table says the OSS build has "Only supports Chrome", "React 19 Support: Not
  supported", no responsive design, "Basic features only". <https://github.com/zalify/easy-email>
  (readme read 2026-10-18)
- **Maintenance.** npm `easy-email-core/-editor/-extensions` 4.17.1, published 2026-06-10; repo
  pushed 2026-08-13 (messages "update demo", "refa"); no GitHub releases; 30 open issues, the
  newest 2026-03-23, one titled "fix: Self-XSS" (2025-10-17), others back to 2024. Peers:
  `react ^18.2.0`, `react-dom ^18.2.0`, `react-final-form ^6.5.7`, `mjml-browser ^4.10.4`.
  Unpacked npm size: core 1.8 MB, editor 1.4 MB, extensions 7.0 MB. TypeScript: yes.
- **Format.** The editor state is a block tree (`IPage`/`IBlockData`); `JsonToMjml` renders it
  (`packages/easy-email-core/src/utils/JsonToMjml.tsx`) and `MjmlToJson`
  (`packages/easy-email-extensions/src/shared/utils/MjmlToJson.ts`) parses an MJML string back.
  `MjmlToJson` reads `mj-font`, `mj-attributes`, `mj-style`, `mj-breakpoint` and head metadata
  into page `data.value` (it filters some `mj-all`/`mj-text`/`mj-section` attributes that match
  its own metadata fields) and **throws `"<tag> block no found"` for any tag without a block**.
  Standard blocks: Accordion, Button, Carousel, Column, Divider, Group, Hero, Image, Navbar,
  Page, Raw, Section, Social, Spacer, Table, Text, Wrapper. No `mj-class` handling found in a
  grep of core and extensions source. So MJML stays storable (parse on open, `JsonToMjml` on
  save), but the editor regenerates the MJML itself (own `mj-attributes` block, own
  `mj-style` for `.mjml-body`): hand-written structure, comments and ordering are not
  guaranteed to round-trip. Not tested.
- **Theming.** UI is Arco Design (`@arco-design/web-react`); the README imports
  `easy-email-editor/lib/style.css`, `easy-email-extensions/lib/style.css` and
  `@arco-themes/react-easy-email-theme/css/arco.css`. A second design system next to Mantine and
  three stylesheets; no Mantine or dark mode integration found in `packages/*/src`.
- **Brand kit.** `fontList` prop and the page block's head attributes; locking is not a
  documented feature of the OSS build.
- **Merge tags and logic.** `easy-email-core/src/utils/TemplateEngineManager.tsx` wraps blocks
  in generated `{% for item in source limit:N %}` / condition blocks (Liquid syntax) using
  `mj-raw`; `MergeTags.tsx` is an attribute-panel component. This is the one editor whose
  built-in data model already emits Liquid control flow around MJML blocks, matching our
  Liquid-first pipeline in spirit. Whether sphericon's `osteele/liquid` accepts exactly the
  generated output is untested.
- **Image upload.** `onUploadImage: (data: Blob) => Promise<string>` (README config table).
- **Agent fit.** None documented; the block JSON is a typed schema (`IBlockData`) an agent could
  be given, but nothing official says so.

### React Email 6 editor (`@react-email/editor`, Resend)

- **License.** MIT (package.json, `LICENSE.md` "Copyright 2024 Plus Five Five, Inc"). No key or
  hosted service stated in the docs. <https://github.com/resend/react-email> (accessed 2026-10-18)
- **Maintenance.** `@react-email/editor` 1.7.14 on 2026-10-16 (changelog: a slash-command null
  fix), repo pushed 2026-10-16, 31 open issues, 19.8k stars. Peers `react ^18.0 || ^19.0`.
  Dependencies: TipTap 3.17, Radix popover, `prismjs`, and **`react-email` 6.11.3** (unpacked
  3.5 MB) on top of its own 1.6 MB. TypeScript: yes (`.d.mts`). Docs note a slow-dev-compile
  issue from `CodeBlock` pulling every Prism grammar (workaround: patching `react-email`).
- **Format.** TipTap JSON (`"type": "doc"`) or an HTML string in; out: `composeReactEmail({
  editor, preview })` returns `{ html, text }` through React Email's `render`; the component ref
  exposes `getEmailHTML()`, `getEmailText()`, `getEmail()`. The docs do **not** describe a JSON
  export/import API in the pages read, and nothing about MJML. The serializer walks
  `EmailNode`/`EmailMark` extensions (`renderToReactEmail()`). Resend's own MCP tool descriptions
  call setting raw HTML "lossy" (see the landscape note). **Keeping MJML would mean either
  replacing MJML with TipTap JSON as the stored format (ADR 0003 supersession, a new renderer
  in Go or a Node sidecar, since the server renders MJML with `gomjml`), or writing a custom
  `EmailNode` set that serializes to MJML, which is custom serializer work with no upstream
  support.** Output is email-safe HTML; the editor never meets `gomjml`.
- **Theming.** `import '@react-email/editor/themes/default.css'` pulls colors plus bubble menu,
  slash command and inspector styles; individual `styles/*.css` files exist. Email theme is
  configured in JS: `EmailTheming.configure({ theme: 'basic' | 'minimal' })`, `extendTheme`,
  `createTheme`, style keys `body`, `container`, `h1`..`h3`, `paragraph`, `link`, `button`,
  `footer`, `image`. Docs: dark example is a custom theme object switched with React state; no
  CSS variables; font-family handling not documented; host-app dark mode not stated (all from
  the theming page, accessed 2026-10-18).
- **Brand kit.** Theme objects are the brand kit (colors, sizes, button radius); locking blocks
  is not documented. Agents: the hosted Resend MCP can `connect-to-editor` and
  `get-tiptap-json-content` (landscape note); that is Resend's hosted editor, not something an
  embedder gets for free.
- **Gaps.** Merge tags: docs "not covered"; the `Mention` extension is a dependency (an
  inference that variables would be built on it). Image upload: paste, drop, slash command and an
  upload plugin in `@react-email/editor/plugins`; hook names not given. Preview and dark-mode
  preview: not covered.

### Maily (`maily.to`, `@maily-to/core`)

- Not on the original list; found as the editor Novu documents. MIT (repo `license` file; npm
  `license` field empty). `@maily-to/core` 0.3.7 (2026-10-14), `@maily-to/render` 0.2.3,
  5 open issues, 4.0k stars, peers `react ^18 || ^19`. TipTap 2 based; `@maily-to/core` depends
  on **Tailwind v4** and ships `./style.css`; render package uses `juice` and
  `@react-email/render`. Components listed in the README: Logo, Buttons, Variables, Image,
  Divider, Spacer, Footer, Link Cards, Section, Columns, **Repeat**, **Show If Condition**.
  Output is HTML through React Email, not MJML. Unpacked size 4.2 MB. 0.x version line.
  <https://github.com/arikchakma/maily.to>, npm metadata (accessed 2026-10-18)
- Variables, repeat and conditional blocks are first-class, which is the closest built-in match
  to sphericon's Liquid use, but the output model is the same as the React Email editor's.

### Unlayer `react-email-editor` (proprietary, reference)

- **License.** The React wrapper is MIT (repo and npm, 2.1.3, Oct 4, 247 open issues, peers
  `react >=16.8`, 52 KB). It is a loader: `src/loadScript.ts` sets `defaultScriptUrl =
  'https://editor.unlayer.com/embed.js?2'`, so the editor itself is Unlayer's hosted, proprietary
  code, executed in the page. Docs: `projectId` is optional but needed "to access project
  settings and premium features in production"; it comes from a Developer Console project; no
  self-host option is described (installation page, accessed 2026-10-18). Pricing page:
  Free $0 (email, page, popup, document builders); White-Labeling from Launch ($250/mo, 5,000
  exports/mo); Custom Blocks and "Smart Merge Tags" on Scale ($750/mo); AI on Launch and up with
  add-on credits (<https://unlayer.com/pricing>, accessed 2026-10-18). Fails the self-host,
  no-phone-home and no-key constraints, and white-labeling is paid.
- **Format.** Unlayer design JSON plus exported HTML; no MJML. Easy Email Pro advertises
  migration from Unlayer templates, which confirms the JSON is a distinct format.
- **Agent.** Vendor MCP `https://api.unlayer.com/mcp` (landscape note).

### Beefree SDK (proprietary, reference)

- **License and access.** npm `@beefree.io/sdk` is Apache-2.0 (11.7.2, 2026-10-13) but it only
  drives the service. Docs: backend must call `POST https://auth.getbee.io/loginV2` with
  `client_id`, `client_secret` and a `uid`; "Never expose `client_id` or `client_secret` in
  frontend code"; tokens valid 12 hours; self-hosting not described
  (<https://docs.beefree.io/beefree-sdk/getting-started/installation>, accessed 2026-10-18).
  Pricing page: Free $0 (10 users, "Prototype in your own app"), Essentials $400/mo (100 users),
  Core $1,200/mo (800), Superpowers $3,000/mo (unlimited); white-label, custom rows and merge
  tags are listed but their tiers are not stated on the page
  (<https://developers.beefree.io/pricing-plans>, accessed 2026-10-18). A search summary also
  gave per-extra-user fees and $2 per HTML import on lower plans (see Unverified). A per-user
  fee model contradicts "no per-tenant fees" for a multi-tenant SaaS; the phone-home auth call
  contradicts self-hosting.
- **Format.** Beefree JSON and HTML; no MJML. **Agent:** MCP and Brand Rules for AI (landscape
  note), the most complete agent and brand-lock story of any candidate, only on paid plans.

### MJML's own tooling

- `mjml` 5.4.1 and `mjml-browser` 5.4.1 (MIT, 2026-10-09; repo pushed 2026-10-16), a
  `v6.0.0-alpha.1` pre-release (2026-10-09) adding "Dark mode via specific `--dark` modifier
  attributes" and "Responsive layout changes via specific `--responsive` modifier attributes".
  `mjml-browser` is a compiler (1.2 MB unpacked), not an editor; the monorepo's package list has
  no editor or builder package. The 5.4.1 release notes include "Note that templating tags break
  with `mj-html-attributes`", a reminder that MJML tooling is not Liquid-aware.
  <https://github.com/mjmlio/mjml/releases> (accessed 2026-10-18)
- A client-side `mjml-browser` live preview next to the textarea is the "middle" option from the
  landscape note, but the server uses `gomjml` (`go.mod`: `github.com/preslavrachev/gomjml
  v0.13.0`), whose README says it is "validated against MRML (Rust)"; a browser preview with the
  JS compiler can drift from what is sent. A server preview endpoint avoids that. (Not a
  candidate editor; included because every editor above also previews with `mjml-browser`.)
- Other MJML-native editors found: `cytostatic/mjml-editor` is a VS Code extension built on
  Easy Email (search result only), not embeddable. Keila has a "Visual MJML editor" on its
  roadmap as in progress (<https://www.keila.io/roadmap>, issue #288, accessed 2026-10-18): no
  implementation to reuse yet.

## 4. What each would cost sphericon to adopt

Common constraints from the repo: `emailrender` runs Liquid then `gomjml`; ADR 0003 defines
Message content as subject + MJML body; the broadcast form is a Mantine `Textarea`
(`src/pages/broadcasts/BroadcastForm.tsx`); agents write the same MJML through `internal/mcpserver`.

| Option                   | Format and ADR impact                                                                                                                                                                                                                     | Frontend impact                                                                                                                                                                                                        | Backend/agent impact                                                                                                                                                                                                              |
| ------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| GrapesJS + grapesjs-mjml | MJML stays the stored format; the editor is a view. No ADR change if "editor loses unsupported MJML on save" is acceptable. A new ADR is advisable on round-trip policy and Liquid placement.                                             | One editor component, one library stylesheet in `src/main.tsx`; theme variables must come from Mantine or it is custom CSS (a grill question); wrapper peer mismatch; large bundle; own toolbars unrelated to Mantine. | None for the pipeline. Agent text keeps working; an agent edit and a human edit write the same string. Components outside the plugin's list (accordion, carousel, table, `mj-class`) are not editable and may be dropped on save. |
| Easy Email (OSS)         | Same as above (MJML in/out), but state is its own block tree; MJML is regenerated, not preserved.                                                                                                                                         | React 18 only against our React 19 (`^19.3.1`); Arco CSS x3 plus Mantine; Chrome-only; peers `mjml-browser` 4. Likely blocked until the OSS build supports React 19 or the project takes the Pro licence.              | Liquid control-flow generation is built in; MJML 4 compiler in the browser vs `gomjml` on the server.                                                                                                                             |
| `@react-email/editor`    | **Supersedes ADR 0003's format**: stored content becomes TipTap JSON or HTML, `emailrender` stops compiling MJML (or an MJML serializer is written as custom `EmailNode`s). The `Validate` and merge-tag-before-compile pipeline changes. | Best React 19/TS fit; theme is JS objects; one `default.css` import; TipTap packages and `react-email` come along.                                                                                                     | MCP tools must expose TipTap JSON plus a schema (the Resend pattern) or keep accepting MJML and lose round-trip.                                                                                                                  |
| Maily                    | Same as React Email editor: HTML/JSON, not MJML.                                                                                                                                                                                          | Tailwind v4 in the dependency tree; `style.css` import.                                                                                                                                                                | Built-in variables, repeat and show-if blocks vs Liquid in `emailrender`.                                                                                                                                                         |
| Unlayer / Beefree        | Own JSON plus HTML; breaks MJML-only and the AGPL self-host promise.                                                                                                                                                                      | iframe/remote script; paid white-labeling (Unlayer) or per-user plans (Beefree).                                                                                                                                       | Vendor MCPs exist, hosted outside the product; operator and customer data leave the instance.                                                                                                                                     |

Common to all: a template or broadcast that already holds hand-written or agent-written MJML
must open without losing content, so a lossy-import policy (warn, block, or fall back to the
raw MJML text editor) is an ADR-level decision; `internal/emailrender` needs a decision on where
Liquid control flow may sit relative to MJML elements.

## 5. Open questions for a grill

1. Is **losing unsupported MJML on the first human save** acceptable, or must the editor refuse
   to load MJML it cannot represent and fall back to a raw MJML view?
2. Do we want a **block editor that rewrites MJML** (Easy Email, GrapesJS) or **one that owns the
   format and emits HTML** (React Email editor, Maily)? The second contradicts ADR 0003 and the
   "MJML-only" principle; is that supersession on the table?
3. Where may Liquid appear: only inside text and attribute values, or also as `{% if %}` /
   `{% for %}` around `mj-section`s? The editor choice follows the answer (only Easy Email and
   Maily generate control flow; none was tested against `osteele/liquid`).
4. Does "no custom CSS" allow a library's own editor stylesheet imported in `src/main.tsx`
   (AGENTS.md: "The only CSS imports are the library stylesheets in `src/main.tsx`")? And is
   emitting a vendor's CSS variables through Mantine's `cssVariablesResolver` acceptable?
5. Who owns brand lock: the editor config (UI-level) or the MJML itself (`mj-attributes` /
   `mj-class` injected server-side, as the landscape note's "brand kit" direction assumes)? An
   MJML-level kit works for agents and humans alike regardless of editor.
6. Is a visual editor needed at all, or is a **server-rendered preview pane** (our `gomjml` output
   via an endpoint, so it matches what is sent) beside the textarea enough? (Landscape section D,
   "Middle".)
7. Compiler parity: browser editors compile with JS `mjml-browser` 4.x while we send with
   `gomjml` v0.13.0. Is a visible difference between editor preview and delivered HTML acceptable?
8. Which editors need ee/ gating (an editor in the AGPL core is fine for every permissive
   candidate; a vendor-hosted one would need an ADR 0014 boundary decision)?
9. Is dark-mode preview of the email (not the editor chrome) required? Only MJML 6 alpha documents
   dark-mode attributes; no candidate editor documents a dark preview.

## 6. Unverified

- **Round-trip fidelity** of hand-written MJML through GrapesJS+plugin and Easy Email: read from
  source only; no import/export was run.
- **Bundle size** (gzipped) of every candidate: only npm unpacked tarball sizes were read.
- **Liquid survival**: whether `{% if %}` between MJML elements, `{{ }}` in attributes, or
  `{% for %}` blocks survive import and export in any editor.
- **GrapesJS MJML locking** (non-draggable or non-editable components), Asset Manager upload hook
  and device preview: core GrapesJS features not read on their docs pages.
- **Easy Email** dark-mode support, Mantine-compatibility and `mj-class` behaviour: nothing found
  in source; absence of a grep hit is not proof.
- **React Email editor** JSON export/import API, `Mention` as merge-tag mechanism, image upload
  hook names, and whether `composeReactEmail` runs in the browser: the fetched pages did not say.
- **Maily** agent surface and exact feature set beyond the README list; licence is from the
  repo `license` file, npm lists none.
- **Beefree**: per-user overage ($5.00 Free, $3.50 Essentials, $1.25 Core, none on Superpowers),
  "$2 per template import" and the Content Services API quotas came from a search-result summary
  of a Beefree usage-fee article, not from a page I read; the fetched pricing page lists tiers
  and prices but not feature-to-tier mapping. Whether the Free tier permits production use is not
  stated on that page. A startup-program discount was only in a third-party-hosted blog snippet.
- **Unlayer** exact free-tier limits and whether merge tags are on Free: the pricing text read was
  ambiguous.
- Third-party leads (not relied on): LogRocket and portalZINE roundups of email editors,
  `madewithreactjs.com/easy-email`, dev.to Puck article.
- MJML app (desktop) and Mailjet Passport were not checked.

## 7. Sources (all accessed 2026-10-18)

- GrapesJS core repo and LICENSE: <https://github.com/GrapesJS/grapesjs>
- grapesjs-mjml repo, README, `src/components`: <https://github.com/GrapesJS/mjml>
- GrapesJS docs: <https://grapesjs.com/docs/getting-started.html>,
  <https://grapesjs.com/docs/modules/Storage.html>
- npm: `grapesjs`, `grapesjs-mjml`, `@grapesjs/react`, `easy-email-core`, `easy-email-editor`,
  `easy-email-extensions`, `@react-email/editor`, `react-email`, `@maily-to/core`,
  `@maily-to/render`, `react-email-editor`, `@beefree.io/sdk`, `mjml`, `mjml-browser`
  (`npm view <pkg>` metadata)
- Easy Email repo: <https://github.com/zalify/easy-email> (readme, `License`,
  `packages/easy-email-core/src/utils/*`, `packages/easy-email-extensions/src/shared/utils/MjmlToJson.ts`)
- React Email repo and editor package: <https://github.com/resend/react-email>
  (`packages/editor/package.json`, `readme.md`, `CHANGELOG.md`)
- React Email editor docs: <https://react.email/docs/editor/overview>,
  <https://react.email/docs/editor/getting-started>,
  <https://react.email/docs/editor/features/email-export>,
  <https://react.email/docs/editor/features/theming>
- Maily: <https://github.com/arikchakma/maily.to>
- Unlayer: <https://github.com/unlayer/react-email-editor>,
  <https://docs.unlayer.com/builder/installation>, <https://unlayer.com/pricing>
- Beefree: <https://docs.beefree.io/beefree-sdk/getting-started/installation>,
  <https://developers.beefree.io/pricing-plans>
- MJML releases and packages: <https://github.com/mjmlio/mjml>
- gomjml: <https://github.com/preslavrachev/gomjml>
- Keila roadmap: <https://www.keila.io/roadmap>
- In-repo: `docs/research/ai-email-design-landscape.md`, `docs/adr/0003-templates-copied-not-referenced.md`,
  `internal/emailrender/emailrender.go`, `src/pages/broadcasts/BroadcastForm.tsx`, `go.mod`, `package.json`
