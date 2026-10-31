# Send limits on bring-your-own-provider platforms: who sets the ceiling

Researched October 2026 from primary sources only: the vendors' own help centers, docs and API references (AWS SES,
Google Workspace, Yandex Cloud Postbox, Spamhaus, Customer.io, Postmark, Klaviyo, Mailchimp, Twilio SendGrid, Loops,
Brevo, Mailcoach, Sendy, EmailOctopus, Acelle Mail) and the source of four open-source senders, shallow-cloned at the
commits below. Code was read, never run. No third-party blog or comparison site is cited. A claim that could not be read
on a primary source is moved to "Unconfirmed" at the end. sphericon's side is read from
[ADR 0023](../adr/0023-send-rate-limit-per-integration.md), [ADR 0009](../adr/0009-usage-metering-and-operator-holds.md),
[ADR 0038](../adr/0038-tracking-domains-are-core-and-the-edge-is-not.md) and [GLOSSARY.md](../../GLOSSARY.md) (Send
rate limit, Deferral, Hold, Staff hold, Workspace suspension, Tracking domain).

| Repo                    | Branch | Commit                                     | Last commit |
| ----------------------- | ------ | ------------------------------------------ | ----------- |
| knadh/listmonk          | master | `b1f8abd7ef1c31315b3d62135437fffb2f4d63f9` | 2026-10-29  |
| pentacent/keila         | main   | `508eea54fa917fe5c866550b2b4939625c7587e1` | 2026-10-28  |
| mautic/mautic           | 7.x    | `ee2b5b60dd2490ecc5a1631d726cb904a5ffd06a` | 2026-10-30  |
| Mailtrain-org/mailtrain | v2     | `7c6f34ef25dba981de58aec9785af48f3ea3315d` | 2022-12-11  |

Base URLs: `https://github.com/knadh/listmonk/blob/b1f8abd/<path>#L<n>`, and likewise for the others.

## Question

Customers bring their own email provider (SES, an SES-compatible service such as Postbox, or any SMTP server) as an
Integration, and each Integration has a customer-editable Send rate limit; for SES the provider quota is read with
`GetAccount` and the lower of the two applies. The platform can stop a Workspace with a Staff hold, but has no
Staff-set send ceiling. Two questions:

1. Does a hosted platform need its own send ceiling per Workspace or account, set by Staff and not editable by the
   customer, even when the customer brings the provider? How do real products cap new accounts, review them and stop
   them, and what risk does the platform carry when customers send through their own SES or SMTP (shared link domains
   on a blocklist, shared worker capacity, fairness between tenants)?
2. Is a customer-set sending-rate limit a common, needed feature? Who exposes one, under what name, and what do the
   others do instead (read provider quotas, back off on throttling replies)? What are the provider limits, and can
   they be read by API?

## Summary of findings

- **Hosted operators keep a lever above the customer's own setting.** Keila (the closest prior art: open core with a
  hosted service) combines a customer-set rate per Sender with an operator rate per adapter, read from the operator's
  environment, and the lower capacity wins. Its hosted accounts pass an onboarding review (`under_review`) before the
  first campaign, and its operator quota counts every send whichever Sender carries it. Acelle and Sendy give the
  operator a per-customer quota too.
- **Platform ESPs gate, ramp and pause; few of them cap the rate for good.** SES, SendGrid, Klaviyo and Keila Cloud
  review an account before or at the first send. Customer.io ramps new accounts from 500 a day to the full limit over
  about six weeks on a rolling 24 hours. Keila Cloud's code raises its shared sender's rate from 5 to 15 per second
  once the sender's domain is verified, though nothing calls that code at the commit read. Postmark and SendGrid
  pause on thresholds with mail queued, and SES pauses at 10% bounces or 0.5% complaints. No source read here caps
  bring-your-own sending for the provider's sake.
- **A customer-set rate limit is common in bring-your-own tools and rare in platform ESPs.** listmonk, Keila,
  Mailcoach, Mailtrain, Acelle, EmailOctopus and Sendy all expose one, under names from "Message rate" to
  "Throttling". Keila, Mailcoach and Mailtrain set it per sending account, as ADR 0023 does. Customer.io is the only
  platform ESP with one, and it is per automation.
- **Only SES lets a tool read the limit.** `GetAccount` returns the quota, the sandbox flag and `EnforcementStatus`;
  an enforcement pause shows only in `EnforcementStatus`, and SES emits no event for it. Postbox's AWS-compatible API
  has no quota call, and new Postbox accounts start at 1 per second and 200 a day. Gmail (2,000 a day per user) and
  the Workspace SMTP relay (10,000 a day per user) are known only from their docs and from refusals.
- **Nobody adapts the rate to throttling replies.** SES drops a throttled message and advises a retry within 10
  minutes. listmonk pauses a campaign after N errors, Keila fails the message, Mautic retries with back-off, and
  EmailOctopus pauses until the quota returns. ADR 0023 already treats throttling as a Deferral.
- **The shared risk is the link domain.** Spamhaus's DBL lists domains found in message bodies, and a listing covers
  every subdomain. sphericon builds all links of a message on the serving Tracking domain or, failing that, on the
  Instance's own host. So every tenant without a serving Tracking domain shares one listable domain, whoever its
  provider is.

### Answers to the two questions

