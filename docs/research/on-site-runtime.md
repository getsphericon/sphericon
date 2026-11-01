# Browser runtime for On-site messages: isolation, size, CSP, accessibility, blockers

Researched 2026-11-01 for [#949](https://github.com/getsphericon/sphericon/issues/949) (map
[#948](https://github.com/getsphericon/sphericon/issues/948)). It extends
[on-site-messages-and-forms.md](https://github.com/getsphericon/sphericon/blob/research/on-site-messages-and-forms/docs/research/on-site-messages-and-forms.md)
(branch `research/on-site-messages-and-forms`, "prior research" below), whose section 7 already covers Google's
interstitial guidance, the web push permission gesture, the CLS definition, the WAI-ARIA modal pattern and WCAG 2.2
SC 2.4.11; none of that is repeated here.

Sources: WHATWG HTML, W3C CSP3 and CSS specs, MDN, web.dev, WebKit's tracking-prevention policy, the uBlock Origin
and Brave projects, the filter lists themselves, the vendors' docs, and **the vendors' served bundles**, measured and
read on 2026-11-01. Sizes were measured with `curl` and `size_download` once per `Accept-Encoding` (`br`, `gzip`,
`identity`), so they are bytes on the wire; kB means 1000 bytes. Reading minified bundles shows what code exists and
when it is fetched, not why; such findings are marked "(read from the bundle)". Limits: **help.sleeknote.com and
help.drip.com refused automated reads (HTTP 403)**, so their lines are from a search engine's index of the linked pages
and marked "(search index)".

## 0. The facts a decision waits on

- **Three isolation models exist, and the vendors use all three.** Klaviyo draws forms in the page's own DOM (the
  site's CSS can restyle them); Braze draws its standard in-app messages in the DOM and HTML messages in an iframe;
  Customer.io draws every in-app message in an iframe served from its own renderer host. None of the bundles read
  calls `attachShadow`.
- **Shadow DOM stops selectors both ways but not inheritance**: the shadow tree inherits `font`, `color`,
  `line-height` from its host, `rem` follows the page's root font size, and `@font-face` inside a shadow root
  behaves inconsistently across engines, none scoping it as specified. An iframe isolates fully but needs a postMessage channel for its height and close,
  makes only its own document inert, and, served cross-origin, cannot ask for web push permission.
- **Size.** sphericon's `t.js` is 16.3 kB gzip today, the size of OptinMonster's loader. Klaviyo loads about 95 kB
  gzip of onsite modules at page load for an account with forms and fetches up to 171 kB gzip more, its renderer, only
  when a form's trigger passes (part of that set may load earlier, §8). Customer.io loads its 37 kB gzip in-app code at init whenever the plugin is enabled. Braze's full
  CDN build is 81 kB gzip, and the build without UI is 61 kB. **The tracker's Vite IIFE build inlines a dynamic
  `import()` into the one file** (tested), so lazy display code needs a second build output and a new gate path.
- **CSP.** The current snippet is an inline script, so a strict CSP already needs a nonce or hash for it. Styles via
  CSSOM (`adoptedStyleSheets`, `insertRule`) are blocked by no browser, so they need no `style-src` change; `<style>`
  elements and `style` attributes do. An iframe needs `frame-src`. Trusted Types (Baseline 2026) makes
  `innerHTML = string` throw.
- **Blockers.** The default uBlock Origin lists block OptinMonster's script host (EasyList), Drip, Customer.io and
  Braze's CDN (EasyPrivacy) by hostname; opt-in annoyance lists also hide popups by **class name**, whichever host
  served the script. Firefox uBO and Brave resolve CNAMEs and apply filters to the canonical name, so the Edge
  target's hostname is a single name whose listing would block every customer at once. Safari deletes script-written
  cookies and storage after 7 days without interaction, which bounds any browser-side "don't show again" state. No
  primary source measures how often a CNAMEd first-party script is blocked.

## 1. The runtime today

- `packages/analytics` builds one IIFE, `t.js`, with Vite library mode (`vite.config.ts`, `formats: ['iife']`); the
  Go binary embeds it and serves it at `/t.js` with `Cache-Control: public, max-age=3600`
  (`internal/server/tracker.go`). Dependencies: `es-toolkit`, `js-cookie`, `ky`, `uuid` and the generated collect
  client.
- **Measured** (source at `0b02d977e`, unchanged at `be577a229`, Vite 8.3.5, `pnpm exec vite build`): **46.9 kB raw, 16.3 kB gzip -9,
  14.7 kB brotli -q 11**.
- **Code splitting.** A test build with the same Vite version, library mode and `formats: ['iife']`, whose entry calls
  `import('./display.js')`, emitted a single `t.js` with the dynamic module inlined and no second file. Rollup
  documents the same for its option: `inlineDynamicImports` "will inline dynamic imports instead of creating new
  chunks", "only possible if a single input is provided"
  ([Rollup options](https://rollupjs.org/configuration-options/#output-inlinedynamicimports)). Display code loaded
  only when needed therefore means a second entry point (another IIFE or an ES module build) fetched by the tracker.
- **Gate paths.** On a Tracking domain only `/t.js` and `/collect/*` answer; every other path is a 404
  (`internal/server/trackinghost.go`, [ADR 0059](../adr/0059-a-tracking-domain-also-serves-the-first-party-tracker.md)).
  A second script, a message-config read, an iframe document or hosted images would each be a new path on that
  gate, a new edge route, and a path customer sites embed
  ([ADR 0036](../adr/0036-paths-embedded-in-sent-mail-and-customer-sites-outlive-a-major.md)).
- **Snippet.** The Tracking settings page hands out an inline `<script>` stub that creates `window._omq` and inserts
  `t.js` with `document.createElement('script')` (`src/pages/workspace/settings/tracking.tsx`). `iife.ts` also
  accepts `<script async src=".../t.js" data-collect-key=...>`, a form that needs no inline script.
- **State in the browser.** `om_vid` is a cookie written by script (`js-cookie`, 365 days, `SameSite=Lax`), the event
  queue is `localStorage` `om_q`. No route-change hook exists: the IIFE calls `initTracking` once, and
  `trackPageView` is only called by the site.

## 2. Isolation: plain DOM, shadow DOM, iframe

### 2.1 What the vendors do

| Vendor                              | How a message is drawn                                                                                                                                                                                                                                              | Source                                                                                               |
| ----------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| Klaviyo sign-up forms               | Page DOM. "your site's CSS may be overriding its styling"; the fix offered is removing `!important` from the site's CSS. The renderer chunk has `role`, `aria-modal`, `"dialog"`, focus and tab handling, and no `attachShadow` or `srcdoc` (read from the bundle). | [troubleshooting](https://help.klaviyo.com/hc/en-us/articles/115005249348)                           |
| Braze in-app (slideup, modal, full) | Page DOM, with the SDK's own CSS; Font Awesome 4.7 loaded by default (`doNotLoadFontAwesome` turns it off).                                                                                                                                                         | [SDK guide](https://braze.com/docs/developer_guide/sdk_repository_guides/web/)                       |
| Braze HTML in-app, Banners          | Iframe; HTML messages talk to the SDK through `brazeBridge` (prior research §2). The 7.1 bundle creates iframes and uses `srcdoc` (read from the bundle).                                                                                                           | prior research                                                                                       |
| Customer.io in-app                  | Iframe: the CSP page asks for `frame-src renderer.gist.build code.gist.build` "for message queues, real-time updates, and rendering".                                                                                                                               | [CSP](https://docs.customer.io/integrations/data-in/connections/javascript/content-security-policy/) |
| OptinMonster                        | Not established. The loader (`api.min.js`) has no `attachShadow`, `iframe` or `srcdoc`; campaign code comes later from `{mDomain}/c/` and was not read.                                                                                                             | (read from the bundle)                                                                               |
| Sleeknote / Drip Onsite             | Not established.                                                                                                                                                                                                                                                    | —                                                                                                    |

### 2.2 Style leakage

- **Shadow DOM, page into message.** Page selectors do not match nodes in a shadow tree, and shadow styles do not
  leak out ([MDN, Using shadow DOM](https://developer.mozilla.org/en-US/docs/Web/API/Web_components/Using_shadow_DOM)).
  But "the top-level elements of a shadow tree inherit from their host element"
  ([CSS Scoping §3.3.2](https://www.w3.org/TR/css-scoping-1/)), so every inherited property the page sets on `body`
  or on the host's ancestors (`font-family`, `font-size`, `color`, `line-height`, `letter-spacing`, `direction`)
  arrives inside unless the shadow root resets it, for example with `all: initial` on `:host`. `rem` is "font size of
  the root element" ([CSS Values 4](https://drafts.csswg.org/css-values-4/#font-relative-lengths)), the page's
  `<html>`, so a site that sets `html { font-size: 62.5% }` shrinks every `rem` in the message. The host element
  itself is in the page's tree and is matched by the page's selectors.
- **Shadow DOM, closed mode** is not a security boundary: "there are ways it can be evaded, for example by browser
  extensions running in the page" (MDN, same page).
- **Plain DOM** needs a reset of every property on every element the message draws and a unique class prefix, and
  still loses to a site's `!important` rule or an ID selector (Klaviyo's article above is the vendor's own account of
  it; Klaviyo's bundle uses a `kl-private-reset-css-*` class, read from the bundle).
- **Iframe** is a separate document: no page CSS reaches it and nothing inherits.

### 2.3 Fonts

- `@font-face` inside a shadow tree: CSS Scoping specifies tree-scoped names, but "browsers currently have a
  confusing panoply of behaviors, likely mostly accidental"; the CSSWG's interop table (undated) records "Doesn't
  work" and "@-rules in shadow trees are ignored. @-rules in document scope match from all scopes"
  ([CSSWG scoped names interop](https://wiki.csswg.org/spec/css-scoping)), and WebKit bug 270664 (2024) says WebKit
  does not tree-scope `@font-face` names
  ([bug](https://bugs.webkit.org/show_bug.cgi?id=270664), seen in search results only). A custom typeface that has to
  work in every engine is declared at document level, which touches the page.
- The page's own web fonts are usable from the shadow tree by family name (same table: document-scope rules match
  from all scopes), so "use the site's font" works there with no download.
- An iframe loads its fonts itself; the site's fonts are reachable only if the frame requests them by URL and their
  host allows it.

### 2.4 Focus, modality and top layer

- `showModal()` puts a `<dialog>` in the top layer, makes the rest of the page inert, closes on Esc, moves focus to the
  first focusable element (or the `autofocus` one) and gives the dialog an implicit `aria-modal="true"`; the
  `closedby` attribute picks which user actions close it
  ([MDN `<dialog>`](https://developer.mozilla.org/en-US/docs/Web/HTML/Reference/Elements/dialog)). The top layer
  escapes any page `z-index` or `overflow` context.
- **Inertness is per document**: "A Document document is blocked by a modal dialog subject if subject is the topmost
  dialog element in document's top layer. While document is so blocked, every node that is connected to document,
  with the exception of the subject element and its flat tree descendants, must become inert"
  ([HTML §6.3.1](https://html.spec.whatwg.org/multipage/interaction.html#modal-dialogs-and-inert-subtrees)). A dialog
  inside a shadow root is still in its document's top layer, so the whole page goes inert. A dialog inside an iframe
  makes only the iframe's document inert: the parent page stays focusable and visible to assistive technology unless
  the parent's own script marks it `inert`, and Esc pressed in the frame does not reach the parent without
  `postMessage`.
- **Non-modal formats** (slide-in, bar, teaser) map to the Popover API: `showPopover()` adds the element to the top
  layer, the rest of the page stays usable; Baseline 2025, newly available
  ([MDN Popover API](https://developer.mozilla.org/en-US/docs/Web/API/Popover_API)).
- **ARIA ID references do not cross shadow boundaries.** A whole dialog inside one shadow root can use
  `aria-labelledby` within it; references between the page and the shadow tree need Reference Target, which Chromium
  filed an Intent to Ship for in 2026, with no signal from Gecko or WebKit at the time
  ([explainer](https://github.com/WICG/webcomponents/blob/gh-pages/proposals/reference-target-explainer.md),
  [Intent to Ship](https://groups.google.com/a/chromium.org/d/msgid/blink-dev/4083781f-5237-46a3-993e-349ab4071a55n%40chromium.org)).

### 2.5 How a form submits in each

- **Plain or shadow DOM.** A `<form>` and its fields inside one shadow tree work as a normal form; the tracker can take
  `submit`, `preventDefault()` and post with `fetch` (`connect-src`), or let a native POST navigate (`form-action`,
  which has no `default-src` fallback,
  [MDN form-action](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Content-Security-Policy/form-action)).
  "All UA-dispatched UI events are composed (click/touch/mouseover/copy/paste, etc.). Most other types of events are
  not composed" ([MDN Event.composed](https://developer.mozilla.org/en-US/docs/Web/API/Event/composed)); `submit` is
  not a UI event, so the site's own document-level form listeners (tag managers) do not see a submit inside a shadow
  root, while clicks do reach them, retargeted to the host.
- **Iframe.** The submit runs in the frame's origin under the frame's own CSP. The frame's document reads no cookie or
  storage of the page: the `om_vid` Visitor id has to be passed in (URL or `postMessage`). A cross-origin frame
  cannot call `Notification.requestPermission()` (prior research §7), so **the web push soft prompt's button cannot
  live in a frame served from the Tracking host**; it has to be in the page's own document.
- **Size of an iframe** follows its content only when the frame reports its height to the parent (`postMessage`);
  nothing in the sources read gives a shipped CSS way to size an iframe to its content.

## 3. Size and lazy loading

Bytes on the wire on 2026-11-01 (kB = 1000 bytes; "—" means the server did not offer that encoding).

| Script                                                                | raw          | gzip       | br    | When it loads                                                                                |
| --------------------------------------------------------------------- | ------------ | ---------- | ----- | -------------------------------------------------------------------------------------------- |
| sphericon `t.js` (built locally, §1)                                  | 46.9         | 16.3       | 14.7  | Every page.                                                                                  |
| OptinMonster `a.omappapi.com/app/js/api.min.js`                       | 46.8         | 16.5       | 16.7  | Every page; then `api.min.css` and per-campaign code from `{mDomain}/c/`, not measured.      |
| Klaviyo loader `klaviyo.js?company_id=XB4Thx`                         | 13.1         | 2.4        | 2.2   | Every page; templated per account, `Cache-Control: max-age=1, stale-while-revalidate=10800`. |
| Klaviyo modules listed by that loader, shared chunks counted once     | 267.1        | 95.3       | 91.1  | Every page.                                                                                  |
| Klaviyo `Render` set (12 chunks, minus those already loaded)          | 647.7        | 171.2      | 157.3 | Only when a form is to be drawn (read from the bundle, below).                               |
| Braze `web-sdk/7.1/braze.min.js` (full, with UI)                      | 271.5        | 81.4       | —     | Every page (CDN build).                                                                      |
| Braze `web-sdk/7.1/braze.core.min.js` (no UI)                         | 183.3        | 60.8       | —     | Every page; the site draws its own messages.                                                 |
| Customer.io `@customerio/cdp-analytics-browser@0.8.1` `standalone.js` | 113.0        | 31.1       | —     | Every page.                                                                                  |
| Customer.io `inAppPlugin.js` + chunk `573.js` (same package)          | 10.7 + 120.1 | 3.1 + 33.8 | —     | At init when the In-App plugin is enabled.                                                   |

Notes:

- **Klaviyo** (read from the bundle). The loader carries the list of modules enabled for the account: a placeholder
  id gets `static`, `event_adapter`, `fender_analytics`; of seven real ids found in public code, six also got
  `signup_forms` and one did not. The page-load row above is for one of the six (`XB4Thx`). Inside `signup_forms`, each form's triggers (delay, scroll, exit intent
  and so on) carry a callback with `onRenderForms`, a function that `import()`s the `Render` set (`Render`,
  `vendors~Render`, `styles` …). So Klaviyo loads targeting and trigger code on every page and **the drawing code only
  when a form is due**. The `Render` chunk alone is 310 kB raw, 68 kB br.
- **Customer.io** (read from the bundle). `standalone.js` loads chunks `573` and `inAppPlugin` with `import()` when
  the `"Customer.io In-App Plugin"` integration is enabled, before any message is known; the message itself is then
  drawn by `renderer.gist.build` in an iframe (§2.1). Chunk `573.js` is 120,083 bytes and
  `customerio-gist-web@3.28.0`'s `gist.min.js` is 120,006: probably the same library, not confirmed. Webpack sets the
  page's nonce on the chunk scripts it inserts (`u.nc`).
- **Braze**: "When using the npm version, JavaScript bundlers remove unused code, including UI code"; the core build
  "Contains the SDK without UI" ([README](https://github.com/braze-inc/braze-web-sdk)). The SDK docs recommend npm
  and say the CDN method "can prevent in-app message types like Banners and Content Cards from displaying"
  ([integration](https://www.braze.com/docs/developer_guide/sdk_integration?sdktab=web)). No lazy load of the UI is
  documented.
- **Sleeknote and Drip Onsite** serve a per-account script; not measured.

## 4. CSP: what a customer must allow

### 4.1 Platform facts

- `style-src` blocks `<style>` elements, `<link>` stylesheets from other hosts, `style` attributes and
  `setAttribute('style')` / `style.cssText`, but not `element.style.prop = …`; and although CSP3 gates CSSOM parsing
  (`insertRule`, `cssText` setters) on `'unsafe-eval'`, "no browser currently blocks these methods"
  ([MDN style-src](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Content-Security-Policy/style-src);
  the spec text calls itself unfinished, [CSP3 §6.1.13](https://w3c.github.io/webappsec-csp/#directive-style-src)).
  Constructable stylesheets (`new CSSStyleSheet()`, `replaceSync`, `adoptedStyleSheets`) are CSSOM; MDN's sentence
  names `insertRule` and `cssText`, not `replaceSync`, so whether any browser blocks `replaceSync` is to be checked in a
  prototype.
- A `<style>` element inside a shadow root is an inline style element; no source read exempts it from `style-src`.
- `'strict-dynamic'`: trust given to a nonced or hashed script "shall be propagated to all the scripts loaded by that
  root script", and host allowlists are then ignored
  ([MDN script-src](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Content-Security-Policy/script-src)).
  The stub's `createElement('script')` and any later chunk inserted by `t.js` load under it without listing the host.
- The `nonce` content attribute reads as an empty string after load; the `nonce` property is "the only way to access
  nonces" ([MDN nonce](https://developer.mozilla.org/en-US/docs/Web/HTML/Reference/Global_attributes/nonce)). The
  tracker can read `document.currentScript.nonce` and copy it to the elements it creates, as Braze does with
  `contentSecurityNonce` and webpack with `__webpack_nonce__`.
- `frame-src` "continues to defer to `child-src` if not present (which defers to `default-src` in turn)"; `srcdoc`,
  `blob:`, `data:` and `about:blank` documents inherit a copy of the creator's policy, so a `srcdoc` iframe is no way
  around the page's CSP ([CSP3 §1.3, §7.8](https://w3c.github.io/webappsec-csp/)).
- Trusted Types, Baseline 2026: with `require-trusted-types-for`, `element.innerHTML = string` throws a `TypeError`;
  a policy name must be listed in `trusted-types` to be created
  ([MDN](https://developer.mozilla.org/en-US/docs/Web/API/Trusted_Types_API)). Building nodes with `createElement`
  and `textContent` avoids the sink.

### 4.2 Directives per approach

What sphericon's own runtime would need, derived from 4.1 (the Tracking host is `track.example.com`):

| Need                               | Plain DOM                                                                                                   | Shadow DOM                                                                 | Iframe from the Tracking host                                                      |
| ---------------------------------- | ----------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------- | ---------------------------------------------------------------------------------- |
| Load `t.js` and display code       | `script-src track.example.com`, or a nonce with `'strict-dynamic'`; the inline stub needs its nonce or hash | same                                                                       | same                                                                               |
| Read message config, post a submit | `connect-src track.example.com`                                                                             | same                                                                       | `connect-src` is the frame's own CSP; the page needs `frame-src track.example.com` |
| Message styles                     | via CSSOM: nothing; via `<style>` or `style` attributes: nonce or `'unsafe-inline'` in `style-src`          | via `adoptedStyleSheets`: nothing; via `<style>` in the root: as plain DOM | nothing on the page                                                                |
| Images                             | `img-src` for their host (and `data:`/`blob:` if used)                                                      | same                                                                       | frame's own CSP                                                                    |
| Custom fonts                       | `font-src` for their host                                                                                   | same, declared at document level (§2.3)                                    | frame's own CSP                                                                    |
| Native form POST                   | `form-action` for the target                                                                                | same                                                                       | frame's own CSP                                                                    |

### 4.3 What the vendors publish

- **Customer.io**: `script-src cdp.customer.io` (not needed for the npm package); `connect-src cdp.customer.io
  *.api.gist.build *.cloud.gist.build`; for in-app `frame-src renderer.gist.build code.gist.build` and
  `style-src 'unsafe-inline'`; nonce on the snippet tag with `cdp.customer.io` still listed "because the snippet loads
  more scripts from it"; neither `'unsafe-inline'` nor `'unsafe-eval'` in `script-src`
  ([CSP](https://docs.customer.io/integrations/data-in/connections/javascript/content-security-policy/)).
- **Braze**: `connect-src` the SDK endpoint, `script-src https://js.appboycdn.com`, `'unsafe-eval'` for the CDN
  snippet only, nonce or `'unsafe-inline'` in `script-src` for custom HTML messages, `img-src appboy-images.com
  braze-images.com cdn.braze.com cdn.braze.eu`, `font-src`/`style-src https://use.fontawesome.com` plus nonce or
  `'unsafe-inline'` for Font Awesome; the nonce passed as `contentSecurityNonce` "to propagate it to newly created
  scripts and styles"; the GTM template cannot pass it
  ([CSP](https://www.braze.com/docs/developer_guide/platforms/web/content_security_policy/)).
- **Sleeknote** (search index): a wildcard `*.sleeknote.com` recommended; a strict list per `connect-src`,
  `font-src`, `img-src` (with `data:` and `blob:`, since images become blob URLs) and `frame-src` for integrations
  ([article](https://help.sleeknote.com/hc/en-us/articles/9708616757789-Whitelisting-Sleeknote-in-Your-Content-Security-Policy-CSP)).
  **Drip Onsite** (search index): the same wildcard first, a warning that Sleeknote may change subdomains, and
  `style-src data:`, Cloudflare and Google Fonts hosts
  ([article](https://help.drip.com/hc/en-us/articles/24083313027981-Safelisting-Onsite-in-Your-Content-Security-Policy-CSP)).
- **Klaviyo, OptinMonster**: no CSP page found (Klaviyo is also in prior research §10).

## 5. Layout shift and SPA route changes

- web.dev on banners and forms: "reserve sufficient space in the viewport for it in advance (for example, using a
  placeholder or skeleton UI) … Alternatively, ensure the element is not part of the document flow by overlaying the
  content where this makes sense"; animate with `transform`, since "Composited animations using translate can't
  impact other elements, and so don't count toward CLS"; even content shifts during scrolling count
  ([Optimize CLS](https://web.dev/articles/optimize-cls)). For On-site message Formats this means: a bar that pushes
  the page down shifts it unless shown after user input (500 ms window, prior research §7) or laid over the content;
  an embedded form shifts unless its slot's height is reserved by the site; modal, slide-in and teaser in the top
  layer do not shift.
- **Navigation API**, Baseline 2026 (newly available since January 2026): one `navigate` event "fired when any type of
  navigation is initiated", where the History API makes it "sometimes hard to detect and respond to all
  navigations"; the event does not fire on first load
  ([MDN](https://developer.mozilla.org/en-US/docs/Web/API/Navigation_API)). Older browsers leave the
  `pushState`/`popstate` route.
- Vendors: Customer.io page rules use the URL unless the site sends `page` calls (prior research §1); OptinMonster
  says it works on SPAs and has a manual reset event `om.Campaign.reset`, while its Page Targeting cannot match
  anything after `?` or `#` ([JS events](https://optinmonster.com/docs/optinmonster-javascript-events-api/),
  [page targeting](https://optinmonster.com/docs/how-to-show-or-hide-your-campaigns-on-specific-pages/)); Klaviyo
  added URL targeting for SPAs in July 2024 without the URL exclusion, per a Klaviyo community post
  ([post](https://community.klaviyo.com/marketing-30/spas-support-for-form-targeting-13752)).

## 6. Ad, tracker and annoyance blockers

### 6.1 What the lists block, 2026-11-01

uBlock Origin's defaults (from its `assets.json`): uBlock filters (Ads, Privacy, Badware, Quick fixes, Unbreak),
EasyList, EasyPrivacy, Online Malicious URL Blocklist, Peter Lowe's list. Fanboy's Annoyance and Newsletter lists are
off by default ([assets.json](https://github.com/gorhill/uBlock/blob/master/assets/assets.json)).

| List (default in uBO?)               | Rules found for on-site vendors                                                                                                                                                                                                                                                        |
| ------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| EasyList (yes)                       | `\|\|a.omappapi.com^` (OptinMonster's script host), `\|\|optinmonster.com^$third-party`                                                                                                                                                                                                |
| EasyPrivacy (yes)                    | `\|\|getdrip.com^$third-party`, `\|\|customer.io^$third-party`, `\|\|appboycdn.com^$third-party` (Braze CDN), `\|\|braze.eu/api/v3/data/`, `\|\|analytics.sleeknote.com^`, Klaviyo analytics paths only (`\|\|klaviyo.com/onsite/track-analytics?`), and `/analytics-js/*` on any host |
| Fanboy's Annoyance / Newsletter (no) | `\|\|sleeknote.com^$third-party`, `\|\|braze.com^$third-party`, `\|\|getdrip.com^$third-party`, cosmetic `##.klaviyo_flyout`, 35+ Mailchimp form selectors                                                                                                                             |
| AdGuard CNAME-disguised trackers     | Sleeknote subdomains                                                                                                                                                                                                                                                                   |

- No generic rule in EasyPrivacy or EasyList matches `/t.js` or `/collect/events`, `/collect/identify` today; 30
  generic EasyPrivacy rules start with `/collect` (such as `/collect?tid=`, `/collect/pageview?`) and 192 with
  `/collect` or `/track`, so a path name is part of the block surface ([EasyPrivacy](https://easylist.to/easylist/easyprivacy.txt)).
- **Cosmetic rules match class names in the page DOM** regardless of the script's host (`##.klaviyo_flyout`). Element
  names and class prefixes are part of the block surface; content inside a shadow root is not matched by page-level
  CSS selectors (§2.2), while its host element is.

### 6.2 CNAME and IP uncloaking

- uBO on Firefox resolves the CNAME of a request and applies filters to the canonical name; Chromium has no DNS API
  for extensions ([uBO wiki](https://github.com/gorhill/uBlock/wiki/uBlock-Origin-works-best-on-Firefox)). It can also
  filter by IP address (`ipaddress=`).
- Brave, since 1.17, checks both the requested URL and the URL with its canonical name against its lists, "applied to
  the initial and canonical domain names" ([Brave](https://brave.com/privacy-updates/6-cname-trickery/)); its own test
  page expects the uncloaked test request to be allowed in Standard and blocked in Aggressive shields
  ([test page](https://dev-pages.brave.software/cname-uncloaking.html)).
- So a customer's `track.example.com` hides sphericon from hostname lists only while the **Edge target's hostname**
  (and its IPs) is on no list; once listed, it is blocked on every customer's Tracking domain at once in those
  browsers. ADR 0059 already does not promise more.

### 6.3 Safari storage caps

"ITP detects third-party CNAME cloaking and third-party IP address cloaking requests and caps the expiry of any
cookies set in the HTTP response to 7 days"; and ITP deletes every cookie created in JavaScript, and all other
storage a script can write (LocalStorage, IndexedDB and others), after 7 days without user interaction with the site
([WebKit tracking prevention](https://webkit.org/tracking-prevention/);
[CNAME defense](https://webkit.org/blog/11338/cname-cloaking-and-bounce-tracking-defense/)). `om_vid` and `om_q` are
script-written, so in Safari a Visitor who stays away 7 days comes back with a new Visitor id and an empty queue, and
any "seen", "closed" or "submitted" state the runtime keeps in the browser is gone with it. A frequency rule longer
than 7 days ("don't show for 30 days after close") holds in Safari only if that state lives on the server (by Visitor
or Contact) or in a cookie set by a response that ITP does not treat as cloaked.

### 6.4 What the vendors do about it

- **OptinMonster**: a Custom Domains add-on CNAMEs a subdomain of the site to `api.optincdn.com` or `a2.optincdn.com`;
  it "reduces the chance of being blocked" by VPNs and ad blockers but "cannot guarantee bypassing advanced filters"
  ([custom domains](https://optinmonster.com/docs/how-to-setup-custom-domains-in-optinmonster/)). An "Ad-Blocker
  Targeting" display rule shows a campaign only to visitors with a blocker
  ([AdBlock targeting](https://optinmonster.com/docs/how-to-create-a-campaign-to-target-adblock-users/)); the loader
  fetches `https://a.optinmonster.com/app/js/prebid-ads.js` as the bait (read from the bundle).
- **Customer.io**: proxy the client and its requests through your own domain; EasyPrivacy blocks `/analytics-js/`
  even there, and the snippet's own path cannot be changed, so it advises the npm package with remapped paths
  ([proxying](https://docs.customer.io/cdp/sources/connections/javascript/js-source-proxy)).
- **Braze**: "most ad blockers do not block the Braze Web SDK", npm gives "immunity from ad-blocker extensions", and
  "Braze does not support routing Web SDK traffic through a customer-owned domain via CNAME records"
  ([integration](https://www.braze.com/docs/developer_guide/sdk_integration?sdktab=web)). EasyPrivacy's
  `||appboycdn.com^$third-party` blocks the CDN build in uBO's defaults.
- **Klaviyo**: an ad blocker that stops the onsite script stops forms (prior research §3).

## 7. Questions this leaves for the grilling tickets

Facts above, framed as the choices they bear on; none is decided here.

- Isolation per Format: shadow DOM with a reset on `:host` (inherits the site's font by default, one document, push
  prompt possible) versus an iframe on the Tracking host (full isolation, a new gate path, per-document inertness, no
  push prompt) versus plain DOM (what Klaviyo ships, at the mercy of the site's CSS and class-name filters).
- Loading: one `t.js` that grows, or a second script fetched only when a message is due (Klaviyo's split), which needs
  a second build output and a new path under ADR 0036.
- Where frequency and dismissal state lives, given Safari's 7-day cap on script-written storage.
- Whether the snippet moves to the `data-collect-key` script-tag form, so a strict CSP needs no nonce for an inline
  stub.

## 8. Unconfirmed

- Whether any browser blocks `CSSStyleSheet.replaceSync()` / `adoptedStyleSheets` under `style-src` without
  `'unsafe-inline'` (MDN names only `insertRule` and `cssText`).
- How OptinMonster, Sleeknote and Drip Onsite draw a message (DOM, shadow, iframe); Sleeknote and Drip script sizes;
  the size of OptinMonster's per-campaign code and `api.min.css`.
- Klaviyo: the exact moment the `Render` set loads (read from minified code, not documented). The `signup_forms`
  entry also hands a ClientStore loader (chunks `4298`, `8667`, `5204`, `1467`, `4813`, about 43.5 kB br, counted in
  the `Render` row) to its startup code; if that runs at page load, page load is about 135 kB br and the on-trigger
  part about 114 kB br; whether embedded forms
  load it at page load; the size of the `styles` CSS chunk.
- That Customer.io's chunk `573` is `gist-web` (sizes differ by 77 bytes).
- Browser autofill (`autocomplete="email"`, `tel`) and password managers on fields inside a shadow root: no source
  read.
- Any primary measurement of how often a first-party script on a CNAMEd host is blocked, or of blocker use in
  general. The PETS 2021 paper "The CNAME of the Game" (Dimova et al.) measures CNAME cloaking itself; its
  filter-list results were not read.
- Sleeknote and Drip CSP pages: search index only (HTTP 403).
