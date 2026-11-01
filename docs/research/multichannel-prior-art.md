# Multichannel prior art: how platforms model channels, Broadcasts and content

Researched 2026-11-01 from primary sources only: each vendor's own docs and published API reference (Braze,
Iterable, Customer.io, Klaviyo, Brevo) and, for the three open-source products, their source code, shallow-cloned at
the default branch. Source permalinks use the commit SHA so line numbers stay valid.

| Repo                | Branch | Commit                                     |
| ------------------- | ------ | ------------------------------------------ |
| mautic/mautic       | 7.x    | `ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a` |
| dittofeed/dittofeed | main   | `91277939a9afcee797ab2562025f6b02d823b00c` |
| novuhq/novu         | next   | `4af47133e2cd5e3de296293e6b21bfd24a6b591f` |

API specs read as files: Iterable's [OpenAPI spec](https://api.iterable.com/api-docs) (cited as "Iterable spec"),
Customer.io's [App API spec](https://docs.customer.io/files/journeys-app.json) ("CIO spec"), Klaviyo's
[`klaviyo/openapi`](https://github.com/klaviyo/openapi) at revision `2026-10-15`
([`create_campaign.json`](https://raw.githubusercontent.com/klaviyo/openapi/main/openapi/stable/apis/create_campaign.json),
[`create_flow.json`](https://raw.githubusercontent.com/klaviyo/openapi/main/openapi/stable/apis/create_flow.json)) and
Brevo's [OpenAPI spec 3.0.0](https://api.brevo.com/v3/swagger_definition_v3.yml) ("Brevo spec"). A remote spec has no
line permalinks, so those claims name the operation or schema instead.

Limits of the sources: help.brevo.com returns HTTP 403 to fetches, so every Brevo help-centre claim (automation steps,
the SMS shortener, push) is marked **unverified**; Iterable's help-centre HTML also returns 403, so its articles were
read through Iterable's public Zendesk JSON API (`support.iterable.com/api/v2/help_center/en-us/articles/<id>.json`)
and are linked below by their human URL. Anything not confirmed in a primary source is marked **unverified**. No price
or cost facts are recorded. This note reports facts only; it proposes no sphericon design.

## Question

sphericon sends email only and is designing SMS, WhatsApp, Telegram, web push and mobile push (map #916). How do
established platforms model multichannel sending? For each of Customer.io, Braze, Klaviyo, Brevo, Iterable, Mautic,
Dittofeed and Novu:

1. Is a broadcast/campaign one object with a channel, one object with several channel variants, or a separate object
   per channel?
2. Where do per-channel addresses live on the profile (fields vs a devices/subscriptions list), and how many per
   person?
3. What templates/content exist per channel, and what is the merge-tag language?
4. How are links tracked in SMS and messengers?
5. What per-channel subscription/consent state exists, and what is it keyed by?
6. How does a journey/workflow step pick its channel and its integration/provider?

## Terms used

Competitor words mapped onto `GLOSSARY.md`:

| Competitor concept                                                              | Nearest sphericon term                |
| ------------------------------------------------------------------------------- | ------------------------------------- |
| Campaign / one-time send / newsletter / blast                                   | Broadcast                             |
| Canvas / Journey / Flow / Automation / Campaign (Mautic) / Workflow (Novu)      | Automation                            |
| Message step / tile / action / message node / workflow step                     | Step (send)                           |
| `phone` field, push token, device, `ChannelEndpoint`, `identifierKey` property  | Destination                           |
| Subscription group, topic, message type, consent group                          | Sending source (an opt-out scope)     |
| STOP keyword opt-out, `unsubscribed` flag, DoNotContact `UNSUBSCRIBED`          | Unsubscribe                           |
| Opt-in subscription group, double opt-in message type, `consent: SUBSCRIBED`    | Confirmation                          |
| Klaviyo `suppression[]`, Mautic DoNotContact `BOUNCED`, Brevo `*Blacklisted`    | Suppression (where it is a hard fact) |
| Template, variation, variant, content per action                                | Template / Message content            |
| Provider, integration, sender, subscription group (as sender pool), sending app | Integration / Send route              |

## Summary of findings

- **The send step is single-channel everywhere.** A Braze Message step, an Iterable tile, a Klaviyo flow action, a
  Customer.io workflow action, a Mautic campaign action, a Dittofeed message node, a Novu step and a Brevo automation
  action each send on exactly one channel, fixed by the step's type. Multichannel journeys are several steps. The only
  per-contact channel choice in a step is Mautic's "Send marketing message" (channel priority from the contact's
  preferences, with fallback) and the engagement-ranking features of Braze (Intelligent Channel) and Iterable
  (Channel optimization tile).
- **Broadcasts split two ways.** Customer.io one-time sends, Iterable campaigns, Brevo campaigns, Mautic messages and
  Dittofeed broadcasts are one object per channel (Brevo even has a separate endpoint per channel). Braze lets one
  campaign carry several channels ("multichannel campaign"), and Klaviyo's API revision `2026-10-15` turned a campaign
  into a channel-less container of one-channel messages, replacing the earlier one-campaign-one-channel model. Novu has
  no campaign object at all: a broadcast is a workflow triggered for a topic or for all subscribers.
- **Email and phone are single scalar fields on every profile; push tokens are a list.** Stated list limits: 25
  devices per person (Customer.io, oldest `last_used` evicted), 500 devices (Iterable, 400 on overflow), 100 tokens per
  subscriber channel (Novu). Braze, Klaviyo and Mautic state no limit. Iterable and Brevo keep a **separate WhatsApp
  number field**; the others reuse the phone. Only Novu models messenger chat ids (Telegram `chatId`) as their own
  per-subscriber records (`ChannelEndpoint`), with no stated limit.
- **Content is per channel; the merge-tag language is one per product, shared across its channels**: Liquid (Braze,
  Customer.io, Dittofeed, Novu), Handlebars (Iterable), Django-style (Klaviyo), `{{contact.X}}` (Brevo),
  `{contactfield=x}` (Mautic). No product has one template object whose body renders on several channels. WhatsApp
  content is everywhere a Meta-approved template referenced by name and language, kept as its own object.
- **SMS and WhatsApp link tracking means link shortening.** Every commercial product rewrites links to a short domain
  (shared vendor domain by default, the customer's own domain via DNS), counts clicks per channel, and expires links
  (Braze 9 weeks, Customer.io and Klaviyo 90 days, Iterable at least 30 days). Customer.io and Klaviyo share the short
  domain between SMS and WhatsApp. Mautic rewrites SMS links to its own `/r/{id}` redirect; Dittofeed and Novu track
  no SMS links.
- **Consent is per channel, and SMS consent behaves as keyed by phone number.** Braze, Iterable, Customer.io and
  Klaviyo all apply a STOP to every profile that shares the number. Braze keys SMS state per subscription group (a
  sender pool) and Customer.io per sender; Iterable per message type when the sender is bound to one. Push consent is
  per device/token (Klaviyo) or per user plus per-device OS permission (Braze, Iterable). WhatsApp consent is separate
  from SMS consent wherever both exist (Braze, Klaviyo).
- **The sender/provider is chosen per message in the commercial products** (Braze SMS subscription group, Customer.io
  sender identity, Iterable sender on the template) or automatically (Klaviyo picks a number by recipient country).
  The open-source products default per channel with an override: Dittofeed's workspace default provider per channel
  plus a per-node `providerOverride`; Novu's `primary` integration per channel plus conditions and a trigger override;
  Mautic has one instance-wide transport per channel.

## A. Braze

**Broadcast.** The ad-hoc send `POST /messages/send` carries one `messages` object keyed by channel (`apple_push`,
`android_push`, `email`, `sms`, `whats_app`, `web_push`, `webhook`, `content_card`, `kindle_push`), each a
channel-specific messaging object
([post_send_messages](https://www.braze.com/docs/api/endpoints/messaging/send_messages/post_send_messages)). In the
dashboard the author picks a single-channel campaign ("one messaging channel per launch") or a multichannel one ("more
than one messaging channel in a single launch", channels added with "Add channel"); in-app messages are excluded from
multichannel
([creating_campaign](https://www.braze.com/docs/user_guide/engagement_tools/campaigns/building_campaigns/creating_campaign)).
The campaign export returns `channels` and `messages` keyed by variation id, each with its own `channel`
([get_campaign_details](https://www.braze.com/docs/api/endpoints/export/campaigns/get_campaign_details)). An
API-triggered campaign (`POST /campaigns/trigger/send`) names a `campaign_id` and carries no content
([post_send_triggered_campaigns](https://www.braze.com/docs/api/endpoints/messaging/send_messages/post_send_triggered_campaigns)).

**Addresses.** `email` and `phone` (E.164 recommended) are single string attributes; push tokens are a list of
`{app_id, token, device_id}` ([user_attributes_object](https://www.braze.com/docs/api/objects_filters/user_attributes_object)).
"Push tokens are unique to both the device and the app", "There will be an entry for every device that belongs to the
user", and a token moves to whoever logs in on the device next
([push_token_lifecycle](https://braze.com/docs/user_guide/channels/push/push_setup/push_token_lifecycle)). Docs state
no limit on tokens per profile. Which attribute outbound WhatsApp uses is **unverified** (the WhatsApp page names
`phone` only for inbound attribution,
[user_phone_numbers](https://www.braze.com/docs/user_guide/message_building_by_channel/whatsapp/user_phone_numbers)).

**Content.** Liquid ([liquid](https://www.braze.com/docs/user_guide/personalization_and_dynamic_content/liquid));
Content Blocks are "reusable, cross-channel content" inserted with `{{content_blocks.${name}}}`
([content_blocks](https://www.braze.com/docs/user_guide/engagement_tools/templates_and_media/content_blocks)). REST
template endpoints exist only for email and Content Blocks ([templates](https://www.braze.com/docs/api/endpoints/templates)).
WhatsApp templates are referenced by `template_name` + `template_language_code`
([whats_app_object](https://www.braze.com/docs/api/objects_filters/messaging/whats_app_object)).

**Link tracking.** SMS/RCS links are shortened to `brz.ai`/`brz.is`/`brz.sh`/`brz.so` or "custom link shortening
domain assigned to your subscription group"; unified short links are valid 9 weeks; campaign-level and user-level
click tracking are separate switches
([link_shortening](https://www.braze.com/docs/user_guide/channels/sms_mms_and_rcs/message_features_and_optimization/link_shortening)),
exposed in the API as `link_shortening_enabled` and `user_click_tracking_enabled`
([sms_object](https://www.braze.com/docs/api/objects_filters/messaging/sms_object)). WhatsApp body and button links
are tracked, but not template messages sent through `/messages/send`
([whatsapp click_tracking](https://www.braze.com/docs/user_guide/message_building_by_channel/whatsapp/whatsapp_campaign/click_tracking/)).

**Consent.** Email and push have a global per-user state (`email_subscribe`, `push_subscribe`: `opted_in`,
`subscribed`, `unsubscribed`) ([user_attributes_object](https://www.braze.com/docs/api/objects_filters/user_attributes_object)).
Push state "applies to their entire user profile", while push enablement is the OS permission per device
([push_subscription_states](https://www.braze.com/docs/user_guide/channels/push/push_setup/push_subscription_states)).
"SMS and RCS use subscription group status, not a separate global channel state", and WhatsApp too
([subscription_status](https://braze.com/docs/user_guide/audience/subscription_preferences/subscription_status/)). An
SMS subscription group is a pool of senders for one purpose; its state "is not shared across subscription groups"; "If
multiple users share this phone number, they are all unsubscribed"; an orphaned number keeps its state up to 30 days
([sms subscription_groups](https://braze.com/docs/user_guide/message_building_by_channel/sms_mms_rcs/subscription_groups)).
STOP/START keywords "update the subscription group state for the phone number"
([optin_optout](https://www.braze.com/docs/user_guide/message_building_by_channel/sms_mms_rcs/keywords/optin_optout)).
Each WhatsApp subscription group "is connected to one unique phone number", and an inbound WhatsApp message "doesn't
automatically subscribe a user"
([multiple_subscription_groups](https://www.braze.com/docs/user_guide/message_building_by_channel/whatsapp/overview/multiple_subscription_groups/),
[opt-ins_and_opt-outs](https://www.braze.com/docs/user_guide/message_building_by_channel/whatsapp/message_processing/opt-ins_and_opt-outs/)).

**Step.** A Canvas Message step selects one messaging channel; a user "not reachable by channel" advances without a
send ([message_step](https://www.braze.com/docs/user_guide/engagement_tools/canvas/canvas_components/message_step)).
The SMS subscription group is picked in the composer and adds an audience filter automatically
([sms subscription_groups](https://braze.com/docs/user_guide/message_building_by_channel/sms_mms_rcs/subscription_groups));
push apps are narrowed by a delivery-validation segment
([message_step](https://www.braze.com/docs/user_guide/engagement_tools/canvas/canvas_components/message_step)).

**Also.** Frequency caps per channel or across all channels, counting every campaign and Canvas step
([rate-limiting](https://www.braze.com/docs/user_guide/engagement_tools/campaigns/building_campaigns/rate-limiting));
quiet hours in the user's time zone, with workspace defaults per channel
([quiet_hours](https://www.braze.com/docs/user_guide/messaging/messaging_fundamentals/quiet_hours)); delivery events
are per channel (SMS: Send, Carrier Send, Delivery, Delivery Failure, Rejection, Short Link Click; WhatsApp: Send,
Delivery, Read, Failure, Tracked Link Click)
([message_engagement_events](https://www.braze.com/docs/user_guide/data/braze_currents/event_glossary/message_engagement_events)).

## B. Iterable

**Broadcast.** One campaign, one channel. `POST /api/campaigns/create` "Creates a new blast or triggered campaign from
an existing template" and takes `templateId` (copied into the campaign), `listIds`, `sendAt`, `shortenLinks`, with no
channel field; `CampaignDetails` has a single `messageMedium` (Iterable spec). Single-user sends use one endpoint per
channel: `/api/email/target`, `/api/sms/target`, `/api/push/target`, `/api/webPush/target`, `/api/whatsApp/target`,
`/api/rcs/target`, `/api/inApp/target`, taking a campaign and a user but no address or sender (Iterable spec).

**Addresses.** `phoneNumber` for SMS and RCS, a separate `whatsAppPhoneNumber` for WhatsApp, a `devices` array for
mobile push ([User Profile Fields](https://support.iterable.com/hc/en-us/articles/217744303-User-Profile-Fields-Used-by-Iterable));
a WhatsApp campaign cannot reach users "who only have the phoneNumber field"
([Setting Up WhatsApp](https://support.iterable.com/hc/en-us/articles/36861390140820-Setting-Up-WhatsApp)). Each device
entry is `{token, platform, applicationName, endpointEnabled, notificationsEnabled}`; `registerDeviceToken` allows a
"Maximum of 500 devices per user profile" and returns 400 beyond it (Iterable spec). Web push uses a `browserTokens`
array that "can capture multiple values over time" and is appended to, never replaced
([Setting up Web Push](https://support.iterable.com/hc/en-us/articles/31174083562900-Setting-up-Web-Push-Notifications));
a web push goes to all of them, one send event per token
([Web Push Overview](https://support.iterable.com/hc/en-us/articles/115004760086-Web-Push-Overview)).

**Content.** Templates per medium: "you can create templates for email, embedded, in-app, push notification, SMS, RCS,
web push, and WhatsApp", copied into the campaign on use
([Templates Overview](https://support.iterable.com/hc/en-us/articles/205480315-Templates-Overview)). Handlebars for
every medium, with the personalisable fields listed per template type
([Personalizing Templates with Handlebars](https://support.iterable.com/hc/en-us/articles/205480365-Personalizing-Templates-with-Handlebars)).
WhatsApp templates carry a message type, a sender and Meta approval per locale
([Creating WhatsApp Templates](https://support.iterable.com/hc/en-us/articles/36861410753812-Creating-WhatsApp-Templates)).

**Link tracking.** "SMS, RCS, and WhatsApp" share one shortlink system: default `itbl.co` (shared across projects,
no deep links), or a custom domain CNAMEd to `links.iterable.com`; "only one tracking domain per template";
shortlinks kept "for a minimum of 30 days"; click events `smsClick`, `whatsAppClick`, `rcsClick`
([Link Shortening and Click Tracking](https://support.iterable.com/hc/en-us/articles/4403865896724-Link-Shortening-and-Click-Tracking-for-RCS-SMS-and-WhatsApp)).

**Consent.** Message channels (one medium, marketing or transactional) contain message types with a subscription
policy of opt-in, opt-out, or double opt-in (SMS only)
([Message Channels and Message Types](https://support.iterable.com/hc/en-us/articles/204780529-Message-Channels-and-Message-Types-Overview)).
State is id lists on the user: `unsubscribedChannelIds`, `unsubscribedMessageTypeIds`, `subscribedMessageTypeIds`
([User Profile Fields](https://support.iterable.com/hc/en-us/articles/217744303-User-Profile-Fields-Used-by-Iterable)).
An SMS STOP updates the unsubscribe "for all users who have this phone number" and also applies at the provider;
preference-centre changes are not synced to the provider
([SMS Unsubscribes and Resubscribes](https://support.iterable.com/hc/en-us/articles/8147569050516-SMS-Unsubscribes-and-Resubscribes)).
A sender bound to a message type unsubscribes from that type; an unbound sender from the "entire SMS channel"
([Managing SMS Opt-Outs by Message Type](https://support.iterable.com/hc/en-us/articles/17788735317012-Managing-SMS-Opt-Outs-by-Message-Type)).
WhatsApp STOP handling is **unverified**.

**Step.** "When you add a message tile to a journey, it creates a new campaign"; one tile per medium, plus a Channel
optimization tile (email, SMS and push only); a push tile selects "the app or apps"
([Message Tiles](https://support.iterable.com/hc/en-us/articles/12649121962260-Message-Tiles)). The SMS and WhatsApp
sender is chosen on the template, limited to senders bound to the template's message type
([Creating SMS Templates](https://support.iterable.com/hc/en-us/articles/360044426031-Creating-SMS-Templates)); a
sender is a Twilio messaging service or a Telnyx messaging profile
([Setting up SMS](https://support.iterable.com/hc/en-us/articles/17712346382100-Setting-up-SMS)). "Iterable supports
one web push integration per project"
([Web Push Overview](https://support.iterable.com/hc/en-us/articles/115004760086-Web-Push-Overview)).

**Also.** Frequency caps per channel and per message type, "don't combine to an overall limit", per device for push
([Frequency Management](https://support.iterable.com/hc/en-us/articles/15342990564372-Frequency-Management)); quiet
hours per campaign in the recipient's `timeZone`, on by default for SMS
([Quiet Hours](https://support.iterable.com/hc/en-us/articles/16928857601940-Quiet-Hours)); system webhook events per
channel (`smsSend`, `smsBounce`, `whatsAppSeen`, `pushUninstall`, `*SendSkip`, ...)
([System Webhooks](https://support.iterable.com/hc/en-us/articles/208013936-System-Webhooks)).

## C. Customer.io

**Broadcast.** A one-time send ("newsletter" in the API) is one object with one channel, chosen by `type` at creation:
`POST /v1/newsletters`, body a `oneOf` per channel, `type` ∈ `email | twilio | push | in_app | inbox | webhook`;
WhatsApp and Slack are not one-time-send types (CIO spec). Variants inside one are A/B tests and translations of the
same channel ([One-time sends](https://docs.customer.io/journeys/newsletters/)). An API-triggered broadcast is a
workflow of actions; its trigger `POST /v1/campaigns/{broadcast_id}/triggers` carries audience and data, no channel
or content, and each action carries its own `type` (CIO spec;
[API-triggered broadcasts](https://docs.customer.io/journeys/api-triggered-broadcasts/)). Transactional sends have a
separate endpoint per channel (`/v1/send/email`, `/v1/send/sms`, `/v1/send/push`, `/v1/send/whatsapp`, ...) (CIO spec).

**Addresses.** Phone is a plain attribute, "We recommend that you call this attribute `phone`", and Liquid can choose
"between multiple phone attributes" ([SMS: get started](https://docs.customer.io/messaging/channels/sms/get-started/));
the SMS To field is Liquid such as `{{customer.phone}}`
([Send SMS/MMS](https://docs.customer.io/messaging/channels/sms/send-messages/)). Push is a device list: "We store up to
25 devices", and "If you add a 26th device, we'll remove the device with the oldest `last_used` timestamp"
([Push developer guide](https://docs.customer.io/messaging/channels/push/developer-guide/)); a device has `token`,
`platform`, `last_used`, `app_identifier`
([Manage devices](https://docs.customer.io/messaging/people/manage/managing-customer-mobile-devices/)). Each browser that
allows web push is a device on the `web` platform, identified by its subscription endpoint
([Registering device tokens](https://docs.customer.io/messaging/channels/push/device-tokens/)). A one-time send can
dedupe "one message per email address or phone number, even when several profiles share it" (`deduped`, CIO spec).

**Content.** No cross-channel template object: content lives in the send or in each action (CIO spec). Liquid for every
channel, including sender fields ([Personalize messages with liquid](https://docs.customer.io/messaging/liquid/using-liquid/));
snippets work "anywhere that you can use liquid, including email content, subject lines, SMS"
([Snippets](https://docs.customer.io/messaging/liquid/snippets/)). WhatsApp messages "are based on templates" approved by
WhatsApp ([WhatsApp: get started](https://docs.customer.io/messaging/channels/whatsapp/get-started/)).

**Link tracking.** "By default, we track when people click links in SMS messages", per-link opt-out
`{% cio_link url:"..." track:false %}`, shortened links "expire after 90 days"
([Send SMS/MMS](https://docs.customer.io/messaging/channels/sms/send-messages/)). Default short domains `a.cx.io`/`e.cx.io`;
custom domains (CNAME plus a `_cio.<domain>` TXT record) "are shared across SMS and WhatsApp"
([Link shortening](https://docs.customer.io/messaging/channels/sms/link-shortening/)). Click events are per channel
(`clicked_email`, `clicked_twilio`, `clicked_push`) (CIO spec).

**Consent.** Layers on the person: a global `unsubscribed` ("unsubscribed from email, SMS, and push"), then
subscription-centre topics and channels in `cio_subscription_preferences` (`{"topics": {...}, "channels": {"email":
true, "push": false}}`), combined as AND
([Overview of consent](https://docs.customer.io/messaging/channels/subscriptions/overview/),
[Set preferences](https://docs.customer.io/messaging/channels/subscriptions/set-preferences-outside-center/)). A STOP
"opts them out of messages from that number only", and "anyone in the workspace who shares the same `phone` number is
also opted out of SMS" ([SMS opt-outs](https://docs.customer.io/messaging/channels/sms/keyword-opt-out/)). Consent
records are an audit trail: they "don't decide who receives your messages"
([Consent records](https://docs.customer.io/messaging/channels/subscriptions/consent-records/)).

**Step.** The action `type` is the channel (CIO spec). "You can connect more than one provider, and you can connect the
same provider twice"; "When you compose a message, you pick one of your synced senders"
([SMS: get started](https://docs.customer.io/messaging/channels/sms/get-started/)), and the From can be Liquid
([Send SMS/MMS](https://docs.customer.io/messaging/channels/sms/send-messages/)). WhatsApp actions take
`sender_identity_id` (CIO spec). A workspace can hold several mobile apps "each with its own name, platforms, and push
credentials" ([Push FAQ](https://docs.customer.io/messaging/channels/push/faq/)); the composer picks the app and a device
target: last used (default), a specific token, or all devices
([Send push](https://docs.customer.io/messaging/channels/push/send-push/)).

**Also.** Activity types are `<state>_<channel>` (`sent_twilio`, `delivered_twilio`, `suppressed_whatsapp`,
`bounced_push`) (CIO spec). Frequency limits per channel or shared across channels, applied only to workflows they are
assigned to ([Message frequency limits](https://docs.customer.io/messaging/send/message-limits/overview/)). SMS is named
`twilio` in message types but `sms` in preference keys (CIO spec;
[Set preferences](https://docs.customer.io/messaging/channels/subscriptions/set-preferences-outside-center/)).

## D. Klaviyo

**Broadcast.** Revision `2026-10-15` (GA) changed the shape: "A campaign no longer has a channel." "A campaign is now
a container." "Each message belongs to one audience and sends on one channel. To reach more channels, create more
messages." "Each message has one variation." ([Changelog](https://developers.klaviyo.com/en/docs/changelog_)). The
hierarchy is campaign → audiences → messages → variations, and `details.channel` (`email | sms | push | whatsapp`)
on the variation picks the channel
([Campaigns API overview (omnichannel)](https://developers.klaviyo.com/en/reference/campaigns_omni_api_overview)).
`POST /api/campaigns` takes inline audiences, messages and variations (`create_campaign.json`). The sources disagree
on variations per message (the changelog says one; the spec's `variations` is an array). Earlier revisions were one
campaign, one channel ([Create Campaign v2025-01-15](https://developers.klaviyo.com/en/v2025-01-15/reference/create_campaign)).

**Addresses.** `email` and `phone_number` are single strings
([Create Profile](https://developers.klaviyo.com/en/reference/create_profile)). Push tokens are a separate resource,
`POST /api/push-tokens` with `token`, `platform`, `vendor`, `enablement_status`, `device_metadata` and the owning
profile ([Create Push Token](https://developers.klaviyo.com/en/reference/create_push_token)); "A profile may have more
than 1 push token" ([Push delivery](https://help.klaviyo.com/hc/en-us/articles/15594685536539)); no limit stated
([Push FAQ](https://help.klaviyo.com/hc/en-us/articles/20582984332059)). "Klaviyo does not support web push"
([Intro to mobile push](https://help.klaviyo.com/hc/en-us/articles/22334048422683)).

**Content.** Django-based tags shared by email, SMS and push: `{{ person.first_name|title|default:'there' }}`
([Message personalization reference](https://help.klaviyo.com/hc/en-us/articles/115005084927-Template-Tags-and-Variable-Syntax),
[First name in email, SMS or push](https://help.klaviyo.com/hc/en-us/articles/11613154130843)). The Templates API has
email-shaped fields and no channel (Klaviyo spec `stable.json`). WhatsApp templates are separate UI objects with an
intent (Marketing or Transactional), submitted to Meta
([Create a WhatsApp template](https://help.klaviyo.com/hc/en-us/articles/40116644987675)).

**Link tracking.** SMS clicks are tracked only through Klaviyo's shortener; links shortened by third parties are not
([SMS metrics](https://help.klaviyo.com/hc/en-us/articles/360035345572),
[SMS vs WhatsApp shortened links](https://help.klaviyo.com/hc/en-us/articles/17649677926299)). `shorten_links` is a
per-message flag on SMS and WhatsApp (`create_campaign.json`). Domains: shared `klv3.io`, a branded prefix, or the
customer's own domain; "all shortened links, for both SMS and WhatsApp, stop redirecting 90 days after a message is
sent" ([Branded shortened link](https://help.klaviyo.com/hc/en-us/articles/17649597637147),
[SMS vs WhatsApp shortened links](https://help.klaviyo.com/hc/en-us/articles/17649677926299)).

**Consent.** On the profile, per channel and per purpose: `email.marketing` (with `suppression[]` of `HARD_BOUNCE`,
`SPAM_COMPLAINT`, `UNSUBSCRIBE`, ...), `sms.marketing`, `sms.transactional`, `mobile_push.marketing`,
`whatsapp.marketing|transactional|conversational` ([Get Profile](https://developers.klaviyo.com/en/reference/get_profile)).
"SMS consent is tied to the phone number. Removing it also affects any other profiles in your account with the same
phone number" ([Remove SMS consent](https://help.klaviyo.com/hc/en-us/articles/360051013832)); a keyword opt-out removes
both transactional and promotional SMS consent
([SMS opt-out options](https://help.klaviyo.com/hc/en-us/articles/26977831996315)). "For push notifications, consent is
tracked per push token" ([Push consent](https://help.klaviyo.com/hc/en-us/articles/14781686592283)). "An SMS
subscription in Klaviyo does not automatically subscribe someone to WhatsApp"
([WhatsApp consent collection](https://help.klaviyo.com/hc/en-us/articles/40116301104539)).

**Step.** The flow action type is the channel: `send-email`, `send-sms`, `send-mobile-push`, `send-whatsapp`
(`create_flow.json`). SMS and push actions have no sender, number or app field; "Klaviyo uses the country associated
with each recipient's phone number to select a sending number for that country"
([Multinational SMS sending](https://help.klaviyo.com/hc/en-us/articles/23740503987099)). A push message targets
Both/iOS/Android and all devices or the "Primary Device"
([Push delivery](https://help.klaviyo.com/hc/en-us/articles/15594685536539)).

**Also.** "Smart Sending is channel specific" ([Push settings](https://help.klaviyo.com/hc/en-us/articles/12932500186907));
SMS quiet hours 8 p.m. to 11 a.m. by default
([Text message settings](https://help.klaviyo.com/hc/en-us/articles/360035285472)); one SMS message can carry an RCS →
SMS fallback chain (`message_hierarchy`, `create_flow.json`). Push is `mobile_push`, `push` or `send-mobile-push`
depending on the endpoint ([Get Profile](https://developers.klaviyo.com/en/reference/get_profile),
[Bulk Subscribe](https://developers.klaviyo.com/en/reference/bulk_subscribe_profiles)).

## E. Brevo

**Broadcast.** A separate resource per channel with its own create schema: `/emailCampaigns`, `/smsCampaigns`,
`/whatsappCampaigns` (Brevo spec). An SMS campaign is `name`, `sender` (≤11 letters or ≤15 digits) and plain-text
`content`, with no template, params or tracking field
([create-sms-campaign](https://developers.brevo.com/reference/create-sms-campaign)). A WhatsApp campaign has no text,
only `templateId` of an approved template
([create-whats-app-campaign](https://developers.brevo.com/reference/create-whats-app-campaign)). The spec has no push
campaign and no automation resource (Brevo spec).

**Addresses.** Named contact attributes with one value each: `SMS`, `WHATSAPP`, `LANDLINE_NUMBER`, all usable as the
contact identifier (`identifierType` `phone_id`, `whatsapp_id`, `landline_number_id`)
([update-contact](https://developers.brevo.com/reference/update-contact)); they are unique per account, and a clash
errors unless `forceMerge` is set (Brevo spec, `createContact`). Push devices are not modelled on the contact in any
source read; **unverified**.

**Content.** Templates per channel and per resource: email `/smtp/templates`, SMS templates (list only),
WhatsApp templates with `category` `MARKETING|UTILITY` submitted for approval
([create-whats-app-template](https://developers.brevo.com/reference/create-whats-app-template)). Merge tags
`{{contact.FNAME}}` and `{{params.x}}`
([batch-send guide](https://developers.brevo.com/docs/batch-send-transactional-emails)); the older `{FNAME}` syntax
still appears in the spec (Brevo spec, `toField`).

**Link tracking.** The only primary fact is `short_url_enabled` on SMS templates; SMS campaign stats have no clicks
(Brevo spec, `getSMSTemplates`, `getSmsCampaignStats`). An "Automatically shorten links" editor option is
**unverified** ([Create an SMS campaign](https://help.brevo.com/hc/en-us/articles/16603051344146-Create-an-SMS-campaign)).
WhatsApp events have no click (Brevo spec).

**Consent.** Per-channel booleans on the contact: `emailBlacklisted`, `smsBlacklisted`, `whatsappBlacklisted` (the
last read-only) (Brevo spec, `getContactDetails`). Consent groups (`{id, name, signupMode}`, contact status
`subscribed|unsubscribed`) have no channel field
([get-consent-groups](https://developers.brevo.com/reference/get-consent-groups)). A transactional SMS containing an
opt-out code "is automatically switched to `marketing` type"
([transactional SMS guide](https://developers.brevo.com/docs/transactional-sms-endpoints)).

**Step.** Each message type is its own automation action and the SMS step has its own sender name; **unverified**
([Available triggers, actions, and rules](https://help.brevo.com/hc/en-us/articles/15445989568402-Available-triggers-actions-and-rules-in-an-automation),
[Send SMS through an automation](https://help.brevo.com/hc/en-us/articles/4424283046418-Send-SMS-messages-through-an-automation)).
No provider selection exists in the spec: Brevo is the SMS and WhatsApp sender; the WhatsApp config is a single
business account (Brevo spec, `/whatsappCampaigns/config`).

## F. Mautic (source at `ee2b5b6`)

Base: `https://github.com/mautic/mautic/blob/ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a/`. Core has email, SMS, web push
(`notification`), mobile push (`mobile_notification`) and tweets; no WhatsApp or Telegram channel.

**Broadcast.** One entity per channel: `Email`, `Sms` (table `sms_messages`), and `Notification` (one entity for web
and mobile push, told apart by a `mobile` boolean)
([Sms.php L67](https://github.com/mautic/mautic/blob/ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a/app/bundles/SmsBundle/Entity/Sms.php#L67),
[Notification.php L186-L244](https://github.com/mautic/mautic/blob/ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a/app/bundles/NotificationBundle/Entity/Notification.php#L186-L244)).
"Segment message" vs "template message" is a type column on each (`smsType` `list|template`)
([Sms.php L132](https://github.com/mautic/mautic/blob/ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a/app/bundles/SmsBundle/Entity/Sms.php#L132)).
A **Marketing Message** is a wrapper: rows `(message_id, channel, channel_id, is_enabled)` pointing at each channel's
own entity, unique on `(message_id, channel)`, so at most one variant per channel
([Channel.php L88-L113](https://github.com/mautic/mautic/blob/ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a/app/bundles/ChannelBundle/Entity/Channel.php#L88-L113)).

**Addresses.** One `email` (unique), one `mobile`, one `phone` column per contact
([FieldModel.php L86-L105](https://github.com/mautic/mautic/blob/ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a/app/bundles/LeadBundle/Model/FieldModel.php#L86-L105));
campaign SMS goes to `mobile ?: phone`
([Lead.php L1573-L1576](https://github.com/mautic/mautic/blob/ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a/app/bundles/LeadBundle/Entity/Lead.php#L1573-L1576)),
while segment SMS filters on `mobile` only
([BroadcastQuery.php L51-L52](https://github.com/mautic/mautic/blob/ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a/app/bundles/SmsBundle/Broadcast/BroadcastQuery.php#L51-L52)).
Push addresses are `PushID` rows, many per contact, with `enabled` and `mobile` flags; source states no limit
([PushID.php L38-L62](https://github.com/mautic/mautic/blob/ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a/app/bundles/NotificationBundle/Entity/PushID.php#L38-L62)).
A push goes to every matching PushID
([Notification CampaignSubscriber L194-L217](https://github.com/mautic/mautic/blob/ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a/app/bundles/NotificationBundle/EventListener/CampaignSubscriber.php#L194-L217)).

**Content.** Content lives on each channel entity. One token syntax for email, SMS and push: `{contactfield=alias}`
with `|default` ([TokenHelper.php L11-L13](https://github.com/mautic/mautic/blob/ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a/app/bundles/LeadBundle/Helper/TokenHelper.php#L11-L13)).
SMS and push have per-locale translation children chosen per contact at send
([SmsModel.php L288](https://github.com/mautic/mautic/blob/ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a/app/bundles/SmsBundle/Model/SmsModel.php#L288)).

**Link tracking.** SMS links are rewritten to the instance's own `/r/{redirectId}` redirect unless
`sms_disable_trackable_urls` is set
([SmsSubscriber.php L73-L113](https://github.com/mautic/mautic/blob/ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a/app/bundles/SmsBundle/EventListener/SmsSubscriber.php#L73-L113));
hits are counted per `(redirect, channel, channelId)`
([Trackable.php L42-L62](https://github.com/mautic/mautic/blob/ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a/app/bundles/PageBundle/Entity/Trackable.php#L42-L62)).
A shortener is a pluggable interface, applied after tracking, with no implementation shipped; unconfigured it returns
the URL unchanged ([Shortener.php L64-L71](https://github.com/mautic/mautic/blob/ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a/app/bundles/CoreBundle/Shortener/Shortener.php#L64-L71),
[TrackableModel.php L90-L113](https://github.com/mautic/mautic/blob/ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a/app/bundles/PageBundle/Model/TrackableModel.php#L90-L113)).

**Consent.** `DoNotContact`: rows `(lead_id, channel, reason, channel_id)`, reason `UNSUBSCRIBED=1`, `BOUNCED=2`,
`MANUAL=3`, keyed by contact and channel string
([DoNotContact.php L17-L93](https://github.com/mautic/mautic/blob/ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a/app/bundles/LeadBundle/Entity/DoNotContact.php#L17-L93)).
Web and mobile push both check DNC under `notification`
([Notification CampaignSubscriber L180-L189](https://github.com/mautic/mautic/blob/ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a/app/bundles/NotificationBundle/EventListener/CampaignSubscriber.php#L180-L189)).
An inbound `stop` adds an `sms` DNC
([StopSubscriber.php L25-L33](https://github.com/mautic/mautic/blob/ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a/app/bundles/SmsBundle/EventListener/StopSubscriber.php#L25-L33)).
`FrequencyRule` holds per-contact, per-channel caps, a pause window and a `preferred_channel` boolean
([FrequencyRule.php L62-L101](https://github.com/mautic/mautic/blob/ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a/app/bundles/LeadBundle/Entity/FrequencyRule.php#L62-L101));
a capped message is rescheduled, not dropped
([MessageQueueModel.php L68-L118](https://github.com/mautic/mautic/blob/ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a/app/bundles/ChannelBundle/Model/MessageQueueModel.php#L68-L118)).

**Step.** Channel actions fix their channel (`email.send`, `sms.send_text_sms`, `notification`, `mobile_notification`)
([CampaignSendSubscriber.php L33-L50](https://github.com/mautic/mautic/blob/ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a/app/bundles/SmsBundle/EventListener/CampaignSendSubscriber.php#L33-L50)).
The provider is instance-wide: every SMS goes through `TransportChain::getPrimaryTransport()` (Twilio is the only one
shipped) ([TransportChain.php L49-L67](https://github.com/mautic/mautic/blob/ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a/app/bundles/SmsBundle/Sms/TransportChain.php#L49-L67)).
"Send marketing message" picks the channel per contact: the contact's channel rules first (unpaused, preferred, then
more allowed messages), DNC channels skipped, then the rest; it falls through priorities until one channel sends
([ChannelBundle CampaignSubscriber L89-L141](https://github.com/mautic/mautic/blob/ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a/app/bundles/ChannelBundle/EventListener/CampaignSubscriber.php#L89-L141),
[Lead.php L1783-L1873](https://github.com/mautic/mautic/blob/ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a/app/bundles/LeadBundle/Entity/Lead.php#L1783-L1873)).
The docs say email is the default channel
([marketing_messages](https://docs.mautic.org/en/7.0/_sources/channels/marketing_messages.rst.txt)); the code has no
such default (row order decides).

**Also.** SMS delivery state is thin: the SMS `Stat` has sent and failed, no delivered or read
([Stat.php L83-L132](https://github.com/mautic/mautic/blob/ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a/app/bundles/SmsBundle/Entity/Stat.php#L83-L132)),
and the SMS callback interface reads inbound replies only
([CallbackInterface.php L20-L41](https://github.com/mautic/mautic/blob/ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a/app/bundles/SmsBundle/Callback/CallbackInterface.php#L20-L41)).

## G. Dittofeed (source at `9127793`)

Base: `https://github.com/dittofeed/dittofeed/blob/91277939a9afcee797ab2562025f6b02d823b00c/`. Channels: `Email`,
`MobilePush`, `Sms`, `Webhook`
([types.ts L130](https://github.com/dittofeed/dittofeed/blob/91277939a9afcee797ab2562025f6b02d823b00c/packages/isomorphic-lib/src/types.ts#L130));
no WhatsApp, Telegram or web push.

**Broadcast.** One channel per broadcast. The `Broadcast` row has one `messageTemplateId`, one `segmentId`, one
`subscriptionGroupId` and a `config`
([schema.ts L385](https://github.com/dittofeed/dittofeed/blob/91277939a9afcee797ab2562025f6b02d823b00c/packages/backend-lib/src/db/schema.ts#L385));
the channel is the template's `definition.type`
([types.ts L1820](https://github.com/dittofeed/dittofeed/blob/91277939a9afcee797ab2562025f6b02d823b00c/packages/isomorphic-lib/src/types.ts#L1820)).
Upserting a V2 broadcast rejects one whose template, subscription group and `config.message.type` disagree: "must all
be the same channel type"
([broadcasts.ts L510-L526](https://github.com/dittofeed/dittofeed/blob/91277939a9afcee797ab2562025f6b02d823b00c/packages/backend-lib/src/broadcasts.ts#L510-L526)).

**Addresses.** Ordinary user properties `email`, `phone`, `deviceToken`, one value each
([bootstrap.ts L158-L185](https://github.com/dittofeed/dittofeed/blob/91277939a9afcee797ab2562025f6b02d823b00c/packages/backend-lib/src/bootstrap.ts#L158-L185),
[schema.ts L186-L206](https://github.com/dittofeed/dittofeed/blob/91277939a9afcee797ab2562025f6b02d823b00c/packages/backend-lib/src/db/schema.ts#L186-L206)).
The default property per channel is a constant
([channels.ts L3](https://github.com/dittofeed/dittofeed/blob/91277939a9afcee797ab2562025f6b02d823b00c/packages/isomorphic-lib/src/channels.ts#L3)),
overridable per template by `identifierKey`, e.g. to send to a manager; subscription and delivery state stay on the
original user ([custom recipient properties](https://docs.dittofeed.com/guide/custom-recipient-properties)).

**Content.** One template type per channel with its own fields; Liquid (liquidjs, `strictVariables`) for all of them
([liquid.ts L48](https://github.com/dittofeed/dittofeed/blob/91277939a9afcee797ab2562025f6b02d823b00c/packages/backend-lib/src/liquid.ts#L48)).

**Link tracking.** None: Dittofeed rewrites no links on any channel; email clicks come from the ESP's webhook, and SMS
has only `DFSmsDelivered`/`DFSmsFailed`
([types.ts L95-L99](https://github.com/dittofeed/dittofeed/blob/91277939a9afcee797ab2562025f6b02d823b00c/packages/isomorphic-lib/src/types.ts#L95-L99)).

**Consent.** A subscription group is `{name, type: OptIn|OptOut, channel}`, several per channel allowed, one per
channel created by default ([schema.ts L317](https://github.com/dittofeed/dittofeed/blob/91277939a9afcee797ab2562025f6b02d823b00c/packages/backend-lib/src/db/schema.ts#L317);
[subscription groups](https://docs.dittofeed.com/resources/subscription-groups)). State is event-sourced from
`DFSubscriptionChange` events into segment membership keyed by `(workspace, user, group)`
([subscriptionGroups.ts L332-L390](https://github.com/dittofeed/dittofeed/blob/91277939a9afcee797ab2562025f6b02d823b00c/packages/backend-lib/src/subscriptionGroups.ts#L332-L390)).
Unsubscribe links carry the identifier value and identifier key, HMAC-signed
([subscriptionGroups.ts L453-L500](https://github.com/dittofeed/dittofeed/blob/91277939a9afcee797ab2562025f6b02d823b00c/packages/backend-lib/src/subscriptionGroups.ts#L453-L500)).

**Step.** A `MessageNode` holds `variant.type` (the channel), `templateId`, optional `subscriptionGroupId`, and an
optional `providerOverride` (SMS also `senderOverride`)
([types.ts L1054-L1185](https://github.com/dittofeed/dittofeed/blob/91277939a9afcee797ab2562025f6b02d823b00c/packages/isomorphic-lib/src/types.ts#L1054-L1185)).
Providers are unique per `(workspace, type)` with one `DefaultEmailProvider` and one `DefaultSmsProvider` per workspace
([schema.ts L351](https://github.com/dittofeed/dittofeed/blob/91277939a9afcee797ab2562025f6b02d823b00c/packages/backend-lib/src/db/schema.ts#L351),
[schema.ts L782](https://github.com/dittofeed/dittofeed/blob/91277939a9afcee797ab2562025f6b02d823b00c/packages/backend-lib/src/db/schema.ts#L782));
resolution is override, else default, with no failover
([messaging.ts L604-L660](https://github.com/dittofeed/dittofeed/blob/91277939a9afcee797ab2562025f6b02d823b00c/packages/backend-lib/src/messaging.ts#L604-L660)).
Mobile push is modelled but `sendMessage` throws "not implemented" for it
([messaging.ts L2416-L2424](https://github.com/dittofeed/dittofeed/blob/91277939a9afcee797ab2562025f6b02d823b00c/packages/backend-lib/src/messaging.ts#L2416-L2424)).

## H. Novu (source at `4af4713`)

Base: `https://github.com/novuhq/novu/blob/4af47133e2cd5e3de296293e6b21bfd24a6b591f/`. Channels: `in_app, email,
sms, chat, push`; WhatsApp and Telegram are **chat providers**, not channels
([channel.ts L1-L8](https://github.com/novuhq/novu/blob/4af47133e2cd5e3de296293e6b21bfd24a6b591f/packages/shared/src/types/channel.ts#L1-L8),
[providers.ts L151-L182](https://github.com/novuhq/novu/blob/4af47133e2cd5e3de296293e6b21bfd24a6b591f/packages/shared/src/types/providers.ts#L151-L182)).

**Broadcast.** No campaign object. A workflow has ordered steps, each one channel (its type)
([message-template.entity.ts L14-L67](https://github.com/novuhq/novu/blob/4af47133e2cd5e3de296293e6b21bfd24a6b591f/libs/dal/src/repositories/message-template/message-template.entity.ts#L14-L67));
step `variants` are conditional content of the same step, not channel alternatives
([notification-template.entity.ts L147-L193](https://github.com/novuhq/novu/blob/4af47133e2cd5e3de296293e6b21bfd24a6b591f/libs/dal/src/repositories/notification-template/notification-template.entity.ts#L147-L193)).
Mass sends trigger the workflow for a topic ("Novu's primary mechanism for broadcasting notifications to large
audiences", [trigger-workflow](https://docs.novu.co/platform/workflow/trigger-workflow)) or for all subscribers via
`POST /v1/events/trigger/broadcast`
([events.controller.ts L180-L221](https://github.com/novuhq/novu/blob/4af47133e2cd5e3de296293e6b21bfd24a6b591f/apps/api/src/app/events/events.controller.ts#L180-L221)).

**Addresses.** Three places. Scalar `email` and `phone` on the subscriber
([subscriber.entity.ts L7-L56](https://github.com/novuhq/novu/blob/4af47133e2cd5e3de296293e6b21bfd24a6b591f/libs/dal/src/repositories/subscriber/subscriber.entity.ts#L7-L56));
phone-based chat providers (WhatsApp Business) resolve from `subscriber.phone`
([send-message-chat.usecase.ts L96-L104](https://github.com/novuhq/novu/blob/4af47133e2cd5e3de296293e6b21bfd24a6b591f/apps/worker/src/app/workflow/usecases/send-message/send-message-chat.usecase.ts#L96-L104)).
Legacy `subscriber.channels[]`, deprecated in favour of channel endpoints but still what push reads: per integration
`{providerId, credentials: {deviceTokens[], webhookUrl, ...}}`
([send-message-push.usecase.ts L196-L256](https://github.com/novuhq/novu/blob/4af47133e2cd5e3de296293e6b21bfd24a6b591f/apps/worker/src/app/workflow/usecases/send-message/send-message-push.usecase.ts#L196-L256)),
limited to 100 device tokens per subscriber channel by default
([resource-validator.service.ts L37-L49](https://github.com/novuhq/novu/blob/4af47133e2cd5e3de296293e6b21bfd24a6b591f/libs/application-generic/src/services/resource-validator.service.ts#L37-L49);
[push](https://docs.novu.co/platform/integrations/push)). And the newer `ChannelEndpoint` collection: one document per
address, `{subscriberId, channel, providerId, integrationIdentifier, type, endpoint}`, with types such as
`telegram_chat {chatId}`, `phone {phoneNumber}`, `slack_user`, `webhook`
([channel-endpoint.ts L6-L85](https://github.com/novuhq/novu/blob/4af47133e2cd5e3de296293e6b21bfd24a6b591f/packages/shared/src/types/channel-endpoint.ts#L6-L85));
`telegram_chat` and `phone` "may legitimately repeat"
([channel-endpoint.schema.ts L65-L130](https://github.com/novuhq/novu/blob/4af47133e2cd5e3de296293e6b21bfd24a6b591f/libs/dal/src/repositories/channel-endpoint/channel-endpoint.schema.ts#L65-L130)).
The Telegram subscriber connects through a link or a registered chat id
([telegram](https://docs.novu.co/platform/integrations/chat/telegram)); the link route is **unverified** in source.

**Content.** Per step, channel-specific controls; "All Novu template editors support LiquidJS"
([personalize-content](https://docs.novu.co/platform/workflow/add-notification-content/personalize-content)); legacy
v1 workflows render with Handlebars
([compile-template.usecase.ts L4](https://github.com/novuhq/novu/blob/4af47133e2cd5e3de296293e6b21bfd24a6b591f/libs/application-generic/src/usecases/compile-template/compile-template.usecase.ts#L4)).
WhatsApp uses Meta templates passed in a `chat.template` override
([whats-app](https://docs.novu.co/platform/integrations/chat/whats-app)).

**Link tracking.** None in Novu; the only SMS link feature is Twilio's own `shortenUrls` passed through
([twilio.schema.ts L77-L81](https://github.com/novuhq/novu/blob/4af47133e2cd5e3de296293e6b21bfd24a6b591f/packages/framework/src/schemas/providers/sms/twilio.schema.ts#L77-L81)),
and SMS statuses have no click
([provider.interface.ts L311-L321](https://github.com/novuhq/novu/blob/4af47133e2cd5e3de296293e6b21bfd24a6b591f/packages/stateless/src/lib/provider/provider.interface.ts#L311-L321)).

**Consent.** Preferences, not lists: `{all: {enabled, readOnly}, channels: {<channel>: {enabled}}}` at workflow,
subscriber-global and subscriber-per-workflow levels, the most specific winning; a boolean per channel type, not per
address or provider
([workflow-channel-preferences.ts L27-L83](https://github.com/novuhq/novu/blob/4af47133e2cd5e3de296293e6b21bfd24a6b591f/packages/shared/src/types/workflow-channel-preferences.ts#L27-L83);
[preferences](https://docs.novu.co/platform/concepts/preferences)). "Critical" workflows (`readOnly`) ignore subscriber
preferences (same page).

**Step.** Channel = step type; the integration is chosen at send time. An `Integration` is `{providerId, channel,
identifier, active, primary, priority, conditions}` per environment
([integration.entity.ts L20-L68](https://github.com/novuhq/novu/blob/4af47133e2cd5e3de296293e6b21bfd24a6b591f/libs/dal/src/repositories/integration/integration.entity.ts#L20-L68)).
Email and SMS use exactly one: a trigger override by identifier, else the highest-priority integration whose
conditions match, else the `primary` one
([select-integration.usecase.ts L36-L81](https://github.com/novuhq/novu/blob/4af47133e2cd5e3de296293e6b21bfd24a6b591f/libs/application-generic/src/usecases/select-integration/select-integration.usecase.ts#L36-L81);
[integration-conditions](https://docs.novu.co/platform/integrations/integration-conditions)). Push and chat fan out:
"all active integrations are used in parallel"
([integrations](https://docs.novu.co/platform/concepts/integrations)). A missing address is a skip, not a failure
(`USER_MISSING_PUSH_TOKEN`)
([send-message-push.usecase.ts L201-L219](https://github.com/novuhq/novu/blob/4af47133e2cd5e3de296293e6b21bfd24a6b591f/apps/worker/src/app/workflow/usecases/send-message/send-message-push.usecase.ts#L201-L219)).

## Comparison

| Product     | Broadcast shape                                                                   | Addresses on the profile (stated limit)                                                                 | Content and merge tags                                   | SMS / messenger link tracking                                             | Per-channel consent state, keyed by                                                                                | Step: channel, provider                                                             |
| ----------- | --------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- | -------------------------------------------------------- | ------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------- |
| Braze       | Single- or multichannel campaign; send API keyed by channel                       | `email`, `phone` scalars; push tokens list per device and app (none stated)                             | Per channel; Liquid; cross-channel Content Blocks        | Shortener, `brz.*` or custom domain per subscription group; WhatsApp too  | Email/push per user; SMS and WhatsApp per subscription group, applied to every profile with the number             | One channel per step; SMS sender pool = subscription group; push app by segment     |
| Iterable    | One campaign = one medium, from one template                                      | `phoneNumber`, separate `whatsAppPhoneNumber`; `devices[]` (500), `browserTokens[]` (none stated)       | Template per medium; Handlebars                          | Shared shortener for SMS, RCS, WhatsApp; `itbl.co` or custom CNAME        | Channel → message type (opt-in/out/double), id lists on user; SMS STOP fans out by number                          | One tile = one campaign; sender on the template, bound to a message type            |
| Customer.io | One-time send = one channel (`type`); triggered broadcast = workflow of actions   | `phone` attribute (Liquid-picked); devices list incl. web push (25, oldest evicted)                     | Per action; Liquid; cross-channel snippets               | On by default; `a.cx.io` or custom domain shared by SMS and WhatsApp; 90d | Global `unsubscribed`, topics AND channels on the person; STOP per sender, fans out by `phone`                     | Action `type`; sender identity picked per message; push app and device target       |
| Klaviyo     | Campaign is a channel-less container of one-channel messages (since `2026-10-15`) | `email`, `phone_number` scalars; push-token resources (none stated); no web push                        | Per variation; Django-style; WhatsApp templates separate | Only via Klaviyo shortener, `klv3.io`, branded or own domain; 90d         | `subscriptions.<channel>.<purpose>` on profile; SMS tied to the number; push per token; WhatsApp separate from SMS | Action type; SMS number chosen by recipient country; no sender field                |
| Brevo       | Separate resource and endpoint per channel                                        | `SMS`, `WHATSAPP`, `LANDLINE_NUMBER` attributes, unique per account (one each)                          | Templates per channel; `{{contact.X}}`, `{{params.x}}`   | `short_url_enabled` on SMS templates; rest **unverified**                 | `emailBlacklisted`, `smsBlacklisted`, `whatsappBlacklisted` on the contact; consent groups have no channel         | Action per message type (**unverified**); Brevo is the provider                     |
| Mautic      | Entity per channel; Marketing Message wraps ≤1 per channel                        | `email`, `mobile`, `phone` columns; `PushID` rows (none stated)                                         | On each entity; `{contactfield=x}` for all               | Own `/r/{id}` redirect; shortener interface, none shipped                 | `DoNotContact` (contact, channel, reason); `FrequencyRule` per channel with preferred flag                         | Action per channel, or per-contact pick with fallback; provider instance-wide       |
| Dittofeed   | One channel per broadcast, enforced                                               | User properties `email`, `phone`, `deviceToken`, one value; `identifierKey` per template                | Template per channel; Liquid                             | None                                                                      | Subscription groups (channel, opt-in/out), event-sourced, keyed by user                                            | Node `variant.type`; workspace default provider per channel, per-node override      |
| Novu        | No campaign; workflow triggered for a topic or everyone                           | `email`, `phone` scalars; push tokens per integration (100); `ChannelEndpoint` docs for chat ids (none) | Per step; Liquid (Handlebars on v1)                      | None (Twilio `shortenUrls` passthrough)                                   | Preferences: boolean per channel at workflow / subscriber / subscriber-workflow level                              | Step type; email/SMS one integration (override, conditions, primary); push/chat all |

## Patterns that recur

1. **A step sends on one channel.** All eight products fix the channel by the step's type, and a multichannel journey
   is a sequence of one-channel steps. Per-contact channel choice inside one step is rare and needs per-contact
   channel preference data (Mautic's FrequencyRule) or engagement history (Braze, Iterable).
2. **A one-channel broadcast is the common shape; the multichannel container is recent.** Five products keep one
   channel per broadcast object. Braze and Klaviyo allow several channels in one campaign; Klaviyo moved there in its
   `2026-10-15` API revision, and still keeps each message to one channel with its own content.
3. **Email and phone are scalars; push addresses are a growing list.** Every product holds one email and one phone
   per profile. Push tokens are rows tied to an app or integration, often with a cap and eviction (25, 100, 500), and
   invalid tokens are removed on provider rejection. A distinct WhatsApp number field appears in two products
   (Iterable, Brevo); messenger chat ids as their own records appear only in Novu.
4. **SMS consent outlives the profile boundary.** Where STOP is handled, it applies to the phone number across every
   profile that holds it (Braze, Iterable, Customer.io, Klaviyo), and Braze keeps an orphaned number's state for 30
   days. Scope below the channel varies: per sender pool (Braze), per sender (Customer.io), per message type
   (Iterable), none (Klaviyo, Mautic).
5. **Push consent splits into the user's choice and the device's permission.** Braze and Iterable store a user-level
   opt-out plus per-device enablement; Klaviyo keys push consent per token; in Customer.io a device's push permission
   produces no consent record.
6. **One merge-tag language per product, content per channel.** No product shares one body across channels; the reuse
   unit is a snippet or content block inserted by the shared template language. WhatsApp content is always a
   pre-approved Meta template referenced by name and language.
7. **SMS and messenger click tracking is a short-link service with a custom domain and an expiry.** It is shared
   between SMS and WhatsApp where both exist, and it is the only source of SMS click events (no opens on SMS anywhere).
8. **The sender or provider is part of the message, not only of the workspace.** Commercial products choose the sender
   per message (subscription group, sender identity, template sender); the open-source ones choose a workspace or
   environment default per channel with a per-step or per-trigger override. Fan-out to every integration is used only
   for push and chat (Novu).
9. **Delivery events are named per channel.** Each product has its own event set per channel (`sent_twilio`,
   `smsBounce`, `DFSmsDelivered`, WhatsApp `Read`/`Seen`), and SMS reports delivered/failed/clicked but never opened.