1. **A Staff ceiling.** The prior art does not require one for bring-your-own sending. It does show hosted operators
   keeping one, together with review before the first send and a pause at thresholds. sphericon has the gate (a Staff
   hold) and the pause (suspension). The missing piece is a ceiling short of a stop, which would be a Staff term in
   ADR 0023's minimum. Section 5 lists the options and trade-offs.
2. **A customer-set rate limit.** It is a standard feature of bring-your-own tools. It is needed most where the
   provider cannot be read (Postbox, SMTP, Gmail) or where the provider account is shared with other software.

## 1. Bring-your-own-provider tools

### listmonk (source)

listmonk is self-hosted only; every limit is an instance-wide setting of the operator.

- **Message rate is per worker, so the real rate is `concurrency × message_rate`.** Defaults are `app.concurrency` 10
  and `app.message_rate` 10 ([schema.sql#L237-L238](https://github.com/knadh/listmonk/blob/b1f8abd/schema.sql#L237-L238)).
  Each worker goroutine keeps its own counter and sleeps one second when it reaches `MessageRate`
  ([internal/manager/manager.go#L501-L525](https://github.com/knadh/listmonk/blob/b1f8abd/internal/manager/manager.go#L501-L525)),
  and the help text says so: "If concurrency = 10 and message_rate = 10, then up to 10x10=100 messages may be pushed
  out every second. This, along with concurrency, should be tweaked to keep the net messages going out per second
  under the target message servers rate limits if any"
  ([i18n/en.json#L554](https://github.com/knadh/listmonk/blob/b1f8abd/i18n/en.json#L554)).
- **A sliding window caps a longer period**, off by default: `app.message_sliding_window` false, duration `1h`, rate
  10 000 ([schema.sql#L241-L243](https://github.com/knadh/listmonk/blob/b1f8abd/schema.sql#L241-L243)). "On reaching
  this limit, messages are held from being sent until the time window clears"
  ([i18n/en.json#L557](https://github.com/knadh/listmonk/blob/b1f8abd/i18n/en.json#L557)). The counter lives on the
  Manager, so it is shared by every running campaign, and when it is reached the campaign's pipe blocks in
  `time.Sleep` for the rest of the window
  ([internal/manager/pipe.go#L120-L166](https://github.com/knadh/listmonk/blob/b1f8abd/internal/manager/pipe.go#L120-L166)).
  It is a fixed window that resets after its duration, not a rolling one.
- **The only per-SMTP-server knob is `max_conns`** ("Maximum concurrent connections to the server",
  [i18n/en.json#L515](https://github.com/knadh/listmonk/blob/b1f8abd/i18n/en.json#L515); default 10 in
  [schema.sql#L285](https://github.com/knadh/listmonk/blob/b1f8abd/schema.sql#L285)). There is no rate per SMTP
  server or per messenger.
- **Errors pause a campaign, they do not slow it.** `app.max_send_errors` defaults to 1000
  ([schema.sql#L240](https://github.com/knadh/listmonk/blob/b1f8abd/schema.sql#L240)): "The number of errors (eg:
  SMTP timeouts while e-mailing) a running campaign should tolerate before it is paused for manual investigation or
  intervention" ([i18n/en.json#L552](https://github.com/knadh/listmonk/blob/b1f8abd/i18n/en.json#L552),
  [internal/manager/pipe.go#L172-L187](https://github.com/knadh/listmonk/blob/b1f8abd/internal/manager/pipe.go#L172-L187)).
  No code path recognises a throttling reply or backs off on one.

### Keila (source, including the Keila Cloud code)

Keila is AGPL; the hosted service's own code sits in `extra/`, which "is excluded from the AGPLv3 license and
provided for informational purposes only" ([extra/README.md](https://github.com/pentacent/keila/blob/508eea5/extra/README.md)).
It is described here, not reused.

- **The customer sets a rate per Sender**: "Rate limit per second", "per minute" and "per hour" on the Sender form
  ([lib/keila_web/templates/sender/\_config_edit.html.heex#L11-L28](https://github.com/pentacent/keila/blob/508eea5/lib/keila_web/templates/sender/_config_edit.html.heex#L11-L28)),
  stored as `rate_limit_per_second|minute|hour` on the Sender's config
  ([lib/keila/mailings/schemas/sender_config.ex#L12-L14](https://github.com/pentacent/keila/blob/508eea5/lib/keila/mailings/schemas/sender_config.ex#L12-L14)).
  Unset, the Sender is unlimited
  ([lib/keila/mailings/rate_limiter.ex#L169-L180](https://github.com/pentacent/keila/blob/508eea5/lib/keila/mailings/rate_limiter.ex#L169-L180)).
- **A second, operator-side limit applies to every Sender of an adapter.** The adapter behaviour has an optional
  `adapter_rate_limit/0`, "global rate limits for all Senders using the adapter"
  ([lib/keila/mailings/sender_adapters/adapter.ex#L190-L205](https://github.com/pentacent/keila/blob/508eea5/lib/keila/mailings/sender_adapters/adapter.ex#L190-L205)).
  No adapter in `lib/` implements it. Keila Cloud's shared "Send with Keila" adapter does, from the environment
  variables `SWK_RATE_LIMIT_HOUR|MINUTE|SECOND`
  ([extra/keila_cloud/config/runtime_config.ex#L45-L54](https://github.com/pentacent/keila/blob/508eea5/extra/keila_cloud/config/runtime_config.ex#L45-L54),
  [extra/keila_cloud/mailings/send_with_keila.ex#L154-L158](https://github.com/pentacent/keila/blob/508eea5/extra/keila_cloud/mailings/send_with_keila.ex#L154-L158)).
- **Both are token buckets and the lower capacity wins.** "Module for enforcing Sender-based and Adapter-based rate
  limits implemented using the token bucket algorithm"
  ([lib/keila/mailings/rate_limiter.ex#L1-L10](https://github.com/pentacent/keila/blob/508eea5/lib/keila/mailings/rate_limiter.ex#L1-L10)).
  A message is scheduled only after it takes a Sender token and an adapter token, and a Sender's capacity is the
  minimum of its own, its adapter's and a fixed 500
  ([lib/keila/mailings/scheduler.ex#L208-L235](https://github.com/pentacent/keila/blob/508eea5/lib/keila/mailings/scheduler.ex#L208-L235),
  [#L341-L351](https://github.com/pentacent/keila/blob/508eea5/lib/keila/mailings/scheduler.ex#L341-L351)). Bucket
  state lives in ETS and is persisted to the database
  ([lib/keila/mailings/scheduler.ex#L6-L8](https://github.com/pentacent/keila/blob/508eea5/lib/keila/mailings/scheduler.ex#L6-L8)).
- **Fairness between tenants is round-robin.** Senders with ready messages are grouped by adapter, and each tick
  walks the senders of a partition with a rotating offset, one message per turn, until the adapter's tokens (at most
  500 per tick) run out
  ([lib/keila/mailings/scheduler.ex#L195-L235](https://github.com/pentacent/keila/blob/508eea5/lib/keila/mailings/scheduler.ex#L195-L235),
  [#L296-L313](https://github.com/pentacent/keila/blob/508eea5/lib/keila/mailings/scheduler.ex#L296-L313)). One
  large campaign therefore cannot starve another Sender on the same adapter.
- **The hosted shared sender has a trust step tied to domain verification.** `Mx2.rate_limit/1` returns
  `[hour: 2500, second: 5]` while the Sender uses the fallback domain and `[hour: 15000, second: 15]` once its own
  domain is verified
  ([extra/keila_cloud/mailings/send_with_keila/mx2.ex#L250-L266](https://github.com/pentacent/keila/blob/508eea5/extra/keila_cloud/mailings/send_with_keila/mx2.ex#L250-L266)).
  At this commit the per-Sender `rate_limit/1` callback has no caller outside the adapter's own wrapper: the
  `RateLimiter` reads only the Sender's config and `adapter_rate_limit/0`. Whether those numbers are enforced
  elsewhere could not be traced.
- **Hosted accounts are reviewed before the first campaign.** A Keila Cloud account has the statuses `default`,
  `active`, `suspended`, `onboarding_required` and `under_review`
  ([extra/keila_cloud/schemas/account.ex#L24-L31](https://github.com/pentacent/keila/blob/508eea5/extra/keila_cloud/schemas/account.ex#L24-L31)).
  Onboarding asks for a `sending_purpose` and whether an import is planned and from where
  ([extra/keila_cloud/schemas/account_onboarding_review_data.ex#L10-L28](https://github.com/pentacent/keila/blob/508eea5/extra/keila_cloud/schemas/account_onboarding_review_data.ex#L10-L28)),
  after which the account moves to `under_review`
  ([extra/keila_cloud_web/controllers/cloud_account_onboarding_live.ex#L185-L186](https://github.com/pentacent/keila/blob/508eea5/extra/keila_cloud_web/controllers/cloud_account_onboarding_live.ex#L185-L186)).
  A campaign of an account that is not `active` is rolled back as `:account_not_active`
  ([lib/keila/mailings/mailings.ex#L631-L646](https://github.com/pentacent/keila/blob/508eea5/lib/keila/mailings/mailings.ex#L631-L646)).
- **The operator's quota counts every send, whichever Sender carries it.** With `ENABLE_QUOTAS` on
  ([config/runtime.exs#L401-L403](https://github.com/pentacent/keila/blob/508eea5/config/runtime.exs#L401-L403)),
  each campaign and transactional message consumes credits of the account and is refused when they run out
  ([lib/keila/mailings/mailings.ex#L631-L646](https://github.com/pentacent/keila/blob/508eea5/lib/keila/mailings/mailings.ex#L631-L646),
  [lib/keila/mailings/transactional_message.ex#L164-L171](https://github.com/pentacent/keila/blob/508eea5/lib/keila/mailings/transactional_message.ex#L164-L171)).
  An operator can also forbid Senders that do not use a shared sender, with `DISABLE_SENDER_CREATION`
  ([config/runtime.exs#L397-L399](https://github.com/pentacent/keila/blob/508eea5/config/runtime.exs#L397-L399)).
- **A provider error is final.** The delivery worker marks the message failed and cancels the job on any error it does
  not recognise ("Another error occurred. Sending is not retried.",
  [lib/keila/mailings/delivery_worker.ex#L186-L196](https://github.com/pentacent/keila/blob/508eea5/lib/keila/mailings/delivery_worker.ex#L186-L196)).
  No code reads an SES quota.

### Mautic (source)

Mautic has no send-rate limit in the core. What looks like one is something else:

- `email_frequency_number` and `email_frequency_time` (default 0 per `DAY`) are a per-contact frequency rule: how
  many messages one contact may receive, not how fast the instance sends
  ([app/bundles/EmailBundle/Config/config.php#L200-L201](https://github.com/mautic/mautic/blob/ee2b5b6/app/bundles/EmailBundle/Config/config.php#L200-L201)).
- `mautic:broadcasts:send` takes `--limit` ("how many contacts to load from database to process"), `--batch` ("how
  many messages to send at once") and `--max-threads`
  ([app/bundles/ChannelBundle/Command/SendChannelBroadcastCommand.php#L60-L82](https://github.com/mautic/mautic/blob/ee2b5b6/app/bundles/ChannelBundle/Command/SendChannelBroadcastCommand.php#L60-L82)).
  These bound one cron run; throughput is the cron frequency times the limit.
- The Messenger transport retries a failed send 3 times, 1 s apart with a multiplier of 2
  ([app/bundles/MessengerBundle/Config/config.php#L10-L13](https://github.com/mautic/mautic/blob/ee2b5b6/app/bundles/MessengerBundle/Config/config.php#L10-L13)).
  That is failure back-off, not pacing.

### Mailtrain v2 (source, dormant since December 2022)

- **Throttling is per send configuration, in messages per hour**: "Maximum number of messages to send in an hour.
  Leave empty or zero for no throttling. If your provider uses a different speed limit (messages/minute or
  messages/second) then convert this limit into messages/hour (1m/s => 3600m/h). This limit is per sending process"
  ([locales/en-US/common.json#L959](https://github.com/Mailtrain-org/mailtrain/blob/7c6f34e/locales/en-US/common.json#L959)).
  For SES the value is passed to the transport as `sendingRate` per second
  ([server/lib/mailers.js#L234-L251](https://github.com/Mailtrain-org/mailtrain/blob/7c6f34e/server/lib/mailers.js#L234-L251)).
- **A warm-up scales it linearly.** `throttlingWarmUpDays` and `throttlingWarmUpFrom` multiply the hourly rate by
  days elapsed over warm-up days until the period ends
  ([server/lib/mailers.js#L156-L170](https://github.com/Mailtrain-org/mailtrain/blob/7c6f34e/server/lib/mailers.js#L156-L170),
  [#L271-L332](https://github.com/Mailtrain-org/mailtrain/blob/7c6f34e/server/lib/mailers.js#L271-L332)). This is a
  working version of the warmup term ADR 0023 reserves.

### Mailcoach (docs)

- **Per mailer, self-hosted.** Each mailer in `config/mail.php` takes `timespan_in_seconds` and `mails_per_timespan`;
  "By default, five emails per second will be sent"
  ([advanced usage](https://www.mailcoach.app/self-hosted/documentation/v10/other/advanced-usage-for-php-devs/)). "The
  rate limits themselves are configured per mailer", and the scaling guide warns that the cache-backed limiter "can
  cause race conditions" that end in SES's "Maximum sending rate exceeded", so it recommends a Redis limiter
  ([scaling Mailcoach](https://www.mailcoach.app/self-hosted/documentation/v10/getting-started/scaling-mailcoach/)).
- **The SES setup asks the user for the number**: "The value for the mails per second field depends on your AWS
  account and settings. A good starting point is 5 mails per second"
  ([Amazon SES, v5](https://mailcoach.app/docs/self-hosted/v5/configuring-mail-providers/amazon-ses)). No page says
  Mailcoach reads the quota itself.

### Sendy (docs and forum)

- Sendy sends at the SES "Send rate": "Your Amazon SES quota shows you your 'Send rate'… Sendy will send your email
  newsletters according to this sending rate" (the author, on Sendy's own forum:
  [sending rate](https://sendy.co/forum/discussion/6364/sending-rate-basic-question)). The troubleshooting page adds
  that "Lowering your send rate may yield better sending speed and stability" and that a send that times out resumes
  "within 5 to 10 minutes" ([troubleshooting](https://sendy.co/troubleshooting)). Sendy is self-hosted; per-brand
  "monthly limits" are the operator's lever over each client brand ([get started](https://sendy.co/get-started)).

### EmailOctopus Connect (help center)

- **Speed is the SES account's, minus an optional share.** "We aim to send emails as quickly as your Amazon SES
  account allows" ([how quickly](https://help.emailoctopus.com/article/67-how-quickly-will-emails-be-sent)). "By
  default, we use 100% of your send capacity"; the "send speed limit" setting exists for SES accounts shared with
  other applications
  ([less than 100%](https://help.emailoctopus.com/article/111-can-emailoctopus-use-less-than-100-of-my-send-capacity)).
- **The daily quota pauses and resumes.** The account must "have enough quota available when you begin the send";
  if it runs out mid-send, "we'll pause sending and will automatically resume once you have the capacity available"
  ([SES quotas](https://help.emailoctopus.com/article/98-how-do-amazon-ses-send-quotas-and-max-send-rates-work)).

### Acelle Mail (API reference and knowledge base)

- **Two layers: the sending server and the operator's per-customer package.** The customer package carries
  `sending_quota` ("-1 for unlimited"), `sending_quota_time` and `sending_quota_time_unit` ("day|hour|minute"), shown
  in the example as `"sending_limit":"1000_per_hour"` ([API reference](https://api.acellesend.com/)). A package
  either uses the "System's Sending Servers: One shared pool" or "Own Sending Servers: Each customer has their own
  pool", with a "Maximum Sending Servers" quota
  ([sending server for plan](https://acellesend.com/kb/articles/sending-server-for-plan)). Whether the package quota
  also binds the customer's own servers is not stated (see Unconfirmed).

## 2. Platform-provider ESPs: the caps the platform sets

These vendors own the sending infrastructure, so their caps protect their own IPs and domains. They show which levers
a platform uses on a tenant it does not yet trust. Tier-bound numbers are left out unless they shape the mechanism.

### Customer.io

- **A new-account daily cap that rises with history.** New accounts on one tier "start with a daily email sending
  limit that increases automatically as you build a healthy sending history": 500 emails from signup, 10,000 per day
  after about 2 weeks of "Established, healthy sending", the tier's standard limit after about 6 weeks. "Sending to
  unengaged or invalid addresses, or a sudden spike in volume, can damage your reputation and slow or reverse
  increases". It is measured "over a rolling, trailing 24-hour window", grouped in hourly blocks
  ([plan features](https://docs.customer.io/accounts/billing/plan-features/)).
- **Over the cap, mail is dropped.** "The limit applies to all emails you send, including transactional messages.
  When you reach the limit, Customer.io marks your emails as undeliverable with the reason 'Daily email limit reached
  for compliance tier' and does not retry them later", except one-time sends, which continue on later days. Support
  "can review your account and adjust your limits" (same page).
- **Custom SMTP leaves tracking on Customer.io.** Mail through the customer's own SMTP server is not counted toward the
  account's email allotment, and "We'll still handle your open and click tracking"; Customer.io gets no delivery,
  bounce or complaint feedback from that provider
  ([use your SMTP server](https://docs.customer.io/messaging/channels/email/deliverability/custom-smtp/use-your-smtp-server/)).
  The page names no rate limit of Customer.io's own on custom SMTP.
- **A customer-set rate per automation.** "This is a per-automation setting, not a workspace cap", "a range of 1 to
  60,000 messages per minute for each channel"; unset, an automation "sends messages as fast as Customer.io can
  deliver them" ([automations](https://docs.customer.io/messaging/send/automations/overview/)). One-time sends have a
  daily ramp (start volume, target volume, days, "up to 60 days")
  ([domain warming](https://docs.customer.io/messaging/channels/email/deliverability/domain-warming/)).

### Postmark

- **No daily cap; thresholds and a pause with mail queued.** "We monitor for spam and bounces regularly but not based
  on a daily send limit." The thresholds are a spam complaint rate below "1 in 1,000 emails (0.1%)" and a bounce rate
  below "10% of all emails sent"; an account over them "will be contacted (via email) and the account will be paused,
  with messages queued"
  ([daily send limit](https://postmarkapp.com/support/article/does-postmark-have-a-daily-send-limit)). A server shows
  orange as it "nears the limit" and red when it "exceeds the limit"
  ([servers FAQ](https://postmarkapp.com/support/article/1137-servers-faq)).

### Klaviyo

- **A verification check on the first sends, with a manual review for some.** It "occurs automatically, and in the
  background, as you start sending"; "Some accounts, however, will need to participate in a one-time, manual account
  review", and "During the manual review process, users will not be able to send any emails". Campaigns cancelled
  while the account was disabled "need to be rescheduled"
  ([account verification](https://help.klaviyo.com/hc/en-us/articles/115000628331)).

### Mailchimp

- **Omnivore screens new addresses before a send.** It runs "When you import new contacts or try to send an email",
  looks at addresses "that haven't received emails from your Mailchimp account before", and if it predicts trouble
  "we'll disable sending to that audience until you remove the bad addresses"; other audiences keep working
  ([about Omnivore](https://mailchimp.com/help/about-omnivore/)).
- **Suspension is a partial freeze under review.** It "disables certain features on your account, which can include
  sending and import capabilities"; "Most of the time, we'll issue a warning before we suspend an account"; the
  abuse-complaint threshold is not published ([about suspension](https://mailchimp.com/help/about-suspension/)).

### Twilio SendGrid

- **Vetting before the account is unlocked.** Accounts must confirm email, authenticate a domain, turn on 2FA and add
  payment details: "You can't skip any of the verification steps, or your account won't be unlocked"; "In some
  instances, the Twilio compliance team must verify your account information before you can start sending"
  ([verifying your account](https://www.twilio.com/docs/sendgrid/ui/account-and-settings/verifying-your-account)).
- **A volume restriction on new dedicated-IP accounts.** They "have a volume restriction applied for the first 3 days,
  in order to prevent abuse", and some accounts are "suspended upon creation, which prevents email sending"
  ([emails stuck in processing](https://support.sendgrid.com/hc/en-us/articles/31236228514587-Emails-Stuck-in-Processing-Status)).
- **Graded account states.** "Newly created accounts will be directly placed in a vetting process." While suspended,
  new mail is "queued for up to 72 hours"; a deactivated account's queue is deleted and its tracking and redirects
  stop ([account under review](https://support.sendgrid.com/hc/en-us/articles/360041790293-Account-Under-Review)).

### Loops

- **A per-second ceiling by account tier, queued, not dropped.** The lowest tier "can send up to 10 emails per
  second", higher tiers "up to 1,000 emails per second", and "Emails over the limit stay queued until they can be
  sent" ([account](https://loops.so/docs/account/billing)).

### Brevo

- The help center refused automated reads (HTTP 403), so the account-validation and suspension behaviour is in
  Unconfirmed. The one confirmed line is from the API reference, for SMS campaigns: "accounts under validation are
  limited to 4 total campaigns and campaigns with more than 10 recipients will be saved as draft"
  ([create SMS campaign](https://developers.brevo.com/reference/create-sms-campaign)).

## 3. Provider limits the customer's own account imposes

### AWS SES

- **Two quotas, both rolling, per Region, counted in recipients.** "Sending quota—The maximum number of emails that you
  can send in a 24-hour period. This quota is calculated on a rolling time period"; "Sending rate" per second, which
  "You can exceed … for short bursts, but not for sustained periods of time"; "Sending quotas are based on recipients
  rather than on messages" ([sending quotas](https://docs.aws.amazon.com/ses/latest/dg/manage-sending-quotas.html)).
- **Sandbox: 200 per 24 hours and 1 per second, verified recipients only**; production access is a review, with "an
  initial response to your request within 96 hours" because "we have to consider each request carefully"
  ([request production access](https://docs.aws.amazon.com/ses/latest/dg/request-production-access.html)).
- **Quotas rise automatically or on request**: "we might automatically increase the sending quotas for your account"
  when daily volume regularly approaches the maximum without exceeding it
  ([request an increase](https://docs.aws.amazon.com/ses/latest/dg/manage-sending-quotas-request-increase.html)).
- **`GetAccount` (v2) reads it all.** The response has `SendQuota` (`Max24HourSend`, `MaxSendRate`,
  `SentLast24Hours`), `ProductionAccessEnabled` ("If the value is `false`, then your account is in the _sandbox_"),
  `SendingEnabled` and `EnforcementStatus`: `HEALTHY`, `PROBATION` ("We're placing your account under review") or
  `SHUTDOWN` ("Your account's ability to send email is currently paused")
  ([GetAccount](https://docs.aws.amazon.com/ses/latest/APIReference-V2/API_GetAccount.html)). `Max24HourSend` of -1
  means unlimited ([SendQuota](https://docs.aws.amazon.com/ses/latest/APIReference-V2/API_SendQuota.html)).
- **An enforcement pause is visible only in `EnforcementStatus`.** "Don't use the SendingEnabled field that GetAccount
  returns for this purpose … It doesn't change when SES pauses or reinstates your account for enforcement", and "SES
  does not emit an Amazon EventBridge event when it pauses or reinstates sending for your entire account"; automation
  "must read the account status" ([enforcement FAQ](https://docs.aws.amazon.com/ses/latest/dg/faqs-enforcement.html)).
- **Enforcement thresholds.** Bounce rate "5% or greater, we'll place your account under review … 10% or greater, we
  might pause"; complaint rate "0.1% or greater … under review … 0.5% or greater, we might pause" (same FAQ).
- **Throttling is a drop with a retry hint.** After the quota or rate is reached "Amazon SES drops the message and
  doesn't attempt to redeliver it". The API returns `ThrottlingException` with "Daily message quota exceeded" or
  "Maximum sending rate exceeded"; SMTP returns "454 Throttling failure: …". "If you encounter a throttling error,
  you should program your application to wait for an interval of up to 10 minutes, and then retry"
  ([sending-limit errors](https://docs.aws.amazon.com/ses/latest/dg/manage-sending-quotas-errors.html)).
- **SES tenants, for ISVs on their own SES account**: "SES can automatically pause problematic tenants through
  reputation policies", but "their combined sending activity still affects your overall account reputation—tenants
  that develop poor sending practices could put your entire account at risk"
  ([tenants](https://docs.aws.amazon.com/ses/latest/dg/tenants.html)). No per-tenant send quota is documented.

### Yandex Cloud Postbox

- **New-account quotas match the SES sandbox**: `postbox.send.rate` "1 email per second" and `postbox.emails_sent.count`
  200 "within a 24-hour time interval", 200 recipients per email, and `SendBulkEmail` at 1 request per second per cloud
  ([quotas and limits](https://yandex.cloud/en/docs/postbox/concepts/limits); source text read from the docs repo at
  `yandex-cloud/docs@fdf8f73`,
  [`en/_includes/postbox/postbox-limits.md`](https://github.com/yandex-cloud/docs/blob/fdf8f7398604321e8981bafde811f36c334c71f1/en/_includes/postbox/postbox-limits.md)).
  They are quotas, raised on request through Quota Manager.
- **No quota read in the AWS-compatible API.** Its reference lists 29 methods (send, identities, configuration sets,
  event destinations, suppression, dedicated IPs); neither `GetAccount` nor `GetSendQuota` is among them
  ([API reference](https://yandex.cloud/en/docs/postbox/aws-compatible-api/api-ref/), directory
  [`en/postbox/aws-compatible-api/api-ref`](https://github.com/yandex-cloud/docs/tree/fdf8f7398604321e8981bafde811f36c334c71f1/en/postbox/aws-compatible-api/api-ref)).
  So for a Postbox Integration ADR 0023's provider term is always empty, and the Integration is unlimited unless the
  user sets a value, against a provider that starts at 1 per second.

### Google Workspace and Gmail

- **Gmail, per Workspace user**: "Daily sending limit per user account" 2,000 (1,500 for mail merge, 500 for trial
  accounts), total recipients per day 10,000, external recipients per day 3,000. "Sending limits can change without
  notice. Limits per day are applied over a rolling 24-hour period"; after a limit, "users can't send new messages for
  up to 24 hours"
  ([Gmail sending limits](https://knowledge.workspace.google.com/admin/gmail/gmail-sending-limits-in-google-workspace)).
- **SMTP relay**: "Each user can send up to 10,000 messages in a 24-hour period"; "a 100-recipient limit per SMTP
  transaction for smtp-relay.gmail.com"; per organization "4.6 million in a 24-hour period" and "319,444 per 10-minute
  window"; "If a user exceeds a limit, they get an SMTP error"
  ([SMTP relay](https://knowledge.workspace.google.com/admin/gmail/advanced/route-outgoing-smtp-relay-messages-through-google)).
- **Consumer Gmail**: more than 500 recipients in one message, or more than 500 messages a day, gets "You have reached a
  limit for sending mail" ([Gmail help](https://support.google.com/mail/answer/22839)).
- Neither page names an API that reads the limits, and none was found.

### Other SMTP relays

None of the sources read here documents a way for an SMTP server to announce its sending limits; SES and Google
report them only as a refusal (SES's `454`, Google's "SMTP error" above). ADR 0023 gives SMTP no default for that
reason.

## 4. Platform risk when customers bring the provider

The provider's reputation is the customer's, but three things stay shared with every other tenant of the Instance.

- **Domains in the message body.** Spamhaus's DBL "ONLY lists domains", and receivers query it "during content
  inspection - by looking up domains appearing in the mail headers and body e.g., URLs"
  ([DBL](https://www.spamhaus.org/blocklists/domain-blocklist/)); "DBL lists at the main domain level, and all
  hostnames and subdomains of that domain also return a 'listed' result"
  ([DBL FAQ](https://www.spamhaus.org/faqs/domain-blocklist/)). A redirector has its own return code (127.0.1.103) and
  is told to check every target, recheck later, refuse changing the landing URL after creation and suspend abused
  URLs with 404 or 410 (same FAQ). Spamhaus says nothing specific about an ESP's shared click-tracking domain; the
  listing rule above is what applies to it.
  - In sphericon every link of a message and its `List-Unsubscribe` URL share one host, the serving Tracking domain
    of the Sending domain if there is one, otherwise the Instance's own host
    ([`internal/outbound/outbound.go#L652-L661`](../../internal/outbound/outbound.go) at `d0dcd21`;
    [ADR 0038](../adr/0038-tracking-domains-are-core-and-the-edge-is-not.md)). A Tracking domain is
    `track.<sending domain>`, so a listing caused by one tenant lands on that tenant's own domain. Every tenant
    without a serving Tracking domain puts the Instance's host in its bodies, and one listing of it affects all of
    them.
  - Customer.io shows the same exposure: with the customer's own SMTP, "We'll still handle your open and click
    tracking" (section 2).
- **Worker capacity.** Broadcast recipient jobs run on one River queue with `MaxWorkers: 10` for the Instance
  ([`internal/jobs/worker.go#L164`](../../internal/jobs/worker.go)). ADR 0023's per-Integration buckets and pacing at
  enqueue time stop one Integration from outrunning its provider; they do not decide whose jobs a free worker takes
  next. Keila's scheduler is the one design read here that does: round-robin over Senders of an adapter, one message
  per turn (section 1).
- **The Instance's own standing with providers.** SES's tenant guide says the combined sending of tenants "still
  affects your overall account reputation" on an ISV's own SES account (section 3). With bring-your-own SES this is
  the customer's account, not the platform's; it becomes the platform's risk only for mail the platform sends from
  its own account (sign-up, notices), which ADR 0023 leaves out of scope.

## 5. What it means for sphericon

This section lists options and trade-offs; it decides nothing. The current code matches ADR 0023: `EffectiveOf` is
"the one place manual, provider (and later warmup) values are folded into a ceiling", each value carrying its source
([`internal/providerlimit/effective.go#L41-L61`](../../internal/providerlimit/effective.go)), and the SES driver reads only
`SendQuota` from `GetAccount` ([`internal/messaging/ses/ses.go#L159-L180`](../../internal/messaging/ses/ses.go)).

### Question 1: a ceiling set by Staff

What the prior art shows: platforms that own the infrastructure gate new accounts (SES production access, SendGrid
vetting, Klaviyo review, Keila Cloud's `under_review`), cap them while trust builds (Customer.io 500 a day rising
over about six weeks, SendGrid's first three days, Keila Cloud's shared sender coded at 5 per second until the domain is
verified), and pause on reputation thresholds with mail queued (Postmark, SendGrid, SES). Among bring-your-own tools,
the hosted operator keeps a lever above the customer's own setting: Keila's `adapter_rate_limit` from the operator's
environment and its per-account quota that counts sends through any Sender, Acelle's per-customer `sending_quota`,
Sendy's per-brand limits. No source read here caps bring-your-own sending for the provider's sake; where an operator
caps it, the reason is the operator's own (capacity, quota, review).

sphericon already has the gate and the pause: a Staff hold set by the external service (ADR 0009, which already lets
that service read new Workspaces since a cursor) covers "review before the first send", and Workspace suspension
covers the reputation pause. What it lacks is the middle lever, a ceiling short of a stop.

- **Option A: no Staff ceiling.** Rely on Staff hold and suspension. Simplest and already built; a stop is all or
  nothing, so a new tenant either sends at its provider's full rate or not at all.
- **Option B: a Staff term in the minimum.** The effective ceiling becomes the minimum of manual, provider, warmup and
  Staff, added in `EffectiveOf` as one more `Source`. Following ADR 0009's shape it is set through the Staff surface,
  is opaque to the core (a number with the setter recorded, no meaning attached), is absent on a self-hosted Instance
  with no external service, and its name and UI copy carry no commercial words. Open trade-offs:
  - **Deferral or Hold when it binds.** ADR 0023 makes a spent limit a Deferral (busy, with an ETA), which fits a
    ceiling. But a Deferral carries no reason, and a customer who set 100 per second while Staff set 5 sees a slow
    ETA with no cause; the `Source` on the effective value is where the UI would say "set by the platform".
  - **Per Workspace or per Integration.** A per-Integration term is what `EffectiveOf` reads today, but a Workspace can
    add Integrations, so a cap meant for the tenant is spread over them. A per-Workspace cap needs a third bucket
    shared by the Workspace's Integrations, which ADR 0023's one-row-per-Integration design does not have.
  - **Expiry.** ADR 0009 makes a Staff hold expire so an unavailable service cannot freeze a Workspace for good. For a
    ceiling the failure runs the other way: an expired ceiling lifts. Whether it expires, and to what, is a choice.
  - **Transactional.** ADR 0023 counts Transactional against the buckets but never delays it, so a Staff ceiling
    would not hold back Transactional volume unless that rule changes for this term.
- **Option C: the hosted service sets the ceiling from domain state.** Keila Cloud ties its shared sender's rate to
  whether the sender's domain is verified. The same rule could live entirely in the external service (for example, a
  lower ceiling while a Workspace has no serving Tracking domain, which is when it puts the Instance's host in its
  mail), with Option B as the core mechanism and no meaning in the core.
- **Fairness is a separate lever.** None of A to C makes the shared broadcast queue fair between tenants; Keila's
  round-robin is the prior art if that becomes a problem.

### Question 2: the customer's own rate limit

- **Common among bring-your-own tools, rare among platform ESPs.** listmonk (instance-wide "Message rate" and "Sliding
  window"), Keila ("Rate limit per second/minute/hour" per Sender), Mailcoach ("mails per timespan" per mailer),
  Mailtrain ("Throttling" per send configuration), Acelle (per sending server), EmailOctopus ("send speed limit", a
  share of SES capacity) and Sendy ("send rate") all expose one. Of the platform ESPs only Customer.io does, and per
  automation. The scope that matches ADR 0023 (per provider account) is Keila's, Mailcoach's and Mailtrain's.
- **The one cited reason to set it below the provider's quota** is a provider account shared with other software
  (EmailOctopus). SES's `SentLast24Hours`, which sphericon does not read, reports that other traffic; ADR 0023 already
  names two Workspaces on one SES account as an accepted overshoot.
- **Reading the provider is possible for SES only.** SES `GetAccount` returns the quota, sandbox state and
  `EnforcementStatus`; Postbox's API has no such call and starts new accounts at 1 per second and 200 a day; Google
  and SMTP relays report limits only as refusals. Two gaps follow from the code: a Postbox Integration with no manual
  value is unlimited against a 1-per-second provider, and an SES `PROBATION` or `SHUTDOWN` is not surfaced, although
  SES says `EnforcementStatus` is the only way to see it.
- **Throttling replies.** SES drops a throttled message and advises a retry within 10 minutes. ADR 0023 already treats
  it as a Deferral and does not auto-tune. Of the tools read, none adapts the rate to throttling replies: listmonk
  pauses a campaign after `max_send_errors`, Keila fails the message, Mautic retries with exponential back-off,
  EmailOctopus pauses until the daily quota returns.

## Unconfirmed

- **Customer.io acceptable use policy**: suspension or "the requirement that you utilize your own SMTP" at a spam
  complaint rate of 0.1% over 30 days. Seen only in search snippets; the policy URL returned 404.
- **Postmark manual approval of every new account**: in a search summary only; the FAQ page read does not mention it.
- **Klaviyo review triggers** (first send to an imported list) and deliverability benchmarks: search snippets only.
- **Mailchimp's daily send cap on its lowest tier**: search snippet only.
- **Brevo**: suspension right after sign-up for "an anomaly during the registration process", no sending until
  reactivated through support, and a daily cap on its lowest tier. The help center returned HTTP 403.
- **Loops "Manual review" and "Send pacing"**: marketing copy in a search snippet only.
- **SendGrid's 2026 cap at a multiple of the account allotment**: conflicting snippets, not read on a primary page.
- **Mailcoach**: what happens when the limit is reached, and whether it ever reads the SES quota.
- **Sendy and EmailOctopus**: whether they call SES's quota API themselves; their pages imply it but no code was read.
- **Acelle**: whether a customer package's `sending_quota` also binds the customer's own sending servers.
- **Keila Cloud**: whether `Mx2.rate_limit/1` (5 or 15 per second) is enforced; at `508eea5` nothing in the
  scheduler calls it.
- **Yandex Cloud Quota Manager**: whether `yc quota-manager quota-limit list` returns a cloud's live Postbox values
  and which credentials it needs.
- **Google**: that no API reads Gmail or SMTP relay limits is an absence, not a statement on Google's pages.
