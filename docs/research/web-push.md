# Web push: subscriptions, the service worker origin and browser support

Researched 2026-11-01 for [#920](https://github.com/getsphericon/sphericon/issues/920) (map
[#916](https://github.com/getsphericon/sphericon/issues/916)) from primary sources only: the W3C Push API (the
[Working Draft](https://www.w3.org/TR/push-api/) and the [Editor's Draft](https://w3c.github.io/push-api/), which now
holds Declarative Web Push), the [Service Workers Editor's Draft](https://w3c.github.io/ServiceWorker/), the WHATWG
[Notifications Living Standard](https://notifications.spec.whatwg.org/) (last updated 19 October 2026), RFC
[8030](https://www.rfc-editor.org/rfc/rfc8030.html), [8291](https://www.rfc-editor.org/rfc/rfc8291.html) and
[8292](https://www.rfc-editor.org/rfc/rfc8292.html), the browser vendors' own docs and blogs (WebKit, Apple Developer,
Chrome for Developers, Mozilla Hacks, Mozilla autopush, Chromium source, Bugzilla), MDN's
[browser-compat-data](https://github.com/mdn/browser-compat-data) 8.1.7 (2026-10-22) for version tables, and the docs
of two vendors as prior art (OneSignal, Customer.io). The three specs were read as raw text; text in quotation marks is
copied from them. Vendor pages are paraphrased with a link. No blog or comparison site is cited. A claim no primary
source confirmed is listed under "Unconfirmed" at the end.

sphericon's side is read from [ADR 0059](../adr/0059-a-tracking-domain-also-serves-the-first-party-tracker.md),
[ADR 0071](../adr/0071-a-tracking-domain-is-the-workspaces-own-not-a-sending-domains.md), the tracker in
`packages/analytics/src/index.ts` (the `visitor_id` cookie is set by the script on the page's own host: `path: '/'`,
365 days, `SameSite=Lax`) and [GLOSSARY.md](../../GLOSSARY.md) (Visitor, Contact, Identify, Destination, Tracking
domain).

## Question

What does it take for sphericon to send web push to visitors of a customer's site, given the first-party tracker
served from the customer's Tracking domain (`t.customer.com`)? What a subscription is and who stores it; whether the
service worker must live on the site's own origin or the Tracking domain can own the subscription; browser support
including Safari on macOS and iOS; permission prompt rules; payload limits and notification fields; click handling;
and how a subscription, which belongs to a browser, is tied to a known Contact.

## Summary of findings

- **The service worker must be on the site's own origin, not just its registrable domain.** The Register algorithm
  rejects a script URL or a scope whose origin is not the registering page's origin, the script fetch refuses
  redirects and needs a JavaScript MIME type. ADR 0059's "same site" argument does not carry over: `t.customer.com`
  cannot register a worker for `www.customer.com`. The customer must host one file on every origin that subscribes.
  That file can be one line, `importScripts("https://t.customer.com/sw.js")`: the spec names cross-origin
  `importScripts()` as the way around the origin rule, and OneSignal's whole setup is exactly that.
- **The Tracking domain cannot own the subscription in practice.** A cross-origin iframe cannot ask for notification
  permission (Chrome since 2019, Firefox 70). Only a top-level window on `t.customer.com` could, and then the prompt,
  the permission and every notification name `t.customer.com`. OneSignal ran that model (`<label>.os.tc`) for HTTP
  sites and dropped it in SDK v16; its subscribers had to resubscribe on the real origin. It cannot work on iOS at all.
- **A subscription is an endpoint URL plus two keys, held by our server.** The endpoint is unique per subscription
  and is the natural Destination key (Customer.io uses it as the device id). It belongs to one service worker
  registration in one browser profile: a Visitor, never a person.
- **The VAPID key is a permanent commitment.** A subscription is bound to the `applicationServerKey` it was created
  with; subscribing again with another key fails with `InvalidStateError`, and the push service rejects pushes signed
  by another key. Changing the key means every browser resubscribes on its next visit; subscribers made with another
  platform's key (OneSignal) cannot be imported without that platform's private key.
- **There is no unsubscribe callback.** Revocation by the user, Chrome's automatic revocation (October 2025) and
  Safari's revocation after a push that shows nothing are silent. `pushsubscriptionchange` is not reliable (Chrome
  138+, Firefox partial, no iOS). The server learns at the next send: 404 and 410 are both terminal (delete the
  Destination); 201 means accepted, not delivered.
- **Every push must show a notification.** Chrome rejects `userVisibleOnly: false`; Safari revokes the permission
  when a push shows nothing. There is no silent or data-only message on this channel.
- **Payload: 4096 bytes on the wire, at most 3993 of plaintext,** and as little as 2744 through Mozilla's bridged
  (mobile) path. Images and icons are URLs. Title and body are universal; `image`, `actions`, `badge`,
  `requireInteraction` are Chrome and Edge only; Safari ignores `icon` and shows the site's icon.
- **Safari:** web push for websites in Safari 16 on macOS 13+; on iOS and iPadOS 16.4+ only for a Home Screen web app,
  which needs a manifest with `display: standalone` or `fullscreen` on the customer's origin and the visitor adding
  the site to the Home Screen. Declarative Web Push (Safari 18.4 iOS Home Screen apps, 18.5 macOS) needs no service
  worker at all. No Apple Developer account is needed; allow `*.push.apple.com`.
- **The prompt must follow a click.** Firefox 72+ and Safari reject `requestPermission()` / `subscribe()` without a
  user gesture; Chrome sends sites with very low accept rates to a quieter prompt, and Chrome 155 on Android lets an
  ignored prompt time out. Prompting on page load is out.
- **Click tracking needs no same-origin trick.** `clients.openWindow()` opens a cross-origin URL (it returns `null`),
  and `navigate` in the Notifications standard and the declarative payload has no same-origin check, so the click URL
  can be a tracked redirect on the Tracking domain, as in email.

## 1. What a subscription is and who stores it

- The Push API: a push subscription has an associated scope (a URL), "an associated push endpoint. It MUST be the
  absolute URL exposed by the push service where the application server can send push messages to. A push endpoint
  MUST uniquely identify the push subscription", a P-256 ECDH key pair and an authentication secret, and "MAY have an
  associated subscription expiration time"
  ([Push API ED, §2](https://w3c.github.io/push-api/#push-subscription)).
- The browser hands the page a `PushSubscription` with `endpoint`, `expirationTime` and `keys` (`p256dh`, `auth`). The
  page sends it to the application server, which stores it; Google's guide does exactly that
  ([web.dev, Subscribing a user](https://web.dev/articles/push-notifications-subscribing-a-user)). Apple's guide says
  to "store the push notification subscription endpoint and encryption keys together with the user's account
  information" ([Apple Developer](https://developer.apple.com/documentation/usernotifications/sending-web-push-notifications-in-web-apps-and-browsers)).
- `expirationTime` is `null` when the subscription has no expiration
  ([MDN](https://developer.mozilla.org/en-US/docs/Web/API/PushSubscription/expirationTime)).
- A subscription made through a service worker belongs to that registration: "a service worker registration's
  associated push subscription is the push subscription whose scope equals the service worker registration's scope
  URL" ([Push API ED](https://w3c.github.io/push-api/#dfn-associated-push-subscription)). One registration, one
  subscription.
- The payload is encrypted to the subscription's keys with `aes128gcm` in a single record, and the push service cannot
  read it ([RFC 8291 §§1, 4](https://www.rfc-editor.org/rfc/rfc8291.html)). The server therefore needs `p256dh` and
  `auth` for every send, not just the endpoint.
- Sending is an HTTP `POST` to the endpoint with a required `TTL` header and optional `Urgency` (`very-low`, `low`,
  `normal`, `high`, default `normal`) and `Topic` (a pending message with the same topic is replaced; at most 32
  base64url characters) ([RFC 8030 §§5.2–5.4](https://www.rfc-editor.org/rfc/rfc8030.html)). Apple stores an
  undeliverable message for 30 days at most, depending on `TTL`
  ([Apple Developer](https://developer.apple.com/documentation/usernotifications/sending-web-push-notifications-in-web-apps-and-browsers)).

### VAPID

- The application server signs a JWT with ECDSA on P-256 (ES256) and sends it as `Authorization: vapid t=<jwt>,
  k=<public key>`. `aud` is the origin of the push endpoint, `exp` at most 24 hours ahead, `sub` a `mailto:` or
  `https:` contact ([RFC 8292 §§2, 3](https://www.rfc-editor.org/rfc/rfc8292.html)). Apple asks not to refresh the JWT
  more than once an hour.
- The public key passed as `applicationServerKey` at subscribe time restricts the subscription to that key: the push
  service must reject a message whose JWT is not signed by it ([RFC 8292 §4](https://www.rfc-editor.org/rfc/rfc8292.html)).
  Apple answers `403` with reason `VapidPkHashMismatch`
  ([Apple Developer](https://developer.apple.com/documentation/usernotifications/sending-web-push-notifications-in-web-apps-and-browsers)).
  Chrome requires the key: `subscribe()` without `applicationServerKey` fails
  ([MDN BCD](https://github.com/mdn/browser-compat-data), `PushManager.subscribe`).
- In `subscribe()`, if a subscription already exists and "any attribute on options contains a different value to that
  stored for subscription", the promise is rejected with `InvalidStateError`
  ([Push API ED, `subscribe()`](https://w3c.github.io/push-api/#subscribe-method)). Changing the key means
  `unsubscribe()` and a new subscription from the page, which only happens when the visitor comes back.
- Consequence for the model: the key pair's scope (per Workspace or per Instance) is decided once. A per-Workspace key
  lets a Workspace move between Instances with its subscribers if the private key moves with it; a key that is lost or
  rotated loses every subscriber who does not return. Subscribers collected under another platform's key are useless
  without that platform's private key.

## 2. Expiry, 404 and 410

- RFC 8030: a push service may expire a subscription at any time, and a push to an expired subscription gets `404`
  ([§7.3](https://www.rfc-editor.org/rfc/rfc8030.html)). `410` is defined there for receipts; in practice push
  services answer `410` for a subscription that is gone. `201` means the message was accepted, which does not mean it
  was delivered ([§5](https://www.rfc-editor.org/rfc/rfc8030.html)).
- Google's guide: `404` means the subscription expired, delete it; `410` means it is no longer valid, remove it
  (`unsubscribe()` on the client produces a `410`); `413` too large; `429` rate limited with `Retry-After`; `400` bad
  headers ([web.dev, Web push protocol](https://web.dev/articles/push-notifications-web-push-protocol)).
- Apple: `410` "the device token has expired", `413` payload over 4 KB, `429` too many requests to the same device,
  `403` authentication (with a JSON `reason`: `BadJwtToken`, `VapidPkHashMismatch`, ...)
  ([Apple Developer](https://developer.apple.com/documentation/usernotifications/sending-web-push-notifications-in-web-apps-and-browsers)).
- Mozilla autopush: `404` "Push subscription is invalid", `410` "Push subscription is no longer available"; mobile
  routing records expire after 60 days without check-in
  ([autopush HTTP API](https://mozilla-services.github.io/autopush-rs/http.html)).
- When a subscription is deactivated, "both the user agent and the push service MUST delete any stored copies of its
  details. Subsequent push messages for this push subscription MUST NOT be delivered"; it is deactivated when its
  registration is unregistered, and the user agent "MUST deactivate the affected subscriptions" when the permission is
  revoked ([Push API ED](https://w3c.github.io/push-api/#subscription-deactivation)). The Service Workers spec says
  registrations "should be cleared when users intend to clear them"
  ([§6.8](https://w3c.github.io/ServiceWorker/#privacy)), so the subscription lives no longer than the site's storage.
- On revocation the user agent "MAY fire the `pushsubscriptionchange` event"; a refresh fires it with the old and new
  subscription ([Push API ED](https://w3c.github.io/push-api/#subscription-refreshes)). Support: Chrome 138, Firefox 44
  without `oldSubscription`/`newSubscription`, Safari 16 on macOS, **not iOS** ([MDN BCD](https://github.com/mdn/browser-compat-data),
  `ServiceWorkerGlobalScope.pushsubscriptionchange_event`). The server cannot rely on it.
- Revocation without any signal to the site: Chrome removes the notification permission of sites with very low
  engagement and a high volume of notifications, on Android and desktop, except installed web apps
  ([Google, 10 October 2025](https://blog.google/chromium/automatic-notification-permission/)); Safari revokes the
  permission when the worker does not show a notification for a push
  ([Apple Developer](https://developer.apple.com/documentation/usernotifications/sending-web-push-notifications-in-web-apps-and-browsers)).
- Chrome rate-limits disruptive senders to no less than 1000 messages a minute, with `429` above it, from January 2026
  ([Chrome for Developers](https://developer.chrome.com/blog/web-push-rate-limits)).

What this means: a send result of 404 or 410 deletes the Destination; 429 and 5xx are a Deferral; nothing tells us
about a revoked permission until the next send fails. Re-posting `getSubscription()` on each page view is the only
way to learn about a new or changed subscription.

## 3. The service worker origin

### The rules

- Register: "If job's script url's origin and job's referrer's origin are not same origin, then: Invoke Reject Job
  Promise with job and "SecurityError" DOMException"; the same for the scope URL
  ([Service Workers ED, Register](https://w3c.github.io/ServiceWorker/#register-algorithm)). Origin means scheme, host
  and port: `www.customer.com`, `customer.com` and `shop.customer.com` are three origins, each with its own worker,
  permission and subscriptions.
- The script fetch: "Set request's redirect mode to "error"", and a response whose MIME type "is not a JavaScript MIME
  type" is rejected with `SecurityError` ([Update algorithm](https://w3c.github.io/ServiceWorker/#update-algorithm)). A
  redirect from `www.customer.com/sw.js` to the Tracking domain does not work.
- Scope: without a `Service-Worker-Allowed` header the maximum scope is the script's directory; the header may widen it
  within the same origin only ([Update algorithm](https://w3c.github.io/ServiceWorker/#update-algorithm),
  [§6.5](https://w3c.github.io/ServiceWorker/#path-restriction)).
- The non-normative §6.3.1: "A service worker executes in the registering service worker client's origin. [...]
  Therefore, service workers cannot be hosted on CDNs. But they can include resources via importScripts()"
  ([Service Workers ED](https://w3c.github.io/ServiceWorker/#origin-restriction)). `importScripts()` works only while
  the worker is parsed or installing, and the update check compares the imported scripts byte for byte, so a change
  to the imported script reaches installed workers on their next update check
  ([§6.3.2](https://w3c.github.io/ServiceWorker/#importscripts), [Update](https://w3c.github.io/ServiceWorker/#update-algorithm)).
- Service workers and their clients must be secure contexts: HTTPS
  ([§6.1](https://w3c.github.io/ServiceWorker/#secure-context)); `subscribe()` rejects a scope that is not `https`
  ([Push API ED](https://w3c.github.io/push-api/#subscribe-method)).

### What the customer must host

- One JavaScript file on each origin where visitors subscribe, served over HTTPS with a JavaScript MIME type, without
  a redirect and without authentication, containing `importScripts("https://<tracking host>/sw.js")`. OneSignal's
  setup is the same one line (`importScripts("https://cdn.onesignal.com/sdks/web/v16/OneSignalSDK.sw.js")`) and says
  the file "must be hosted on the same domain as your site. CDNs and subdomains are not allowed"
  ([OneSignal, service worker](https://documentation.onesignal.com/docs/en/onesignal-service-worker)).
- A push worker needs no page-controlling scope: OneSignal recommends a dedicated directory that serves no pages
  (`/push/onesignal/`), because a root-scoped file conflicts with a site's own worker (a PWA, an offline cache). The
  registration's scope is then that directory, and the push subscription belongs to it
  ([OneSignal](https://documentation.onesignal.com/docs/en/onesignal-service-worker)).
- The imported script can come from the Tracking domain or from the Instance's tracker host; that choice only affects
  hostname blocklists (ADR 0059), not the subscription, whose origin is the site's.
- On iOS the site must also serve a web app manifest with `display` `standalone` or `fullscreen`, and the visitor must
  add the site to the Home Screen (section 5).

### Can the Tracking domain own the subscription?

Only as a top-level window on that host, and the result is a subscription of `t.customer.com`, not of the site:

- **Not from an iframe.** Chromium (July 2019) made the `notifications` permission requestable only from browsing
  contexts same-origin with the top-level page; a cross-origin subframe without an earlier grant sees it as denied and
  cannot use Push or Notifications ([Chromium commit f39e22b](https://chromium.googlesource.com/chromium/src.git/+/f39e22b4d1c2d1e4f4bd35f48522c6d325cca024)).
  Firefox 70 does the same ([Bug 1560741](https://bugzilla.mozilla.org/show_bug.cgi?id=1560741); MDN BCD notes on
  `Notification.requestPermission`). Chrome's earlier deprecation suggested opening a new window instead
  ([Chrome 58 deprecations](https://developer.chrome.com/blog/chrome-58-deprecations/)).
- **A popup window on `t.customer.com`** could subscribe: the permission, the prompt's origin and the origin shown on
  each notification would be `t.customer.com`. A visitor who allowed it on `www.customer.com`'s popup has not allowed
  `www.customer.com`. OneSignal shipped exactly this for HTTP sites (`https://your-label.os.tc`), stopped supporting it
  in SDK v16, and its customers must have their users resubscribe on the real origin
  ([OneSignal troubleshooting](https://documentation.onesignal.com/docs/en/troubleshooting-web-push)).
- **Not on iOS.** iOS push exists only for a Home Screen web app of the site the visitor installed (section 5); a
  popup on another host has no Home Screen app.
- **Declarative Web Push** removes the worker file but not the origin: a subscription made from `window.pushManager`
  has a "window-accessible scope" whose path is `/` of the page's own URL
  ([Push API ED](https://w3c.github.io/push-api/#dfn-window-accessible-scope)). The tracker script, running in the
  page, could subscribe for the site's origin with no file to host, on Safari only (section 5).

## 4. Permission prompt rules

- The Push API asks for permission inside `subscribe()`: "Let permission be request permission to use "push"", and
  a `denied` permission rejects with `NotAllowedError` ([Push API ED](https://w3c.github.io/push-api/#subscribe-method)).
  Calling `subscribe()` asks for the notification permission if the site lacks it
  ([web.dev](https://web.dev/articles/push-notifications-subscribing-a-user)).
- `userVisibleOnly`: the spec rejects `false` when the user agent requires `true`
  ([Push API ED](https://w3c.github.io/push-api/#subscribe-method)); Chrome requires `true`
  ([web.dev](https://web.dev/articles/push-notifications-subscribing-a-user)); Safari revokes the permission when a
  push shows nothing ([WebKit, Meet Web Push](https://webkit.org/blog/12945/meet-web-push/);
  [Apple Developer](https://developer.apple.com/documentation/usernotifications/sending-web-push-notifications-in-web-apps-and-browsers)).
- **User gesture.** Firefox 72+ rejects `Notification.requestPermission()` and `PushManager.subscribe()` at once unless
  called from a user gesture, and shows only an address bar icon
  ([Mozilla Hacks](https://hacks.mozilla.org/2019/11/upcoming-notification-permission-changes-in-firefox-72/)).
  Safari requires an explicit user gesture and asks to call `subscribe()` immediately from the gesture's handler
  ([WebKit](https://webkit.org/blog/12945/meet-web-push/),
  [Apple Developer](https://developer.apple.com/documentation/usernotifications/sending-web-push-notifications-in-web-apps-and-browsers)).
- **Chrome.** Chrome automatically enrolls sites with very low accept rates in the quieter prompt, and advises: "Avoid
  showing prompts and/or overlays without context or immediately after a user lands on the site"
  ([Chrome, CrUX notification data](https://developer.chrome.com/blog/notification-permission-data-in-crux)). From Chrome 155 on Android the
  prompt is non-blocking and times out to `default`; users can opt in later from Site Controls, so a site should also
  watch `navigator.permissions.query({name: "notifications"})` for changes
  ([Chrome, 22 July 2026](https://developer.chrome.com/blog/notification-prompts-android)).
- Best practice from all three: an in-page explanation and a button (a "soft prompt"), the browser prompt only from
  that button's click handler, never on page load. The prompt's placement belongs to the deferred on-site surface the
  map lists under "Not yet specified".

## 5. Browser support

Versions from MDN BCD 8.1.7 unless another source is linked.

| Browser               | Push API (`PushManager`)                     | Note                                                                |
| --------------------- | -------------------------------------------- | ------------------------------------------------------------------- |
| Chrome / Edge desktop | 42 / 17                                      | `applicationServerKey` required; `userVisibleOnly: true` required   |
| Chrome Android        | 42                                           | Chrome 155 prompt times out (section 4)                             |
| Firefox               | 44                                           | gesture required from 72; Android uses a bridged path (section 6)   |
| Safari macOS          | 16 (macOS 13 Ventura+)                       | gesture required; no silent push                                    |
| Safari iOS / iPadOS   | 16.4, Home Screen web apps only              | `Notification` is undefined unless the page is an installed web app |
| Declarative Web Push  | iOS/iPadOS 18.4 Home Screen apps; macOS 18.5 | `window.pushManager`; no service worker; Chrome and Firefox: no     |

- Safari: "Add web push to Home Screen web apps in iOS 16.4 or later and Webpages in Safari 16 for macOS 13 or
  later"; no Apple Developer Program membership; allow `https://*.push.apple.com`; HTTP/1.1 or HTTP/2 with SNI
  ([Apple Developer](https://developer.apple.com/documentation/usernotifications/sending-web-push-notifications-in-web-apps-and-browsers)).
- iOS: the visitor adds the site from the Share menu with "Add to Home Screen"; the site's manifest needs `display`
  `standalone` or `fullscreen`; the permission request must follow direct user interaction; notifications respect
  Focus; the app can set a badge with the Badging API
  ([WebKit, Web Push for Web Apps on iOS and iPadOS](https://webkit.org/blog/13878/web-push-for-web-apps-on-ios-and-ipados/)).
  A marketing site that is not an installable web app cannot reach iOS visitors.
- Declarative Web Push: iOS and iPadOS 18.4 for Home Screen web apps
  ([WebKit, Safari 18.4](https://webkit.org/blog/16574/webkit-features-in-safari-18-4/)), macOS from Safari 18.5
  ([WebKit, Safari 18.5](https://webkit.org/blog/16923/webkit-features-in-safari-18-5/); BCD lists
  `Window.pushManager` as Safari 18.4); "without requiring an installed service worker", `window.pushManager` for
  subscription management ([WebKit, Meet Declarative Web Push](https://webkit.org/blog/16535/meet-declarative-web-push/)).
  Chrome and Firefox have not shipped `Window.pushManager` (BCD, with open bugs
  [crbug 382298314](https://crbug.com/382298314) and [bug 1935325](https://bugzil.la/1935325)).

## 6. Payload limits and notification fields

### Size

- A push service must not answer `413` to a body of 4096 bytes or less
  ([RFC 8030 §7.2](https://www.rfc-editor.org/rfc/rfc8030.html)); with one `aes128gcm` record that is at most 3993
  bytes of plaintext ([RFC 8291 §4](https://www.rfc-editor.org/rfc/rfc8291.html)). Apple's `PayloadTooLarge` is
  "over the limit of 4 KB" ([Apple Developer](https://developer.apple.com/documentation/usernotifications/sending-web-push-notifications-in-web-apps-and-browsers)).
- Mozilla's bridged connections (Firefox on mobile, through FCM/APNs) base64-encode the data, which may cut it to 2744
  bytes ([autopush HTTP API](https://mozilla-services.github.io/autopush-rs/http.html)).
- So images and icons are URLs fetched by the browser, and the content budget for title, body, URLs and data is about
  2.7 KB of JSON if Firefox on Android matters.

### Fields

`showNotification(title, options)`: `NotificationOptions` has `dir`, `lang`, `body`, `navigate`, `tag`, `image`,
`icon`, `badge`, `vibrate`, `timestamp`, `renotify`, `silent`, `requireInteraction`, `data`, `actions`; an action has
`action`, `title`, `navigate`, `icon`, and the number of actions is implementation-defined (`Notification.maxActions`)
([Notifications Standard](https://notifications.spec.whatwg.org/#api)). Support (MDN BCD, `Notification.*`):

| Field                | Chrome | Firefox           | Safari macOS   | Safari iOS     |
| -------------------- | ------ | ----------------- | -------------- | -------------- |
| `title`, `body`      | yes    | yes               | yes            | `body` 16.4    |
| `icon`               | yes    | yes               | set, no effect | set, no effect |
| `image`              | 56     | no                | no             | no             |
| `badge`              | 53     | no                | no             | no             |
| `actions`            | 53     | 152               | no             | no             |
| `requireInteraction` | 47     | 117, Windows only | no             | no             |
| `tag`                | yes    | yes               | no             | no             |
| `data`               | yes    | yes               | 16             | 16.4           |
| `navigate`           | no     | 158               | 18.5           | 18.5           |

The content model can therefore count on a title, a body, a click URL and opaque data; an image, buttons and a large
icon are progressive extras that some browsers drop.

### Declarative payload

A declarative push message is JSON with `"web_push": 8030` and a `notification` map that needs `title` and
`navigate` and may carry `dir`, `lang`, `body`, `tag`, `image`, `icon`, `badge`, `vibrate`, `timestamp`, `renotify`,
`silent`, `requireInteraction`, `data` and `actions` (each action needs `action`, `title`, `navigate`), plus a
top-level `mutable` boolean ([Push API ED, declarative push message parser](https://w3c.github.io/push-api/#declarative-push-message)).
On receipt, a browser that implements it parses every push: if it is declarative and `mutable` is true and a worker
exists, a `push` event fires with the proposed notification; if the worker shows nothing, or `mutable` is false, the
browser shows the declarative notification itself ([Push API ED, Receiving a push message](https://w3c.github.io/push-api/#receiving-a-push-message)).
A browser without declarative support hands the same JSON to the worker's `push` event, so one payload format can
serve every browser if our worker parses it.

## 7. Click handling

- Activation: if the notification (or the clicked action) has a navigation URL, the user agent navigates a window to it
  and stops; otherwise a persistent notification fires `notificationclick` in the worker with the action name
  ([Notifications Standard, activating a notification](https://notifications.spec.whatwg.org/#activating-a-notification)).
  The `navigate` URL is parsed against the base URL with no origin check.
- In `notificationclick` the worker typically calls `clients.openWindow(url)`, which needs transient activation in a
  window of its origin ("If no Window in this origin has transient activation, return a promise rejected with an
  "InvalidAccessError" DOMException") and, for a URL whose storage key differs, resolves to `null` after opening it
  ([Service Workers ED, `openWindow`](https://w3c.github.io/ServiceWorker/#clients-openwindow)). MDN: the promise
  resolves to `null` for a cross-origin URL; Firefox allows the popup only from a notification click
  ([MDN, `Clients.openWindow`](https://developer.mozilla.org/en-US/docs/Web/API/Clients/openWindow)).
- So the click URL can be a tracked link on the Tracking domain that redirects to the landing page, the email model of
  ADR 0036/0038, and it works for `notificationclick`, for `navigate` and for declarative pushes, where no worker code
  runs. A worker that runs can also report a shown or clicked notification with a `fetch()` to `/collect` before
  opening the URL; a declarative non-mutable push cannot.

## 8. A subscription belongs to a browser, not a person

- The endpoint "MUST uniquely identify the push subscription", and the subscription belongs to one service worker
  registration of one origin in one browser profile ([Push API ED](https://w3c.github.io/push-api/#push-subscription)).
  It knows nothing about who is at the keyboard and dies with the site's storage (section 2). That is a Visitor.
- **OneSignal:** a Subscription is a device, browser, email or phone; a User has one or more Subscriptions (up to 20);
  without an External ID each Subscription is its own anonymous user and can be messaged. `OneSignal.login(externalId)`
  attaches the current Subscription to that user and merges; `logout()` returns it to a new anonymous user
  ([OneSignal, Users](https://documentation.onesignal.com/docs/en/users)).
- **Customer.io:** each browser where a person allows notifications is a device on the `web` platform, identified by the
  subscription endpoint URL; a device token alone is anonymous and Customer.io sends no anonymous push; call
  `clearIdentify` on logout so a shared device does not get another person's messages
  ([Customer.io, device tokens](https://docs.customer.io/messaging/channels/push/device-tokens/)).
- For sphericon: the subscription arrives from the page together with the `visitor_id` cookie the tracker already set
  on the site's host, so it attaches to the Visitor; Identify then reaches it through the Visitor's Contact, exactly
  as it stitches Events. Whether an anonymous Visitor's subscription can be sent to (OneSignal) or only a Contact's
  (Customer.io) is a model decision. The cookie (365 days) and the subscription live and die independently: clearing
  cookies keeps the subscription while the permission stands, and the tracker would find it again with
  `getSubscription()`. Re-posting the current subscription with the current `visitor_id` on every page view moves a
  known endpoint to the current Visitor instead of duplicating it, and catches silent refreshes.

## Unconfirmed

- MDN BCD lists `notificationclick` as unsupported on Safari iOS while `push` and `showNotification` are supported
  from 16.4. No WebKit or Apple page read here says iOS lacks it; it looks like missing compat data. Click tracking on
  iOS should be tested, not assumed absent.
- Whether Safari applies the declarative parser to a push sent to a subscription made through a service worker (the
  spec says it does, for any subscription) was not confirmed on a WebKit page.
- Whether Chrome shows a default "site updated in the background" notification when a worker shows nothing, and how
  many such pushes Safari tolerates before revoking, were not found on a primary source.
- No expiry period for a Chrome (FCM) or Apple web push subscription was found; only Mozilla's 60 days without
  check-in for its mobile routing records.
- How Chrome and Firefox present the notification's origin (`t.customer.com` vs `www.customer.com`) is shown in the
  browsers' UI but is not specified in the Notifications Standard.
