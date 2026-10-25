# Outbound calls to other services' hooks and endpoints: prior art for sphericon

Researched October 2026 from primary sources: shallow clones of each repository's default branch
(commit SHAs below; cloned code was read, never run), the Standard Webhooks spec, Drip's official API
docs, and sphericon's own tree at `ad4e5c2e`. Permalinks use the commit SHA, so line numbers stay valid.
River's retry formula is cited from the Go module cache (`riverqueue/river@v0.49.0`, the version in
`go.mod`), which has no commit SHA here.

| Repo                                | Branch | Commit                                                       |
| ----------------------------------- | ------ | ------------------------------------------------------------ |
| knadh/listmonk                      | master | `33896f18bcc9136327cee26e38ebbb23e9f39717`                   |
| pentacent/keila                     | main   | `2308bebd2b54edc8ac8d484be5c9b4e972aed7cc`                   |
| mautic/mautic                       | main   | `0c202684bf53ab94f3d25cb352b10582beb73dcf`                   |
| Notifuse/notifuse                   | main   | `7516397cc5c1eadc2a6aa67c733261043a630a62`                   |
| Billionmail/BillionMail             | dev    | `fc36c76c050c3775c5e899faf7403cf0262d2744`                   |
| hatchet-dev/hatchet                 | main   | `4cc02dd69f31d0cfb83fb3ecd32e88f058678e88`                   |
| coder/coder                         | main   | `121393842cc1ee2ca8cb769e400c4b3d17a49a82`                   |
| zitadel/zitadel                     | main   | `5cd91e0` (full: `5cd91e00d62fa5fbbe100eb9fd21c01d4426da84`) |
| mattermost/mattermost               | master | `f38678d08d67a4d21ea3821f78b530ea7f8f8ec2`                   |
| temporalio/temporal                 | main   | `cc912e101f346e228a397508eadc25e8ea4dfecd`                   |
| standard-webhooks/standard-webhooks | main   | `7537d2a2d3d52d8f2e0ecd12527af4a9307fd81b`                   |
| sphericon (this repo)               | main   | `ad4e5c2e075ddfbfb9ce3353607b69a79a17c53f`                   |

Base URLs: `https://github.com/<owner>/<repo>/blob/<first 7 of SHA>/<path>#L<n>`. Sphericon paths are
repo-relative with line numbers at `ad4e5c2e`. Listmonk's SHA differs from the one in
`outbound-send-prior-art.md` (`82db22c`): this note was read at the newer head.

## Question

The simplest kind of integration is the one where sphericon calls someone else: it POSTs to a customer's
URL, a Zapier or Make catch hook, a Slack incoming webhook or a CRM's REST endpoint. (Inbound provider
callbacks are ADR 0037 and out of scope.) For each project: how are outbound calls configured and stored,
how are they signed, what stops SSRF, what are the timeouts, retries, dead-letter/disable rules, delivery
log and replay, payload templating, event selection, queueing and tests? Then sphericon's current state.
Facts first; a short last section lists gaps and ideas.

## Summary of findings

- **Only some of the email/marketing projects have outbound event webhooks.** Mautic (a full WebhookBundle
  plus a campaign "send webhook" action) and Notifuse (subscriptions, delivery table, worker) do. Listmonk
  has only a "messenger" (a postback of the campaign message to one configured URL). Keila and BillionMail
  have no outbound webhooks at all (searched, nothing found); Drip documents them but is closed source.
- **Notifuse is the most complete and the closest in shape to sphericon**: per-workspace subscriptions with
  Standard Webhooks signing, deliveries written in the same Postgres transaction as the change (PL/pgSQL
  triggers), a 10-rung backoff ladder, a failure counter plus a time window that auto-disables a dead endpoint,
  410 handling, 429 that does not count as a failure, a stored delivery row with status/response/error, a
  "test" endpoint, and a real e2e test of the worker. **But its delivery worker uses a plain `http.Client` with
  no SSRF guard** (the safe client exists in `pkg/safehttpclient` and is wired to five other call sites, not
  to webhook delivery).
- **Mautic is the only one with a pre-flight private-address check on webhooks**, and it is a DNS lookup
  before the request (a second lookup happens at connect time, so this is a rebinding window; my reading,
  not tested). Private targets are allowed through an explicit config list (`webhook_allowed_private_addresses`),
  which matters for self-hosters. Its campaign "send webhook" action (user-templated URL, headers, body with
  contact tokens) **does not go through that check** (the checker is referenced only from `Http/Client.php`).
  Mautic disables a webhook on 410 at once and after N consecutive failures (default 100), and keeps a
  capped log table (default 1000 rows) per webhook.
- **SSRF at the dial layer, on the resolved IP, is the pattern the Go projects converge on.** Mattermost
  (`dialContextFilter`, its CIDR list is copied from `doyensec/safeurl`, which sphericon uses), Zitadel
  (`net.Dialer.Control` hook, explicit configurable `DenyList`), Hatchet's unwired `safeclient` (safeurl
  plus an explicit list, https and port 443 only, no redirects) and sphericon (safeurl, no redirects).
  Every one of these except sphericon and Hatchet's `safeclient` offers an operator allowlist for internal
  targets (`AllowedUntrustedInternalConnections`, `webhook_allowed_private_addresses`, editable `DenyList`).
  Temporal inverts the model: callback targets must match a per-namespace **allowlist** of host patterns,
  default empty.
- **Retry design splits in two families.** Durable queue with persisted attempts and backoff: Notifuse
  (own table, ladder 30s to 24h, about 9h53m reachable at max_attempts 10), Coder (`notification_messages`,
  `max_send_attempts` 5), Temporal (exponential 1s to 1h, no expiry, plus a per-destination circuit
  breaker), sphericon (river, `MaxAttempts: 10`, river's default `attempt^4` seconds, about 4h15m in total
  by my arithmetic). Fire-and-forget or cancel-on-error: Mattermost (one goroutine, log and drop), Zitadel
  (any error is `river.JobCancel`, no retry), Listmonk (a `Retries` option exists in the struct and is
  read nowhere).
- **Signing is uneven.** Standard Webhooks (id, timestamp, signature headers): Notifuse and sphericon.
  Timestamp-less HMAC over the body: Mautic (`Webhook-Signature`, null when no secret). Own scheme with
  timestamp: Zitadel (`ZITADEL-Signature`, optional JWT/JWE payload). Plain shared token inside the body:
  Mattermost (`token` field). No signature at all: Coder (only `X-Message-Id`), Listmonk (optional basic
  auth), Drip (docs mention none).
- **Delivery log and replay**: the spec (Standard Webhooks section on failed messages, line 321) says
  producers should let customers list failures and replay them. Among the projects read, only Notifuse
  (deliveries table with `last_response_status/body/error`, list endpoint) and Mautic (webhook_logs with
  status code, runtime, note) store attempts. **No project read has a replay endpoint**: none was found in
  Notifuse's handler/service, Mautic or Coder.
- **Sphericon today**: URL (http/https only), HMAC secret generated server-side and stored sealed,
  event-type filter (empty = all customer events), `enabled` flag, river job per (event, endpoint), safeurl
  client with 15 s timeout and no redirects, Standard Webhooks signature. Not present: delivery log or
  status, replay, auto-disable, 410/429 handling, secret rotation, custom headers, test-send, per-endpoint
  concurrency limit, an operator allowlist for internal targets, and an e2e scenario.

### Comparison table

