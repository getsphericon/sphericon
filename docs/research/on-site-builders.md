# Embeddable builders and content schemas for On-site messages

Researched 2026-11-01 for [#951](https://github.com/getsphericon/sphericon/issues/951) (map
[#948](https://github.com/getsphericon/sphericon/issues/948)). Sources are primary: the libraries' repositories (shallow
clones of `puckeditor/puck`, `prevwong/craft.js`, a sparse clone of `GrapesJS/grapesjs`, other repos read through the
GitHub API), their LICENSE files, npm registry metadata (`npm view`, release date = `time[<version>]`), files fetched from
unpkg, and the vendors' own docs and help centers. Docs pages were read through a summarising fetcher; anything it said a
page did not state is recorded as "not stated", not as "absent". **Nothing was installed or run in the app**: no
round-trip, theming or rendering behaviour was tested. Bundle sizes are gzip -9 of the file published on npm, measured
here, not a bundler's output.

Scope: facts a decision waits on; no pick. The decision belongs to the prototype ticket
[#958](https://github.com/getsphericon/sphericon/issues/958). Display rules, triggers, frequency and delivery were
researched in [`on-site-messages-and-forms.md`](https://github.com/getsphericon/sphericon/blob/research/on-site-messages-and-forms/docs/research/on-site-messages-and-forms.md)
(branch `research/on-site-messages-and-forms`) and are not redone. The email editor facts come from
[ADR 0040](../adr/0040-email-content-is-html-authored-in-an-embedded-editor.md) and
[embeddable-email-editors.md](embeddable-email-editors.md).

Constraints from the repo that every candidate is measured against:

- React `^19.3.1`, Mantine `^9.7.4`; "no custom CSS": the only CSS imports are library stylesheets in `src/main.tsx`
  (`CODING_STANDARDS.md`, `package.json`).
- No hosted service, no key, permissive licence (the ticket).
- The On-site message renders **in the browser, on the customer's origin**, shown by the tracker (`packages/analytics`,
  `t.js`), not in goja (map #948, Notes). `t.js` has a size budget of **17 kB gzip** (`size-limit` in `package.json`:
  `"path": "packages/analytics/dist/t.js", "limit": "17 kB"`).
- ADR 0040's agent pattern: the agent writes the stored document from a schema, the server checks it and answers `422`
  with a reason, never repairs in silence.

## 1. Summary

- **Only one actively maintained, MIT, React 19 visual builder that is a component of our app and needs no service:
  Puck** (`@puckeditor/core` 0.24.0, 2026-10-29; the old `@measured/puck` is deprecated in its favour). It stores a
  plain JSON tree of `{type, props}` against a closed component config we write, and the same config's `render`
  functions draw it, in the editor and in `<Render>`. Its UI is its own (CSS with `--puck-*` tokens), not Mantine.
- **GrapesJS** (BSD-3-Clause, very active) is the other live candidate. It is HTML/CSS-shaped: it stores project JSON
  but its output is HTML plus global CSS, per-device styles are media-query rules, and its React wrapper (`@grapesjs/react`
  2.0.0, released 2025-02-20) peers on `grapesjs ^0.22.5` while core is 0.23.8. The Grapes **Studio SDK** is proprietary
  (licence key, watermark).
- **Craft.js** (MIT, headless, all UI ours, so the best fit for Mantine) has had **no release or commit since
  2025-02-14** and 212 open issues. **Easyblocks** is out on three counts (React ≤18 peer, no release since 2024-07-24,
  repo `LICENSE.md` is AGPL-3.0 while npm says ISC).
- **`@react-email/editor`** (already a dependency, 1.7.18) has **no form input node** among its node types and its
  export path is email HTML built on the server (ADR 0040). Reusing it for On-site messages means custom TipTap nodes
  for every form element and a browser renderer it does not ship.
- **Form-specific libraries** model multistep forms and a thank-you step natively but are heavy in the browser:
  SurveyJS Form Library (MIT; its **Survey Creator builder is a paid per-developer licence**), Form.io `@formio/js`
  (MIT renderer and builder, Bootstrap-styled). Their runtimes are ~400 kB gzip, against a 17 kB tracker.
- **No candidate's renderer fits inside `t.js`.** React 19.2 + react-dom client alone is ~99 kB gzip (unminified CJS
  build), Preact 10 is 4.8 kB. A builder whose document is plain JSON leaves the on-site renderer free to be written
  separately (a small DOM renderer over the same JSON, possibly loaded as a second script only when a message shows); a
  builder whose output is HTML/CSS puts that HTML on the customer's page, which then needs sanitising and style isolation.
- **Proprietary popup builders exist** (Unlayer `displayMode: 'popup'`, Beefree Popup Builder) but both are hosted
  services with keys; Beefree's output is an HTML partial with no open/close script and per-device settings.
- **Vendors converge on one content model** (section 4): a message is a short sequence of screens (steps, views,
  pages) moved through by button actions (next, submit, close, redirect), with a success screen at the end (not at
  Braze) and a teaser that belongs to the same message (before, after close, or both). Inside a screen: rows, columns,
  blocks. Mobile is **one design with linked per-device overrides** (style overrides, per-block visibility), not a
  second design. Brand values are palettes copied at authoring time; no vendor updates existing forms when the brand
  changes. **Only Klaviyo publishes the document**: a read/write Forms API,
  `versions[] → steps[] → columns[] → rows[] → blocks[] → action`, draft-only edits that replace the whole definition,
  publish as a job.
- **Agent schema**: Puck's `Data` and SurveyJS's JSON are closed, declarative trees that a JSON Schema can describe
  (SurveyJS even generates one: `Serializer.generateSchema()`); GrapesJS components plus CSS are effectively free-form.
  A plain JSON document under a closed schema can be validated in Go with no JS runtime (section 5).

## 2. Comparison table

Dates are release dates of the version named (npm `time[<version>]`). "Renders outside the editor" means: what draws the
stored document on a page that does not run the editor.

|                            | Puck                                                                                                                                                | GrapesJS (+ `@grapesjs/react`)                                                                                 | Craft.js                                                       | Easyblocks                              | `@react-email/editor`                                                      | SurveyJS (Form Library + Creator)                              | Form.io `@formio/js`                                    | coltorapps Builder                                               |
| -------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------- | --------------------------------------- | -------------------------------------------------------------------------- | -------------------------------------------------------------- | ------------------------------------------------------- | ---------------------------------------------------------------- |
| Licence                    | MIT                                                                                                                                                 | BSD-3-Clause (core); wrapper MIT on npm; **Studio SDK proprietary**                                            | MIT                                                            | npm ISC, repo `LICENSE.md` **AGPL-3.0** | MIT                                                                        | library MIT; **Creator commercial**                            | MIT                                                     | MIT                                                              |
| Last release               | 0.24.0 (2026-10-29)                                                                                                                                 | core 0.23.8 (2026-10-15); wrapper 2.0.0 (2025-02-20)                                                           | 0.2.12 (2025-02-14)                                            | 1.0.10 (2024-07-24)                     | 1.7.18 (2026-10-30)                                                        | 3.2.3 (2026-10-27)                                             | 5.7.0 (2026-10-09)                                      | 0.2.4 (2025-07-08)                                               |
| Activity (2026-11-01)      | pushed 10-29, 13.6k stars, 177 open issues                                                                                                          | pushed 10-21, 26.4k stars, 31 issues                                                                           | last commit 2025-02-14, 212 issues                             | last push 2024-08-15                    | pushed 10-30, 11 issues                                                    | pushed 11-01                                                   | pushed 10-30                                            | last push 2025-10-19, 198 stars                                  |
| React 19                   | peer `^18 \|\| ^19`                                                                                                                                 | wrapper `^18 \|\| ^19`, but peers `grapesjs ^0.22.5`                                                           | peer up to `^19`                                               | **`≤ ^18`**                             | `^18 \|\| ^19`                                                             | Creator peer up to `^19`                                       | `@formio/react` `>=17`                                  | `^18 \|\| ^19`                                                   |
| Stored document            | JSON `Data`: `{content: [{type, props}], root, zones}`; nested content in `slot` props; richtext prop is an HTML string                             | project JSON (`assets`, `styles`, `pages`); docs: rely on it, not on HTML/CSS                                  | JSON map of nodes keyed by id (`SerializedNodes`)              | not read                                | TipTap JSON (`doc`)                                                        | survey JSON: `pages[].elements[]`                              | form JSON `components[]`, `display: "wizard"` for steps | JSON of entities with zod-validated attributes                   |
| Renders outside the editor | `<Render config data>` (React) with the same component `render` functions; or any renderer we write over the JSON                                   | `editor.getHtml()` / `getCss()` from an editor instance (`headless: true` exists); output is HTML + global CSS | `<Editor enabled={false}><Frame json>` (React + Craft runtime) | not read                                | email HTML via React Email, run on our server in goja (ADR 0040)           | `survey-js-ui` (Preact inside, framework-free) + `survey-core` | `Formio.createForm` (plain JS)                          | interpreter components (React)                                   |
| Renderer size, gzip        | React 19.2 + react-dom client ≈ 99 kB (unminified CJS) + Puck render code (not measured)                                                            | none beyond the HTML/CSS (plus our own behaviour script)                                                       | React + Craft (not measured)                                   | n/a                                     | n/a in the browser                                                         | `survey-core` 400 kB + `survey-js-ui` 60 kB                    | `formio.form.min.js` 391 kB                             | React (not measured)                                             |
| Desktop / mobile editing   | iframe viewports (360, 768, 1280, full) resize the preview; per-device props **not documented** in the data model                                   | Device Manager: devices with `widthMedia`; styles per device stored as media-query rules                       | none built in (headless)                                       | not read                                | email preview only                                                         | not read                                                       | not read                                                | none built in (headless)                                         |
| Editor UI and theming      | own UI, `@puckeditor/core/puck.css` (`no-external.css` avoids loading Inter from a CDN); `--puck-*` tokens; field and UI overrides; composition API | own UI themed by `--gjs-*` vars on `:root`, or custom React UI around the iframe canvas via `@grapesjs/react`  | headless: all UI ours                                          | Stitches CSS-in-JS                      | `themes/default.css`, `--re-*` vars mapped to Mantine (proven in ADR 0040) | own UI and theme JSON                                          | Bootstrap CSS + icons                                   | headless: all UI ours                                            |
| Agent / schema             | none exported; `Data` is typed by the config (TS); hosted **Puck AI** (Puck Cloud, beta) generates `Data` from the config                           | none; components + CSS rules                                                                                   | none                                                           | not read                                | schema derived from extensions (ADR 0040)                                  | `Serializer.generateSchema()` in `survey-core`                 | form JSON documented by Form.io                         | attributes are zod schemas, validated server-side by the library |

## 3. Per-candidate notes

### Puck (`@puckeditor/core`)

- **Licence and maintenance.** `LICENSE`: MIT, "Copyright (c) The Puck Contributors". npm `@puckeditor/core` 0.24.0
  released 2026-10-29; `@measured/puck` is deprecated with "Puck has moved. Please use @puckeditor/core instead".
  0.23.0 was 2026-08-07 (`CHANGELOG.md`). Still a 0.x line. Peers `react ^18.0.0 || ^19.0.0`. Dependencies include
  `@dnd-kit/*`, TipTap 3 packages (the `richtext` field), `zustand`, `happy-dom`. Unpacked 2.9 MB.
  <https://github.com/puckeditor/puck>, npm.
- **Document.** `Data` is `{ "content": [ { "type": "HeadingBlock", "props": { "id": "HeadingBlock-1234", "title":
  "Hello, world" } } ], "root": { "props": {...} }, "zones": {} }` (docs `api-reference/data-model/data.mdx`). `zones` is
  to be deprecated: nested regions are `slot` fields, whose content lives inside the parent's props. A component type and
  its fields come from **our** config (`components: { X: { fields, defaultProps, render } }`); field types are `text`,
  `textarea`, `number`, `select`, `radio`, `checkbox`, `array`, `object`, `slot`, `richtext`, `external`, `custom`
  (docs `api-reference/fields/*`). `migrate()` upgrades payloads across Puck versions
  (`integrating-puck/data-migration.mdx`).
- **Rich text.** The `richtext` field is TipTap; the editor writes `editor.getHTML()` into the prop
  (`components/RichTextEditor/lib/use-synced-editor.ts`), and rendering parses an HTML string (or accepts a TipTap JSON
  doc) and emits HTML with `@tiptap/html`'s `generateHTML` into `dangerouslySetInnerHTML`
  (`components/RichTextEditor/components/Render.tsx`). So a richtext prop is an HTML string in the stored JSON.
- **Rendering outside the editor.** `<Render config data />` draws the `Data` with the same `render` functions
  (`api-reference/components/render.mdx`); an `rsc` entry exports a server-safe `Render`, `resolveAllData`, `migrate`,
  `walkTree` (`packages/core/bundle/rsc.tsx`). It needs React. Because the document is plain `{type, props}`, a
  renderer of our own (no React) can walk the same tree; `walkTree` is the library's helper for that shape.
- **Desktop and mobile.** "The Puck preview renders in a same-origin iframe that can be resized to simulate different
  viewports"; defaults Small 360px, Medium 768px, Large 1280px, Full-width (`integrating-puck/viewports.mdx`).
  Per-viewport props are not documented in the data model; a mobile variant would be modelled in our fields.
- **Theming and fit with Mantine.** "Puck's default interface is styled with CSS custom properties (design tokens)…
  to create a custom theme… or to implement dark mode" (`extending-puck/theming.mdx`); tokens are set on any ancestor
  element, e.g. `--puck-color-interactive`. By default it loads Inter from a CDN; `@puckeditor/core/no-external.css`
  avoids that. Hypothesis, **not tested**: emitting `--puck-*` from Mantine's `cssVariablesResolver`, as ADR 0040 did for
  `--re-*`. Field rendering can be replaced through `overrides`/`fieldTypes` and the layout through the composition API
  (`<Puck>` with `<Puck.Preview />`, `<Puck.Fields />`…), so Mantine inputs can stand in for Puck's. The canvas iframe
  syncs the host's stylesheets (`integrating-puck/styling.mdx`). `permissions` (global, per component,
  `resolvePermissions`) toggle drag, delete, duplicate, edit (`api-reference/permissions.mdx`).
