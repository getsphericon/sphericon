# On-site popups, banners and forms on marketing platforms: channel, capture surface, or both

Researched 2026-11-01 for [#942](https://github.com/getsphericon/sphericon/issues/942) (map
[#916](https://github.com/getsphericon/sphericon/issues/916)) from primary sources: the vendors' own docs, help centers and
JS SDK references (Customer.io, Braze, Klaviyo, Mautic, OptinMonster, Sumo/BDOW!, WonderPush), plus Google Search
Central, MDN, W3C WAI and web.dev for the constraints. No third-party blog is cited for a product claim; a vendor's own
marketing blog is named as such where used. Quotes are kept to exact identifiers and option labels; everything else is a
paraphrase of the linked page. Limits: **Brevo's help center (help.brevo.com) refused automated reads (HTTP 403)**, so
every Brevo claim below was seen only in a search engine's index of the linked page and is marked "(search index)";
WonderPush (Brevo's push and pop-up engine) docs were read directly.

Terms as in [GLOSSARY.md](../../GLOSSARY.md): **Visitor**, **Identify**, **Event**, **Confirmation**, and "On-site popup /
form" under "Channels & future surfaces". The tracker is the one of
[ADR 0059](../adr/0059-a-tracking-domain-also-serves-the-first-party-tracker.md): `/t.js` and `/collect` served from the
Workspace's Tracking domain. Web push facts already settled in #920 (a subscription belongs to a Visitor, the service
worker lives on the site's own origin) are not repeated.

## 0. The question in one paragraph

Two families of product put things on a customer's page. **Messaging engines** (Customer.io, Braze, WonderPush/Brevo)
treat the on-site message as a **sending channel**: a message is addressed to a person (or, in a separate mode, to
anonymous traffic on matching pages), queued or synced to the browser, and reported like any other send. **Form
specialists** (Klaviyo sign-up forms, OptinMonster, Sumo, Mautic focus items) treat it as a **capture surface**: display
is decided in the browser by page, time, scroll and exit rules plus a cookie, and the point is the submit that creates or
updates a profile and its consent. The engines have grown capture templates inside their message unit (Braze email and
phone sign-up forms, Customer.io Lead Form), so the big ones do both, but through one object: the in-app message.

## 1. Customer.io: in-app messages (web)

**Delivery.** The web in-app plugin loads with the Customer.io JavaScript snippet once in-app is turned on for the
workspace; its options sit under the `"Customer.io In-App Plugin"` key of `integrations` (`siteId`, `anonymousInApp`,
`events`, `colorScheme`)
([JS in-app](https://docs.customer.io/integrations/data-in/connections/javascript/in-app/)). Identified visitors get
messages in real time while there are active messages; otherwise the SDK polls. The getting-started page gives 10 s while
a message is active, 60 s if one was received in the last 30 days, 180 s otherwise
([getting started](https://docs.customer.io/journeys/in-app-getting-started/)); the plugin page says only that it checks
"about once a minute" before the first message and then switches to real-time for identified visitors
([JS in-app](https://docs.customer.io/integrations/data-in/connections/javascript/in-app/)).

**Addressing.** A message is sent to a **person** (by id or email), not to a device: it is queued and shown in the first
device or browser the person visits after the send, and not again on another device unless the person re-enters the
automation or is re-broadcast ([getting started](https://docs.customer.io/journeys/in-app-getting-started/)). It is a
step in any automation or broadcast, and can be created through the API with page rules in the payload
([send an in-app message](https://docs.customer.io/messaging/channels/in-app/send-in-app-message/)).

**Targeting and display.** Page rules (include/exclude, `*` wildcard, Liquid allowed; the page is the URL unless the site
sends `page` calls). Messages expire after 30 days by default, 60 days at most. A message shows once unless marked
_Persist_, which keeps it across page views until closed or expired (the way banners are made). Priority orders the queue
but does not replace a shown message; one modal at a time, up to six overlays. Positions: modal, overlay, inline
("replace an empty element"), tooltip anchored to a CSS selector (web only)
([send an in-app message](https://docs.customer.io/messaging/channels/in-app/send-in-app-message/)). No delay, scroll or
exit-intent trigger is documented on that page.

**Anonymous mode.** A separate object, "anonymous messages": shown to every unidentified visitor on pages matching the
page rules, and explicitly excludes identified people; it has its own frequency setting and goal. Metrics are views,
clicks per button and conversions, where a conversion is by default the visitor becoming identified within a week of
clicking ([anonymous messages](https://docs.customer.io/messaging/channels/in-app/anonymous/overview/)).

**Forms.** Forms and surveys are message components; a submission is recorded as an event, can trigger an automation
("Form submission" trigger), and feeds an automatic per-form segment; fields are read in Liquid as
`event.submissions.<field_name>` ([forms](https://docs.customer.io/messaging/channels/in-app/forms/overview/)). In
anonymous messages a submission is linked to a profile once the person is identified
([anonymous forms](https://docs.customer.io/messaging/channels/in-app/anonymous/forms/)). The **Lead Form** component
(name, email) adds the submitter to the workspace as a person, but does **not** identify the browser: activity after the
submit stays anonymous ([lead form](https://docs.customer.io/messaging/channels/in-app/forms/lead-form/)).

**Events.** Browser-side: `in-app:message-opened` (displayed), `in-app:message-dismissed`, `in-app:message-action`
(with `actionName`, `actionValue`), `in-app:message-error`, `in-app:message-changed`, each with `deliveryId` and
`messageId` ([JS in-app](https://docs.customer.io/integrations/data-in/connections/javascript/in-app/)). An outside click
on a modal sends `in-app:message.dismissed`
([send an in-app message](https://docs.customer.io/messaging/channels/in-app/send-in-app-message/)).

## 2. Braze: in-app messages, Banners, Content Cards (Web SDK)

**In-app messages.** Types: slideup (a toast in a corner on web), modal, full, and HTML (rendered in an iframe, talks to
the SDK through `brazeBridge`, needs the `allowUserSuppliedJavascript` init option)
([in-app messages](https://www.braze.com/docs/developer_guide/in_app_messages/)). Delivery: at session start the SDK
receives every eligible message and prefetches assets; templated messages receive only the trigger and fetch the content
when triggered. Triggers are local SDK events: session start, push click, any or a specific purchase, a custom event.
Messages cannot be triggered by the API or server-side events, and the Web SDK does not support server-side triggering.
The SDK rate-limits triggered messages to one per 30 s by default (`minimumIntervalBetweenTriggerActionsInSeconds`).
Showing is automatic via `braze.automaticallyShowInAppMessages()`; a site that wants control removes that call and uses
`braze.subscribeToInAppMessage` + `braze.showInAppMessage`
([triggering](https://www.braze.com/docs/developer_guide/in_app_messages/triggering_messages/)).

**Analytics.** Impressions and clicks are logged automatically with the built-in display; custom UIs call
`logInAppMessageImpression`, `logInAppMessageClick`, `logInAppMessageButtonClick`, `logInAppMessageHtmlClick`
([logging](https://www.braze.com/docs/developer_guide/in_app_messages/logging_message_data/)). In-app messages and Content
Cards are not counted as or toward global frequency caps
([frequency capping](https://www.braze.com/docs/user_guide/messaging/messaging_fundamentals/frequency_capping/)).

**Banners.** Named placements the site creates; the SDK requests a refresh per placement (up to 10 per request) and
Braze returns the highest-priority Banner the user qualifies for, rendered inline, typically in an iframe, at the full
width of the container. Refresh runs automatically at session start and on `changeUser`; explicit `requestBannersRefresh`
calls draw from a token bucket (5 per session, one refilled per 180 s). Liquid and segmentation are recomputed on every
refresh. Banners do not support API-triggered or action-based campaigns ([Banners](https://www.braze.com/docs/developer_guide/banners/)).

**Content Cards.** Persistent cards embedded in the app or site; Braze contrasts them with in-app messages as permanent
versus short-lived, both needing no opt-in
([Content Cards](https://www.braze.com/docs/user_guide/message_building_by_channel/content_cards/)).

**Capture.** Drag-and-drop templates: an email sign-up form (collected emails get the global email subscription by
default; double opt-in is built as a separate action-based Canvas that sends a confirmation email with the
`set_user_to_opted_in_url` link) ([email capture](https://www.braze.com/docs/user_guide/messaging/templates/in_app_message_templates/email_capture/))
and an SMS/RCS or WhatsApp sign-up form, one subscription group per form, with country restriction and a placeholder SMS
disclaimer (message types, consent not a purchase condition, frequency, data rates, terms and privacy links, HELP/STOP)
([phone capture](https://www.braze.com/docs/user_guide/messaging/templates/in_app_message_templates/phone_number_capture/)).
Opt-ins are found as a segment of users who received the message and submitted the form (same pages).

**Web push soft prompt.** A modal in-app message with the extra `msg-id` = `push-primer`, triggered by a custom event; the
site's `subscribeToInAppMessage` handler skips it when `braze.isPushSupported()` is false or permission is already
granted or blocked, and calls `braze.requestPushPermission()` from the button click
([soft push prompts](https://www.braze.com/docs/developer_guide/push_notifications/soft_push_prompts/)).

**CSP.** `connect-src` for the SDK endpoint; `script-src` for the CDN, `'unsafe-eval'` for the snippet form (avoided with
the npm build), and a nonce or `'unsafe-inline'` for custom HTML messages; `img-src` for the image CDNs; the nonce is
passed as `contentSecurityNonce`
([CSP](https://www.braze.com/docs/developer_guide/platforms/web/content_security_policy/)).

## 3. Klaviyo: sign-up forms

**Types.** Popup, flyout, full page, embed, banner (top or bottom bar), plus a teaser to reopen a closed popup, flyout or
full page. Forms need Klaviyo's onsite JavaScript; an ad blocker that stops it stops forms
([getting started](https://help.klaviyo.com/hc/en-us/articles/360026474752)).

**Targeting and behaviours** ([targeting & behaviors](https://help.klaviyo.com/hc/en-us/articles/4413544555547)):

- Display priority Low to Highest (not for embeds or custom-triggered forms).
- Timing: immediately; or rules combined: exit intent, time delay, scroll percentage, number of pages seen; or only on a
  custom trigger. Exit intent on desktop is the cursor leaving the page, on mobile a quick upward scroll.
- Frequency (popup, flyout, full page): show again N days after close (0 = every page load); don't show again after
  submit or a go-to-URL click.
- Devices: all, desktop, mobile.
- Audience: don't show to existing profiles, show to all, to any existing profile, to email-only or SMS-only subscribers,
  to a list or segment (include/exclude). Also URL contains/equals with `*`, UTM parameters (AND), IP-based location,
  Shopify cart contents. Under EU/EEA/UK/CH Shopify consent, non-consenting visitors may be treated as anonymous.

**Consent.** A phone field collects promotional SMS consent by default (transactional or both selectable), with matching
disclosure required ([getting started](https://help.klaviyo.com/hc/en-us/articles/360026474752)). Smart Opt-in confirms
the phone with a one-time code sent from a Klaviyo-managed short code; the profile gets SMS consent "Subscribed" and a
"Subscribed to SMS Marketing" event; Klaviyo strongly recommends double opt-in for SMS
([Smart Opt-in](https://help.klaviyo.com/hc/en-us/articles/24743883751451)). For email, a double opt-in list adds the
submitter only after the confirmation email is clicked
([consent pages](https://help.klaviyo.com/hc/en-us/articles/115005251848)).

**Events.** In the page: a `klaviyoForms` window event with `detail.type` `open`, `embedOpen`, `close`,
`redirectedToUrl`, `submit` (once per form, at the main conversion), `stepSubmit` (per step). The page states that form
analytics are not tied to profiles; its examples push their own profile events with `_learnq`
([form activity JS](https://developers.klaviyo.com/en/docs/track-klaviyo-form-activity-using-javascript)). Welcome flows
start from the form's list ([getting started](https://help.klaviyo.com/hc/en-us/articles/360026474752)).

**Web push.** Not offered: the glossary says web push is not offered and only mobile push is
([push glossary](https://help.klaviyo.com/hc/en-us/articles/21997060301979)).

## 4. Brevo (and WonderPush, its engine)

All Brevo lines are **(search index)**, see the limits above.

- Sign-up forms: pop-up, full-page or embedded; a pop-up needs the website URL added first; display trigger page-load
  delay or exit intent, a visit frequency, and pages (all, up to 20 listed, or all except)
  ([pop-up form](https://help.brevo.com/hc/en-us/articles/20791391306770-Create-a-pop-up-sign-up-form-in-Brevo)).
- Double opt-in is a form setting available only when the form has an email field; the contact is added after clicking
  the confirmation link; Brevo suggests putting the GDPR consent text in the confirmation email as proof
  (same page; [DOI](https://help.brevo.com/hc/en-us/articles/208733449-Double-opt-in-DOI-What-it-is-and-how-to-track-user-sign-ups)).
- Pop-up and in-app campaigns are a separate campaign type that reuses the push setup
  ([pop-up campaign](https://help.brevo.com/hc/en-us/articles/36487953771538)); web push subscription prompts are a bell,
  the native prompt, a custom HTML dialog or an inline link, under Settings > Campaigns > Push > Subscription prompts
  ([push and pop-ups setup](https://help.brevo.com/hc/en-us/articles/4609653630354-Set-up-push-notifications-and-pop-ups)).

WonderPush, read directly: popup formats card, modal, banner (auto-dismissed, top or bottom), image, HTML templates;
triggers website open, a custom event (`trackEvent`), specific URLs, exit intent (web only); by default shown once per
user, optionally repeated with a minimum gap ([composing in-apps](https://docs.wonderpush.com/docs/composing-in-app-messages)).
Subscription prompts: bell, native prompt (schedulable), HTML dialog with `closeSnooze`/`negativeSnooze`, and a switch;
any element can call `WonderPush.subscribeToNotifications(event)`; URL include/exclude rules
([subscription prompts](https://docs.wonderpush.com/docs/subscription-prompts)).

## 5. Mautic: focus items and forms

**Focus items** ([docs](https://docs.mautic.org/en/5.x/channels/focus_items.html)):

- Types: "Collect Data" (shows an existing Form; a submit is a conversion), "Display a Notice" (no conversion), "Emphasize a
  link" (a click is a conversion).
- Styles: "Bar", "Modal", "Notification" (corner), "Full page".
- When: "Upon arrival", "After slightly scrolling down", "After scrolling to the middle", "After scrolling to the bottom",
  "Visitor intends to leave", plus "Timeout before engage" seconds.
- How often: "Every page", "Once per session", "Every 2 minutes", "Every 15 minutes", "Once per hour", "Once per day";
  stop after a conversion, stop after closing (cookie).
- Embedding: a snippet from the focus item page, with the site's domain on the CORS allowlist; or shown by a Campaign
  action after a "Visits a Page" decision through the tracking code (the doc calls that path sometimes unreliable).

**Forms** ([docs](https://docs.mautic.org/en/5.x/components/forms.html)): campaign forms (start a campaign) and standalone
forms (run submit actions at once: change segments, tags, points, record UTM tags, remove from Do Not Contact, send an
email, push to an integration, POST to another form). Email is the unique contact identifier; an anonymous contact that
converts merges into the known one with its points. A single checkbox field carries consent, with a full statement as
label and a short stored value. Progressive profiling hides known fields or reveals fields after N submissions, and
does not work in static-HTML embeds. Embeds: JavaScript, iframe, or copied HTML. Kiosk mode creates a new contact per
submit and does not track the device.

## 6. OptinMonster and Sumo (BDOW!): form specialists

**OptinMonster.** Display rules, grouped into rulesets (a campaign shows when every condition in any one ruleset holds;
ALL/ANY ONE inside a group): "Time Delay" (time on page or site, not a trigger on its own), "Distance Scrolled", "URL
Path", "Number of Pages Viewed", "InactivitySensor", "Device Targeting", "Browser Targeting", "Exit-Intent",
"MonsterLink" (open on click), "Referral URL Targeting", "URL Query Argument", "URL Hash", "Cookie Targeting",
"JavaScript Variable", "Campaign Scheduling", "HTML Element", "New or Returning Visitor", "Geo-Location Targeting",
"AdBlock Targeting", and "MonsterChains" (visitor has / has not opted in to, closed, seen another campaign). Repeat
display after close or success is a cookie setting
([display rules](https://optinmonster.com/docs/how-to-use-display-rules-in-optinmonster/)).

Events in the page: a long lifecycle, notably `om.Campaign.show`, `om.Campaign.afterShow`, `om.Campaign.close`,
`om.Optin.init.submit`, `om.Optin.success`, `om.Optin.error`, and analytics `om.Analytics.impression`,
`om.Analytics.conversion`, `om.Analytics.track`; OptinMonster itself counts impressions and conversions
([JS events API](https://optinmonster.com/docs/optinmonster-javascript-events-api)).

Where the lead goes: OptinMonster does not send. A submit is passed to the connected email provider or a webhook whose
payload carries `lead` (`email`, `firstName`, `lastName`, `phone`, `ipAddress`, `referrer`, `timestamp`,
`privacyConsent`), `campaign` (`id`, `title`), `meta`, and `smart_tags` (`page_url`, `referrer_url`, `pages_visited`,
`time_on_site` …) ([webhook](https://optinmonster.com/docs/how-to-connect-optinmonster-with-a-webhook/)). The built-in
Privacy Checkbox passes no data with the lead, so it does not meet GDPR consent; a mapped custom checkbox field is
recommended instead ([privacy notice](https://optinmonster.com/docs/how-to-add-a-privacy-policy/)).

**Sumo / BDOW! List Builder.** Two trigger modes: Smart Mode (default, exit intent; navigation inside the site does not
count) and Manual Timer Mode (a delay after load), with a "not seen within" window in minutes to years
([smart and manual mode](https://help.bdow.com/en/articles/9239056-list-builder-smart-mode-and-manual-timer-mode)).

## 7. Practical constraints

- **Google.** Search Central: intrusive interstitials and dialogs obstruct the content and may lead to poor search
  performance; don't cover the whole page; don't redirect to a separate page for consent or input; use banners that take a
  small fraction of the screen; mandatory interstitials (e.g. an age gate) are exempt and should overlay the content
  ([avoid intrusive interstitials](https://developers.google.com/search/docs/appearance/avoid-intrusive-interstitials)).
  OptinMonster's own guidance follows from this: trigger on interaction (scroll, click, exit) rather than on page load
  ([vendor blog](https://optinmonster.com/the-new-google-mobile-friendly-rules-for-popups/)); its claims that the signal
  is mobile-only and exempts cookie dialogs are not on Google's current page.
- **Web push permission.** `Notification.requestPermission()` is HTTPS-only and should be called from a user interaction;
  cross-origin iframes may not ask ([MDN](https://developer.mozilla.org/en-US/docs/Web/API/Notification/requestPermission_static)).
  Since Firefox 72, `requestPermission()` and `PushManager.subscribe()` work only in response to a user gesture
  ([Firefox 72 notes](https://developer.mozilla.org/en-US/docs/Mozilla/Firefox/Releases/72)). Chrome treats prompts
  without a prior gesture as poor practice and shows quiet UI for blocked permissions
  ([Chrome permissions chip](https://developer.chrome.com/blog/permissions-chip)). A denied prompt cannot be asked again
  (Braze, Brevo (search index) above). So the soft prompt is an on-site message whose button click calls the browser API.
- **CSP.** A third-party on-site layer needs the customer's CSP to allow its script and API hosts, and inline styles or
  scripts for HTML messages unless a nonce is passed (Braze's list above is the worked example; Mautic needs a CORS
  allowlist instead).
- **Layout shift and flicker.** Inserting an element before existing content moves it and counts towards CLS (good is
  ≤ 0.1); shifts within 500 ms of user input are excluded ([web.dev CLS](https://web.dev/articles/cls)). The vendors avoid
  it for inline content by rendering into an element the site reserves: Customer.io inline replaces an empty element,
  Braze Banners fill a site-created placement (sections 1 and 2). Overlays (modal, slideup, flyout) do not shift layout.
  No primary source was found that measures "flicker" itself.
- **Accessibility.** A modal needs `role="dialog"`, `aria-modal="true"`, a label (`aria-labelledby` or `aria-label`), focus
  moved in on open, Tab kept inside, Escape to close, focus returned on close
  ([WAI-ARIA APG](https://www.w3.org/WAI/ARIA/apg/patterns/dialog-modal/)). WCAG 2.2 SC 2.4.11: a sticky banner fails if it
  entirely hides a focused component; non-modal overlays that neither take focus nor close on blur are at risk
  ([Understanding 2.4.11](https://www.w3.org/WAI/WCAG22/Understanding/focus-not-obscured-minimum.html)).
- **Mobile.** Exit intent has no cursor on touch devices: Klaviyo and OptinMonster use a fast upward scroll instead
  (sections 3 and 6). Braze full messages take the whole window on narrow screens
  ([in-app messages](https://www.braze.com/docs/developer_guide/in_app_messages/)); Klaviyo and OptinMonster target devices
  separately.

## 8. Comparison

| Product                | Role                                                  | Addressed to                                                     | How it reaches the page                                                   | Triggers                                                                     | Frequency                                                           | Capture and consent                                                                     | Events                                                                      |
| ---------------------- | ----------------------------------------------------- | ---------------------------------------------------------------- | ------------------------------------------------------------------------- | ---------------------------------------------------------------------------- | ------------------------------------------------------------------- | --------------------------------------------------------------------------------------- | --------------------------------------------------------------------------- |
| Customer.io in-app     | Channel (step in automation/broadcast) + forms inside | A person (first browser they visit); separate anonymous mode     | Snippet plugin; real-time for identified, polling otherwise               | Page rules; queue priority; no delay/scroll/exit documented                  | Once by default, or persistent until closed; anonymous: own setting | Forms/surveys as events; Lead Form creates a person but leaves the browser anonymous    | opened, dismissed, action, error, changed                                   |
| Braze in-app / Banners | Channel + capture templates                           | The SDK user (anonymous or identified)                           | Synced at session start, triggered locally; Banners fetched per placement | Session start, purchase, custom event, push click; no server triggers on web | 30 s min between triggers; exempt from global caps                  | Email form (global subscription), phone form (one subscription group); DOI via a Canvas | impression, click, button click, HTML click                                 |
| Klaviyo sign-up forms  | Capture                                               | Visitors filtered by profile status, list/segment, URL, UTM, geo | Onsite JS                                                                 | Immediately, delay, scroll %, pages seen, exit intent, custom trigger        | Re-show N days after close; never after submit                      | Email DOI per list; SMS consent with one-time code; per-channel consent status          | `klaviyoForms`: open, embedOpen, close, redirectedToUrl, submit, stepSubmit |
| Brevo (search index)   | Capture forms + pop-up campaigns + push prompts       | Website visitors; pop-up campaigns by segment                    | Brevo/WonderPush script                                                   | Delay, exit intent; WonderPush: open, event, URL, exit intent                | Visit frequency; WonderPush once per user by default                | DOI only with an email field; push prompt bell/native/HTML/link                         | not confirmed                                                               |
| Mautic                 | Capture (focus item shows a Form) + campaign action   | Tracked contacts (anonymous or known)                            | Focus snippet (CORS allowlist) or tracking code                           | Arrival, scroll (slight/middle/bottom), exit intent, + timeout               | Every page … once a day; stop after conversion / close              | Consent checkbox field; progressive profiling; email merges anonymous contact           | conversions, link clicks, form submissions                                  |
| OptinMonster           | Capture only (no sending)                             | Visitors by rules incl. cookies, referrer, geo, prior campaigns  | Embed script                                                              | Time, scroll, pages, inactivity, exit intent, click, cookie, JS variable     | Cookie after close / success                                        | Lead to ESP or webhook; privacy checkbox passes no data, map a consent field            | `om.*` lifecycle; impression, conversion                                    |
| Sumo / BDOW!           | Capture                                               | Visitors by page rules                                           | Site script                                                               | Exit intent (Smart) or timer (Manual)                                        | "Not seen within" window                                            | not researched                                                                          | not researched                                                              |

## 9. Recurring patterns

1. **Two roles, one runtime.** Where a vendor sends (Customer.io, Braze, WonderPush), the on-site message is a channel:
   it is a step of an automation or broadcast, addressed to a person, with sent/shown/clicked/dismissed reporting. Where a
   vendor only collects (OptinMonster, Sumo, Klaviyo forms, Mautic focus items), the display decision is made in the
   browser from page, time, scroll, exit and cookie rules, and the product is the submit. The engines put capture
   templates inside the message unit rather than shipping a second object.
2. **Anonymous traffic is a separate mode.** Customer.io's anonymous messages exclude identified people and are aimed by
   page rules; capture tools target "not yet a profile" by default (Klaviyo "don't show to existing Klaviyo profiles").
   A send to a known person and a capture prompt to unknown visitors are different audiences, not one message.
3. **The browser decides when; the server decides whom.** Even channel products evaluate triggers in the browser (Braze:
   synced at session start, triggered locally, no server-side triggers on web; Customer.io: page rules on the client).
   Server work is selecting the eligible set; only Braze Banners re-evaluate segment and Liquid on each fetch.
4. **Submit does not always mean Identify.** Customer.io's Lead Form creates a person yet leaves the browser anonymous;
   Mautic merges the anonymous contact into the known one by email; Klaviyo ties form events to a profile only when one
   is known. A form that should identify must say so explicitly.
5. **Consent is per channel, and a form records it per field.** Braze: one phone subscription group per form; Klaviyo:
   SMS consent separate from email, confirmed by one-time code; Brevo (search index) and Klaviyo: email DOI as a list or
   form setting; OptinMonster: a decorative privacy checkbox carries no consent, a mapped field does. Double opt-in is
   always a follow-up email or code, not part of the popup.
6. **The web push permission prompt is an on-site message.** Every engine with web push ships a soft prompt (Braze in-app
   with `push-primer`, WonderPush HTML dialog/bell/switch) whose click calls the browser API, because browsers require a
   user gesture and a denial is final.
7. **Frequency is local and per message.** Cookies or SDK state (Mautic, OptinMonster, Klaviyo, Sumo, Braze's 30 s
   interval); Braze exempts in-app messages from global caps. No vendor read applies a cross-channel cap to on-site
   messages.
8. **Inline needs a slot.** Banners and inline messages render into an element the site provides (Braze placements,
   Customer.io inline, Klaviyo embed, Mautic embed); overlays need nothing but bring the interstitial and focus rules.

## 10. Unconfirmed

- **Brevo, all of it**: help.brevo.com refused automated reads; section 4 comes from the search index.
- Braze: whether dismissals are logged on web; what an email or phone capture submit does to an anonymous SDK user
  (merge, new profile); the Banner analytics events (page not read).
- Customer.io: the exact event name of a form submission; any consent field handling in forms; the two polling
  descriptions (10/60/180 s vs about once a minute) are not reconciled; any CSP guidance.
- Klaviyo: CSP hosts and directives; a programmatic open API beyond the "custom trigger" option; whether form events
  can start a flow directly (only list-triggered welcome flows were confirmed).
- Mautic: native double opt-in (the forms page does not mention one); view counts for focus items.
- WonderPush: popup analytics (not on the pages read).
- OptinMonster: mobile exit-intent sensitivity; whether Google's guidance is mobile-only (only in a 2017 vendor post).
- Sumo/BDOW!: fields, consent and events; display rules in Smart Mode (two help articles disagree, per search index).
- Chrome's criteria for the quieter notification UI (the 2020 Chromium blog post body was not readable).