| Project                | Config and storage                                                                                                     | Signing                                          | SSRF guard                                                                               | Timeout                                                                     | Retry / backoff                                                              | Disable / dead-letter                                                            | Log / replay                                                               | Events / payload                                                               | Queue                                                  |
| ---------------------- | ---------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------ | ---------------------------------------------------------------------------------------- | --------------------------------------------------------------------------- | ---------------------------------------------------------------------------- | -------------------------------------------------------------------------------- | -------------------------------------------------------------------------- | ------------------------------------------------------------------------------ | ------------------------------------------------------ |
| Listmonk (messenger)   | `messengers` JSON in settings: name, URL, basic auth, max_conns, timeout                                               | none (optional basic auth)                       | none found                                                                               | per-messenger `timeout`                                                     | `retries` option never read; failed send counts toward campaign error limit  | campaign pauses at `max_send_errors`                                             | none                                                                       | campaign message only, fixed JSON                                              | in-process manager pipes                               |
| Keila                  | no outbound webhooks                                                                                                   | n/a                                              | n/a                                                                                      | n/a                                                                         | n/a                                                                          | n/a                                                                              | n/a                                                                        | n/a                                                                            | n/a                                                    |
| BillionMail            | no outbound webhooks (daily usage POST to vendor, see section 4)                                                       | n/a                                              | n/a                                                                                      | n/a                                                                         | n/a                                                                          | n/a                                                                              | n/a                                                                        | n/a                                                                            | n/a                                                    |
| Mautic webhooks        | `webhooks` table: URL, secret, events, category, health fields                                                         | HMAC-SHA256 base64 of body, no timestamp         | DNS pre-check vs private ranges; opt-in allowlist                                        | `webhook_timeout` 15 s defined, not passed to the request in `Client::post` | queue rows retried after `webhook_retry_delay` 3600 s; recursion in same run | 410 kills; unhealthy marking; `webhook_disable_limit` 100                        | `webhook_logs`, capped 1000; no replay                                     | events chosen per webhook; fixed serialized payload; batches of 10             | immediate or cron command                              |
| Mautic campaign action | in the campaign step config                                                                                            | none                                             | none (not routed through the checker)                                                    | per-action `timeout`                                                        | none                                                                         | none                                                                             | none                                                                       | URL/headers/body with `{contactfield}` tokens                                  | campaign executor                                      |
| Notifuse               | `webhook_subscriptions` (per workspace DB): URL, `whsec_` secret, event types, list/segment filters, enabled, counters | Standard Webhooks `v1,` HMAC                     | none on delivery (plain client)                                                          | 10 s client, per-claim budget                                               | 10-rung ladder 30 s to 24 h, `max_attempts` 10                               | 20 failures plus a 2 h failing window auto-disable; 410 retires; 429 not counted | `webhook_deliveries` rows, 7-day retention, list API, test send; no replay | event types plus list/segment ids; fixed payload built in PL/pgSQL             | Postgres rows, `SKIP LOCKED`, 10 s poll, 100 per batch |
| Zitadel Actions v2     | target: URL, type, timeout, signing key (encrypted), payload type                                                      | `ZITADEL-Signature` with timestamp; JSON/JWT/JWE | dial-time `Control` hook, DenyList (RFC1918 etc. default), redirects 5, no https to http | per target; job cap 10 s                                                    | none: error cancels the job                                                  | `interrupt_on_error` stops the chain                                             | log store to stdout                                                        | conditions on request/response/event/function; target can rewrite the response | river queue, 1 worker                                  |
| Mattermost             | outgoing webhook: channel, trigger words, callback URLs, content type, token                                           | `token` field in payload                         | dial filter on resolved IPs; `AllowedUntrustedInternalConnections`                       | `OutgoingIntegrationRequestsTimeout` default 30 s                           | none                                                                         | none                                                                             | none (optional delivery tracking per post)                                 | trigger words/channel; form or JSON; response can post a reply                 | goroutine per URL                                      |
| Coder notifications    | one deployment-wide URL flag                                                                                           | none (`X-Message-Id`)                            | none (default transport clone)                                                           | `dispatch_timeout` 1 m                                                      | `max_send_attempts` 5, retryable flag per error                              | permanent failure after attempts                                                 | `notification_messages` rows                                               | templates per event; fixed envelope v1.1                                       | DB-leased queue                                        |
| Temporal callbacks     | callback URL and headers attached to an execution                                                                      | Nexus callback token header                      | namespace allowlist of host patterns; https required unless `AllowInsecure`              | 10 s per request                                                            | exponential 1 s to 1 h, no expiry; non-retryable errors fail                 | per-destination circuit breaker                                                  | metrics                                                                    | completion payload only                                                        | history outbound queue                                 |
| Hatchet Slack alert    | encrypted incoming webhook URL from Slack OAuth install                                                                | Slack's own (URL is the secret)                  | none (URL comes from Slack, not typed)                                                   | library default                                                             | none visible                                                                 | `MaxFrequency` throttle                                                          | none                                                                       | fixed alert blocks                                                             | in alerter                                             |
| Drip (docs only)       | `post_url`, `events`, `include_received_email`                                                                         | none documented                                  | not documented                                                                           | not documented                                                              | rate-limited sends retried up to 3 days                                      | endpoint errors for 3 days: disabled and owner notified                          | not documented                                                             | per-event JSON `{event, data, occurred_at}`                                    | not documented                                         |
| sphericon              | `webhook_endpoints`: URL, sealed secret, event types, enabled                                                          | Standard Webhooks via library                    | safeurl on resolved IP, no redirects, http(s)                                            | 15 s                                                                        | river `attempt^4` s, 10 attempts                                             | none                                                                             | none                                                                       | event types; fixed envelope                                                    | river queue `webhooks`, 5 workers                      |

## 1. Reference: what the Standard Webhooks spec asks of a producer

