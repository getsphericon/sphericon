# SMS providers: send APIs, delivery reports, sender IDs and STOP handling

Researched November 2026 for [#917](https://github.com/getsphericon/sphericon/issues/917) (map
[#916](https://github.com/getsphericon/sphericon/issues/916)), from primary sources only. Those are the providers' own
API references and help centres (Twilio, Vonage, Sinch, Plivo, Bird and legacy MessageBird, SMSC, SMS.ru, SMS Aero,
MTS Exolve), the US eCFR rule text as published in the Federal Register, FCC orders, and Russian statutes on
consultant.ru. No blog or comparison site is cited. The Twilio and Vonage help centres block scripted fetches, so their
article bodies were read through each help centre's public Zendesk JSON API and are cited here by their `html_url`.
smsc.ru serves windows-1251 and was read as raw text. SMS Aero's docs are rendered by JavaScript, so its facts come from
the rendered page and are marked where they could not be checked word for word. A claim that no primary source
confirmed is listed under "Unconfirmed" at the end. Prices are left out on purpose: the open app says nothing about
them.

sphericon's side is read from [ADR 0037](../adr/0037-inbound-provider-callbacks-are-presets-on-an-integration.md)
(inbound callbacks are Presets on an Integration),
[ADR 0053](../adr/0053-delivery-and-engagement-events-cover-every-send.md) (Delivery notices, the provider message id)
and [ADR 0067](../adr/0067-the-send-cap-is-the-platforms-limit-the-provider-limit-the-customers.md) (Provider limit,
Send cap).

## Question

What does sending marketing SMS through a customer's own provider look like at the API level, and what does the
platform have to handle itself? That covers the send API and credentials, sender IDs, delivery reports, STOP and
opt-out, encoding and segments, throughput and inbound webhooks, for five global providers and four Russian ones.

## Summary of findings

- **One provider message id per send is not a given.** Twilio returns one SID per message. Vonage's SMS API returns one
  `message-id` per part, with a DLR per part. Sinch returns a batch id, with status keyed by (batch, recipient). Plivo
  returns a `message_uuid` array. SMSC gives every recipient of a request the same id, the id is "не обязательно
  уникален", and status is looked up by (id, phone). The Outbound message needs a provider reference that can hold
  (id, destination), or the adapter must send one recipient per request.
- **"Delivered" is weaker than it sounds, and many messages never get a final DLR.** Twilio's `delivered` is
  confirmation "from the carrier, (and, where available, the destination handset)", and Vonage says some carrier DLRs
  "may be fake". Twilio and Plivo `sent` can stay final when no receipt arrives. Sinch has a final `Unknown`, Vonage
  `unknown`, Exolve `delivery_status` 0 and SMSC `err` 245. The model needs four outcomes: sent, delivered, failed and
  unknown. Callbacks also arrive out of order (Twilio, Bird), so a later report must never move a message back.
- **Providers that handle STOP scope the block to the sender, not the person.** Twilio blocks per number and per
  Messaging Service, Vonage per number, Plivo per number or per 10DLC campaign, and Bird per (sender, subscriber).
  Coverage is partial too: Twilio filters long codes and toll-free numbers (no country limit stated, short codes only
  with Advanced Opt-Out), Vonage and Plivo cover only US/Canada number types, and Bird only its catalogue countries.
  Consent keyed by (channel, destination) is therefore broader, and the platform must handle STOP itself.
- **An opt-out reaches the sender in three ways.** The STOP may be forwarded as an inbound message (Twilio, Vonage,
  Bird, SMSC replies). It may arrive as a provider event (Bird `sms_suppression.created`). Or it may show only as a
  refused send with a provider-specific code: Twilio 21610, Vonage err-code 9 or Messages 1240, Plivo 200, Bird
  E12077 or E25000, SMS.ru 209 or 215. A Preset has to map all three to one consent signal. Ambiguous codes (SMSC 23,
  Sinch `Deleted`, Exolve 6) also cover other refusals, so they are failures, not opt-outs.
- **Russia has no keyword STOP at all.** None of the four Russian providers documents STOP-keyword handling; each
  offers a stop list or blacklist API instead (except Exolve, whose blacklist looks call-only). Since 2025 a
  subscriber can also refuse mailings at their mobile operator (126-FZ art. 44.1 part 1.1), which the sender never
  sees except as rejected or undelivered DLRs. 38-FZ art. 18 requires the advertiser to prove prior consent and to
  stop "немедленно" on request.
- **Sender IDs are per country and mostly registered.** An alphanumeric sender has at most 11 characters, is one-way
  (no replies, so no STOP by reply), is not allowed in the US or Canada, and needs registration in many countries.
  US traffic needs a registered 10DLC brand and campaign, a verified toll-free number or a short code. In Russia every
  alpha name is approved per mobile operator, and at Exolve only legal entities and sole proprietors may register one.
- **Webhook authentication varies, and Russia mostly has none.** Bird uses a Standard-Webhooks-style signature.
  Twilio signs HMAC-SHA1 over the URL plus sorted form fields with the Auth Token. Vonage's Messages API sends a JWT
  with a body hash, while its SMS API is unsigned by default. Plivo V2 signs the URL and a nonce but not the body.
  SMSC can add a hash of a few fields with a secret. SMS.ru, SMS Aero and Exolve document no authentication. ADR
  0037's closed verify set would need a Twilio-style scheme, and for the Russian providers the per-Integration ingest
  key in the URL is the only protection.
- **Segments drive the count estimate.** GSM-7 allows 160 characters, or 153 per part. UCS-2 allows 70, or 67 per
  part. Any character outside GSM-7, which includes all Cyrillic, switches the whole message to UCS-2. Caps on parts
  differ: SMS.ru 8, Exolve 10, Bird 12, SMS Aero 14 Cyrillic or 6 Latin, Twilio 1,600 characters, Sinch 2,000
  characters. Link shortening is offered by Twilio (own domain, click webhook), SMSC and SMS Aero, and SMS Aero warns
  that operators sometimes block shortened links.
- **Throughput lives at the provider, per sender type.** Twilio allows 1 MPS on a long code abroad, 3 on toll-free, 10
  on US to other countries and 100 on a short code, with a 10-hour queue per sender. Vonage allows 30 requests/s per
  API key, Plivo 5 MPS per account, and Sinch a rate per service plan. A 429, or a "throttled" status, is the common
  signal, which fits the Deferral of ADR 0023/0067.

## 1. Global providers

### 1.1 Twilio (Programmable Messaging)

**Send.**

- `POST https://api.twilio.com/2010-04-01/Accounts/{AccountSid}/Messages.json`, form-encoded, one `To` per request.
  Auth is an API Key SID and Secret, or the Account SID and Auth Token. `From` takes an E.164 number, an alphanumeric
  sender ID, a short code or a channel address. `MessagingServiceSid` lets Twilio pick from a sender pool instead. The
  returned id is `SM…` followed by 32 hex characters. `Body` takes up to 1,600 characters. Other parameters:
  `ValidityPeriod` (1–36000 s, default 36000), a per-message `StatusCallback`, `SmartEncoded`, and `ShortenUrls`
  (Messaging Service only). <https://www.twilio.com/docs/messaging/api/message-resource>
- Webhook verification needs the **Auth Token** even when sending with an API key: `X-Twilio-Signature` is HMAC-SHA1
  keyed with the account Auth Token over the full URL plus the POST parameters, sorted and appended. "If you decode or
  re-encode the URL, signature validation fails." <https://www.twilio.com/docs/usage/webhooks/webhooks-security>

**Delivery reports.**

- **Where they come from:** a per-message `StatusCallback`, or the Messaging Service's delivery status callback. There
  is no list endpoint for reports; you GET the Message resource.
  <https://www.twilio.com/docs/messaging/guides/track-outbound-message-status>
- **Ordering:** "no guarantee that the status callback requests always arrive at your endpoint in the order they were
  sent". No callback is sent for the initial status.
  <https://www.twilio.com/docs/messaging/guides/outbound-message-status-in-status-callbacks>
- **States:** `queued`, `accepted`, `scheduled`, `canceled`, `sending`, `sent`, `delivered`, `undelivered`, `failed`,
  `receiving`, `received` and `read` (RCS/WhatsApp only). `ErrorCode` is set on failed and undelivered messages, but
  "Users should not use the `error_code` and `error_message` fields programmatically".
  <https://www.twilio.com/docs/messaging/api/message-resource>
- **Final states:** `sent`, `delivered`, `undelivered` and `failed`. `delivered` means "confirmation of message delivery
  from the carrier, (and, where available, the destination handset)". If no receipt arrives the message stays `sent`:
  "Once a message is older than 72 hours, receiving a further status is unlikely."
  <https://support.twilio.com/hc/en-us/articles/223134347-Describing-SMS-and-MMS-Message-Statuses>
- **Retries:** by default one retry (`rc=1`), and only on a TCP connect or TLS failure (`rp=ct`), not on a 4xx or 5xx.
  The connect timeout is 5 s and the total 15 s. URL fragments such as `#rc=5&rp=all` override this.
  <https://www.twilio.com/docs/usage/webhooks/webhooks-connection-overrides>

**STOP / opt-out.**

- **Default handling:** "By default, Twilio handles standard English-language reply messages such as STOP… for
  Toll-Free and Long Code messages". The opt-out keywords are STOP, STOPALL, UNSUBSCRIBE, CANCEL, END, REVOKE, OPTOUT
  and QUIT; START, YES and UNSTOP opt back in, and HELP and INFO ask for help. Only a single-word message triggers the
  block. Twilio adds a block-list entry "and then pass[es] the message on to your webhook". Later sends fail with
  error 21610. <https://support.twilio.com/hc/en-us/articles/223134027-Twilio-Support-for-Opt-Out-Keywords-SMS-STOP-Filtering>,
  <https://www.twilio.com/docs/api/errors/21610>
- **Scope:** the number that received the STOP, plus the Messaging Service. "If you are using more than one Toll-Free
  number or Long Code… you must take additional action separate from Twilio." Short codes are not handled unless
  Advanced Opt-Out is on. Alphanumeric senders are "one-way messaging only… STOP, START, HELP keywords are not
  applicable". On toll-free numbers STOP is handled by the carrier. From 2026-03-16, opt-outs are unified across SMS,
  MMS and RCS per sender. Same article.
- **Advanced Opt-Out:** per Messaging Service and off by default. It adds `OptOutType` (START, STOP or HELP) to the
  inbound webhook and supports languages and country overrides. It "does not support changing or reporting on blocked
  phone numbers via the Console or the REST API"; the support FAQ points to a Consent Management API instead.
  <https://www.twilio.com/docs/messaging/tutorials/advanced-opt-out>

**Inbound.** Inbound messages are POSTed to the message URL of the number or Messaging Service, and Twilio expects
TwiML in response. The fields are `MessageSid`, `From`, `To`, `Body`, `NumSegments`, `MessagingServiceSid` and
others. The request is signed the same way as a status callback.
<https://www.twilio.com/docs/messaging/guides/webhook-request>

**Sender IDs.**

- **Alphanumeric:** up to 11 characters (letters, digits, space), "can't be numbers only".
  <https://www.twilio.com/docs/glossary/what-alphanumeric-sender-id>
- **By country:** a table marks each country Yes, Registration Required or No. The US and Canada are No.
  <https://support.twilio.com/hc/en-us/articles/223133767-International-Support-for-Alphanumeric-Sender-ID>
- **US long codes (10DLC):** brand and campaign registration is required for everyone, "including individuals and
  hobbyists" (<https://www.twilio.com/docs/messaging/compliance/a2p-10dlc>). Without it, sends are blocked with
  error 30034 (<https://www.twilio.com/docs/api/errors/30034>).
- **US toll-free:** an unverified toll-free number gets error 30032. <https://www.twilio.com/docs/api/errors/30032>

**Encoding.**

- GSM-7 allows 160 characters, or 153 per segment; UCS-2 allows 70, or 67 per segment. Toll-free traffic to US/CA gets
  152 / 66 per segment. The maximum is 1,600 characters, and Twilio recommends staying under 320.
- Smart Encoding "replaces certain non-GSM characters with equivalent GSM characters".

<https://www.twilio.com/docs/glossary/what-sms-character-limit>

**Link shortening.** Works on a Messaging Service with a domain you own. It sends a click webhook with `sms_sid`, `to`,
`link` and `click_time`, and keeps links for 90 days.
<https://www.twilio.com/docs/messaging/features/link-shortening>

**Throughput.**

- **Queue:** each sender has a queue holding "up to 10 hours' worth of message segments". Some accounts are moving to
  account-level or campaign-level throughput.
- **MPS by sender type:**
  - a long code to other countries: 1
  - US/CA toll-free: 3
  - a US number or an alphanumeric sender to other countries: 10
  - a short code: 100
- **Unverified accounts:** at most 20,000 messages a day.

<https://support.twilio.com/hc/en-us/articles/115002943027-Understanding-Twilio-Rate-Limits-and-Message-Queues>

- **Overload:** a REST overload is HTTP 429 with error 20429, "safe to retry after backing off"
  (<https://www.twilio.com/docs/api/errors/20429>). A queue overflow is error 30001
  (<https://www.twilio.com/docs/messaging/guides/scaling-queueing-latency>).

### 1.2 Vonage

Vonage has two send APIs: the older SMS API and the Messages API.

**SMS API.**

- **Send:** `POST https://rest.nexmo.com/sms/json`, one `to`. Auth is Basic `api_key:api_secret`, or a signed request
  using the separate signature secret (<https://developer.vonage.com/en/getting-started/concepts/signing-messages>).
  `from` is a number or a name, and alphanumeric is "not supported in all countries". The response holds one
  `message-id` per part in `messages[]`. A per-message `callback` overrides the dashboard DLR URL.
  <https://developer.vonage.com/en/api/sms>
- **Send status 0 is acceptance, not delivery.** Status 1 is Throttled.
  <https://developer.vonage.com/en/messaging/sms/guides/troubleshooting-sms>
- **DLR statuses:** `accepted`, `delivered`, `buffered`, `expired`, `failed`, `rejected` and `unknown`.
- **Carrier and handset receipts:** "Carrier – returned when the service provider receives the message"; "Handset –
  returned when the user's handset receives the message". Some carrier DLRs "may be fake". "You should receive a
  carrier DLR for each part of the concatenated SMS."
- **`err-code` 9:** "Recipient opted out of messages from this service". Codes 50–53 are regulatory refusals (entity,
  header, content ID, consent). <https://developer.vonage.com/en/messaging/sms/guides/delivery-receipts>
- **Inbound:** a webhook set per account or per number. Long messages arrive **in parts** (`concat-ref`,
  `concat-part`), and Vonage resends "for the next 24 hours" until it gets a 2xx.
  <https://developer.vonage.com/en/messaging/sms/guides/inbound-sms>
- **Signing:** inbound and DLR webhooks are unsigned by default. "To enable either signed webhooks… please contact
  support."

**Messages API.**

- **Send:** `POST https://api.nexmo.com/v1/messages` with `channel: sms`. Auth is a JWT (application) or Basic. `text`
  takes up to 3,200 characters, and `sms.encoding_type` is `auto` by default. The response is 202 with
  `message_uuid`.
- **Status webhook:** `submitted`, `delivered`, `rejected` or `undeliverable`, with an `error` object and per-segment
  statuses. Inbound messages carry `sms.keyword`. <https://developer.vonage.com/en/api/messages>
- **Signing is on by default:** an HS256 JWT in `Authorization` carries `payload_hash`, the SHA-256 of the body. "During
  internal errors… may send an unsigned callback". <https://developer.vonage.com/en/messages/concepts/signed-webhooks>

**STOP / opt-out.**

- **Opt-Out Assist** is something the customer "can elect". It covers 10DLC and short-code traffic in the US and Canada
  only, and "applies only to the number the end user texts STOP to". The STOP is still forwarded to the inbound
  webhook. No opt-out list API is documented. <https://developer.vonage.com/en/opt-out-assist/overview>,
  <https://developer.vonage.com/en/opt-out-assist/technical-details>
- **A send to an opted-out number is accepted, then fails:** a DLR with err-code 9 on the SMS API, or `rejected` with
  error 1240 on the Messages API. <https://developer.vonage.com/en/opt-out-assist/guides/messaging-opted-out-numbers>

**Sender IDs.**

- An alphanumeric sender has "up to 11 supported characters", and "a message recipient can only respond if the `from`
  parameter contains a valid mobile number". <https://developer.vonage.com/en/messaging/sms/guides/custom-sender-id>
- "Alphanumeric sender ID is not allowed in US/CA", and "The US requires opt-out in response to STOP messages".
  <https://api.support.vonage.com/hc/en-us/articles/204017023-United-States-SMS-Features-and-Restrictions>

**Encoding and throughput.**

- GSM allows 160 characters, or 153 per part; Unicode allows 70. The SMS API does not detect the encoding (you set
  `type=unicode`), and Vonage recommends at most 6 parts.
  <https://developer.vonage.com/en/messaging/sms/guides/concatenation-and-encoding>
- "All API keys are set with 30 API requests per second… by default", and US numbers add their own limits.
  <https://api.support.vonage.com/hc/en-us/articles/203993598-What-is-the-Throughput-Limit-for-Outbound-SMS>

### 1.3 Sinch (SMS REST API)

**Send.**

- `POST https://{us|eu|au|br|ca}.sms.api.sinch.com/xms/v1/{service_plan_id}/batches` with `Authorization: Bearer
  <API token>`. The credentials are a service plan id, a token and a region.
  <https://developers.sinch.com/docs/sms/getting-started>
- A batch takes 1–1000 recipients in `to[]`. `from` is a "valid phone number, short code or alphanumeric". `body` takes
  up to 2,000 characters. `expire_at` defaults to, and is capped at, 3 days. The response is a batch `id`.
- Reports are chosen per batch with `delivery_report`: none, summary, full, `per_recipient` or `per_recipient_final`,
  sent to an optional `callback_url`.

<https://developers.sinch.com/docs/sms/api-reference/sms/tag/Batches/>

**Delivery reports.**

- **Polling and states:** reports can be polled per batch and per recipient.
  - Queued and Dispatched are intermediate.
  - Final states are Aborted, Cancelled, Rejected, Delivered, Failed, Expired, **Deleted** ("e.g., for an invalid
    number or an opted-out subscriber") and **Unknown** ("no usable delivery receipt").

  <https://developers.sinch.com/docs/sms/api-reference/sms/tag/Delivery-reports/>

- **Error codes:** 402 unroutable, 404 temporary failure, 411 parts limit and others. No REST error code for opt-out is
  documented. <https://developers.sinch.com/docs/sms/api-reference/sms/delivery-reports/delivery-report-error-codes>
- **Retries:** a 5xx or 429 is retried with a doubling back-off from 5 s; the "last retry [is] 22 hours 45 minutes
  after the initial failed attempt".
- **Authentication:** Basic, OAuth 2.0, HMAC or SNS, configured by the account manager. The HMAC header and algorithm
  are not public.

<https://developers.sinch.com/docs/sms/api-reference/sms/tag/Webhooks/>

**Inbound.** The `incomingSMS` callback (`from`, `to`, `body`, `received_at`) goes to a URL set in the Dashboard, and
inbound messages can also be listed through the API.
<https://developers.sinch.com/docs/sms/api-reference/sms/inbounds/incomingsms>

**Encoding and throughput.**

- **Encoding:** GSM-7 allows 160 characters, or 153 per part; UCS-2 allows 70, or 67 per part. "The API automatically
  detects the encoding", and a body over 2,000 characters returns 400.
  <https://developers.sinch.com/docs/sms/resources/message-info/character-support>
- **Throughput:** "Each service plan comes with a rate limit", and "A batch with 10 recipients will count as 10
  messages". Each plan has its own first-in-first-out queue.
  <https://developers.sinch.com/docs/sms/resources/message-info/rate-limits>

### 1.4 Plivo

**Send.**

- `POST https://api.plivo.com/v1/Account/{auth_id}/Message/` with Basic `auth_id:auth_token`.
- `src` takes a number, short code or alphanumeric sender, or `powerpack_uuid` names a number pool. `dst` takes several
  numbers joined by `<`, and the response returns a `message_uuid` **array**.
- `url` sets the per-message status callback. `message_expiry` defaults to 10,800 s.

<https://www.plivo.com/docs/messaging/api/message/send-a-message>

**Delivery reports.**

- **Callback:** sent "as and when the message status changes". Fields include `MessageUUID`, `Status`, `ErrorCode`,
  `Sequence` and the queued, sent and delivery-report times.
  <https://docs.plivo.com/docs/messaging/api/message/message-status-callbacks>
- **States:** queued, sent, failed, delivered and undelivered. "The final status of a message may remain as sent in
  cases where delivery receipts are not received."
  <https://docs.plivo.com/docs/messaging/api/message/the-message-object>
- **Error codes:** **200 is "opted out" (STOP)**, 30 is a spam filter, 420 means queued for more than 3 hours, and 1000
  is unknown. <https://www.plivo.com/docs/messaging/troubleshooting/error-codes>
- **Signing:** `X-Plivo-Signature-V2` is HMAC-SHA256 with the Auth Token over the URL plus a nonce, **without the
  body**. <https://www.plivo.com/docs/messaging/concepts/signature-validation>

**STOP / opt-out ("DND service").**

- **Keywords:** STOP, END, QUIT, CANCEL, UNSUBSCRIBE, UNSUB and STOP ALL opt out; START, YES, RESUME, UNSTOP and GO opt
  back in.
- **Coverage:** on by default for US/CA long codes and toll-free numbers; short codes need Support to enable it.
- **Scope and effect:** the block covers the number, or every number on the same 10DLC campaign. Blocked sends fail
  with error 200.

<https://www.plivo.com/docs/messaging/concepts/dnd-service>

Whether the STOP is forwarded to `message_url`, and whether an opt-out list API exists, are not documented.

**Sender IDs and throughput.**

- **Alphanumeric senders** are "Country-dependent": Dynamic, Pre-registration, or Not Supported.
  <https://docs.plivo.com/docs/faq/messaging/messaging-compliance>
- **Throughput:** an account allows 5 SMS MPS by default, and an unregistered US long code 1 msg/s and 2,000 a day.
  "Messages remaining in queue after 3 hours expire automatically". <https://www.plivo.com/docs/faq/messaging/messaging-api>
- **Overload:** a 429 asks the client to back off. <https://www.plivo.com/docs/messaging/api/overview>

### 1.5 Bird (formerly MessageBird)

Every docs.bird.com URL tried redirects to a new API at bird.com/docs. The old Channels API (workspace id, channel id
and access key) could not be confirmed on any current page.

**Send.**

- `POST https://{us1|eu1}.platform.bird.com/v1/sms/messages` takes one `to`, `from`, `text`, and a `category`
  (transactional, marketing, authentication or service) or a template. It also accepts an `Idempotency-Key` header and
  `options.smart_encoding`, caps a message at 12 segments, and does not support `track_clicks` (it returns 422).
  `POST /v1/sms/batches` takes up to 100 messages. <https://bird.com/docs/guides/sms/sending-sms>
- Auth is a workspace-scoped, region-bound Bearer key `bk_{region}_…`, shown once.
  <https://bird.com/docs/api/authentication>

**Events.**

- **The event set:**
  - `sms.accepted`, and `sms.sent` ("awaiting a delivery receipt");
  - the final events `sms.delivered` ("carrier confirmed the message reached the handset"), `sms.undelivered`,
    `sms.failed`, `sms.expired` and `sms.rejected`;
  - `sms.received` for inbound messages;
  - `sms_suppression.created`.

  <https://bird.com/docs/guides/sms/events>

- **Webhooks:** endpoints are per workspace. The `webhook-id`, `webhook-timestamp` and `webhook-signature` headers carry
  `v1,<base64 HMAC-SHA256>` over `{id}.{timestamp}.{body}`, with a `whsec_` secret; this is the Standard Webhooks
  scheme that ADR 0037 already verifies.
- **Delivery guarantees:** 8 attempts over about 27.5 h, at-least-once, "not ordered", with replay.

<https://bird.com/docs/guides/webhooks>

**STOP / opt-out.**

- **Coverage:** keywords follow a per-country catalogue. "Where it is not [covered], Bird recognizes no keyword, sends
  no reply, and records no opt-out."
- **Scope and effect:** "A suppression covers one sender and one subscriber", and a send to a suppressed pair "is
  refused with E12077 SMSRecipientSuppressed". A workspace-wide preference opt-out returns E25000.
- **Suppression list API:** `GET /v1/sms/suppressions` lists the pairs, with a reason of `keyword_stop`,
  `carrier_opted_out` or `manual`.

<https://bird.com/docs/guides/sms/opt-outs-and-keywords>

A STOP still produces `sms.received`. <https://bird.com/docs/guides/sms/events>

**Sender IDs.** An alphanumeric sender has "3 to 11 ASCII letters, digits, spaces, hyphens, underscores, or dots, with
at least one letter… cannot reply… some, including the United States, do not accept one at all". A US long code needs
a registered brand and campaign, and toll-free needs verification. <https://bird.com/docs/guides/sms/senders>

**Rate limits.** Limits are set per organization and policy. A 429 carries `Retry-After` and `RateLimit` headers.
<https://bird.com/docs/guides/rate-limits>

**Legacy MessageBird SMS API**, still documented:

- `POST https://rest.messagebird.com/messages` with `AccessKey`, up to 50 recipients and an `originator` of at most 11
  alphanumeric characters.
- Statuses carry a `statusReason`, which can be "opted out".
- POST is limited to 500 requests per second.

<https://developers.messagebird.com/api/sms-messaging/>

## 2. Russian providers

None of the four sends a request flag that marks traffic as advertising or service. The class follows from
operator-approved templates instead: a text that matches a template is service, and anything else counts as
advertising.

### 2.1 SMSC (smsc.ru)

**Send.**

- `https://smsc.ru/sys/send.php` (GET or POST; JSON at `/rest/send/`), with `login` and `psw`, or an `apikey`.
- `phones` takes a list, and `list` gives each recipient its own text and sender.
- `id` can be set by the client. It is "не обязательно уникален", and one `list` request shares a single id
  ("Сообщениям в запросе присваивается единый идентификатор").
- `sender`: "Длина – 11 символов или 15 цифр. Все имена регистрируются в личном кабинете".
- Error 6 is "Сообщение запрещено… при попытке отправки массовых и (или) рекламных сообщений без заключенного
  договора".
- `mes` takes up to 1000 characters. `maxsms` caps the number of parts, `translit` transliterates, and `tinyurl=1`
  shortens links (a click gives status 4).

<https://smsc.ru/api/http/send/sms/>

**Statuses.**

- **In progress:** -1 queued, 0 handed to the operator.
- **Delivered:** 1 delivered, and 4 "Нажата ссылка" (link clicked).
- **Failed:** 3 expired, 20 cannot deliver, 22 invalid number, 23 "Запрещено", 24 insufficient funds, 25 unreachable.
  Status 23 covers blacklisted numbers, duplicates, flood, the spam filter and the sender, so it does not identify an
  opt-out uniquely. <https://smsc.ru/api/http/status_messages/statuses/>
- **Error codes:**
  - `err` 245: "нельзя точно сказать, было сообщение доставлено или нет", an unknown outcome;
  - 243: no contract for advertising mailings;
  - 254: "Незарегистрированный sender id";
  - 219: after a SIM swap, a subscriber cannot receive from alpha senders for 24 h.

  <https://smsc.ru/api/http/status_messages/errors/>

- **Polling:** statuses can be polled "в течение суток" by (phone, id).

**Callback.**

- **Where it is set:** one account-wide URL in the cabinet. It receives DLRs and inbound SMS, but not messages sent
  from the cabinet.
- **Format and retries:** a POST form, or JSON with `fmt=3`. On any reply other than 200 or 404 it retries "1 запрос
  каждые 4 минуты, всего 50 попыток".
- **Authentication:** optional. A `?md5=`/`?sha1=` secret in the URL makes SMSC send a hash of
  `id:phone:status:<secret>`, or of `phone:mes:to:<secret>` for an inbound message.
- **Traffic class:** the DLR `flag` bits 9–10 carry the template type, where "11 - рекламное" is advertising.

<https://smsc.ru/api/http/miscellaneous/callback/>

**Opt-out and inbound.**

- **Blacklist API:** `phones.php?add_black|del_black|get_black` takes a type: all messages, mass mailings only, or
  single messages only. <https://smsc.ru/api/http/contact/contact_actions/>
- **No STOP keyword** handling is documented.
- **Inbound:** replies arrive on the callback with `sms_id`, the message they answer, through rented virtual numbers.
  <https://smsc.ru/api/http/miscellaneous/receive/>

**Sender names** are registered per operator through `senders.php`, with per-operator flags and a trademark or domain
document. <https://smsc.ru/api/http/senders/senders_actions/>

### 2.2 SMS.ru

**Send.**

- `https://sms.ru/sms/send` with an `api_id`, or a login and password.
- `to` takes a comma-separated list, "до 100 штук за один запрос", and `json=1` returns an `sms_id` per number.
- `from` must be "согласовано с администрацией".
- Other parameters: `ttl` (1–1440 min) and `daytime=1`, which holds night sends until 10:00 in the recipient's time.
- A message has at most 8 parts (error 205).

<https://sms.ru/api/send>

**Templates.** A text that differs from its template "посчитают рекламным" (is counted as advertising).
<https://sms.ru/docs/templates>

**Statuses.** 100–102 are in progress, 103 is delivered, and 104 expired, 105 deleted by the operator, 106 handset
failure, 107 unknown reason, 108 rejected and 150 no route are failures. <https://sms.ru/api/status>

**Callbacks.**

- **Registration:** account-wide, through `callback/add`, and the URL "должен начинаться на https://".
  <https://sms.ru/api/callback_add>
- **Payload:** documented only in the official PHP library (<https://sms.ru/php>): POST `data[]` lines of `sms_status`,
  id and code, and the handler must answer `100`.
- **Authentication:** none documented.

**Opt-out.**

- **Stop-list API:** `stoplist/add` (a reason is required), `stoplist/del` and `stoplist/get`.
  <https://sms.ru/api/stoplist_add>
- **Two send errors:**
  - 209: the number is in the customer's own stop list;
  - 215: "Этот номер находится в стоп-листе SMS.RU (от получателя поступала жалоба на спам)", a provider-wide list the
    customer cannot read.

  <https://sms.ru/api/status>

- **No STOP keyword handling and no inbound API** is documented.

**Sender IDs.** "Sender ID — это текст длиной до 11 латинских символов". Approval runs in two stages, at SMS.ru and then
at each operator. "Отправка с буквенного отправителя становится доступна только после одобрения оператором."
<https://sms.ru/docs/senders>

### 2.3 SMS Aero (API v2)

Facts from the JavaScript-rendered page <https://smsaero.ru/integration/documentation/api/>:

- **Send:** `https://gate.smsaero.ru/v2/sms/send` with HTTP Basic, the account e-mail and an API key. The base URL and
  auth details were not checked word for word.
- **Parameters:** `number` or `numbers[]`, `sign` (the sender), `text`, and a per-message `callbackUrl`. The callback
  "в ответ система ждет статус 200", and otherwise SMS Aero retries "в течение 24 часов".
- **Other:** `shortLink=1` shortens links. The response carries `channel` (FREE SIGN, PAY SIGN or SERVICE). Every SMS
  goes through manual moderation "до 5-10 минут" unless a contract turns it off.
- **Statuses:** 0 queue, 1 delivered, 2 undelivered, 3 sent, 4 waiting, 6 rejected and 8 moderation. The pairing of
  numbers to names is inferred from list order.
- **Opt-out:** a blacklist API (`blacklist/add|delete|list`). Its effect on sends is not documented, and no STOP
  keyword handling is documented. Callbacks carry no signature.

**Contract terms.** The offer makes the customer obtain "согласия на получение СМС-сообщений" and "Удалять из Базы
абонентов… номера Абонентов, которые выразили отказ". <https://smsaero.ru/offer/>

**Sender names.** A name has 3–11 Latin characters, digits, `.`, `-` and `_`. Until it is approved, "сообщения будут
отправляться c общим именем типа SERVICE или INFORM". <https://smsaero.ru/support/2/13/>

**Limits.** "Максимальный размер СМС — 14 частей СМС на кириллице или 6 частей на латинице", and "Операторы связи
иногда блокируют сообщения с сокращенными ссылками". <https://smsaero.ru/support/sms-rassylka/trebovania-knbspsms/>

### 2.4 MTS Exolve

**Send.**

- `POST https://api.exolve.ru/messaging/v1/SendSMS` with `Authorization: Bearer <API-ключ приложения>`. The body holds
  `number` (the sender), **one** `destination` and `text`, and the response returns `message_id`. Bulk sending goes
  through the separate Campaign API.
- A text matching an approved template is templated automatically; anything else is advertising, and "Для каждого
  оператора можно выбрать только одну категорию".

<https://docs.exolve.ru/docs/ru/api-reference/sms-api/sending-sms/>

**Events.**

- **Where they go:** an events URL set per application in the cabinet receives JSON POSTs. Each status stage is a
  separate event with the same `message_id`.
- **Codes:** `delivery_status` is 0 unknown, 1 queued, 2 "доставлено в SMS-центр", 3 "доставлено получателю", 4 cannot
  deliver, 5 retries exhausted or 6 blocked.
- **Inbound:** messages arrive as `DIRECTION_INCOMING` events.
- **Not documented:** retries and authentication.

<https://docs.exolve.ru/docs/ru/api-reference/sms-api/sms-events/>

**Opt-out.** The Blacklist API's prompt methods are for "при звонке" (calls), and nothing ties it to SMS.
<https://docs.exolve.ru/docs/ru/api-reference/blacklist-api/>

**Sender names.**

- **Who and how:** "могут зарегистрировать только юрлица и ИП", the name is approved per operator in 10–25 business
  days, and "Отправлять СМС с Имени можно только после того, как оператор его согласует".
  <https://exolve.ru/faq/how-to-register-sms-sender-name/>
- **Status per operator:** `GetAlphaNames` returns it.
  <https://docs.exolve.ru/docs/ru/api-reference/sms-api/getting-alpha-names/>

**Segments.** "Максимальное количество сегментов в одном сообщении — 10", with at most 67 Cyrillic or 153 Latin
characters per part. <https://docs.exolve.ru/docs/ru/api-reference/sms-smpp/sending-requirements/>

## 3. Regulation that shapes the model

### United States: 47 CFR 64.1200 (FCC 24-24)

**Revoking consent.** Section 64.1200(a)(10) says:

- "A called party may revoke prior express consent… by using any reasonable method".
- Replying "stop," "quit," "end," "revoke," "opt out," "cancel," or "unsubscribe" to a text "constitutes a reasonable
  means per se".
- Other words count too "if a reasonable person would understand those words to have conveyed a request to revoke
  consent".
- The request "must be honored within a reasonable time not to exceed ten business days".

**Confirmation.** Section 64.1200(a)(12) allows one confirmation text, presumed within consent "if… sent within five
minutes of receipt". <https://www.govinfo.gov/content/pkg/FR-2024-03-05/html/2024-04587.htm>

**Dates.** These amendments took effect on April 11, 2025. Only the "revoke all" part, under which a revocation also
covers unrelated messages, is waived until January 31, 2027 (DA 26-12, released January 6, 2026).
<https://docs.fcc.gov/public/attachments/DA-26-12A1.txt>

**What this means for the platform:**

- STOP handling must accept free-text replies, not only a provider's single-word list. Twilio, for example, triggers
  only on a single word.
- A STOP must take effect across every sender of the customer, which is the (channel, destination) key, and not per
  number.

### Russia

**38-FZ "О рекламе", art. 18 part 1.** Advertising over telecom networks "допускается только при условии
предварительного согласия абонента". It counts as sent without consent "если рекламораспространитель не докажет, что
такое согласие было получено", and the advertiser "обязан немедленно прекратить распространение рекламы в адрес лица,
обратившегося к нему с таким требованием".
<https://www.consultant.ru/document/cons_doc_LAW_58968/f892dec1383709792452f18d36e7043306e2be0a/>

**126-FZ "О связи".**

- **Art. 2 p. 22.1** defines "рассылка" as any automated SMS, or SMS sent from numbering outside the Russian plan, which
  includes alpha senders. So it covers service traffic too, not only advertising.
  <https://www.consultant.ru/document/cons_doc_LAW_43224/b819c620a8c698de35861ad4c9d9696ee0c3ee7a/>
- **Art. 44.1:** consent must be given by actions "однозначно идентифицирующих этого абонента". The burden of proof is
  on the customer who initiated the mailing. Since 41-FZ of 01.04.2025 (part 1.1), the subscriber can refuse at their
  operator, who "обязан прекратить рассылку". A mailing goes out under a contract with the recipient's operator.
  <https://www.consultant.ru/document/cons_doc_LAW_43224/7caf446de1e18caa61180692ea35572c94080475/>
- **How an operator refusal works:** set by Government Decree No. 1994 of 30.12.2024, section XIII. The refusal can be
  filed online, takes effect from the next day, and can be withdrawn.
  <https://www.consultant.ru/document/cons_doc_LAW_495347/051dcbbcd870aa9428dd0978f759edd12e5bd2cd/>

**What this means for the platform:**

- Consent needs stored evidence, for both advertising and service traffic.
- An operator-level refusal is invisible to the sender, and shows only as rejected or undelivered DLRs: SMSC 23/255,
  SMS.ru 108, SMS Aero 6, Exolve 6.
- No statute sets sender-name rules. Registration is operator and contract practice.

## 4. Implications for the channel model

These are inferences from the facts above, not decisions.

1. **Destination and consent.**
   - The Destination is an E.164 number. Consent is keyed by (sms, number), as the map assumes.
   - Provider-side blocks are narrower: per sender, per service or per campaign, and only in the US/CA or in catalogue
     countries. So the platform's own opt-out is authoritative, and it is checked before calling the provider.
2. **Opt-out signals a Preset must emit.** Three kinds:
   - an inbound message whose text the platform classifies as STOP, since free text counts under the FCC rule and the
     provider's own keyword list is narrower;
   - a provider suppression event (Bird `sms_suppression.created`);
   - a send refused with an opt-out code (Twilio 21610, Vonage 9 or 1240, Plivo 200, Bird E12077 or E25000, SMS.ru 209
     or 215).

   Codes that do not identify an opt-out uniquely are failures, not opt-outs: SMSC 23, Sinch `Deleted`, Exolve 6.

3. **Delivery notice states.**
   - Delivery notices map to sent, delivered, failed and unknown, with the provider's raw code kept.
   - Transitions only move forward, because callbacks are unordered.
   - "No final report after N hours" has to be representable. Twilio says further status is unlikely after 72 h, Sinch
     caps expiry at 3 days, and SMSC archives after a day.
4. **Provider reference.** It is (provider id, destination), or the adapter sends one recipient per request. Vonage
   reports per part, so "delivered" means every part, or the first carrier DLR.
5. **Integration credentials** hold the send credential and, separately, the webhook verification secret:
   - Twilio's Auth Token even with an API key;
   - Vonage's signature secret;
   - Bird's `whsec_` secret;
   - the SMSC hash secret.

   SMS.ru, SMS Aero and Exolve have no webhook authentication, so the per-Integration ingest key is the only guard
   (ADR 0037).

6. **Verify schemes missing from ADR 0037's set.**
   - Twilio: HMAC-SHA1 over the URL plus sorted form fields.
   - Vonage Messages: a JWT with a body hash.
   - Plivo V2: the URL plus a nonce.
   - SMSC: a hash over chosen fields with a secret in our own URL.
7. **Sender.** A sender is a value of the Integration, or a pool or service id (Twilio Messaging Service, Plivo
   Powerpack), validated per country:
   - an alpha sender is at most 11 characters and one-way;
   - an alpha sender is invalid for the US and Canada;
   - in Russia an alpha name must be approved per operator.

   An alpha sender cannot receive STOP replies, so a message from one needs an opt-out path in its text, such as a link.

8. **Segment estimate.** The count estimate is computed locally:
   - GSM-7 is 160, or 153 per part; UCS-2 is 70, or 67 per part;
   - GSM-7 extension characters count as two;
   - one non-GSM character switches the whole message to UCS-2;
   - a per-provider part cap rejects a message before the send.
9. **Throughput.** It is a Provider limit per Integration, ADR 0067's term. A 429, Vonage status 1, or a queue-expiry
   error such as Plivo 420 or Twilio 30001 is a Deferral, not a failure.

## Conflicting sources

- **Twilio 21610:** the support article describes it as a synchronous 400 on send, while the Advanced Opt-Out tutorial
  describes an asynchronous failure.
- **Twilio MPS:** the scaling guide gives Canada long code as both 10 and 1 MPS, and toll-free as 3 and 25.
  <https://www.twilio.com/docs/messaging/guides/scaling-queueing-latency>
- **Plivo toll-free MPS:** 25 in the FAQ and 40 on the features page.
- **Plivo opt-out keywords:** 5 on the features page and 7 on the DND page.
- **SMS.ru:** the recipient limit is 100 per request on the send page but 5000 in error 213. The callback URL "must
  start with https" (`callback/add`), but error 901 says "http://".
- **SMSC flood guard:** 60 s on the send page and 30 s on the errors page (`err` 251).

## Unconfirmed

- **Sinch:** STOP handling, forwarding and any opt-out API on the REST API; whether `Delivered` is confirmed by the
  handset; and the HMAC header and algorithm for callbacks.
- **Plivo:** whether a STOP is forwarded to `message_url`, any opt-out list API, and the DLR callback retry schedule.
- **Vonage:** the DLR webhook retry schedule, and the per-country alphanumeric list, which is spread over per-country
  articles.
- **Bird:** whether a message without a DLR stays `sent`, the numeric MPS, and whether the old Channels API still
  works.
- **Twilio:** the surface of the Consent Management API, which is only a pointer in a support FAQ.
- **SMS.ru:** inbound SMS, and the callback payload outside the PHP library.
- **SMS Aero:** the auth and base-URL details and the status pairing (both from the rendered page), inbound SMS, and
  whether a blacklisted number blocks sends.
- **Exolve:** webhook retries and authentication, which number types can receive SMS, whether the blacklist covers SMS,
  and rate limits.
- **GSM-7 extension table:** the exact list of double-width characters was not read from 3GPP TS 23.038 itself. Twilio
  says only that characters "such as `{` and `]`, need an escape character", counting as two.
  <https://www.twilio.com/docs/glossary/what-is-gsm-7-character-encoding>
