# Telegram Bot API: reaching a user, binding a chat to a Contact, limits

Researched 2026-11-01 for [#919](https://github.com/getsphericon/sphericon/issues/919) (map
[#916](https://github.com/getsphericon/sphericon/issues/916)), from primary sources only: Telegram's own developer
pages on `core.telegram.org` (the Bot API reference as published at **Bot API 10.3, 2026-08-24**, the Bots FAQ, the
bot features guide, the bots introduction, the Telegram Login page and the archived legacy Login Widget page), the
legal pages on `telegram.org` (Terms of Service, Bot Platform Developer Terms, Privacy Policy) and the source of the
official Bot API server. Code was read, never run; no bot was created or called. No blog, library README or Stack
Overflow answer is cited. A claim that could not be read on a primary source is moved to "Unconfirmed" at the end.
sphericon's side is read from [GLOSSARY.md](../../GLOSSARY.md) (Identify, Destination, Integration) and
[ADR 0037](../adr/0037-inbound-provider-callbacks-are-presets-on-an-integration.md),
[ADR 0023](../adr/0023-send-rate-limit-per-integration.md),
[ADR 0053](../adr/0053-delivery-and-engagement-events-cover-every-send.md) and
[ADR 0067](../adr/0067-the-send-cap-is-the-platforms-limit-the-provider-limit-the-customers.md).

| Repo                   | Branch | Commit                                     | Last commit |
| ---------------------- | ------ | ------------------------------------------ | ----------- |
| tdlib/telegram-bot-api | master | `e3e9dd8e5b3d7ab8537cd5a10dc31d5ffa8f82d1` | 2026-08-25  |

Short names used below:

- **API**: `https://core.telegram.org/bots/api#<anchor>`
- **FAQ**: `https://core.telegram.org/bots/faq#<anchor>`
- **Source**: `https://github.com/tdlib/telegram-bot-api/blob/e3e9dd8/telegram-bot-api/<file>#L<n>`
- **Developer Terms**: `https://telegram.org/tos/bot-developers#<section>`

## Question

Each Workspace connects its own Telegram bot (its token from @BotFather) as an Integration. How can that bot send
marketing messages to the Workspace's Contacts, and how is a Telegram chat bound to a known Contact (Identify)? What
are the update transport, the limits, the signals that a chat is gone, the delivery feedback, the message features,
Telegram's rules on promotional messaging, and Telegram Login as another way to learn a user id?

## Summary of findings

- **A bot cannot open a conversation.** "Bots can't start conversations with users. A user must either add them to a
  group or send them a message first"
  ([core.telegram.org/bots](https://core.telegram.org/bots#how-are-bots-different-from-users)). The precise rule is
  wider than "only users who pressed Start": a user can also grant a bot **write access** without messaging it,
  through Telegram Login's `telegram:bot_access` scope, the legacy widget's "Request Access" option, or a Mini App
  (`WriteAccessAllowed`). Section 1.
- **`/start <payload>` is the binding mechanism.** `https://t.me/<bot>?start=<payload>` delivers `/start <payload>` to
  the bot when the user opens the chat; the payload is at most 64 characters of `A-Z a-z 0-9 _ -`. Telegram itself
  names "an authentication token to connect the user's Telegram account to their account on another platform" as a
  use. Section 2.
- **One bot, one update consumer.** Webhook and `getUpdates` are mutually exclusive, a bot has one webhook URL, and
  pending updates live at most 24 hours. Connecting a bot to sphericon takes over its updates. The webhook can carry a
  secret in `X-Telegram-Bot-Api-Secret-Token`. Section 3.
- **Limits.** About 1 message per second per chat, 20 per minute per group, and "about 30 messages per second" for
  bulk notifications per bot; over that, HTTP 429 with `retry_after`. A paid mode raises the bot to 1000 per second
  but needs 100,000 monthly active users and a Stars balance, so it is out of reach for nearly every customer bot.
  Section 4.
- **The gone-chat signals.** `403 Forbidden: bot was blocked by the user`, `403 Forbidden: user is deactivated`, a
  `my_chat_member` update when the user blocks or unblocks the bot (without any send), and `401 Unauthorized: invalid
  token specified` when the token is revoked. Section 5.
- **No delivery or read receipts.** A successful send returns the `Message` and nothing later reports delivery or
  reading. The only engagement the bot sees is a callback button press; URL clicks must go through tracked links.
  Section 6.
- **Rich content.** Text up to 4096 characters with HTML, Markdown or entities; captions up to 1024; photos, video,
  documents and albums of 2 to 10; inline keyboards with URL and callback buttons; link previews controllable per
  message; a newer `sendRichMessage` for block-structured messages. Section 7.
- **Policy.** No Telegram rule targets promotional messages as such. The Developer Terms forbid harassing or spamming
  "users with unsolicited messages", describe broadcasting as being "to users who have granted permission to contact
  them", require a privacy policy and deletion of user data on request, and forbid operating "by proxy (i.e., using
  Bot API credentials supplied by other users) in an attempt to circumvent bans or content moderation". Section 8.
- **Telegram Login** is now an OpenID Connect provider; it returns the Telegram user id (in the `profile` scope) and,
  with `telegram:bot_access`, lets the bot message the user. It needs a second per-bot credential (Client ID and
  Secret) and pre-registered URLs. Section 9.

## 1. Who a bot may message

- "Bots can't start conversations with users. A user must either add them to a group or send them a message first.
  People can search for your bot's username or start a chat via its unique t.me/bot_username link"
  ([core.telegram.org/bots](https://core.telegram.org/bots#how-are-bots-different-from-users)).
- In private chats a bot receives every message: "All bots, regardless of settings, will receive: … All messages from
  private chats with users" ([FAQ#what-messages-will-my-bot-get](https://core.telegram.org/bots/faq#what-messages-will-my-bot-get)).
- **Write access without a message.** The `WriteAccessAllowed` service message exists for "a user allowing a bot to
  write messages after adding it to the attachment menu, launching a Web App from a link, or accepting an explicit
  request from a Web App sent by the method requestWriteAccess"
  ([API#writeaccessallowed](https://core.telegram.org/bots/api#writeaccessallowed)). Telegram Login has a scope
  `telegram:bot_access`: "Allows your bot to send direct messages to the user after login"
  ([telegram-login#available-scopes](https://core.telegram.org/bots/telegram-login#available-scopes)); the archived
  widget had a "Request Access … to send messages from your bot" option
  ([widgets/login-legacy](https://core.telegram.org/widgets/login-legacy)).
- **Identifiers.** `User.id` and `Chat.id` "may have more than 32 significant bits … But it has at most 52
  significant bits, so a signed 64-bit integer or double-precision float type are safe for storing"
  ([API#user](https://core.telegram.org/bots/api#user), [API#chat](https://core.telegram.org/bots/api#chat)). A
  `sendMessage` addresses `chat_id` ([API#sendmessage](https://core.telegram.org/bots/api#sendmessage)). A message
  update carries both the sender (`from`) and the chat (`chat`), so a bot that received `/start` holds the `chat.id`
  to send to.
- **Sending to a user the bot has never seen fails.** In the server source, a private chat with an unknown user fails
  with `400 Bad Request: private chat not found`
  ([Source Client.cpp#L8809-L8811](https://github.com/tdlib/telegram-bot-api/blob/e3e9dd8/telegram-bot-api/Client.cpp#L8809-L8811)),
  and a chat id the bot cannot resolve with `400 Bad Request: chat not found`
  ([Client.cpp#L7266](https://github.com/tdlib/telegram-bot-api/blob/e3e9dd8/telegram-bot-api/Client.cpp#L7266)).
- **`file_id` is per bot**: "file_id is unique for each individual bot and can't be transferred from one bot to
  another" ([API#sending-files](https://core.telegram.org/bots/api#sending-files)).

**Implication (inference).** Reachability belongs to a (bot, user) pair. A user who started Workspace A's bot is not
reachable by Workspace B's bot, and a second Telegram Integration in the same Workspace starts with nobody reachable.
Unlike an email address, a Telegram Destination is only usable through the bot it was collected by: the Destination
is in effect (Integration, chat id), not a bare id.

## 2. Binding a chat to a known Contact: `/start` deep links

- "Telegram bots have a deep linking mechanism that allows additional parameters to be passed to the bot on startup.
  It could be a command that launches the bot – or an authentication token to connect the user's Telegram account to
  their account on another platform"
  ([features#deep-linking](https://core.telegram.org/bots/features#deep-linking)).
- Format: "A-Z, a-z, 0-9, \_ and - are allowed. We recommend using base64url to encode parameters with binary and
  other types of content. The parameter can be up to 64 characters long." The link
  `https://t.me/your_bot?start=airplane` makes the bot "receive: `/start airplane`"
  ([features#deep-linking](https://core.telegram.org/bots/features#deep-linking)).
- `/start` is the conventional first command: it "begins the interaction with the user … This command can also be
  used to pass additional parameters to the bot (see Deep Linking)"
  ([features](https://core.telegram.org/bots/features)).
- Other ways to learn more about the user in the chat: a reply-keyboard button with `request_contact` sends "the
  user's phone number … as a contact when the button is pressed. Available in private chats only"
  ([API#keyboardbutton](https://core.telegram.org/bots/api#keyboardbutton)); the `Contact` object carries
  `phone_number` and `user_id` ([API#contact](https://core.telegram.org/bots/api#contact)). `User.language_code` is
  the "IETF language tag of the user's language" ([API#user](https://core.telegram.org/bots/api#user)).

**Flow this supports (inference).** A Contact known on the customer's site (identified by subject_id, email or phone)
is shown a link `t.me/<bot>?start=<token>`, where `<token>` is an opaque server-side reference, not the Contact's id
or email: the payload is visible in the link and arrives as plain message text. On `/start <token>` the webhook
resolves the token to the Contact and records the chat id as that Contact's Telegram Destination for this
Integration. Because anyone who opens the link binds their own Telegram account, the token should be short-lived and
single-use. A bare `/start` (no payload) yields a reachable chat with no known Contact, which can only become an
anonymous Contact or wait for a later binding (Login, a shared phone number).

## 3. Receiving updates: webhook, long polling, one consumer per bot

- "There are two mutually exclusive ways of receiving updates for your bot - the getUpdates method on one hand and
  webhooks on the other. Incoming updates are stored on the server until the bot receives them either way, but they
  will not be kept longer than 24 hours" ([API#getting-updates](https://core.telegram.org/bots/api#getting-updates)).
  `getUpdates` "will not work if an outgoing webhook is set up"
  ([API#getupdates](https://core.telegram.org/bots/api#getupdates)).
- `setWebhook` takes one `url`; `WebhookInfo` has a single `url` field
  ([API#setwebhook](https://core.telegram.org/bots/api#setwebhook),
  [API#webhookinfo](https://core.telegram.org/bots/api#webhookinfo)). So a bot has one webhook at a time.
- On a non-2xx response "we will repeat the request and give up after a reasonable amount of attempts"
  ([API#setwebhook](https://core.telegram.org/bots/api#setwebhook)). `update_id` lets the receiver "ignore repeated
  updates or … restore the correct update sequence" ([API#update](https://core.telegram.org/bots/api#update)).
- **Authenticating the webhook.** `secret_token`, 1–256 characters of `A-Z a-z 0-9 _ -`, is "sent in a header
  'X-Telegram-Bot-Api-Secret-Token' in every webhook request"
  ([API#setwebhook](https://core.telegram.org/bots/api#setwebhook)). The FAQ's older advice is a secret path in the URL
  ([FAQ#how-can-i-make-sure-that-webhook-requests-are-coming-from-telegram](https://core.telegram.org/bots/faq#how-can-i-make-sure-that-webhook-requests-are-coming-from-telegram)).
- Constraints: HTTPS; ports 443, 80, 88 and 8443; no redirects
  ([FAQ#im-having-problems-with-webhooks](https://core.telegram.org/bots/faq#im-having-problems-with-webhooks)).
  `max_connections` 1–100, default 40; `allowed_updates` filters update types, and by default excludes
  `chat_member`, `message_reaction` and `message_reaction_count`
  ([API#setwebhook](https://core.telegram.org/bots/api#setwebhook)).
- `getWebhookInfo` reports `pending_update_count`, `last_error_date` and `last_error_message`
  ([API#webhookinfo](https://core.telegram.org/bots/api#webhookinfo)), usable as an Integration health check.
- All calls go to `https://api.telegram.org/bot<token>/METHOD_NAME`; the response has `ok`, `result` or
  `description`, and an `error_code` whose "contents are subject to change in the future"
  ([API#making-requests](https://core.telegram.org/bots/api#making-requests)).

**Implication (inference).** The Integration can register its own webhook with `setWebhook` at connect time (the
customer pastes only the token), with a per-Integration URL and secret. That matches ADR 0037's "one URL is one
Integration" and its shared-token verify, but ADR 0037's transform returns Delivery failures only, while Telegram's
webhook carries `/start` bindings, blocks and button presses: the inbound side for Telegram needs more than that
fixed output, which reopens ADR 0037. Connecting a bot that already serves something else (a support bot) replaces
that service's webhook, so the customer should connect a bot dedicated to sphericon.

## 4. Rate limits

- From the FAQ ([FAQ#my-bot-is-hitting-limits-how-do-i-avoid-this](https://core.telegram.org/bots/faq#my-bot-is-hitting-limits-how-do-i-avoid-this)):
  - "In a single chat, avoid sending more than one message per second. We may allow short bursts that go over this
    limit, but eventually you'll begin receiving 429 errors."
  - "In a group, bots are not be able to send more than 20 messages per minute."
  - "For bulk notifications, bots are not able to broadcast more than about 30 messages per second, unless they
    enable paid broadcasts to increase the limit."
  - "consider spreading them over longer intervals (e.g. 8-12 hours) to avoid hitting the limit. The API will not
    allow bulk notifications to more than ~30 users per second – if you go over that, you'll start getting 429
    errors" ([FAQ#how-can-i-message-all-of-my-bots-subscribers-at-once](https://core.telegram.org/bots/faq#how-can-i-message-all-of-my-bots-subscribers-at-once)).
- **Paid broadcasts.** With paid broadcasts enabled a bot may "broadcast up to 1000 messages per second. Each message
  broadcasted over the free amount of 30 per second incurs a cost of 0.1 Stars per message … a bot must have at least
  100,000 Stars on its balance and at least 100,000 monthly active users"
  ([FAQ](https://core.telegram.org/bots/faq#how-can-i-message-all-of-my-bots-subscribers-at-once)); the per-call
  switch is `allow_paid_broadcast` ([API#sendmessage](https://core.telegram.org/bots/api#sendmessage)); the same
  terms are in [Developer Terms 6.2.5](https://telegram.org/tos/bot-developers#6-2-5-broadcasting-messages-with-stars).
- **The 429 shape.** `ResponseParameters.retry_after` is "the number of seconds left to wait before the request can
  be repeated" ([API#responseparameters](https://core.telegram.org/bots/api#responseparameters)); the server builds
  the text `Too Many Requests: retry after <n>`
  ([Client.cpp#L62-L71](https://github.com/tdlib/telegram-bot-api/blob/e3e9dd8/telegram-bot-api/Client.cpp#L62-L71)).
- The Developer Terms forbid circumventing limits: "Your TPA must not attempt to circumvent or otherwise undermine
  Telegram rate limits and moderation" ([Developer Terms 5.2(f)](https://telegram.org/tos/bot-developers#5-2-operation)).

**Implication (inference).** The provider limit of a Telegram Integration is about 30 per second and is not readable
by API; it is a documented constant, fitting ADR 0023's customer-set Send rate limit with a channel default, and
ADR 0067's provider limit. 30 per second is 108,000 an hour, so a Broadcast to a large audience takes hours, as the
FAQ itself advises. A 429 is a Deferral with a known wait. The per-chat limit (1 per second) matters only if one
Contact gets several messages in a burst. Since the paid mode is the customer's own bot setting and is used per call,
the Integration could expose it as an option; the open app says nothing about its cost.

## 5. Signals that a chat is gone or the Integration is broken

- **Blocked.** The server maps Telegram's `USER_IS_BLOCKED` to code 403 and "bot was blocked by the user"
  ([Client.cpp#L127-L129](https://github.com/tdlib/telegram-bot-api/blob/e3e9dd8/telegram-bot-api/Client.cpp#L127-L129)),
  and prefixes 403 messages with `Forbidden: `
  ([Client.cpp#L169-L203](https://github.com/tdlib/telegram-bot-api/blob/e3e9dd8/telegram-bot-api/Client.cpp#L169-L203)),
  so the client sees `403 Forbidden: bot was blocked by the user`.
- **Deleted account.** `INPUT_USER_DEACTIVATED` becomes `403 Forbidden: user is deactivated`
  ([Client.cpp#L124-L126](https://github.com/tdlib/telegram-bot-api/blob/e3e9dd8/telegram-bot-api/Client.cpp#L124-L126),
  [#L8807](https://github.com/tdlib/telegram-bot-api/blob/e3e9dd8/telegram-bot-api/Client.cpp#L8807)).
- **Block without a send.** `my_chat_member`: "The bot's chat member status was updated in a chat. For private
  chats, this update is received only when the bot is blocked or unblocked by the user"
  ([API#update](https://core.telegram.org/bots/api#update)). The update is a `ChatMemberUpdated` with `old_chat_member`
  and `new_chat_member` ([API#chatmemberupdated](https://core.telegram.org/bots/api#chatmemberupdated)); member
  statuses include `member`, `left` and `kicked` ([API#chatmemberbanned](https://core.telegram.org/bots/api#chatmemberbanned)).
- **Token revoked or wrong.** `401 Unauthorized: invalid token specified`
  ([ClientManager.cpp#L76](https://github.com/tdlib/telegram-bot-api/blob/e3e9dd8/telegram-bot-api/ClientManager.cpp#L76),
  [#L84](https://github.com/tdlib/telegram-bot-api/blob/e3e9dd8/telegram-bot-api/ClientManager.cpp#L84)).
- **Error codes are not a stable contract.** `error_code`'s "contents are subject to change in the future"
  ([API#making-requests](https://core.telegram.org/bots/api#making-requests)).

**Implication (inference).** A block is the user's Telegram-level opt-out, and it reaches sphericon two ways: a 403 on
the next send, and a `my_chat_member` update at the moment of blocking. Both map to (channel, destination) consent or
Suppression. An unblock is also reported, so a block is reversible by the user, unlike a hard bounce. A 401 belongs
to the Integration, not to a Contact, and should stop sends through it. Classification should use code plus
description plus the update type, not the code alone.

## 6. Delivery and engagement feedback

- On success `sendMessage` returns "the sent Message" ([API#sendmessage](https://core.telegram.org/bots/api#sendmessage)).
- The `Update` type lists every event a bot can receive
  ([API#update](https://core.telegram.org/bots/api#update)); none of them reports that a message was delivered to a
  device or read. Reactions (`message_reaction`) require that "The bot must be an administrator in the chat".
- A callback button (`callback_data`, "1-64 bytes") sends a `callback_query` to the bot when pressed
  ([API#inlinekeyboardbutton](https://core.telegram.org/bots/api#inlinekeyboardbutton)). A `url` button opens the URL
  with no report to the bot.
- For inline text links, "Telegram clients will display an alert to the user before opening an inline link ('Open
  this link?' together with the full URL)" ([API#formatting-options](https://core.telegram.org/bots/api#formatting-options)).
- When a user follows a link, "the bot can potentially get your IP address (provided that it controls the website to
  which the link leads)" ([Privacy Policy 6.3](https://telegram.org/privacy#6-3-what-data-bots-receive)).

**Implication (inference).** For ADR 0053, a Telegram send has "sent" (accepted by Telegram) and a send failure, never
"delivered" or "opened". Clicks come from sphericon's own tracked links (with the full tracking URL shown in the
"Open this link?" alert for inline links) and from callback buttons, which arrive on the webhook.

## 7. Message content

- **Text.** `text` is "1-4096 characters after entities parsing"; `parse_mode` or `entities`;
  `link_preview_options`; `disable_notification` ("Users will receive a notification with no sound");
  `protect_content` ("from forwarding and saving"); `reply_markup`
  ([API#sendmessage](https://core.telegram.org/bots/api#sendmessage)).
- **Formatting.** "bold, italic, underlined, strikethrough, spoiler text, block quotations as well as inline links and
  pre-formatted code … You can specify text entities directly, or use markdown-style or HTML-style formatting"
  ([API#formatting-options](https://core.telegram.org/bots/api#formatting-options)).
- **Link previews.** `LinkPreviewOptions` has `is_disabled`, `url`, `prefer_small_media`, `prefer_large_media` and
  `show_above_text` ([API#linkpreviewoptions](https://core.telegram.org/bots/api#linkpreviewoptions)).
- **Media.** A photo caption is "0-1024 characters after entities parsing"
  ([API#sendphoto](https://core.telegram.org/bots/api#sendphoto)); the other media methods state the same range. Files can be sent by `file_id`, by URL ("5 MB max size for
  photos and 20 MB max for other types of content") or by upload ("10 MB max size for photos, 50 MB for other
  files") ([API#sending-files](https://core.telegram.org/bots/api#sending-files)). `sendMediaGroup` sends an album
  that "must include 2-10 items" ([API#sendmediagroup](https://core.telegram.org/bots/api#sendmediagroup)).
- **Buttons.** `InlineKeyboardMarkup` is rows of `InlineKeyboardButton`; a button has `text` and exactly one action:
  `url`, `callback_data`, `web_app`, `login_url` and others; an optional `style` is "danger", "success" or "primary"
  ([API#inlinekeyboardbutton](https://core.telegram.org/bots/api#inlinekeyboardbutton)).
- **Rich messages** (Bot API 10.1–10.3, June–August 2026). `sendRichMessage` sends an `InputRichMessage` given as
  `html`, `markdown` or `blocks`, with embedded media referenced by `tg://photo?id=` style links
  ([API#sendrichmessage](https://core.telegram.org/bots/api#sendrichmessage),
  [API#rich-message-formatting-options](https://core.telegram.org/bots/api#rich-message-formatting-options)).

**Implication (inference).** The Telegram content model is: text with a small markup (bold, italic, links, quotes),
an optional single media or an album with a caption limit, an optional button grid, and per-message preview and
silent flags. Media sent by URL is the simplest across bots, since `file_id` cannot cross bots.

## 8. Telegram's rules on promotional and bulk messaging

- Telegram ToS: users agree not to "Use our service to send spam or scam users" ([telegram.org/tos](https://telegram.org/tos)).
- Developer Terms 5.2: "(b) Your TPA must not harass or spam users with unsolicited messages" and "(f) … your TPA must
  not operate by proxy (i.e., using Bot API credentials supplied by other users) in an attempt to circumvent bans or
  content moderation" ([Developer Terms 5.2](https://telegram.org/tos/bot-developers#5-2-operation)). The same section
  requires that "users must always be informed of any significant changes pertaining to the operational scope of
  your TPA, and should be given an option to opt out of these changes".
- Developer Terms 6.2.5: "By default, TPA are able to broadcast up to 30 messages per second to users who have granted
  permission to contact them" and those on raised limits "must ensure that their broadcasts are strictly conducted in
  good faith, for legitimate business purposes … It is expressly prohibited to exploit increased limits to spam or
  harass users in any capacity"
  ([Developer Terms 6.2.5](https://telegram.org/tos/bot-developers#6-2-5-broadcasting-messages-with-stars)).
- Privacy and retention: "all TPA must be bound by a privacy policy that is easily accessible to their users"; a
  Standard Privacy Policy applies to bots without their own, and a bot that needs a different one sets it in
  @BotFather ([Developer Terms 4](https://telegram.org/tos/bot-developers#4-privacy)). A bot must "Delete user data
  upon their (or our …) request" and when "retention thereof becomes unnecessary"
  ([Developer Terms 4.2](https://telegram.org/tos/bot-developers#4-2-data-storage-and-retention)). Data "submitted
  directly and voluntarily" may be used if users "give their individual, explicit, active and revocable consent"
  ([Developer Terms 4.3](https://telegram.org/tos/bot-developers#4-3-data-scraping)).

**Implication (inference).** Telegram does not separate promotional from service messages the way WhatsApp templates
do; its line is "unsolicited". A user who started the bot (or granted write access) is the permission the Terms
name. A per-Workspace bot with the customer's own token is the customer's TPA, not proxying to evade a ban; pooling
one Workspace's sends through another's token, or rotating tokens after a ban, would be the circumvention 5.2(f)
names. Deletion on request feeds the map's open erasure item for chat ids.

## 9. Telegram Login as another id source

- The current page describes "the Telegram Login library and the new OpenID Connect login flow"; the iframe widget is
  archived ([telegram-login](https://core.telegram.org/bots/telegram-login)).
- A bot represents the app; its owner registers "Allowed URLs" in @BotFather and gets a Client ID and Client Secret;
  "Telegram will only process logins or redirect users using your pre-registered URLs"
  ([telegram-login](https://core.telegram.org/bots/telegram-login)).
- OIDC: Authorization Code Flow with PKCE, discovery at `https://oauth.telegram.org/.well-known/openid-configuration`.
  Scopes: `openid` (required; `sub`, `iss`, `iat`, `exp`), `profile` (`id`, `name`, `preferred_username`,
  `picture`), `phone` (`phone_number`, "Requires user consent") and `telegram:bot_access` ("Allows your bot to send
  direct messages to the user after login")
  ([telegram-login#available-scopes](https://core.telegram.org/bots/telegram-login#available-scopes)).
- The ID token is a JWT to be verified against the JWKS: "Check that iss is https://oauth.telegram.org, aud matches
  your Bot ID". Its documented example has `sub` "1234123412341234123" and `id` 987654321, different values
  ([telegram-login#user-data-structure](https://core.telegram.org/bots/telegram-login#user-data-structure)).
- The browser library's `scope` option takes `profile`, `phone` and `write` ("permission for your bot to message the
  user") and returns the `id_token` to a JS callback ([telegram-login](https://core.telegram.org/bots/telegram-login)).
- The legacy widget returned `id, first_name, last_name, username, photo_url, auth_date and hash`, verified by
  HMAC-SHA-256 keyed with `SHA256(<bot_token>)` ([widgets/login-legacy](https://core.telegram.org/widgets/login-legacy)).

**Implication (inference).** Login gives the customer's site the Telegram user id plus, with `write`/`bot_access`, the
right for the bot to message that user, without the user opening the bot. To be a chat target, the `profile` scope
is needed (`sub` is not the Bot API id). It needs per-bot setup the deep link does not: a Client Secret for the code
flow (the JS library's `id_token` is verifiable with the public JWKS and the bot id as `aud`) and the customer's site
URLs registered in @BotFather. The deep link remains the simpler binding; Login is an alternative for sites that
already show a Telegram sign-in.

## Unconfirmed

- **Which `ChatMember` status a private-chat block carries** in `my_chat_member` (commonly `kicked`, back to `member`
  on unblock): the reference says the update is sent on block and unblock but not which statuses it holds.
- **Whether a private chat's `chat.id` equals the user's `User.id`.** No primary page states it; Login and
  `bot_access` imply sending to the user id, but the Bot API reference does not say so.
- **Whether a Login with `bot_access` produces any update to the bot.** `WriteAccessAllowed` lists Mini App and
  attachment-menu cases, not Login.
- **Whether `/start <payload>` is delivered when the user already has a chat with the bot** (opening the link again):
  the features page describes only "When someone opens a chat with your bot via this link".
- **The text `Forbidden: bot can't initiate conversation with a user`**, which libraries report for a user who never
  started the bot, is not in `tdlib/telegram-bot-api` at `e3e9dd8`; it would come from the Telegram servers. The source
  read shows `private chat not found` for a user the bot has never seen.
- **Whether `message_reaction` updates reach a bot in a private chat**: the field says the bot must be an
  administrator in the chat, which has no meaning in a private chat.
- **Exact webhook retry schedule** ("a reasonable amount of attempts") and whether undelivered webhook updates count
  toward the 24-hour retention.
