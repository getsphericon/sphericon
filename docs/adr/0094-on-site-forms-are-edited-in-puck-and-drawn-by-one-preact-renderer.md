---
status: accepted
---

# On-site forms are edited in Puck and drawn by one Preact renderer in a shadow root

An On-site form (ADR 0085) needs an editor in the app and a renderer on the customer's site. The
research (`research/on-site-builders`) found two live embeddable builders, Puck (MIT, a closed JSON
tree drawn by our own components) and GrapesJS (BSD, free-form HTML and CSS), and that no React
renderer fits beside the tracker (React alone is ~99 kB gzip). The runtime research
(`research/on-site-runtime`) left the isolation open. A throwaway prototype
(`prototype/onsite-form-builder`, `packages/storefront/src/prototype-onsite/`) compared three
editors on the demo storefront: Puck's canvas, a fixed layout filled by a form, and a block list
with an inspector. The maintainer chose Puck for how it lays out the space: a block palette and an
outline on the left, the canvas in the middle, the selected block's fields on the right. Decided in
the prototype ticket (getsphericon/sphericon#958).

## Decisions

- **The editor is Puck.** Its layout, drag and drop between zones, outline, viewports, zoom, undo
  and field types come ready-made. What it lacks for forms (properties per device, page-only
  controls such as its own publish button) we hide through its overrides or add upstream by pull
  request. Its 0.x churn and its own stylesheet (`--puck-*`, mapped onto Mantine's variables where
  it can be) are accepted.
- **The stored content is Puck's `Data`** with our components only, so there is no second document
  to translate. The steps, the success step and the teaser are slot fields of the root, and the
  theme (copied into the form at authoring, ADR 0003) is a root prop. A block's phone override is
  its `mobile` prop: hidden, and the properties unlinked from desktop. The Format stays on the form
  (ADR 0085). A JSON Schema written from our component config checks the content at Publish and is
  what an agent gets over MCP (as ADR 0040 did for email). Puck's `migrate()` is JavaScript, so the
  editor brings a document to the current format when it opens it, and the renderer reads the
  format the live version was saved in.
- **One renderer, Preact.** Each Form block is written once as a Preact component. Puck's canvas
  draws every leaf block through it (Columns stays Puck's own React drop zone), and the site gets
  the same code: about 12 kB gzip with Preact and a QR library (`uqr`), loaded from the Form host
  only when a form may show (ADR 0090). The prototype measured 3 kB for hand-written DOM; Preact
  costs 9 kB more and buys components shared with the editor.
- **Isolation: an open shadow root in the page's own document.** `:host { all: initial }`, styles
  through `adoptedStyleSheets` (a sheet built by the document that hosts the form, since Puck's
  canvas is an iframe), the theme as custom properties set through CSSOM, sizes in pixels, and a
  popup as `<dialog>` opened with `showModal()`. In Chrome a host page with a 62.5 % root font
  size, a global font and `!important` rules on `button` and `input` changed nothing inside; under
  a `style-src 'self'` policy the form kept its styles; the open modal made the page inert.
  An iframe was rejected: a cross-origin frame cannot ask for web push permission, and a modal
  inside a frame leaves the page reachable.
- **ALTCHA's widget is not bundled** (about 58 kB gzip, its solver library about 23 kB). The form
  uses ALTCHA's protocol and its Go library on the server (ADR 0088) and loads the solver only when
  the visitor submits.

## Considered options

- **Our own editor from ready-made parts** (RJSF inspector from one JSON Schema per block, dnd-kit
  reordering, ahooks undo): all Mantine, per-device properties built in, but the canvas, drag and
  drop between zones and the outline would be ours to write.
- **A fixed layout filled by a form:** the least to build, but a document made elsewhere opens
  lossily and authors cannot compose.
- **Our own stored schema with Puck as a view:** a translation layer to keep in step with every
  Puck release, for a document that is already closed over our components.
- **A plain-DOM renderer** (3 kB): smaller, but a second implementation of every block beside the
  editor's.