- **Agent.** No JSON Schema export found in the source. Puck AI (`@puckeditor/cloud-client` `generate()`, an editor
  plugin proxied through `puckHandler()`) generates `Data` from a prompt and the config, through **Puck Cloud**; docs
  call it a beta (<https://puckeditor.com/docs/ai/headless-generation>, <https://puckeditor.com/docs/ai/getting-started>).
  The editor itself does not need it.

### GrapesJS

- **Licence and maintenance.** BSD-3-Clause (repo LICENSE, npm), core 0.23.8 released 2026-10-15, last commit
  2026-10-21 ("Add page frame events"). Unpacked 12.5 MB. `@grapesjs/react` 2.0.0 (MIT on npm) released 2025-02-20,
  repo not pushed since; peers `react ^18 || ^19`, `grapesjs ^0.22.5` (core is 0.23.x). The **Grapes Studio SDK**
  (`@grapesjs/studio-sdk` 1.2.1) is under its own licence: "a limited, non-exclusive, non-transferable, revocable
  license", "a valid key may be required to activate and/or continue your licensed use of certain features", may show
  a watermark that cannot be removed without a separate agreement
  (<https://github.com/GrapesJS/studio/blob/main/LICENSE.md>). `grapesjs-plugin-forms` (BSD-3-Clause) last released
  2.0.6 on 2023-06-07.
- **Document.** Project JSON with `assets`, `styles`, `pages`; the docs say it "doesn't contain HTML/CSS of your pages"
  and "You should only rely on the JSON project data in order to load your project properly in the editor"; HTML/CSS
  can be produced at save time with `editor.getHtml({ component })` / `editor.getCss({ component })`
  (<https://grapesjs.com/docs/modules/Storage.html>). Components may carry arbitrary HTML attributes and styles: the
  document is HTML-shaped, not a closed vocabulary.
- **Rendering outside the editor.** No standalone renderer: HTML/CSS come from an editor instance; the editor config
  has `headless?: boolean` ("Init headless editor", `packages/core/src/editor/config/config.ts`). What lands on the
  customer's page is HTML plus CSS rules that apply globally unless scoped by us. Behaviour (submit, steps, close) is not
  in the export.
- **Desktop and mobile.** Devices (`desktop`, `tablet` 770px/`widthMedia` 992px, `mobileLandscape`, `mobilePortrait`
  320px/480px by default); `widthMedia` is "the width which will be used in media queries", `priority` "the order of
  media queries" (`packages/core/src/device_manager/config/config.ts`, `docs/api/device.md`). Styling an element while a
  device is selected writes a media-query rule.
- **Theming.** Default UI uses `--gjs-*` variables set on `:root` and its own stylesheet (prior research,
  [embeddable-email-editors.md](embeddable-email-editors.md)); or `@grapesjs/react` "Custom UI": "Using the `<Canvas/>`
  component inside the main wrapper will disable the core default UI", so the panels can be ours (Mantine) around the
  iframe canvas (wrapper README).
- **Agent.** No schema; an agent would write components and CSS.

### Craft.js

- MIT ("Copyright (c) 2020 Previnash Wong Sze Chuan"). `@craftjs/core` 0.2.12, 2025-02-14; the repository's last commit
  is that release ("chore: release (#729)"), 212 open issues. Peer `react ^16.8 || ^17 || ^18 || ^19`.
- Headless: "A React Framework for building extensible drag and drop page editors"; every panel is ours.
- Document: `query.serialize()` returns JSON; `<Frame json={…}>` loads it (`site/docs/overview.md`,
  `site/docs/guides/save-load.md`). Type: `SerializedNodes = Record<NodeId, SerializedNode>`, a flat map keyed by node id
  with the component's resolved name, props, child ids and parent (`packages/core/src/interfaces/nodes.ts`).
- Rendering without editing: `<Editor enabled={false}>` ("all editing capabilities will be disabled",
  `site/docs/api/Editor.md`), i.e. React plus the Craft runtime. No device editing built in.

### Easyblocks

- npm `@easyblocks/core` and `@easyblocks/editor` 1.0.10, released 2024-07-24; peers `react … || ^18.0.0` (no 19).
  npm `license` field is ISC on core and absent on editor; the repo (`easyblockshq/easyblocks`, last push 2024-08-15)
  has a root `LICENSE.md` with the GNU AGPL-3.0 text. The licence conflict was not resolved; the React peer and the
  dormancy exclude it either way.

### `@react-email/editor` (ADR 0040's editor)

- 1.7.18 released 2026-10-30, MIT, already in `package.json`; used in `src/components/VisualEditor.tsx`.
- The node and mark names in the installed `dist/*.mjs` are: `body`, `button`, `columnsColumn`, `container`, `div`,
  `Heading`, `image`, `section`, `table`/`tableRow`/`tableCell`/`tableHeader`, `twoColumns`/`threeColumns`/`fourColumns`,
  `Text`, `Title`, `Subtitle`, `Quote`, `Code`, `previewText`, marks (`bold`, `italic`, `strike`, `underline`, `sup`,
  `uppercase`, `code`) and attribute extensions. **No input, checkbox, select or form node.**
- Its export is email HTML (React Email `render`), which sphericon runs on the server in goja (ADR 0040). The map says
  on-site renders in the browser, not in goja. Carrying it over unchanged would mean custom TipTap nodes for each form
  element and a browser renderer for its JSON that the package does not ship. Its prose editing (TipTap JSON with the
  Mantine mapping already proven) is the part that could be reused for text inside an element.

### Form-specific libraries

- **SurveyJS.** Form Library is "A free MIT-licensed UI component"; Survey Creator (the builder) is proprietary,
  per developer: Basic $589 / €499, PRO $1059 / €899 one-time with 12 months of updates
  (<https://surveyjs.io/licensing>). 3.2.3 released 2026-10-27, active (pushed 2026-11-01). Survey JSON: `pages[]`, each
  with `elements[]`; navigation by `visibleIf`, `pageNextText`, `completeText`; a complete page with `completedHtml` and
  `completedHtmlOnCondition` (`expression` + `html`); `showCompletePage`
  (<https://surveyjs.io/form-library/documentation/design-survey/create-a-multi-page-survey>). `survey-core` exposes
  `generateSchema(className?)` on its serializer (`typings/src/jsonobject.d.ts`) and a `./linter` export.
  `survey-js-ui` is "UI component that renders dynamic, JSON-based forms… in HTML/CSS/JavaScript apps" with no runtime
  dependencies besides `survey-core`. Measured: `survey.core.min.js` 400 kB gzip, `survey-js-ui.min.js` 60 kB gzip.
- **Form.io (`@formio/js`).** MIT, 5.7.0 released 2026-10-09. README: a "plain JavaScript form renderer and SDK", with a
  "Complete Form Builder which creates the JSON schema used to render the forms" (`Formio.builder`); a form is a wizard
  when its `display` is `wizard`; the examples load Bootstrap and Bootstrap Icons from a CDN
  (<https://github.com/formio/formio.js>). Measured: `formio.form.min.js` 391 kB gzip.
- **coltorapps Builder.** MIT, `@coltorapps/builder-react` 0.2.4 released 2025-07-08, 198 stars. README: "Headless",
  "Full-stack: … validate schemas on the back-end", "Zero dependencies"; attributes are zod schemas
  (<https://github.com/coltorapps/builder>). Small project, quiet since mid-2025.
- **react-jsonschema-form** (`@rjsf/mantine` 6.11.0, Apache-2.0) is already in `package.json`; it renders a form from a
  JSON Schema in React with Mantine. It is a renderer, not a visual builder; listed because it is in the tree.

### Proprietary popup builders (reference only)

- **Unlayer** popup builder: `unlayer.init({ id, displayMode: 'popup', projectId })`; position, overlay colour and a
  "Display Delay" set in the editor; `unlayer.exportHtml(cb, { popupId })` produces HTML; no open/close script is
  documented (<https://docs.unlayer.com/builder/popup-builder>). The editor loads from Unlayer's CDN (prior research).
- **Beefree** Popup Builder: returns HTML partials ("will not return a full HTML page because the host application is
  the final destination"), "doesn't provide the scripts to control the popup's behavior, such as opening and closing";
  "you can apply every setting for both desktop and mobile design modes" (`backgroundImageMobile`,
  `contentWidthMobile`); layout presets `classic-center`, `classic-top-right`, …, `drawer-left`, `drawer-right`,
  `bar-bottom`, `bar-top` (<https://docs.beefree.io/beefree-sdk/visual-builders/popup-builder/popup-builder-getting-started>).
  Needs client id and secret against Beefree's auth (prior research).

### What the tracker can carry

| Runtime (file published on npm)                                      | gzip    |
| -------------------------------------------------------------------- | ------- |
| `t.js` budget (`size-limit`)                                         | 17 kB   |
| `react@19.2.0/cjs/react.production.js`                               | 4.4 kB  |
| `react-dom@19.2.0/cjs/react-dom-client.production.js` (not minified) | 94.7 kB |
| `preact@10.27.2/dist/preact.min.js`                                  | 4.8 kB  |
| `survey-core@3.2.3/survey.core.min.js`                               | 400 kB  |
| `survey-js-ui@3.2.3/survey-js-ui.min.js`                             | 59.7 kB |
| `@formio/js@5.7.0/dist/formio.form.min.js`                           | 391 kB  |

A minified react-dom is smaller than the CJS file measured, but still several times the whole `t.js` budget. Options
that follow from the numbers, recorded, not recommended: a renderer of our own over a JSON document inside or beside
`t.js`; or a second script the tracker loads only when a message is to be shown, outside the `t.js` budget.

## 4. How vendors model an on-site message

Content and builder structure only (display rules are in the earlier research). help.drip.com and help.sleeknote.com
refuse automated page reads (403); their articles were read in full through the Zendesk Help Center JSON API
(`/api/v2/help_center/en-us/articles/<id>.json`) and are cited by their `html_url`. "(search index)" marks a claim seen
only in a search result, not on the page.

### Klaviyo sign-up forms

- **Blocks.** Elements: Text, Button, Image, Coupon, SMS disclosure, Countdown timer, Sign-up counter; inputs: Email,
  Phone number (collects SMS consent by default), Text, Radio buttons, Multi checkboxes, Date, Dropdown; marketing
  consent: "Tracking consent" ("Separate checkboxes" or "Combined checkbox"), "Age gate". One email input per step per
  device view; "Email inputs cannot appear on the success step"
  (<https://help.klaviyo.com/hc/en-us/articles/4413550187035>).
- **Document.** The Forms API publishes the whole document, read and write: `data.attributes.definition.versions[]`;
  a version has `name`, `type`, `triggers`, `teaser`, `location`, `status`, `ab_test`, `steps[]`; then
  `steps[] → columns[] → rows[] → blocks[] → action`. "Forms may only have multiple columns if they also have a side
  image." Create with `POST /api/forms/` ("All forms must be created in a `draft` status"); `PATCH` "replaces the stored
  definition entirely", only on drafts, and versions cannot be added or removed; publishing is
  `POST /api/form-publish-jobs` and replaces the live version with the draft; no A/B tests through the API
  (<https://developers.klaviyo.com/en/reference/forms_api_overview>, read twice, once to check the agent's notes).
  The Create Form schema lists block types `html_text, email, phone_number, button, image, coupon, countdown_timer,
  signup_counter, spin_to_win, age_gate, opt_in_code, promotional_sms_checkbox, text, checkboxes, radio_buttons,
  dropdown, date, sms_disclosure`, action types `close, next_step, open_form, promotional_sms_subscription, redirect,
  resend_opt_in_code, submit_opt_in_code, subscribe_via_sms, subscribe_via_whatsapp, go_to_inbox,
  submit_back_in_stock, skip_to_success`, version `type` `banner | embed | flyout | full_screen | popup`, and teasers
  as `teasers[]` with `display_order` `after | before | before_and_after`, `teaser_type` `circle | corner | rectangle`,
  `device_type` (<https://developers.klaviyo.com/en/reference/create_form>; the overview writes a singular `teaser`).
  `next_step` carries `properties.list_id`: the list is attached to the submitting button.
- **Steps.** "(+) Step" with a type: "Mobile opt-in" (channel "Text messaging", "WhatsApp" or both), "Personal
  information", "Blank step" (<https://help.klaviyo.com/hc/en-us/articles/4404213604251>). "The maximum number of pages
  in a form is 4, plus an optional success message"; steps are linear, with no answer-based branching; the only jump is
  "Skip to success step" (<https://help.klaviyo.com/hc/en-us/articles/4404256496283>). Button actions in the UI: "Close
  form", "Submit form" (then "After Submit": "Show next step"), "Go to URL", "Subscribe via text messaging".
- **Teaser.** "(+) Teaser", its own text and style; Rectangle, Circle, Corner; "Show after form is closed", "Show before
  displaying form", or both; popup, flyout and full page only, not embeds or banners
  (<https://help.klaviyo.com/hc/en-us/articles/4411540984859>).
- **Desktop and mobile.** One form; "By default, equivalent blocks in desktop and mobile are linked"; "Unlink" a block
  to change its content, position, font size and spacing per device; "Form-wide settings, such as form type and input
  field styles, still apply to both devices"; email and phone inputs, disclosure text, coupon, spin-to-win and opt-in
  code cannot be unlinked (<https://help.klaviyo.com/hc/en-us/articles/44513639011099>). Each block has a "Visibility"
  of Desktop, Mobile or All Devices (`properties.display_device` `both | desktop | mobile` in the schema).
- **Brand.** The Brand library (logos, colours, fonts, buttons): "Changing these settings will not update any existing
  blocks, templates, or flows"; live forms are edited by hand
  (<https://help.klaviyo.com/hc/en-us/articles/4403537778331>). Form "Styles" apply to all steps and the success
  message; blocks override them (<https://help.klaviyo.com/hc/en-us/articles/4413537049883>).
- **AI.** Composer "generates ready-to-publish sign-up forms" from a scan of the storefront (marketing page
  <https://www.klaviyo.com/composer/what-is-an-ai-form-builder>; no help-center article found).

### Drip Onsite (Sleeknote)

- **Blocks.** Elements dragged from "Add Elements": Phone Input Field ("Phone Required"), Date Input Field, SMS element
  ("a phone input field and a compliance consent text", which "can't be added to a form step where you're collecting
  email addresses"), Countdown Timer, Spin to Win, video, image, Close Button, Custom Code, product recommendations,
  Coupon Code (static, or "Individual Codes" from a CSV), Answer Buttons
  (<https://help.drip.com/hc/en-us/articles/6416959176589-Phone-Input-Field>,
  <https://help.drip.com/hc/en-us/articles/6453286843789-SMS-Element>,
  <https://help.sleeknote.com/hc/en-us/articles/14148163257885-Coupon-Code>,
  <https://help.sleeknote.com/hc/en-us/articles/38453253505565-Answer-buttons>). No row or column model is documented.
- **Steps.** Every campaign has form step(s), a "Teaser" and a "Success" step
  (<https://help.drip.com/hc/en-us/articles/6361135870989-Teaser>). "Add Step" for more (not for sidebars and
  sleekbars); data is kept per step: "Even if visitors leave the form without filling the second step, their email
  address will still be added" (<https://help.drip.com/hc/en-us/articles/6453447155085-Onsite-Pop-ups-Multistep-Forms>).
  Answer-based routing only in quizzes: "Question steps", form steps and "Outcome steps", with "Manage outcomes" mapping
  answers to an outcome and a "default outcome"
  (<https://help.drip.com/hc/en-us/articles/10475782044813-Onsite-Pop-ups-Quizzes>).
- **Success step.** "The last step of your Onsite campaigns", on or off "with one click"; holds a double opt-in
  reminder, a coupon, or a redirect (<https://help.drip.com/hc/en-us/articles/6033820210573-Success-Step>).
- **Teaser.** A step of the same campaign, shown before and after; campaign triggers do not apply to it; every type but
  the sticky bar has one (6361135870989). On mobile, by default the form opens **only** from the teaser (Google's
  interstitial guidance); the mobile teaser has its own copy, style, position and delay
  (<https://help.drip.com/hc/en-us/articles/5691523511437-Mobile-Teaser>).
- **Desktop and mobile.** One design, a desktop and a mobile tab: desktop style edits flow to mobile, a style edit on
  mobile breaks the link for that element, "Use desktop style" relinks; adding, deleting or configuring an element
  applies to both; "Hide on mobile" removes it on one device; the email field and submit button cannot be hidden
  (<https://help.drip.com/hc/en-us/articles/19591929889805>).
- **Brand.** Sleeknote "Brand Assets" detects a domain's colours for a quick chooser in the builder: a palette, not an
  inherited style (<https://help.sleeknote.com/hc/en-us/articles/26961079889821-Brand-assets>).
- **Document.** No design API or export found. Drip's `GET /v2/:account_id/forms` covers only the legacy form product
  (<https://developer.drip.com/>). The Sleeknote MCP is read-only (analytics, campaign overview)
  (<https://help.sleeknote.com/hc/en-us/articles/39735134034461>).

### OptinMonster

- **Blocks.** Columns, Optin Fields, Text, Button, Image, Embed Form, Video, Spacer, Divider, Icons, HTML, WPForms,
  Yes/No, Countdown Timer
  (<https://optinmonster.com/docs/a-complete-guide-to-choosing-the-right-block-for-your-campaign/>); "A row holds a
  column block" (<https://optinmonster.com/docs/glossary-of-terms/>): view → row → column → blocks. No SMS consent
  field is documented.
- **Views.** "A view is the screen on the campaign the user is seeing"; defaults Yes/No, Optin, Success (glossary);
  custom named views with "+ Add View" (<https://optinmonster.com/docs/how-to-create-a-yes-no-optin-with-optinmonster/>).
  Navigation by "Button Click Action": "Go to a view", "Redirect to a url", "Open email client", "Click to call",
  "Click to copy text", "Close the campaign", "Reload the page", "Open in a new Window"
  (<https://optinmonster.com/docs/how-to-use-actions-for-success-view-and-more/>).
- **Teaser.** A view: "Enable Teaser view"; "Show before the campaign opens" and/or "Show when visitor closes the
  campaign"; "Remember minimized state across pageviews"; per-device "Show On"
  (<https://optinmonster.com/docs/how-to-use-campaign-teasers-in-optinmonster/>).
- **Desktop and mobile.** One design, Desktop → Tablet → Mobile cascade; a style change or a hidden block on a smaller
  device breaks the link (vendor blog, 2026-04-20, <https://optinmonster.com/mobile-popup-design/>, not /docs).
- **Brand.** No theme; "Color Profiles" are saved swatch sets
  (<https://optinmonster.com/docs/how-to-create-and-use-color-profiles-in-optinmonster/>).
- **Document and AI.** No public design API documented. An MCP announced 2026-10-27 builds a campaign from a template,
  revises copy and styling and sets targeting; new campaigns are created paused; no tool list or format is published
  (<https://optinmonster.com/optinmonster-mcp-announcement/>).

### Braze in-app messages (drag-and-drop editor, web)

- **Blocks.** "Title and paragraph", "Button", "Image", "Link", "Spacer", "Custom code"; form blocks "Phone capture"
  (subscribes to an SMS or WhatsApp subscription group), "Email capture", "Short text", "Dropdown", "Checkbox",
  "Checkbox group", "Radio button", "Long text", each bound to a profile attribute
  (<https://www.braze.com/docs/user_guide/message_building_by_channel/in-app_messages/drag_and_drop/editor_blocks/>).
  "Blocks must sit inside a row" with column settings
  (<https://www.braze.com/docs/user_guide/message_building_by_channel/in-app_messages/drag_and_drop/style_settings/>).
- **Pages.** Up to 10 per message, linked by a button's "On-click behavior" "Go to page"; "A page with no links
  prevents the message from launching". On-click: close, web URL, deep link, go to page, request push permission;
  toggles "Submit form when button is clicked", "Set separate behaviors for each platform"
  (<https://www.braze.com/docs/user_guide/message_building_by_channel/in-app_messages/drag_and_drop/create/>). No
  dedicated success step and no teaser found. Display types in the editor: modal and full screen ("slideups aren't
  currently supported in the editor").
- **Desktop and mobile.** One design, previewed on mobile, tablet and desktop; no per-device styles or visibility
  documented (style settings page).
- **Brand.** Message-level styles apply "everywhere in your message except where you override it"; reuse is by saved
  templates; no inherited brand theme found.
- **Document and AI.** No API to create or read a drag-and-drop design; the campaign export returns `channel`, `name`,
  `message`, `extras` (<https://www.braze.com/docs/api/endpoints/export/campaigns/get_campaign_details/>). Operator
  writes "Copy or a full message design", edits individual blocks and shows a diff for approval
  (<https://www.braze.com/docs/user_guide/brazeai/generative_ai/ai_copywriting/>).

### Common concepts

| Concept                | Klaviyo                                                  | Drip / Sleeknote                                        | OptinMonster                                  | Braze                         |
| ---------------------- | -------------------------------------------------------- | ------------------------------------------------------- | --------------------------------------------- | ----------------------------- |
| Screens                | steps, max 4 + success                                   | steps (+ quiz question and outcome steps)               | named views                                   | named pages, max 10           |
| Layout inside a screen | columns → rows → blocks                                  | element list                                            | rows → columns → blocks                       | rows → columns → blocks       |
| Moving between screens | button action (`next_step`, `skip_to_success`); linear   | button; answer-based only in quizzes                    | button "Go to a view"                         | button "Go to page"           |
| Submit                 | an action (+ "After Submit")                             | per step                                                | optin field action                            | a toggle on any action        |
| Success step           | optional final step                                      | built-in step, on/off                                   | default view                                  | none                          |
| Teaser                 | part of the form; before, after close, or both           | a step; before and after; the only entry on mobile      | a view; before and/or after                   | none                          |
| Desktop vs mobile      | one design, per-block link/unlink, visibility per device | one design, style overrides on mobile, "Hide on mobile" | one design, Desktop → Tablet → Mobile cascade | one design, preview only      |
| Brand                  | Brand library; does not update existing forms            | colour palette                                          | colour profiles                               | per-message styles, templates |
| Document exposed       | full read/write Forms API                                | none                                                    | none (MCP, no schema)                         | none                          |

For the planned elements: a phone field with SMS or WhatsApp consent exists at Klaviyo (Mobile opt-in step,
`sms_disclosure`) and Braze (one subscription group per form), and Drip needs a step of its own for it; a web push
permission button only at Braze ("request push permission"; Klaviyo does not offer web push, per the earlier
research); a Telegram start button at none of the four.

## 5. What a schema an agent writes could look like

ADR 0040 needed three things for email: a schema the agent writes against (derived from the editor, with attribute types
and allowed values by hand), a server check of a document against it, and a `422` with the reason for an invalid one.
What each candidate offers towards that:

| Candidate          | Document shape                                                                                 | Schema available                                                                                                                                                               | Server check without a JS runtime                                             |
| ------------------ | ---------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------- |
| Puck               | tree of `{type, props}`; types and props closed by our config; richtext props are HTML strings | none exported; the config's `fields` are declarative (types, options, `array`/`object` subfields, `slot` with `allow`/`disallow`) and could be translated to JSON Schema by us | yes for structure (plain JSON); a richtext HTML string still needs sanitising |
| SurveyJS           | `pages[].elements[]`, conditions as expression strings                                         | `Serializer.generateSchema()`                                                                                                                                                  | structure yes; expressions are SurveyJS's own language                        |
| Form.io            | `components[]` tree                                                                            | Form.io documents the form JSON                                                                                                                                                | structure yes                                                                 |
| Craft.js           | flat map of id → node (resolved name, props, child ids, parent)                                | none                                                                                                                                                                           | yes, but an id-keyed graph is harder for an agent to write than a tree        |
| GrapesJS           | components (HTML-shaped, arbitrary attributes) + CSS rules                                     | none                                                                                                                                                                           | only as HTML/CSS sanitising                                                   |
| coltorapps Builder | entities with attributes                                                                       | zod schemas per attribute, validated by the library in JS                                                                                                                      | needs JS (or the zod schemas re-expressed)                                    |

Fact, not a decision: a plain JSON document under a closed schema can be validated in Go with no JS runtime, which
ADR 0040's email path could not do (it needed goja for the render and `nodeFromJSON`/`check` for validation).

An illustration of the shape the vendor concepts in section 4 imply, so the prototype has something concrete to argue
with (**not a proposal**; field names are placeholders):

```json
{
  "format": "modal",
  "steps": [
    {
      "id": "signup",
      "blocks": [
        { "type": "heading", "text": "Get 10% off" },
        { "type": "email_field", "required": true },
        { "type": "consent_checkbox", "channel": "email", "label": "…" },
        { "type": "button", "label": "Subscribe", "action": { "type": "submit", "next": "thanks" } }
      ]
    },
    {
      "id": "thanks",
      "kind": "success",
      "blocks": [{ "type": "text", "text": "Check your inbox" }]
    }
  ],
  "teaser": { "text": "Get 10% off", "show": "before_and_after_close" },
  "devices": { "mobile": { "hidden": [], "style_overrides": {} } },
  "style": { "brand": "values copied from the Brand kit when authored (ADR 0040 decision 6)" }
}
```

Klaviyo's Forms API is the one published precedent for such a document written by a program: a whole-definition
replace on a draft, publish as a separate job, block and action types as closed enums.

## 6. Unverified

- No candidate was installed in the app: Puck's and GrapesJS's fit with Mantine (tokens through `cssVariablesResolver`,
  field overrides with Mantine inputs), dark mode, and React 19 behaviour are from docs and peers only.
- Bundle sizes are of single published files; Puck's and Craft.js's render paths and a minified React 19 were not
  measured. `@puckeditor/core/rsc` is a re-export entry; its size says nothing about the renderer.
- `@grapesjs/react` 2.0.0 against core 0.23.x: a declared peer mismatch, not a tested failure.
- Easyblocks' document format and the licence conflict (npm ISC vs repo AGPL-3.0) were not resolved.
- SurveyJS Creator's EULA terms on keys or banners: the licensing page did not state them. SurveyJS and Form.io device
  editing and theming against Mantine were not read.
- **Builder.io** (`@builder.io/react` 9.4.10, MIT SDK, 2026-10-20), **Plasmic** (`@plasmicapp/loader-react` 2.0.25,
  MIT, 2026-10-26) and **Webstudio** (AGPL-3.0, 0.309.0, 2026-10-30): only npm and GitHub metadata were read. The pages
  fetched did not state whether their editors can be embedded without their hosted service; not evaluated.
- Puck AI's status (open vs closed beta) and pricing: Puck's own pages disagree; a third-party listing was not used.
- Unlayer's and Beefree's popup builders: read from their docs, not tried.
- Vendors: how Klaviyo's schema marks the success step, and whether an unlinked block becomes two device-scoped blocks;
  Drip's full list of button actions and any row/column model; OptinMonster's device cascade (from a vendor blog post,
  not its docs) and whether editing a Color Profile updates campaigns; Klaviyo Composer (marketing page only). The
  vendor section was gathered by a delegated read of the pages cited; the Klaviyo Forms API overview was re-read to
  confirm its structure.

## 7. Sources (accessed 2026-11-01)

- Puck: <https://github.com/puckeditor/puck> (`LICENSE`, `CHANGELOG.md`, `README.md`, `apps/docs/pages/docs/**`,
  `packages/core/bundle/rsc.tsx`, `packages/core/components/RichTextEditor/**`), npm `@puckeditor/core`,
  `@measured/puck`, <https://puckeditor.com/docs/ai/getting-started>, <https://puckeditor.com/docs/ai/headless-generation>
- GrapesJS: <https://github.com/GrapesJS/grapesjs> (`packages/core/src/editor/config/config.ts`,
  `packages/core/src/device_manager/config/config.ts`, `docs/api/device.md`),
  <https://grapesjs.com/docs/modules/Storage.html>, <https://github.com/GrapesJS/react> (README),
  <https://github.com/GrapesJS/studio/blob/main/LICENSE.md>, npm `grapesjs`, `@grapesjs/react`, `@grapesjs/studio-sdk`,
  `grapesjs-plugin-forms`
- Craft.js: <https://github.com/prevwong/craft.js> (`LICENSE`, `site/docs/**`, `packages/core/src/interfaces/nodes.ts`),
  npm `@craftjs/core`
- Easyblocks: <https://github.com/easyblockshq/easyblocks> (`LICENSE.md`, `packages/core/package.json`), npm
  `@easyblocks/core`, `@easyblocks/editor`
- React Email editor: npm `@react-email/editor`, installed `node_modules/@react-email/editor/dist`,
  <https://github.com/resend/react-email>
- SurveyJS: <https://surveyjs.io/licensing>,
  <https://surveyjs.io/form-library/documentation/design-survey/create-a-multi-page-survey>, unpkg
  `survey-core@3.2.3` (`typings/src/jsonobject.d.ts`, `package.json`), `survey-js-ui@3.2.3`
- Form.io: <https://github.com/formio/formio.js> (README), npm `@formio/js`, `@formio/react`
- coltorapps Builder: <https://github.com/coltorapps/builder> (README), npm `@coltorapps/builder-react`
- react-jsonschema-form: <https://github.com/rjsf-team/react-jsonschema-form>, npm `@rjsf/core`
- Unlayer: <https://docs.unlayer.com/builder/popup-builder>; Beefree:
  <https://docs.beefree.io/beefree-sdk/visual-builders/popup-builder/popup-builder-getting-started>
- Vendors: the help-center, developer and docs pages linked in section 4 (Klaviyo, Drip, Sleeknote, OptinMonster,
  Braze)
- Runtimes measured: unpkg `react@19.2.0`, `react-dom@19.2.0`, `preact@10.27.2`, `survey-core@3.2.3`,
  `survey-js-ui@3.2.3`, `@formio/js@5.7.0`
- In-repo: `package.json`, `packages/analytics/package.json`, `src/main.tsx`, `src/components/VisualEditor.tsx`,
  `docs/adr/0040-email-content-is-html-authored-in-an-embedded-editor.md`, `docs/research/embeddable-email-editors.md`,
  `docs/research/drip-com-feature-analysis.md` (section 6)
