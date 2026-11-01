# WhatsApp Business Platform: templates, the 24-hour window and access paths

Researched 2026-11-01 from primary sources only: Meta's WhatsApp Business Platform developer documentation (read as
the Markdown that `developers.facebook.com/documentation/business-messaging/whatsapp/<page>.md` serves, cited below by
the page URL without `.md`), the WhatsApp Business Messaging Policy, and the BSPs' own docs (Twilio, 360dialog,
Gupshup, Wazzup). No blog or comparison site is cited. A claim no primary source confirmed is under "Unconfirmed" at
the end. Base URL for Meta pages: `https://developers.facebook.com/documentation/business-messaging/whatsapp/`,
abbreviated `META/` below (for example `META/throughput` is
[this page](https://developers.facebook.com/documentation/business-messaging/whatsapp/throughput)). sphericon terms
are used as in [GLOSSARY.md](../../GLOSSARY.md) (Integration, Destination, Suppression, Ingest hook, Preset, Delivery
notice, Provider limit, Message content, Template) and the ADRs the map reopens:
[0037](../adr/0037-inbound-provider-callbacks-are-presets-on-an-integration.md),
[0053](../adr/0053-delivery-and-engagement-events-cover-every-send.md),
[0067](../adr/0067-the-send-cap-is-the-platforms-limit-the-provider-limit-the-customers.md).

Meta is renaming its account model: the WhatsApp Business Account (WABA) is splitting into a **WhatsApp account**
(phone numbers, business identity) and a **Messaging account** (templates, billing, webhook subscriptions); existing
`waba_id` values now denote Messaging account IDs
([META/embedded-signup/overview](https://developers.facebook.com/documentation/business-messaging/whatsapp/embedded-signup/overview)).
This note says "WABA" where the API still does.

## Question

What does business-initiated WhatsApp messaging require from a platform that sends on behalf of its customers? The
customer connects their own WhatsApp Business account or BSP as an Integration; sphericon sends one-way on their
behalf from Broadcasts and Automation steps; consent and opt-out are keyed by (channel, destination). Cover access
paths, templates, the 24-hour window, opt-in and opt-out, status webhooks, the error for a number not on WhatsApp,
per-user marketing limits and throughput tiers.

## Summary of findings

- **Confirmed: outside the 24-hour window only a pre-approved template can be sent.** The window opens when the user
  messages or calls the business and resets with each new message; "When the window closes, you can only send
  pre-approved template messages"
  ([META/messages/send-messages](https://developers.facebook.com/documentation/business-messaging/whatsapp/messages/send-messages)).
  sphericon sends one-way, so in practice every Broadcast and Automation send is a template message.
- **Templates can be created, edited, listed and deleted by API on Cloud API**, and are reviewed (automated plus
  manual, up to 24 hours) before they can be sent; status changes arrive by webhook. Variables are **positional by
  default** (`{{1}}`, `{{2}}`), and **named** (`{{first_name}}`) is an opt-in `parameter_format`. Media headers
  (image, video, document; GIF on the Marketing Messages API only) and up to 10 buttons (URL, quick reply, phone,
  copy code, ...) are defined in the template; the send supplies only the parameter values and the header media.
- **A template is not stable after approval.** Meta can recategorize it (utility to marketing, without notice for
  businesses it has warned), pace it, pause it (3 h, then 6 h), disable it on the third pause, and archive it after
  12 months unused.
- **Marketing is moving off Cloud API.** Marketing templates on Cloud API `/messages` sunset on 2027-10-13; Messaging
  accounts created from 2027-04-13 cannot send marketing there at all. Marketing goes to
  `/<PHONE_NUMBER_ID>/marketing_messages` (Marketing Messages API for WhatsApp), routed by each template's latest
  category.
- **Opt-in is the business's job and may be gathered off WhatsApp** (website, SMS, paper), but must name the business
  and say the person opts in to its messages. Opt-out has two sources: the business must honour any opt-out request
  "either on or off WhatsApp" (a quick-reply "Unsubscribe" is the documented pattern), and WhatsApp itself lets the
  user stop **marketing** messages from a business (webhook `user_preferences` with `stop`/`resume`, send error
  `131050`). That user-side stop does not cover utility messages.
- **Status webhooks:** `sent`, `delivered`, `read`, `failed` (and `played` for voice); `delivered` is skipped when a
  message is read at once. Meta retries webhooks for up to 7 days, so duplicates happen.
- **There is no "not on WhatsApp" error of its own.** `131026` "Unable to deliver message" covers a number that is not
  on WhatsApp, a user who has not accepted the new terms, and an outdated client. Cloud API documents no lookup
  endpoint to check a number before sending.
- **Per-user marketing limit:** WhatsApp silently caps marketing templates per user by engagement (`131049`, retry no
  sooner than 24 h, rapid retries are penalized), and currently delivers **no marketing templates to US (+1) numbers**.
- **Two limits, both readable by API:** throughput per phone number (80 mps default, 1,000 mps auto-upgrade, 20 mps
  for numbers shared with the WhatsApp Business app; error `130429`), and a **messaging limit per business
  portfolio** in _unique users reached outside a window per rolling 24 h_ (250, 2,000, 10,000, 100,000, unlimited),
  shared by every number in the portfolio.
- **Recipients may soon have no phone number.** WhatsApp usernames are rolling out; webhooks carry a business-scoped
  user ID (BSUID) always and the phone number only in some cases. BSUIDs are per business portfolio and regenerate
  when the user changes number.
- **Access paths:** Cloud API directly (the customer's own Meta app and a system-user token), Embedded Signup (needs
  the operator to be a Meta Tech Provider: business verification, App Review, 10 then 200 new customers a week), or a
  BSP. 360dialog and Gupshup's partner API pass Meta's JSON through; Twilio and Wazzup use their own formats; Wazzup
  and Gupshup's passthrough cannot create templates by API.

## 1. The 24-hour customer service window

- "When a WhatsApp user messages you or calls you, a 24-hour timer called a customer service window starts. If the
  user messages or calls you again before the timer expires, the timer resets to 24 hours." While it is open, free-form
  "service messages" (text, media, interactive, ...) need no approval; "When the window closes, you can only send
  pre-approved template messages"
  ([META/messages/send-messages](https://developers.facebook.com/documentation/business-messaging/whatsapp/messages/send-messages)).
- "Template messages are the only type of message that can be sent to WhatsApp users outside of a customer service
  window" ([META/templates/overview](https://developers.facebook.com/documentation/business-messaging/whatsapp/templates/overview)).
- A free-form send outside the window fails with `131047` "More than 24 hours have passed since the recipient last
  replied to the sender number" — "Send the recipient a template message instead"
  ([META/support/error-codes](https://developers.facebook.com/documentation/business-messaging/whatsapp/support/error-codes)).
  Twilio's equivalent is `63016` "Outside messaging window. For WhatsApp, use a Message Template instead"
  ([Twilio 63016](https://www.twilio.com/docs/api/errors/63016)).
- Separately, a "free entry point" window of up to 7 days opens when a user writes from a Click to WhatsApp ad and the
  business answers within the window ([META/pricing](https://developers.facebook.com/documentation/business-messaging/whatsapp/pricing)).
  It concerns ads, not one-way sending.

## 2. Message templates

### Creation and review

- Templates are Messaging-account assets, created with `POST /<WABA_ID>/message_templates` (fields `name`, `category`,
  `language`, `parameter_format`, `components`) or in WhatsApp Manager; at most 100 creations per Messaging account per
  hour. Names are lowercase alphanumerics and underscores, up to 512 characters, and not unique: one name can exist
  in several languages, each counting against the limit. "Template strings and variables are not translated by Meta"
  ([META/templates/overview](https://developers.facebook.com/documentation/business-messaging/whatsapp/templates/overview)).
- Template count limit: 250 per Messaging account when the business portfolio is unverified, up to 6,000 when it is
  verified and has an approved display name (same page).
- Review: "Templates are automatically reviewed upon creation or after editing"; only `APPROVED` templates can be
  sent. Review "can take up to 24 hours"
  ([META/templates/overview](https://developers.facebook.com/documentation/business-messaging/whatsapp/templates/overview)).
  Review is "a combination of automated systems and manual reviews"; samples for every variable (text and media) are
  required; common rejections are mismatched or non-sequential parameters, a template that starts or ends with a
  parameter, too many parameters for its length, and a duplicate body and footer of an existing template
  ([META/templates/template-review](https://developers.facebook.com/documentation/business-messaging/whatsapp/templates/template-review)).
- The outcome arrives as a `message_template_status_update` webhook with `event` (`APPROVED`, `REJECTED`, `PAUSED`,
  `DISABLED`, `ARCHIVED`, ...) and, on rejection, a `reason` (`INVALID_FORMAT`, `INCORRECT_CATEGORY`,
  `ABUSIVE_CONTENT`, ...) plus a recommendation text
  ([META/webhooks/reference/message_template_status_update](https://developers.facebook.com/documentation/business-messaging/whatsapp/webhooks/reference/message_template_status_update)).
  The `status` field can also be polled (`GET /<TEMPLATE_ID>?fields=status`).
- Editing: only `APPROVED`, `REJECTED` or `PAUSED` templates; the edit replaces all components; the category of an
  approved template cannot be edited; an approved template can be edited up to 10 times in 30 days or once in 24 h,
  and goes through review again. Deleting an approved template blocks its name for 30 days
  ([META/templates/template-management](https://developers.facebook.com/documentation/business-messaging/whatsapp/templates/template-management)).

### Categories

- Every template is `MARKETING`, `UTILITY` or `AUTHENTICATION`. Utility must be non-promotional **and** either specific
  to or requested by the user, or essential to them; mixed content and unclear content (a body of only `{{1}}`) are
  marketing. Opt-in and opt-out confirmations on WhatsApp are listed as utility
  ([META/templates/template-categorization](https://developers.facebook.com/documentation/business-messaging/whatsapp/templates/template-categorization)).
- Since 2025-04-09, a template submitted as utility that WhatsApp judges marketing "is approved as `MARKETING`". After
  approval, a recurring process moves utility templates to marketing with 1 day's notice, and with no notice once the
  business has been warned for misuse; repeated misuse caps utility volume and then recategorizes all utility
  templates of the account or portfolio (same page).

### Variables

- "If you do not specify a format, the template uses `positional` format by default." Positional parameters are
  `{{1}}`, `{{2}}`, ... and values are sent in that order; named parameters are lowercase-and-underscore names such
  as `{{first_name}}`, matched by `parameter_name` in any order. An example value is required for each parameter
  ([META/templates/overview](https://developers.facebook.com/documentation/business-messaging/whatsapp/templates/overview)).
- Where parameters may go
  ([META/templates/components](https://developers.facebook.com/documentation/business-messaging/whatsapp/templates/components)):
  text header, 60 characters, 1 parameter; body, 1,024 characters, several parameters; footer, 60 characters, no
  parameter; URL button, one parameter appended to the end of the URL (2,000 characters), which must be
  percent-encoded.
- BSPs: Twilio's Content API examples use numeric keys only (`{{1}}`, `ContentVariables` `{"1": "..."}`)
  ([Twilio Content API](https://www.twilio.com/docs/content/content-api-resources)); Wazzup sends `templateValues` as
  an array of strings ([Wazzup send](https://wazzup24.ru/help/api/otpravka-soobshhenij)). Positional is the one format
  every path accepts.

### Media and buttons

- Media header: `IMAGE`, `VIDEO`, `GIF` or `DOCUMENT`; at creation an example asset is uploaded with the Resumable
  Upload API and reviewed; GIFs (MP4 up to 3.5 MB) only on the Marketing Messages API
  ([META/templates/components](https://developers.facebook.com/documentation/business-messaging/whatsapp/templates/components)).
  At send time the header media is a parameter: a media `id` uploaded to Meta or a public `link`
  ([META/templates/template-media](https://developers.facebook.com/documentation/business-messaging/whatsapp/templates/template-media)).
  A linked asset is cached by Cloud API for 10 minutes
  ([META/messages/send-messages](https://developers.facebook.com/documentation/business-messaging/whatsapp/messages/send-messages)).
- Buttons: up to 10 in total; at most 2 URL, 1 phone number, 1 copy code (20 characters), up to 10 quick reply (label
  25 characters); quick replies must be grouped apart from the other buttons; more than 3 buttons collapse behind
  "See all options"; 4 or more buttons, or a quick reply mixed with other types, cannot be viewed on WhatsApp desktop.
  "A common use case [of a quick reply] is a button that allows your customer to easily opt-out of any marketing
  messages"
  ([META/templates/components](https://developers.facebook.com/documentation/business-messaging/whatsapp/templates/components)).
  A tapped quick reply arrives as an inbound `button` message with the label as `payload` and a `context` naming the
  template message
  ([META/webhooks/reference/messages/button](https://developers.facebook.com/documentation/business-messaging/whatsapp/webhooks/reference/messages/button)).
- Marketing specialties (carousels of up to 10 cards, coupon code, limited-time offer, catalog/product templates) exist
  ([META/templates/marketing-templates](https://developers.facebook.com/documentation/business-messaging/whatsapp/templates/marketing-templates)).

### Life after approval

- **Pacing** (marketing, and utility after a utility pause): new or not-`GREEN` templates send normally up to a
  threshold, then further messages are held (`message_status: held_for_quality_assessment` in the send response)
  until feedback arrives; bad feedback pauses the template and drops the held messages with `failed`/`132015`. Meta
  aims to release held messages within an hour (p99)
  ([META/templates/template-pacing](https://developers.facebook.com/documentation/business-messaging/whatsapp/templates/template-pacing)).
- **Pausing**: at `RED` quality a template is paused 3 h the first time, 6 h the second, and disabled the third
  ([META/templates/template-pausing](https://developers.facebook.com/documentation/business-messaging/whatsapp/templates/template-pausing));
  sends fail with `132015` (paused) and `132016` (disabled)
  ([META/support/error-codes](https://developers.facebook.com/documentation/business-messaging/whatsapp/support/error-codes)).
- **Archival**: templates unused for 12 months are archived, then deleted after 28 days unless unarchived
  ([META/templates/overview](https://developers.facebook.com/documentation/business-messaging/whatsapp/templates/overview)).
- **TTL**: undelivered messages are retried for 30 days by default; a custom TTL is allowed for utility (30 s to
  12 h) and for marketing on the Marketing Messages API (12 h to 30 days)
  ([META/templates/time-to-live](https://developers.facebook.com/documentation/business-messaging/whatsapp/templates/time-to-live)).

### Marketing Messages API for WhatsApp

- "Cloud API will stop supporting marketing messages on **October 13, 2027**"; "Any new Messaging account created on
  or after [April 13, 2027] will not be able to send marketing messages via Cloud API". Service, authentication and
  utility stay on Cloud API
  ([META/marketing-messages/overview](https://developers.facebook.com/documentation/business-messaging/whatsapp/marketing-messages/overview)).
- `POST /<PHONE_NUMBER_ID>/marketing_messages` accepts only marketing templates; it has a `product_policy` of
  `CLOUD_API_FALLBACK` for accounts not yet onboarded; once onboarded, `disable_marketing_messages_on_cloud_api` makes
  `/messages` reject marketing templates with `131063`
  ([META/marketing-messages/send-marketing-messages](https://developers.facebook.com/documentation/business-messaging/whatsapp/marketing-messages/send-marketing-messages)).
- Meta's routing guidance: keep a mapping of (Messaging account, template name, language) to the latest category,
  update it from the template category update webhook, send `MARKETING` to `/marketing_messages` and everything else
  to `/messages`
  ([META/marketing-messages/route-template-messages](https://developers.facebook.com/documentation/business-messaging/whatsapp/marketing-messages/route-template-messages)).
- Onboarding is per Messaging account; eligibility is read from the `marketing_messages_onboarding_status` field
  ([META/marketing-messages/onboarding](https://developers.facebook.com/documentation/business-messaging/whatsapp/marketing-messages/onboarding)).

## 3. Opt-in and opt-out

- Policy: "You may only contact people on WhatsApp if: (a) they have given you their mobile phone number or username;
  and" they have opted in; "You must respect all requests (either on or off WhatsApp) by a person to block,
  discontinue, or otherwise opt out" of communications (WhatsApp Business Messaging Policy, last updated 2026-09-23,
  [whatsappbusiness.com/policy](https://whatsappbusiness.com/policy/)).
- Opt-in "can be general and not specifically for WhatsApp, as long as businesses comply with all local laws". It must
  "clearly state that a person is opting in to receive communication from the business" and "clearly state the
  business's name". Supported methods include SMS, website, IVR, and in person or on paper. Meta suggests separate
  opt-ins per message category and clear opt-out instructions
  ([META/getting-opt-in](https://developers.facebook.com/documentation/business-messaging/whatsapp/getting-opt-in)).
- **There is no platform STOP keyword.** Meta documents no keyword the platform acts on; the business reads inbound
  messages (text, or a quick-reply `button` payload) and honours them itself (sources above).
- **WhatsApp's own marketing stop.** The "Offers and announcements" setting lets a user stop or resume marketing
  templates from one business. A marketing template sent after a stop is accepted by the API but not delivered:
  status `failed`, code `131050`. The `user_preferences` webhook reports `value: stop` or `resume` with
  `category: marketing_messages`; "Interested / Not interested" feedback does not trigger it
  ([META/templates/marketing-templates](https://developers.facebook.com/documentation/business-messaging/whatsapp/templates/marketing-templates),
  [META/webhooks/reference/user_preferences](https://developers.facebook.com/documentation/business-messaging/whatsapp/webhooks/reference/user_preferences)).
  `131050`: "Do not retry sending messages to this user"
  ([META/support/error-codes](https://developers.facebook.com/documentation/business-messaging/whatsapp/support/error-codes)).
- Users can also block or report a business; message quality is computed from "blocks, reports, mutes, archives" over
  7 days ([META/messages/send-messages](https://developers.facebook.com/documentation/business-messaging/whatsapp/messages/send-messages)).
  The business can block a user only within 24 hours of the user's last message; sends to a user the business blocked
  fail with `130403`
  ([META/block-users](https://developers.facebook.com/documentation/business-messaging/whatsapp/block-users),
  [META/support/error-codes](https://developers.facebook.com/documentation/business-messaging/whatsapp/support/error-codes)).

## 4. Sending, status webhooks and errors

### The send call and its identifiers

- `POST /<PHONE_NUMBER_ID>/messages` with `type: template`, the template `name`, `language.code` and `components`
  (parameter values). The response only means "accepted": it returns `contacts[].wa_id` and `messages[].id` (the
  `wamid`) that later status webhooks carry
  ([META/messages/send-messages](https://developers.facebook.com/documentation/business-messaging/whatsapp/messages/send-messages)).
- Phone format: "If the plus sign is omitted, your business phone number's country calling code is prepended to the
  customer's phone number" (same page).
- `biz_opaque_callback_data`, set on the send, comes back on every status webhook
  ([META/webhooks/reference/messages/status](https://developers.facebook.com/documentation/business-messaging/whatsapp/webhooks/reference/messages/status)).

### Status webhooks

- Statuses: `sent` (one check), `delivered` (two checks), `read`, `failed`, `played` (voice). "In some cases, like when
  a user receives a message while in the chat screen, the message is both delivered and read at the same time. In
  these cases, the 'delivered' webhook is not sent." Failures carry `errors[].code`, `title`, `error_data.details`
  ([META/webhooks/reference/messages/status](https://developers.facebook.com/documentation/business-messaging/whatsapp/webhooks/reference/messages/status)).
- If no `delivered` arrives before the TTL ends, "assume the message was dropped"
  ([META/messages/send-messages](https://developers.facebook.com/documentation/business-messaging/whatsapp/messages/send-messages)).
- Errors arrive "either synchronously as a Graph API response, asynchronously via Webhook, or sometimes through both";
  Meta advises building on `code` and `details`, not titles
  ([META/support/error-codes](https://developers.facebook.com/documentation/business-messaging/whatsapp/support/error-codes)).
- Transport: the app's callback URL is verified with a GET carrying `hub.mode`, `hub.challenge` and
  `hub.verify_token`; each POST carries `X-Hub-Signature-256`, an HMAC-SHA256 of the body keyed with the **app secret**
  ([META/webhooks/create-webhook-endpoint](https://developers.facebook.com/documentation/business-messaging/whatsapp/webhooks/create-webhook-endpoint)).
  Meta retries non-200 deliveries "for up to 7 days", which "can result in duplicate webhook notifications"
  ([META/webhooks/overview](https://developers.facebook.com/documentation/business-messaging/whatsapp/webhooks/overview)).
  A partner can set an alternate callback URL per Messaging account or phone number (`override_callback_uri`)
  ([META/webhooks/override](https://developers.facebook.com/documentation/business-messaging/whatsapp/webhooks/override)).
  Endpoints should take 3x the outbound message rate in status webhooks
  ([META/throughput](https://developers.facebook.com/documentation/business-messaging/whatsapp/throughput)).
- Inbound messages arrive on the same `messages` field with a `messages` array (sender `wa_id`, `type`, body)
  ([META/webhooks/reference/messages](https://developers.facebook.com/documentation/business-messaging/whatsapp/webhooks/reference/messages)).

### Error codes that matter for the send model

All from [META/support/error-codes](https://developers.facebook.com/documentation/business-messaging/whatsapp/support/error-codes)
unless noted. The last column is a reading for sphericon, not Meta's wording.

| Code                                       | Meta's meaning                                                                                                                                                                                                 | Reading for sphericon                                |
| ------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------- |
| `131026`                                   | "Unable to deliver message. Reasons can include: The recipient phone number is not a WhatsApp phone number; ... has not accepted the new Terms of Service ...; ... old WhatsApp version"                       | Undeliverable, not proof the number is dead          |
| `131050`                                   | Recipient "has chosen to stop receiving marketing messages ... from your business"; "Do not retry"                                                                                                             | Marketing-only stop on this channel; user can resume |
| `131049`                                   | "not delivered to maintain healthy ecosystem engagement" (per-user marketing limit); wait at least 24 h                                                                                                        | Defer this recipient 24 h or more; not a suppression |
| `131047`                                   | More than 24 hours since the user last replied                                                                                                                                                                 | Free-form outside the window: a content error        |
| `130403`                                   | The business has blocked this user                                                                                                                                                                             | Do not retry until unblocked                         |
| `130429`                                   | Throughput reached                                                                                                                                                                                             | Defer; slow down                                     |
| `131056`                                   | Too many messages to the same recipient in a short time                                                                                                                                                        | Defer this recipient                                 |
| `131048`                                   | Restrictions on the number because too many messages were blocked or flagged as spam                                                                                                                           | Integration-level problem                            |
| `132001`                                   | Template does not exist in that language or is not approved                                                                                                                                                    | Template unavailable                                 |
| `132000`                                   | Parameter count does not match the template                                                                                                                                                                    | Content error                                        |
| `132015`                                   | Template paused for low quality                                                                                                                                                                                | Template unavailable for now                         |
| `132016`                                   | Template paused too many times, permanently disabled                                                                                                                                                           | Template unavailable for good                        |
| `131063`                                   | Marketing template sent to `/messages` after Cloud API marketing was disabled ([source](https://developers.facebook.com/documentation/business-messaging/whatsapp/marketing-messages/route-template-messages)) | Route to `/marketing_messages`                       |
| `131062`                                   | BSUID recipient not supported for this message ([source](https://developers.facebook.com/documentation/business-messaging/whatsapp/business-scoped-user-ids))                                                  | Needs the phone number                               |
| `190`, `131005`, `131031`, `368`, `131042` | Token expired, permission removed, account restricted or disabled, payment method problem                                                                                                                      | Integration cannot send                              |

- **Not on WhatsApp**: `131026` is the only signal, and it also covers terms not accepted and old clients. Cloud API's
  documentation index lists no endpoint that checks whether a number is on WhatsApp before sending
  ([index](https://developers.facebook.com/documentation/business-messaging/whatsapp/llms.txt)). Twilio maps the case
  to `63003` "Channel could not find To address", whose causes include "an unregistered WhatsApp number" and an
  inactive user ([Twilio 63003](https://www.twilio.com/docs/api/errors/63003)); Wazzup reports `failed` with reason
  `bad_contact` ([Wazzup webhooks](https://wazzup24.ru/help/api/webhooks/)).

## 5. Limits: per-user marketing, messaging limit, throughput, quality

- **Per-user marketing limit**: WhatsApp "may limit the number of marketing template messages a WhatsApp user receives
  from any business", adapting to the user's recent read rate and inbox activity. Marketing messages sent inside an
  open window do not count. Resending within 24 h to a user at the limit can block delivery to that user for up to
  24 h. **"WhatsApp does not currently deliver marketing template messages to WhatsApp users with United States phone
  numbers"**; sending one "will result in an error"
  ([META/templates/marketing-templates/per-user-limits](https://developers.facebook.com/documentation/business-messaging/whatsapp/templates/marketing-templates/per-user-limits)).
- **Messaging limit**: "the maximum number of unique WhatsApp user phone numbers your business can deliver messages
  to, outside of a customer service window, within a moving 24-hour period", set "at the business portfolio level and
  ... shared by all business phone numbers within a portfolio". New portfolios start at 250; business verification
  (or 2,000 high-quality deliveries in 30 days) unlocks 2,000; then automatic scaling to 10,000, 100,000 and unlimited
  within 6 hours once half the limit was used in 7 days with high quality. Read it with
  `GET /<PHONE_NUMBER_ID>?fields=whatsapp_business_manager_messaging_limit` (for example `TIER_250`); increases arrive
  as `business_capability_update` webhooks
  ([META/messaging-limits](https://developers.facebook.com/documentation/business-messaging/whatsapp/messaging-limits)).
  Since 2025-10-07 a falling quality rating no longer lowers the limit
  ([META/upcoming-messaging-limits-changes](https://developers.facebook.com/documentation/business-messaging/whatsapp/upcoming-messaging-limits-changes)).
- **Throughput**: per registered phone number, "up to 80 messages per second (mps) by default, and up to 1,000 mps by
  automatic upgrade" (unlimited messaging limit, 100K+ unique users in 24 h, quality at least `YELLOW`). Throughput
  counts inbound and outbound. Numbers shared with the WhatsApp Business app (coexistence) are fixed at 20 mps. Over
  the rate the API returns `130429`; read it with `GET /<PHONE_NUMBER_ID>?fields=throughput`
  ([META/throughput](https://developers.facebook.com/documentation/business-messaging/whatsapp/throughput)).
- **Message quality**: computed from user feedback over the past 7 days; low quality for a sustained period leads to
  rate limiting ([META/messages/send-messages](https://developers.facebook.com/documentation/business-messaging/whatsapp/messages/send-messages),
  [META/getting-opt-in](https://developers.facebook.com/documentation/business-messaging/whatsapp/getting-opt-in)).

## 6. Recipient identity: phone numbers, usernames and BSUIDs

From [META/business-scoped-user-ids](https://developers.facebook.com/documentation/business-messaging/whatsapp/business-scoped-user-ids):

- WhatsApp is rolling out optional usernames in 2026. Every webhook carries a **BSUID** (`user_id`, for example
  `US.13491208655302741918`); "If a WhatsApp user enables the username feature, their phone number will not be
  included in webhooks", unless the business number exchanged messages with them in the last 30 days or the user is in
  the portfolio's Meta-hosted contact book. Supporting BSUIDs "is required for all partners and directly-integrated
  businesses".
- BSUIDs are "unique to each business portfolio-user pair" and are "regenerated if a user changes their phone number"
  (reported as a `user_changed_number` system message carrying the previous ID). Sends accept `to` (phone), `recipient`
  (BSUID) or both, phone taking precedence; one-tap, zero-tap and copy-code authentication templates need the phone
  number.
- The `failed` status omits the `contacts` block, and omits `recipient_user_id` when the message was sent to a phone
  number.

## 7. Access paths and credentials

### Cloud API, the customer's own Meta app

- A direct developer uses a **system user access token** (long-lived, generated in Business settings with
  `whatsapp_business_management` and `whatsapp_business_messaging`)
  ([META/access-tokens](https://developers.facebook.com/documentation/business-messaging/whatsapp/access-tokens)).
  To send and receive, sphericon would hold: the token, the WABA (Messaging account) ID, the phone number ID, and, for
  webhooks, the app secret (signature) and a verify token, with the callback URL set on the customer's app (see
  "Status webhooks" above).

### Embedded Signup as a Tech Provider (the hosted operator's path)

- Partners are Solution Partners (credit line, bill clients), Tech Providers (clients add their own payment method and
  Meta bills them) and Tech Partners
  ([META/solution-providers/overview](https://developers.facebook.com/documentation/business-messaging/whatsapp/solution-providers/overview)).
- Becoming a Tech Provider needs a Meta app, **business verification** and **App Review** with videos of sending a
  message and creating a template, for Advanced access to `whatsapp_business_messaging` and
  `whatsapp_business_management`
  ([META/solution-providers/get-started-for-tech-providers](https://developers.facebook.com/documentation/business-messaging/whatsapp/solution-providers/get-started-for-tech-providers)).
- Embedded Signup (Facebook Login for Business plus the JS SDK) lets the customer create or pick a portfolio, WhatsApp
  account and phone number, accept the terms, and returns `waba_id`, `phone_number_id` and a code the partner exchanges
  for a customer-scoped **business token**; the partner then registers the number and subscribes its app to the
  Messaging account's webhooks. Customers keep ownership and full WhatsApp Manager access. Default onboarding cap: 10
  new customers per rolling 7 days, 200 after verification, App Review and Access Verification. Versions 2 and 3 were
  deprecated on 2026-10-15 in favour of v4
  ([META/embedded-signup/overview](https://developers.facebook.com/documentation/business-messaging/whatsapp/embedded-signup/overview)).
- Coexistence: a number already in the WhatsApp Business app can be onboarded and used by both (same page), at a fixed
  20 mps ([META/throughput](https://developers.facebook.com/documentation/business-messaging/whatsapp/throughput)).

### BSPs

| BSP               | API shape                                                                                                                                                                                     | Templates by API                                                                                                                                                                                                             | Status / inbound webhooks                                                                                                                                                                                         |
| ----------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 360dialog         | Meta's Cloud API JSON at `https://waba-v2.360dialog.io/messages`, `D360-API-KEY` header per number ([messaging](https://docs.360dialog.com/docs/messaging/overview))                          | Yes: `/message_templates` create, list, edit, delete ([templates API](https://docs.360dialog.com/docs/messaging-api/api-reference/templates))                                                                                | WABA- or number-level URL with optional custom headers ([webhooks API](https://docs.360dialog.com/docs/messaging-api/api-reference/webhooks))                                                                     |
| Twilio            | Own Messages API, `whatsapp:+E164` addresses; one WABA per Twilio account; ISVs go through Twilio's Tech Provider program ([Twilio WhatsApp](https://www.twilio.com/docs/whatsapp/api))       | Yes: Content API, then `POST /v1/Content/{sid}/ApprovalRequests/WhatsApp` with `name` and `category`; send with `ContentSid` + `ContentVariables` ([Content API](https://www.twilio.com/docs/content/content-api-resources)) | `StatusCallback`, signed per Twilio's webhook security; read receipts come as `EventType: READ` ([status guide](https://www.twilio.com/docs/messaging/guides/track-outbound-message-status))                      |
| Gupshup (partner) | "raw passthrough APIs which will imitate Meta's format", `POST /partner/app/{APP_ID}/v3/message` ([passthrough](https://partner-docs.gupshup.io/docs/whatsapp-passthrough-apis-for-partners)) | Not in Meta format through the passthrough (same page)                                                                                                                                                                       | Meta-format v3 callbacks plus `gs_app_id` (same page)                                                                                                                                                             |
| Wazzup (RU)       | Own API, `POST https://api.wazzup24.com/v3/message` with `templateId` and `templateValues` (array) ([send](https://wazzup24.ru/help/api/otpravka-soobshhenij))                                | List only (`GET /v3/templates/whatsapp`); created in Wazzup's UI ([templates](https://wazzup24.ru/help/api-ru/shablony-whatsapp-business-api))                                                                               | `message.status_update`: `accepted`, `sent`, `delivered`, `read`, `failed` with a `reason` such as `bad_contact`; Authorization header set by the subscriber ([webhooks](https://wazzup24.ru/help/api/webhooks/)) |

## What this means for sphericon

These are readings of the facts above for the channel model; they decide nothing.

- **WhatsApp Message content is a reference to a provider-held template plus parameter values.** One-way Broadcast and
  Automation sends always fall outside the window, so the content is (template name, language) owned by the
  Integration's Messaging account, plus a value for each parameter (merge tags bound to `{{1}}...`), plus header media
  where the template has a media header. This is the opposite of email's copy-at-author-time rule: Meta can
  recategorize, pause, disable or archive the template after the step is authored, so the send must handle "template
  unavailable" (`132001`, `132015`, `132016`) as an outcome, and authoring needs the template's current status.
- **Template creation by API is not universal.** Cloud API, 360dialog and Twilio allow it; Wazzup and Gupshup's
  passthrough do not. A baseline that works on every path is to list and sync the provider's approved templates and
  bind values to their parameters; submitting templates from sphericon would be an extra where the provider allows it,
  with up to 24 h of review in between.
- **Positional parameters are the common denominator.** Named parameters are Meta-only and opt-in.
- **Consent keyed by (channel, destination) cannot express WhatsApp's marketing-only stop.** `131050` and the
  `user_preferences` `stop` leave utility messages deliverable and can be undone by `resume`. Either consent gains a
  category dimension, or a WhatsApp marketing stop is recorded as a full opt-out on the channel and a `resume` is
  ignored.
- **`131026` should not become a hard Suppression.** It mixes "not on WhatsApp" with fixable states (terms not
  accepted, old client). `131049` is a deferral, not a failure of the destination.
- **The Destination may not be a phone number.** Webhooks can carry only a BSUID, scoped to the customer's portfolio
  and regenerated on a number change. Phone Destinations need E.164 with `+`, or Meta prepends the business's country
  code.
- **The Provider limit has two shapes here.** Throughput is messages per second per phone number; the messaging limit
  is _unique users_ reached outside a window per rolling 24 h per business portfolio, not messages, and several
  Integrations on one portfolio share it. Both are readable by API, as SES's quota is (ADR 0067).
- **A new Cloud API integration should route marketing to `/marketing_messages` from the start**, by each template's
  latest category, since accounts created from 2027-04-13 cannot send marketing on `/messages` at all.
- **Access paths split by who runs the instance.** A self-hoster can connect a customer's own Meta app (system-user
  token, WABA and phone number IDs, app secret, verify token) or a BSP API key without any Meta partnership. Embedded
  Signup requires the operator to be a Tech Provider (verification, App Review, weekly onboarding cap), which fits only
  the hosted operator.
- **The Ingest hook for Cloud API** must answer the `hub.challenge` GET, verify `X-Hub-Signature-256` with the
  customer's app secret, deduplicate (7 days of retries), accept `read` without a prior `delivered`, and can match a
  status to the Outbound message by `wamid` or by `biz_opaque_callback_data`. The same endpoint carries inbound
  messages (consent signals) and `user_preferences`.
- **US recipients**: marketing templates to +1 numbers fail today, which a message count estimate for a Broadcast
  would overstate unless it accounts for it.

## Unconfirmed

- Whether 360dialog signs its webhooks: its webhook API documents only a URL and optional custom headers; no signature
  scheme was found in its docs.
- Whether a user blocking the business produces a distinct error: Meta documents none; it is not listed among the
  `131026` reasons, and no other code names it.
- Whether Twilio Content templates accept named variables: every Content API example read uses numeric keys.
- Whether Wazzup's `templateValues` map to template variables by position: the page shows an array but does not say.
- The exact error code for marketing templates to US numbers: the per-user limits page says only "an error".