Sphericon already signs with this library, so the spec is the yardstick. Source:
[spec/standard-webhooks.md](https://github.com/standard-webhooks/standard-webhooks/blob/7537d2a/spec/standard-webhooks.md).

- Retry with exponential backoff and jitter over multiple days; when failures persist, notify the consumer
  by another channel and disable the endpoint
  ([L252-L256](https://github.com/standard-webhooks/standard-webhooks/blob/7537d2a/spec/standard-webhooks.md#L252-L256)).
  Its example schedule starts at 5 s and 5 min ([L258-L264](https://github.com/standard-webhooks/standard-webhooks/blob/7537d2a/spec/standard-webhooks.md#L258-L264)).
- 2xx is success, everything else failure; 3xx is a failure (do not follow redirects); 410 Gone means disable the endpoint;
  honour `retry-after` ([L275-L286](https://github.com/standard-webhooks/standard-webhooks/blob/7537d2a/spec/standard-webhooks.md#L275-L286)).
- Request timeout recommended 15 to 30 s ([L290](https://github.com/standard-webhooks/standard-webhooks/blob/7537d2a/spec/standard-webhooks.md#L290)).
- SSRF: block internal networks, ideally through a filtering proxy (it names Stripe's smokescreen) and workers in their own subnet
  ([L300-L306](https://github.com/standard-webhooks/standard-webhooks/blob/7537d2a/spec/standard-webhooks.md#L300-L306)).
- Give customers a list of failed messages with reasons and a manual replay, including by range
  ([L321](https://github.com/standard-webhooks/standard-webhooks/blob/7537d2a/spec/standard-webhooks.md#L321)).
- The timestamp is per attempt, not per event; the id is the idempotency key
  ([L135](https://github.com/standard-webhooks/standard-webhooks/blob/7537d2a/spec/standard-webhooks.md#L135), [L233-L234](https://github.com/standard-webhooks/standard-webhooks/blob/7537d2a/spec/standard-webhooks.md#L233-L234)).

## 2. Marketing and email projects

### 2.1 Listmonk: the "messenger" postback (no event webhooks)

Listmonk's only outbound HTTP integration is a Messenger: a web service that receives each campaign
message as JSON ([docs/docs/content/messengers.md#L1-L10](https://github.com/knadh/listmonk/blob/33896f1/docs/docs/content/messengers.md#L1-L10)).
No outbound event webhooks exist: the only files mentioning "webhook" are bounce ingestion
(`internal/bounce/webhooks/*`, inbound) and migrations.

- **Config and storage.** Messengers live in the `messengers` settings row (`schema.sql`: `('messengers', '[]')`,
  [schema.sql#L285](https://github.com/knadh/listmonk/blob/33896f1/schema.sql#L285)), loaded at boot, one HTTP client per
  enabled entry ([cmd/init.go#L795-L815](https://github.com/knadh/listmonk/blob/33896f1/cmd/init.go#L795-L815)). Fields:
  name, username/password (basic auth), root URL, max connections, retries, timeout
  ([postback.go#L50-L59](https://github.com/knadh/listmonk/blob/33896f1/internal/messenger/postback/postback.go#L50-L59)). A saved
  entry with an empty password keeps the stored one, matched by a generated UUID
  ([cmd/settings.go#L280-L293](https://github.com/knadh/listmonk/blob/33896f1/cmd/settings.go#L280-L293)). Settings are global to the
  instance (Listmonk is not multi-tenant).
- **Signing.** None; optional `Authorization: Basic`, `User-Agent: listmonk`
  ([postback.go#L177-L182](https://github.com/knadh/listmonk/blob/33896f1/internal/messenger/postback/postback.go#L177-L182)).
- **SSRF.** No guard found: the client is a plain `http.Client` with a transport that only sets connection and
  timeout fields ([postback.go#L76-L88](https://github.com/knadh/listmonk/blob/33896f1/internal/messenger/postback/postback.go#L76-L88));
  a grep for ssrf, loopback and private-range checks over the Go tree found nothing. The URL is set by an admin, not by a tenant.
- **Timeout / retry.** `Timeout` is applied to the client and the transport. `Retries` is declared
  ([L57](https://github.com/knadh/listmonk/blob/33896f1/internal/messenger/postback/postback.go#L57)) and read nowhere else
  (grep). Any status other than 200 is an error ([L207-L209](https://github.com/knadh/listmonk/blob/33896f1/internal/messenger/postback/postback.go#L207-L209)).
- **Failure handling.** The manager logs the error, bumps the pipe's error counter (which stops the campaign past
  `app.max_send_errors`), and keeps a failed send for retry only when the campaign was stopped
  ([internal/manager/manager.go#L553-L575](https://github.com/knadh/listmonk/blob/33896f1/internal/manager/manager.go#L553-L575)).
- **Payload / events.** One fixed JSON shape per message (subject, body, recipient with attribs, campaign, attachments)
  ([postback.go#L15-L48](https://github.com/knadh/listmonk/blob/33896f1/internal/messenger/postback/postback.go#L15-L48)); no event selection and no templating.
- **Tests.** Not examined (the earlier note `reference-projects-test-architecture.md` already concluded nothing to take).

### 2.2 Keila: no outbound webhooks

`grep -ril webhook lib config test` finds only the inbound SES controller, its parser and a Paddle controller test. The
only outbound `Req` calls are fixed vendor URLs: a release check
([lib/keila/instance.ex#L58](https://github.com/pentacent/keila/blob/2308beb/lib/keila/instance.ex#L58)) and captcha
([captcha.ex#L46](https://github.com/pentacent/keila/blob/2308beb/lib/keila_web/helpers/captcha/captcha.ex#L46)).
One outbound fetch uses a URL from a request body: the SES SNS `SubscribeURL`
([ses_webhook_controller.ex#L38](https://github.com/pentacent/keila/blob/2308beb/lib/keila_web/controllers/ses_webhook_controller.ex#L38)),
behind a signature check plug ([L11](https://github.com/pentacent/keila/blob/2308beb/lib/keila_web/controllers/ses_webhook_controller.ex#L11),
[L50-L55](https://github.com/pentacent/keila/blob/2308beb/lib/keila_web/controllers/ses_webhook_controller.ex#L50-L55)); whether that
check pins the certificate host was not examined.

### 2.3 Mautic: WebhookBundle and the campaign webhook action

Paths below are under `app/bundles/WebhookBundle` unless stated.

**Configuration and storage.** A `webhooks` table via Doctrine metadata: URL (text), secret (string), category, events
(one-to-many `webhook_events`), logs (one-to-many), `events_orderby_dir`, and health fields `marked_unhealthy_at`,
`unhealthy_since`, `last_notification_sent_at` ([Entity/Webhook.php#L135-L163](https://github.com/mautic/mautic/blob/0c20268/app/bundles/WebhookBundle/Entity/Webhook.php#L135-L163)).
The secret is a plain string column; whether it is encrypted at rest was not checked. Webhooks are global to the
Mautic instance (permissions, not a tenant column). Defaults live in
[Config/config.php#L46-L60](https://github.com/mautic/mautic/blob/0c20268/app/bundles/WebhookBundle/Config/config.php#L46-L60):
10 entities per call (`webhook_limit`), 600 s processing window, 1000 logs kept, 300 s unhealthy back-off, 3600 s
queue retry, 100 failures to disable, 15 s timeout, queue mode (immediate or cron), failure-notification timing,
and `webhook_allowed_private_addresses => []`.

**Event selection.** Webhooks subscribe to named events registered by other bundles through
`WEBHOOK_ON_BUILD` (for example Email, Form, Page bundles have a `WebhookSubscriber`); the model queues one row per
(webhook, event) and either processes immediately or leaves it to a cron command
([Model/WebhookModel.php#L229-L284](https://github.com/mautic/mautic/blob/0c20268/app/bundles/WebhookBundle/Model/WebhookModel.php#L229-L284),
[Command/ProcessWebhookQueuesCommand.php](https://github.com/mautic/mautic/blob/0c20268/app/bundles/WebhookBundle/Command/ProcessWebhookQueuesCommand.php)).

**Signing.** `base64(hmac_sha256(json_body, secret))` in `Webhook-Signature`, plus `X-Origin-Base-URL`; the header is `null` when
no secret is set; no timestamp, so no replay protection by the spec's definition
([Http/Client.php#L23-L32](https://github.com/mautic/mautic/blob/0c20268/app/bundles/WebhookBundle/Http/Client.php#L23-L32)).

**SSRF.** `Client::post` calls `PrivateAddressChecker::isAllowedUrl` before the request and throws `PrivateAddressException`
([Client.php#L34-L41](https://github.com/mautic/mautic/blob/0c20268/app/bundles/WebhookBundle/Http/Client.php#L34-L41)). The checker
resolves a hostname with `gethostbynamel` and tests each address against RFC1918, loopback, link-local, ULA ranges
([PrivateAddressChecker.php#L9-L18](https://github.com/mautic/mautic/blob/0c20268/app/bundles/CoreBundle/Helper/PrivateAddressChecker.php#L9-L18),
[L44-L77](https://github.com/mautic/mautic/blob/0c20268/app/bundles/CoreBundle/Helper/PrivateAddressChecker.php#L44-L77)). A private target is
refused unless the host or resolved IP appears verbatim in `webhook_allowed_private_addresses`
([L124-L171](https://github.com/mautic/mautic/blob/0c20268/app/bundles/CoreBundle/Helper/PrivateAddressChecker.php#L124-L171)).
Limits visible in the code: the list omits CGNAT (100.64/10) and the 0.0.0.0/8 range, the check is a separate lookup
from the one Guzzle performs when it connects (so a changing DNS answer can pass the check and still reach a private
address; my reading, not tested), and `Client::post` sets no redirect option. The checker has a unit test with an injected DNS resolver
([CoreBundle/Tests/Helper/PrivateAddressCheckerTest.php#L11-L23](https://github.com/mautic/mautic/blob/0c20268/app/bundles/CoreBundle/Tests/Helper/PrivateAddressCheckerTest.php#L11-L23)).

**Timeout.** `webhookTimeout` is read from `webhook_timeout` into a model property
([WebhookModel.php#L96](https://github.com/mautic/mautic/blob/0c20268/app/bundles/WebhookBundle/Model/WebhookModel.php#L96),
[L729](https://github.com/mautic/mautic/blob/0c20268/app/bundles/WebhookBundle/Model/WebhookModel.php#L729)); there is no other use of it in that
file and `Client::post` passes no timeout option, so the 15 s default does not appear to reach the HTTP call (a gap in my reading
of the Guzzle client's own configuration, which is `GuzzleHttp\Client` autowired with no arguments,
[CoreBundle/Config/services.php#L314](https://github.com/mautic/mautic/blob/0c20268/app/bundles/CoreBundle/Config/services.php#L314)).

**Failure handling.** `processWebhook` ([L295-L384](https://github.com/mautic/mautic/blob/0c20268/app/bundles/WebhookBundle/Model/WebhookModel.php#L295-L384)):
a non-2xx response increments the queue rows' retry count instead of deleting them; every response is logged with status code, runtime and
body ([L331](https://github.com/mautic/mautic/blob/0c20268/app/bundles/WebhookBundle/Model/WebhookModel.php#L331)); **410 kills the webhook
immediately** ([L336-L339](https://github.com/mautic/mautic/blob/0c20268/app/bundles/WebhookBundle/Model/WebhookModel.php#L336-L339));
`isSick` is true when none of the last `webhook_disable_limit` log rows succeeded, and then the webhook is unpublished unless
`disable_auto_unpublish` is set or the user edited it recently ([L343-L353](https://github.com/mautic/mautic/blob/0c20268/app/bundles/WebhookBundle/Model/WebhookModel.php#L343-L353),
[L390-L400](https://github.com/mautic/mautic/blob/0c20268/app/bundles/WebhookBundle/Model/WebhookModel.php#L390-L400)); an unhealthy webhook is skipped for 300 s
and an email notification is sent after one hour of failing, then once a day
([Service/WebhookService.php#L19-L69](https://github.com/mautic/mautic/blob/0c20268/app/bundles/WebhookBundle/Service/WebhookService.php#L19-L69)).
Logs are pruned to the newest 1000 per webhook ([LogRepository.php#L36](https://github.com/mautic/mautic/blob/0c20268/app/bundles/WebhookBundle/Entity/LogRepository.php#L36)).
No replay action was found in the controller or commands.

**The campaign "send webhook" action** (`Helper/CampaignHelper.php`) is a second, separate path: URL, method (get/post/put/patch), headers and payload are
campaign config where `{contactfield}` tokens are replaced from the contact, with a per-action timeout
([L33-L50](https://github.com/mautic/mautic/blob/0c20268/app/bundles/WebhookBundle/Helper/CampaignHelper.php#L33-L50),
[L79-L110](https://github.com/mautic/mautic/blob/0c20268/app/bundles/WebhookBundle/Helper/CampaignHelper.php#L79-L110),
[L122-L131](https://github.com/mautic/mautic/blob/0c20268/app/bundles/WebhookBundle/Helper/CampaignHelper.php#L122-L131)). It calls `$this->client->request`
directly; `PrivateAddressChecker` is referenced only by `Http/Client.php` and its test (grep over `app/`), so this path has no private-address check, no signature, no retry
and no log. This is the only project read where an end user can templatise the outbound request (headers, body, URL).

**Tests.** Unit test of the signature headers ([Tests/Unit/Http/ClientTest.php#L39-L54](https://github.com/mautic/mautic/blob/0c20268/app/bundles/WebhookBundle/Tests/Unit/Http/ClientTest.php#L39-L54)),
functional tests of the cron workflow, failure notification and the unhealthy skip
([Tests/Functional/WebhookFunctionalTest.php#L68](https://github.com/mautic/mautic/blob/0c20268/app/bundles/WebhookBundle/Tests/Functional/WebhookFunctionalTest.php#L68),
[L126](https://github.com/mautic/mautic/blob/0c20268/app/bundles/WebhookBundle/Tests/Functional/WebhookFunctionalTest.php#L126),
[L166-L181](https://github.com/mautic/mautic/blob/0c20268/app/bundles/WebhookBundle/Tests/Functional/WebhookFunctionalTest.php#L166-L181)),
and `CampaignSendWebhookFunctionalTest.php`.

### 2.4 Notifuse: subscriptions, a delivery table and a worker

**Configuration and storage.** `WebhookSubscription` has ID, name, URL, `whsec_` secret, settings (event types, custom event
filters, list ids, segment ids), `enabled`, `source` (user or `zapier`), `consecutive_failures`, `failing_since`, `disabled_reason`,
`last_delivery_at` ([internal/domain/webhook_subscription.go#L49-L97](https://github.com/Notifuse/notifuse/blob/7516397/internal/domain/webhook_subscription.go#L49-L97)).
Subscriptions are per workspace (each workspace has its own database connection). Management API:
`create, list, get, update, delete, toggle, regenerateSecret, deliveries, test, eventTypes`
([internal/http/webhook_subscription_handler.go#L42-L51](https://github.com/Notifuse/notifuse/blob/7516397/internal/http/webhook_subscription_handler.go#L42-L51)).
URL validation is scheme (http or https) and non-empty host only
([webhook_subscription_service.go#L163-L180](https://github.com/Notifuse/notifuse/blob/7516397/internal/service/webhook_subscription_service.go#L163-L180)). Whether the secret
is encrypted at rest was not checked.

**Event selection and the outbox.** Deliveries are written by **PL/pgSQL triggers** on contacts, contact_lists, contact_segments, message_history and custom_events:
each loops over enabled subscriptions whose `event_types` contain the event and inserts a `webhook_deliveries` row with `max_attempts` 10
([internal/database/schema/webhook_triggers.go#L109-L113](https://github.com/Notifuse/notifuse/blob/7516397/internal/database/schema/webhook_triggers.go#L109-L113)),
so the row commits with the change (a database-level outbox). The payload shape is built in SQL; the file warns that a column added to `contacts` is invisible to
webhooks until added to the trigger's list ([L35-L47](https://github.com/Notifuse/notifuse/blob/7516397/internal/database/schema/webhook_triggers.go#L35-L47)).

**Delivery table and queue.** A row has status (pending, delivering, delivered, failed), attempts, max_attempts, next_attempt_at, last response status/body/error and a claim
timestamp ([domain/webhook_subscription.go#L125-L140](https://github.com/Notifuse/notifuse/blob/7516397/internal/domain/webhook_subscription.go#L125-L140)).
The worker polls every 10 s, 100 rows per workspace per batch, claiming with `FOR UPDATE SKIP LOCKED`, a 15 s claim lease renewed per row,
a stale-claim sweep, and 7-day retention of finished rows
([webhook_delivery_worker.go#L23-L40](https://github.com/Notifuse/notifuse/blob/7516397/internal/service/webhook_delivery_worker.go#L23-L40),
[L249-L265](https://github.com/Notifuse/notifuse/blob/7516397/internal/service/webhook_delivery_worker.go#L249-L265),
[webhook_delivery_postgres.go#L65-L90](https://github.com/Notifuse/notifuse/blob/7516397/internal/repository/webhook_delivery_postgres.go#L65-L90)).
A list API serves deliveries per subscription ([L191](https://github.com/Notifuse/notifuse/blob/7516397/internal/repository/webhook_delivery_postgres.go#L191)).
A `SendTestWebhook` sends a sample event of a chosen type ([worker L1434](https://github.com/Notifuse/notifuse/blob/7516397/internal/service/webhook_delivery_worker.go#L1434)).
No replay or resend function was found in the handler, service or repository (grep for retry/replay/resend).

**Signing.** Standard Webhooks: signed content `{id}.{timestamp}.{payload}`, `v1,` plus base64 HMAC-SHA256, headers `webhook-id`, `webhook-timestamp`,
`webhook-signature` ([L874-L877](https://github.com/Notifuse/notifuse/blob/7516397/internal/service/webhook_delivery_worker.go#L874-L877),
[L1245-L1258](https://github.com/Notifuse/notifuse/blob/7516397/internal/service/webhook_delivery_worker.go#L1245-L1258)). A bad secret is retryable, because
rotating it repairs it ([L802](https://github.com/Notifuse/notifuse/blob/7516397/internal/service/webhook_delivery_worker.go#L802)).

**Retries and disable rules.** Ladder 30 s, 1 m, 2 m, 5 m, 15 m, 30 m, 1 h, 2 h, 6 h, 24 h; with `max_attempts` 10 only nine rungs are reachable, about 9h53m
([L136-L158](https://github.com/Notifuse/notifuse/blob/7516397/internal/service/webhook_delivery_worker.go#L136-L158)); past it the row is `failed`
([L1197-L1217](https://github.com/Notifuse/notifuse/blob/7516397/internal/service/webhook_delivery_worker.go#L1197-L1217)). Status table in the code comment:
2xx resets the counter; **410 is terminal** (a Zapier-created subscription is deleted, a hand-made one disabled); **429 retries without counting a failure**; 404 retries like any
error; anything else counts, and the subscription is disabled after 20 consecutive failures **and** a failing run of at least 2 hours (`webhookFailureWindow`; a failure run older than 12 hours is reset, `webhookFailureRunMaxAge`)
([L53](https://github.com/Notifuse/notifuse/blob/7516397/internal/service/webhook_delivery_worker.go#L53),
[L908-L957](https://github.com/Notifuse/notifuse/blob/7516397/internal/service/webhook_delivery_worker.go#L908-L957),
[L1103-L1118](https://github.com/Notifuse/notifuse/blob/7516397/internal/service/webhook_delivery_worker.go#L1103-L1118)). The disable reason is stored on the row.
The comments explain why both conditions are needed: Zapier catch hooks answer success unconditionally, and a 30 s outage during an import can fail twenty rows in one poll.

**SSRF.** `NewWebhookDeliveryWorker` receives the shared `httpClient`, built in `app.go` as `&http.Client{Timeout: 10 * time.Second}` with optional tracing
([internal/app/app.go#L729-L737](https://github.com/Notifuse/notifuse/blob/7516397/internal/app/app.go#L729-L737),
[L1171-L1177](https://github.com/Notifuse/notifuse/blob/7516397/internal/app/app.go#L1171-L1177)). The SSRF-safe client `pkg/safehttpclient.New()`
(dial-time resolved-IP check, 22 reserved CIDRs, redirects revalidated by the same dialer, 10 s timeout:
[safehttpclient.go#L20-L47](https://github.com/Notifuse/notifuse/blob/7516397/pkg/safehttpclient/safehttpclient.go#L20-L47),
[L72-L117](https://github.com/Notifuse/notifuse/blob/7516397/pkg/safehttpclient/safehttpclient.go#L72-L117)) is used by the public page fetcher, inbound SES parsing
and the broadcast data feed (grep `safehttpclient\.`: `public_handler.go:36`, `inbound_webhook_event_service.go:54,59,486`, `broadcast/data_feed.go:56`), **not by webhook delivery**.
Customer-supplied subscription URLs are therefore delivered with a client that has no private-address check and Go's default redirect policy.

**Tests.** Unit tests of the worker, and an integration test that drives the real worker against an `httptest` receiver through claim, POST, retry, 410, 429, auto-disable and the stale
claim sweep ([tests/integration/webhook_worker_e2e_test.go#L24-L40](https://github.com/Notifuse/notifuse/blob/7516397/tests/integration/webhook_worker_e2e_test.go#L24-L40)),
plus payload-sample and trigger tests.

### 2.5 BillionMail: no outbound webhooks

`grep -ril webhook` over the repo (excluding `node_modules`) finds `LAUNCH_PLAN.md` and `e2e/tests/test_15_full_pipeline.py`, where "webhooks" are the inbound
`/webhooks/billionmail/tracking` and `/reply` routes ([test_15 L52-L70](https://github.com/Billionmail/BillionMail/blob/fc36c76/e2e/tests/test_15_full_pipeline.py#L52-L70)).
Other outbound calls are product features (video generation, AI supplier, a daily usage post, see section 4), not user-configured hooks.

### 2.6 Drip (closed source; official API docs only)

Read at <https://developer.drip.com/> on 2026-10-18 (no commit; the page is an API reference). A webhook is created with `post_url` (required), `events` (array) and
`include_received_email` (default false); by default all events except `subscriber.received_email` are sent; creation returns 201. Payload: JSON with `event`,
`data` (`account_id`, `subscriber`, usually `properties`) and `occurred_at`. **No signature or verification header is documented.** No timeout is documented for webhooks.
Retries: "we will attempt to re-send webhooks for up to 3 days if they are rate limited"; the webhook is "temporarily disabled if its endpoint generates any other error for 3 days",
with a notification. Nothing was found about SSRF, delivery log or replay in the docs.

## 3. Other Go projects with outbound calls

### 3.1 Zitadel: Actions v2 targets

A **target** is a stored endpoint plus a type; an **execution** binds conditions (request, response, event or function) to a list of targets.

- **Config and storage.** Target fields: endpoint, type (webhook, call, async), timeout, interrupt-on-error, encrypted signing key, payload type (JSON, JWT, JWE)
  ([internal/execution/target/target.go#L9-L37](https://github.com/zitadel/zitadel/blob/5cd91e0/internal/execution/target/target.go#L9-L37),
  [proto/zitadel/action/v2/target.proto#L47-L54](https://github.com/zitadel/zitadel/blob/5cd91e0/proto/zitadel/action/v2/target.proto#L47-L54)).
  The three types differ on interruption: `webhook` ignores the response, `call` uses it, `async` is fired in a goroutine with a context detached from the request
  ([execution.go#L98-L118](https://github.com/zitadel/zitadel/blob/5cd91e0/internal/execution/execution.go#L98-L118)).
- **Signing.** `ZITADEL-Signature` computed over the body with the target's key and a timestamp (`actions.ComputeSignatureHeader`)
  ([execution.go#L228-L252](https://github.com/zitadel/zitadel/blob/5cd91e0/internal/execution/execution.go#L228-L252)); JWT/JWE payload types sign or encrypt the body itself.
  The notification webhook channel uses the same header with a hard-coded 5 s timeout
  ([internal/notification/channels/webhook/channel.go#L19-L61](https://github.com/zitadel/zitadel/blob/5cd91e0/internal/notification/channels/webhook/channel.go#L19-L61)).
- **SSRF.** `denylist.NewHTTPTransport` installs a `net.Dialer.Control` hook that checks each resolved IP right before the TCP connect, "to avoid DNS rebinding TOCTOU gaps"
  ([internal/denylist/http_transport.go#L10-L37](https://github.com/zitadel/zitadel/blob/5cd91e0/internal/denylist/http_transport.go#L10-L37)). The default `HTTPClient` config is
  timeout 30 s, 5 redirects, no https-to-http downgrade, and a `DenyList` of localhost plus reserved CIDRs; the comment warns that domain rules are string matches and the CIDRs do the real work
  ([cmd/defaults.yaml#L1112-L1135](https://github.com/zitadel/zitadel/blob/5cd91e0/cmd/defaults.yaml#L1112-L1135)). The operator can edit the list (so internal targets can be permitted).
- **Retries and failure.** The worker runs on river with a 10 s job cap and 5 min TTL; **any target error returns `river.JobCancel`**, i.e. no retry
  ([worker.go#L31-L57](https://github.com/zitadel/zitadel/blob/5cd91e0/internal/execution/worker.go#L31-L57),
  [defaults.yaml#L606-L621](https://github.com/zitadel/zitadel/blob/5cd91e0/cmd/defaults.yaml#L606-L621)). A target with `interrupt_on_error` stops the chain and fails the user's request;
  that is the design (synchronous hook), not delivery reliability.
- **Log / replay.** An execution log store (stdout by default, `defaults.yaml#L1067-L1070`) and an optional per-instance quota; no delivery table or replay found.
- **Tests.** `execution_test.go` calls `CallTarget` with a client built from `denylist.NewHTTPTransport` and a case "block request when target in denylist"
  ([execution_test.go#L426](https://github.com/zitadel/zitadel/blob/5cd91e0/internal/execution/execution_test.go#L426),
  [L588](https://github.com/zitadel/zitadel/blob/5cd91e0/internal/execution/execution_test.go#L588)).

### 3.2 Mattermost: outgoing webhooks

- **Config and storage.** An outgoing webhook has team, channel, trigger words, callback URLs (each must be an HTTP URL), content type, a generated `token`
  ([public/model/outgoing_webhook.go#L22-L29](https://github.com/mattermost/mattermost/blob/f38678d/server/public/model/outgoing_webhook.go#L22-L29),
  [L144](https://github.com/mattermost/mattermost/blob/f38678d/server/public/model/outgoing_webhook.go#L144),
  [L181-L182](https://github.com/mattermost/mattermost/blob/f38678d/server/public/model/outgoing_webhook.go#L181-L182)).
- **Signing.** None; the 26-character `token` travels in the payload (JSON or form values) for the receiver to compare. An optional outgoing OAuth connection
  adds an `Authorization` header by audience URL ([channels/app/webhook.go#L143-L160](https://github.com/mattermost/mattermost/blob/f38678d/server/channels/app/webhook.go#L143-L160)).
- **Delivery.** `TriggerWebhook` starts one goroutine per callback URL, waits for all, **logs failures and drops them**; there is no retry and no queue
  ([webhook.go#L100-L211](https://github.com/mattermost/mattermost/blob/f38678d/server/channels/app/webhook.go#L100-L211)). The response body can become a reply post in the channel (templating in
  reverse). Timeout is `ServiceSettings.OutgoingIntegrationRequestsTimeout`, default 30 s
  ([webhook.go#L213-L232](https://github.com/mattermost/mattermost/blob/f38678d/server/channels/app/webhook.go#L213-L232),
  [model/config.go#L266](https://github.com/mattermost/mattermost/blob/f38678d/server/public/model/config.go#L266)).
- **SSRF.** `outgoingWebhookClient = httpService.MakeClient(false)`
  ([server.go#L457](https://github.com/mattermost/mattermost/blob/f38678d/server/channels/app/server.go#L457)). `dialContextFilter` resolves the host, checks each IP
  against a reserved-range list and dials the **checked IP** ([httpservice/client.go#L129-L181](https://github.com/mattermost/mattermost/blob/f38678d/server/public/shared/httpservice/client.go#L129-L181));
  the comment says the list is taken from `doyensec/safeurl`'s `ip.go` ([L80](https://github.com/mattermost/mattermost/blob/f38678d/server/public/shared/httpservice/client.go#L80)). The operator setting
  `AllowedUntrustedInternalConnections` whitelists hosts and IPs ([config.go#L435](https://github.com/mattermost/mattermost/blob/f38678d/server/public/model/config.go#L435),
  [httpservice.go#L76-L108](https://github.com/mattermost/mattermost/blob/f38678d/server/public/shared/httpservice/httpservice.go#L76-L108)); the error message tells the admin to set it. `MakeClient` sets no
  `CheckRedirect` ([httpservice.go#L67-L72](https://github.com/mattermost/mattermost/blob/f38678d/server/public/shared/httpservice/httpservice.go#L67-L72)), so Go's default redirect policy applies and each hop dials through the filter.
- **Tests.** The webhook tests start `httptest` servers and set `AllowedUntrustedInternalConnections = "localhost,127.0.0.1"` so the production guard stays on
  ([channels/app/webhook_test.go#L1667-L1679](https://github.com/mattermost/mattermost/blob/f38678d/server/channels/app/webhook_test.go#L1667-L1679)).

### 3.3 Coder: notification webhook (deployment-wide)

- **Config and storage.** One endpoint for the whole deployment (`--notifications-webhook-endpoint`, `CODER_NOTIFICATIONS_METHOD=webhook`), not per user or workspace
  ([codersdk/deployment.go#L1397](https://github.com/coder/coder/blob/1213938/codersdk/deployment.go#L1397),
  [L4251-L4256](https://github.com/coder/coder/blob/1213938/codersdk/deployment.go#L4251-L4256)).
- **Signing and SSRF.** None. The handler POSTs JSON with `X-Message-Id`; its client is a clone of the default transport ("to avoid cross-contamination"), no guard, default redirects
  ([coderd/notifications/dispatch/webhook.go#L41-L58](https://github.com/coder/coder/blob/1213938/coderd/notifications/dispatch/webhook.go#L41-L58),
  [L94-L104](https://github.com/coder/coder/blob/1213938/coderd/notifications/dispatch/webhook.go#L94-L104)).
- **Retry.** The delivery func returns `(retryable bool, err)`: timeouts, transport errors and any non-2xx are retryable
  ([L103-L126](https://github.com/coder/coder/blob/1213938/coderd/notifications/dispatch/webhook.go#L103-L126)); the notifier keeps a message as a temporary failure until `max_send_attempts`
  (default 5), then a permanent failure ([notifier.go#L344-L362](https://github.com/coder/coder/blob/1213938/coderd/notifications/notifier.go#L344-L362);
  [cli/testdata/coder_server_--help.golden#L669-L673](https://github.com/coder/coder/blob/1213938/cli/testdata/coder_server_--help.golden#L669-L673); dispatch timeout 1 minute). Messages are database rows
  with leasing (`LeasePeriod`, `LeaseCount`, `deployment.go#L1384-L1386`), so status and attempt count are queryable.
- **Tests.** `TestWebhookDispatch` runs a mock HTTP server and asserts the received payload
  ([notifications_test.go#L194-L255](https://github.com/coder/coder/blob/1213938/coderd/notifications/notifications_test.go#L194-L255)).

### 3.4 Temporal: Nexus completion callbacks

Callbacks are the one place the Temporal server calls a user-supplied HTTP URL. The URL and headers are attached to a workflow or activity execution at start.

- **SSRF model.** An **allowlist**: `callback.allowedAddresses` per namespace is a list of host:port patterns (wildcards) with an `AllowInsecure` flag; "the default is no address rules",
  so an external URL is refused until an operator adds a rule; `temporal://system` is always allowed
  ([chasm/lib/callback/config.go#L86-L98](https://github.com/temporalio/temporal/blob/cc912e1/chasm/lib/callback/config.go#L86-L98),
  [common/callbacks/address.go#L19-L43](https://github.com/temporalio/temporal/blob/cc912e1/common/callbacks/address.go#L19-L43), enforced at attach time in
  [validator.go#L142](https://github.com/temporalio/temporal/blob/cc912e1/common/callbacks/validator.go#L142)). The check is on the URL host pattern; no resolved-IP check was found in the files read.
- **Timeout / retry.** 10 s per request, exponential backoff from 1 s to 1 h with no expiry
  ([config.go#L29-L45](https://github.com/temporalio/temporal/blob/cc912e1/chasm/lib/callback/config.go#L29-L45),
  [L62-L76](https://github.com/temporalio/temporal/blob/cc912e1/chasm/lib/callback/config.go#L62-L76)). A Nexus handler error says whether it is retryable; any other error is retryable
  ([invocable_outbound.go#L92-L112](https://github.com/temporalio/temporal/blob/cc912e1/chasm/lib/callback/invocable_outbound.go#L92-L112)).
- **Peer-down behaviour.** A retryable failure is wrapped in `DestinationDownError`
  ([L35-L40](https://github.com/temporalio/temporal/blob/cc912e1/chasm/lib/callback/invocable_outbound.go#L35-L40)), which feeds a per-destination circuit breaker for the outbound queue
  (`OutboundQueueCircuitBreakerSettings`, [service/history/configs/config.go#L205](https://github.com/temporalio/temporal/blob/cc912e1/service/history/configs/config.go#L205)).
  One dead destination therefore backs off without blocking others (the breaker semantics were read from names and comments only).
- **Signing, log.** A callback token header carries identity ([validator.go#L207](https://github.com/temporalio/temporal/blob/cc912e1/common/callbacks/validator.go#L207)); per-destination metrics for outcome and latency, no delivery table.

### 3.5 Hatchet: Slack alerts, a deprecated webhook worker, and an unwired safe client

- **Slack alerts.** The Slack incoming-webhook URL is obtained from Slack's OAuth install (not typed by a tenant) and **stored encrypted**
  ([api/v1/server/handlers/slack-app/oauth_callback.go#L58](https://github.com/hatchet-dev/hatchet/blob/4cc02dd/api/v1/server/handlers/slack-app/oauth_callback.go#L58)); the alerter decrypts it and calls
  `slack.PostWebhook` ([internal/integrations/alerting/slack.go#L12-L32](https://github.com/hatchet-dev/hatchet/blob/4cc02dd/internal/integrations/alerting/slack.go#L12-L32)). Alert frequency is
  throttled by a tenant `MaxFrequency` setting ([pkg/repository/tenant_alerting.go#L17-L22](https://github.com/hatchet-dev/hatchet/blob/4cc02dd/pkg/repository/tenant_alerting.go#L17-L22)).
- **Webhook workers.** The create handler answers "WebhookCreate is deprecated"
  ([api/v1/server/handlers/webhook-worker/create.go#L10-L14](https://github.com/hatchet-dev/hatchet/blob/4cc02dd/api/v1/server/handlers/webhook-worker/create.go#L10-L14)); no engine-to-worker HTTP push was found live.
- **`pkg/operator/safeclient`** is documented as "an SSRF-hardened HTTP client for delivering outbound requests to user-supplied endpoints", built on `doyensec/safeurl`: https only, port 443 only,
  every internal range blocked plus operator-supplied infrastructure CIDRs (construction **fails if none are given** unless explicitly allowed for local dev), redirects never followed, response capped at
  4 MiB, no overall timeout (the caller's context owns the deadline), HTTP/1.1 only so pooled connections are bounded
  ([safeclient.go#L1-L49](https://github.com/hatchet-dev/hatchet/blob/4cc02dd/pkg/operator/safeclient/safeclient.go#L1-L49),
  [L93-L98](https://github.com/hatchet-dev/hatchet/blob/4cc02dd/pkg/operator/safeclient/safeclient.go#L93-L98),
  [blocklist.go#L11-L45](https://github.com/hatchet-dev/hatchet/blob/4cc02dd/pkg/operator/safeclient/blocklist.go#L11-L45)). **At this commit nothing imports it**: the only reference outside the package is a
  config comment saying the engine does not read `OperatorInfraBlockedCIDRs` yet
  ([pkg/config/server/server.go#L295-L302](https://github.com/hatchet-dev/hatchet/blob/4cc02dd/pkg/config/server/server.go#L295-L302)). It is the most explicit statement of a SaaS-grade outbound policy among the projects read.
  Worker-to-engine links (gRPC with tenant API tokens) are service-to-service, not outbound hooks, and are not covered here.

## 4. Short secondary section: instance-to-vendor and service-credential links

Kept short on purpose (the scope moved to outbound hooks). Facts only.

- **Vendor phone-home in the projects read.** BillionMail posts counts of sent mails and relays once a day to a third-party URL
  ([core/internal/service/collect/collect.go#L10-L52](https://github.com/Billionmail/BillionMail/blob/fc36c76/core/internal/service/collect/collect.go#L10-L52),
  registered in `service/timers/timers.go`); whether it can be switched off was not checked. Keila checks for a release only
  ([instance.ex#L58](https://github.com/pentacent/keila/blob/2308beb/lib/keila/instance.ex#L58)).
- **Sphericon.** ADR 0032 defines a daily anonymous Telemetry report (opt-out, fire-and-forget, 5 s timeout, no retry) and a separate release check; the receiver lives outside the repo. ADR 0035: an expired license key loads the core instead of failing boot (`docs/adr/0035-an-expired-license-key-falls-back-to-the-core.md`).
- **Platform-to-instance direction (ADR 0026).** A platform service calls the `/service/*` endpoints with a Service credential `smsc_<prefix>_<secret>` (SHA-256 stored), limited to scopes (`usage:read`,
  `workspaces:read`, `holds:write`), created, rotated (a second credential, then revoke the old) and revoked only by CLI, served only on the internal listener, 404 on the public mux
  ([docs/adr/0026-operator-console-and-abuse-detector.md#L40-L58](../../docs/adr/0026-operator-console-and-abuse-detector.md);
  `ee/staff/service.go#L42-L54` maps each operation to scopes, an unlisted operation is refused to all; `internal/server/server.go#L176-L192` builds the internal handler with a nil handler when no `staff` license).
  This is a pull-style contract for usage and holds; it is not an outbound hook.

## 5. Sphericon today (outbound webhooks)

All paths are at `ad4e5c2e`.

- **Entity.** `WebhookEndpoint` (workspace-scoped through `WorkspaceMixin`): `url` (marked `Sensitive`, because it may carry a token), `secret_encrypted`, `event_types` (empty = all), `enabled`
  ([ent/schema/webhook_endpoint.go#L13-L56](../../ent/schema/webhook_endpoint.go)). It is an audited entity (`Audited{Action: "webhook_endpoint", NameField: "url", NameIsURL: true}`), so changes land in the Audit log. No
  columns for failure counters, last delivery, disabled reason, custom headers or a name. Glossary: "Webhook endpoint (outbound)" signs each delivery (HMAC), outbound only (`GLOSSARY.md#L431-L435`).
- **Module.** `internal/webhooks/webhooks.go`: create/update/get/list/delete under a `*ent.Scoped`; the secret is generated server-side as `whsec_` plus 24 random bytes (Standard Webhooks format), sealed with
  `secrets.Cipher`, and returned decrypted ([L169-L177](../../internal/webhooks/webhooks.go)); `validURL` accepts absolute http(s) URLs only, with a note that network-level SSRF defences are in the delivery worker
  ([L179-L184](../../internal/webhooks/webhooks.go)); selecting `audit.entry` needs an Enterprise license ([L78-L83](../../internal/webhooks/webhooks.go)). There is no way to rotate the secret: `UpdateInput` has URL, event types and
  enabled only ([L52-L57](../../internal/webhooks/webhooks.go)). The site resource returns the secret; the external resource omits it (`typespec/external/resources/webhooks.tsp#L7`). The site TypeSpec doc string still says the secret verifies an
  `X-sphericon-Signature` header ([typespec/site/resources/webhooks.tsp#L18](../../typespec/site/resources/webhooks.tsp)), while the code sends the `webhook-*` headers: documentation drift.
- **Fan-out.** The `webhooks` bus consumer decodes the envelope, picks the action name (or the bus name for an unprojected Audit entry, masking the staff identity), uses `DedupKey` as id when present, marshals the public
  `webhookPayload` (`id, type, occurredAt, workspaceId, subject, contactId, data`) and calls `Dispatch` with `client.Scoped(env.WorkspaceID)`
  ([internal/events/subscriber.go#L201-L262](../../internal/events/subscriber.go)). `Dispatch` loads the workspace's enabled endpoints, filters by event name (`matchesEvent`, empty filter excludes unprojected events), and inserts one river job per
  endpoint on queue `webhooks` with `MaxAttempts: 10` ([internal/jobs/webhook.go#L62-L93](../../internal/jobs/webhook.go)).
- **Delivery.** `DeliverWebhookWorker.Work` loads the endpoint (raw client, a documented allowlist entry), drops silently when it is deleted or disabled, decrypts the secret and calls `webhook.Send`
  ([internal/jobs/webhook.go#L35-L56](../../internal/jobs/webhook.go)). `Send` signs `webhook-id` (the delivery id), `webhook-timestamp` (per attempt), `webhook-signature` with the standard-webhooks library, adds `X-sphericon-Event`,
  POSTs JSON, reads at most 64 KiB of the response and returns an error on non-2xx ([internal/webhook/client.go#L50-L85](../../internal/webhook/client.go)). The id is stable across retries so receivers can dedupe.
- **SSRF.** `NewClient` is `doyensec/safeurl`: resolved-IP check at dial (private, loopback, link-local, CGNAT, metadata), http(s) only, IPv6 on, redirects not followed
  ([internal/webhook/client.go#L34-L48](../../internal/webhook/client.go)); safeurl's own list is in `safeurl@v0.2.5/ip.go` (includes 169.254/16 and 100.64/10, `ip.go#L16`, `#L26`). There is no operator allowlist, so a self-hoster cannot
  point an endpoint at an internal service (such as a n8n or Node-RED container on the same network).
- **Timeouts and concurrency.** 15 s per request ([internal/jobs/worker.go#L26-L27](../../internal/jobs/worker.go), client built at [L79-L83](../../internal/jobs/worker.go)); queue `webhooks` has `MaxWorkers: 5`
  ([L136](../../internal/jobs/worker.go)), shared by every workspace and endpoint.
- **Retry.** Not configured: river's default policy `attempt^4` seconds with jitter (`river@v0.49.0/internal/retrypolicy/default.go#L86-L89`); with 10 attempts the waits are 1, 16, 81, 256, 625, 1296, 2401, 4096 and 6561 s, about 4h15m
  in total by my arithmetic (exact off-by-one on the first wait and the jitter size were not checked). After the last attempt the job is discarded; the centralized `errorHandler` logs each error
  ([internal/jobs/errorhandler.go#L19-L33](../../internal/jobs/errorhandler.go)). No state records that an endpoint is failing.
- **Erasure.** In-flight deliveries carry the body in the job args; `PurgeWebhookJobs` deletes queued deliveries for a contact's erasure
  ([internal/events/purge.go#L34-L38](../../internal/events/purge.go), ADR 0021).
- **Tests.** `internal/webhook/client_test.go` checks that the hardened client refuses loopback and that a standard-webhooks receiver verifies the signature and 2xx/non-2xx outcomes
  (L22-L68); `internal/jobs/workers_test.go#L265` runs the worker against a recording fake `Doer` and fixture endpoints (success, deleted, disabled, 500, transport error);
  `internal/events/subscriber_test.go#L65` and `audit_internal_test.go` cover the payload; `internal/webhooks/webhooks_test.go` covers the module. No end-to-end scenario in `e2e/` mentions webhooks
  (grep), and no test runs the real hardened client against a server it is allowed to reach.

## 6. Gaps and ideas for sphericon

Ideas, not decisions. Each ties to a fact above.

1. **No visible state of delivery.** A customer cannot see that an endpoint fails, why, or what was lost; the job is discarded after about 4h15m. Notifuse (delivery table with last status, body and error, list API) and Mautic (capped
   log per webhook) store this; the spec asks for a failure list and replay (spec L321). River's job table already holds attempts and errors for a while; a stored per-attempt row or an endpoint-level
   `last_failure`/`consecutive_failures` is the choice to make. No project read has a replay endpoint, so replay would be new ground beyond "list failures".
2. **No dead-endpoint rule.** Mautic disables on 410 and after N failures; Notifuse disables after a count plus a time window and records `disabled_reason`; Drip disables after 3 days and notifies; the spec
   says disable on 410 and on persistent failure and notify by another channel. Notifuse's comments are the best statement of the pitfalls (a restart during a burst must not retire a healthy endpoint; 429 is not a failure;
   Zapier catch hooks always answer 2xx).
3. **Retry horizon is short.** River's default gives about 4h15m over ten attempts; the spec recommends days, Notifuse reaches 9h53m, Temporal never expires. A custom `ClientRetryPolicy`/per-job `NextRetry` is a
   configuration, not a new subsystem. 429 and `retry-after` are not special-cased today (`webhook.Send` returns one generic error).
4. **Self-hosters cannot reach internal targets.** Mattermost (`AllowedUntrustedInternalConnections`), Mautic (`webhook_allowed_private_addresses`) and Zitadel (editable `DenyList`) all give the operator an opt-in; sphericon blocks private
   ranges unconditionally. A SaaS instance should keep the unconditional default (Hatchet's `safeclient` goes further: https and 443 only, plus mandatory infrastructure CIDRs), so the question is an instance-level setting that is off by default and
   absent in the SaaS deployment. Safeurl supports allowed hosts and ports in its config builder (`config.go#L98-L128` in the module).
5. **One slow endpoint can hold the queue.** `QueueWebhooks` has 5 workers for all workspaces and each request may take 15 s. Temporal's per-destination circuit breaker and Notifuse's per-row claim with a lease are the two
   designs seen; no project read limits concurrency per endpoint explicitly (not searched exhaustively). This is an inference from the numbers, not an observed incident.
6. **No secret rotation, no test send.** Notifuse has `regenerateSecret` and a `test` endpoint that sends a sample of a chosen event type; sphericon has neither (the secret is fixed for the endpoint's life). The spec's
   signature header can carry several space-separated signatures, which is what a graceful rotation uses (not verified in the library source here).
7. **Customization is absent everywhere except Mautic's campaign action.** No project read lets the endpoint owner template the body or add headers on a general event webhook (Notifuse, Mautic webhooks, Coder, Mattermost all send a fixed
   envelope; Zitadel lets the target rewrite the response, the opposite direction). The Mautic campaign action is the only user-templated outbound call, and it is also the one without the SSRF check: a lesson if sphericon later adds
   an "HTTP request" automation step (a second caller must use `webhook.NewClient`, not a new client).
8. **Stale API text.** `typespec/site/resources/webhooks.tsp` mentions `X-sphericon-Signature`; the real headers are `webhook-id`, `webhook-timestamp`, `webhook-signature`, `X-sphericon-Event`. The generated site client, OpenAPI file and
   docs carry the stale line until the TypeSpec comment is fixed and regenerated.
9. **Test shape.** The strongest example is Mattermost, which keeps the production guard on and permits the `httptest` host through the operator allowlist; Notifuse's e2e drives the real worker to a real receiver. Sphericon's worker test substitutes a recording `Doer` and
   asserts the URLs and bodies it received, which sits against the repo's rule that a test asserts what an operation left behind; an allowlist setting (gap 4) would also let a test run the real client against `httptest`.
