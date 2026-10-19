# Mindbox Product Feature Breakdown (for competitive gap analysis)

Researched October 2026 from Mindbox's own pages only: mindbox.ru product pages, tariffs and security pages,
help.mindbox.ru (the help center and its "what's new" log) and developers.mindbox.ru. Every claim carries a
source tag `[Sn]`, resolved in the Sources list at the end. No third-party review was used. Pain points are not
covered, because no secondary sources were collected. Anything not confirmed on an official page is marked
**unverified**. sphericon's side is read from `GLOSSARY.md`, `docs/adr/*`, `docs/ROADMAP.md` (it lags), and
`typespec/{site,external}` and `ent/schema`.

Fetch caveats: `developers.mindbox.ru` returned a TLS certificate error to the fetcher, so API facts come from
help.mindbox.ru and from search-result excerpts of developers.mindbox.ru pages, not from reading the pages. Several
help articles are flagged by the search tool as years old, so the UI and settings names may have changed.

Mindbox is a **Russian CDP plus omnichannel CRM-marketing platform for retail and e-commerce** (tagline: customer
retention). It is a sales-led, enterprise-leaning product: loyalty, POS and promotion calculation are central.
sphericon is an email-first, self-hostable, open-core product. Most of the gap is therefore _retail CDP breadth_,
not email depth, and much of it is deliberately out of scope.

---

## 1. Product map

Official module list [S1]:

- **Mindbox CDP** (retention): CDP, Recommendations and ML, Mailings, Surveys and quizzes, Email builder,
  Chatbots, Website personalization, Mobile push and In-App, Loyalty.
- **Mindbox Ads**: ML retargeting, Yandex Direct optimization, Media.
- **Mindbox PDP**: a price-personalization platform.
- **Mindbox PRO** (CRM-marketing services), **Mindbox AI** (campaign analysis and hypothesis generation).
- Claimed scale and reliability: up to 2M RPM, processing up to 300 ms, ISO 27001 certified, SLA, public status
  page [S1]. These are marketing figures; the Mailings page cites "2 million emails per hour" and the home page
  "2M RPM", which the source itself does not reconcile [S3].

## 2. Customer data model and identity

- **Entities the help center documents:** Customers, Products, Orders, Custom fields, Exports [S2]. Custom fields
  attach to client, action, touchpoint, product, product-list line, order, promo action, order item, zone and
  discount card. Nine types (string up to 1,000 chars, boolean, integer, decimal, date, datetime, enumeration, unique
  identifier, identifier); a field can be marked sensitive and masked; integer/decimal fields have merge rules [S12].
- **One profile across channels.** The CDP page claims one profile from site, app, offline till, CRM, ad accounts
  and BI [S4].
- **Identity merging is automatic and cannot be turned off.** Cards merge when at least one identifier (phone,
  email, device, ...) matches and nothing contradicts; the priority client's ID becomes primary and the others stay
  in history. A unique-identifier setting "do not merge clients with different IDs" exists; merge events appear in
  the history and can start a scenario ("customer cards merged") [S13][S9]. _(The article is about six years old per
  the search tool; unverified that behavior is unchanged.)_
- **Mindbox ID is the profile key; client IDs from other systems are alias identifiers.** Repeat registration with
  a unique ID is rejected [S13].
- **Multibrand:** one project holds several brands. Per brand: subscriptions, mailings, scenarios, senders and DKIM,
  integration points, loyalty participants. Shared: billing, control groups, pools, pop-up frequency [S14].
- **Anonymizer and on-premise** deployment are offered to corporates [S4][S7].

## 3. Data ingestion

- **JS tracker + API.** Browser-side operations (product/category views, add to cart or favourites, subscribe) run
  through the JS tracker without a secret key; server operations use the V3 API `POST https://api.mindbox.ru/v3/operations/{sync|async}`
  with an optional `Authorization: SecretKey` header configured per operation. Operations are defined in the
  project and can start a scenario [S15][S10]. Web push uses `mindbox("webpush.subscribe")` [S16].
- **Mobile SDK:** free, iOS / Android / Flutter / React Native; integration "up to 14 days" [S17]. The CDP page
  adds HarmonyOS [S4].
- **Imports:** documented sections for new customers, orders, actions, products and discount cards, plus FTP/SFTP
  imports (static IPs for SFTP added Sept 2026) [S2][S18].
- **Ready modules (from the CDP page):** 1C, SET Retail, Frontol 6, 1C-Bitrix, Passteam, OSMI Cards, "Koshelek";
  through API/modules: amoCRM, Bitrix24, RetailCRM, SAP, Dynamics 365, Power BI, Qlik, Tableau, InSales, Magento [S4].
  Loyalty page adds iiko, CSI, ATOL Frontol [S6]. Albato is documented as the no-code bridge [S19].
- **Orders and products as first-class data** (YML product feeds, multi-region price and availability) [S2][S14].

## 4. Segmentation

- **Four segmentation types are documented; three are confirmed:** realtime (evaluated at query time, nothing
  stored), recalculated (schedule, event or manual; result stored) and static (the fourth was not identified);
  segmentations on the _action_ entity are realtime only [S20].
- **Filters:** by actions, orders, mailings, customer data [S2]. Segment entry and exit history is filterable, and a
  scenario can start when a customer enters or leaves a segment [S9][S20].
- **RFM:** automatic, 13 lifecycle segments, based on 365 days of orders, cancelled and returned orders excluded;
  recalculated daily [S21][S5]. Lifecycle segments by days since registration, days since last order, order count
  [S20].
- **Predictive:** churn prediction, next-best product and message [S5].

## 5. Channels

- **Mailings:** email, SMS, push, Viber, Telegram, WhatsApp, wallet passes, OSMI Cards [S3]; doc sections list
  Email, MobilePush, WebPush, SMS, Messengers, Viber [S2]. Messengers: Telegram, VK, Viber, OK, MAX (WhatsApp is
  mentioned with a caveat about Meta's status in Russia) [S8]. WhatsApp via Edna Pulse (Nov 2025) [S18].
- **Cascade sending:** free channel first; a paid SMS goes only if the first message is not delivered; also
  simultaneous multi-channel for critical messages [S3].
- **Mobile push / In-App:** APNs and FCM; up to 2M personalized pushes per hour (claim) [S17].
- **Web push:** supported; subscription is per browser and device, must be triggered by a user click, **not
  supported on iOS** [S16].
- **Website personalization:** pop-ups, banners, widgets, stories, embedded blocks; 100 ready templates; frequency
  control; segment-targeted; recommendation widgets; social-proof widgets (order and view counts, free-delivery
  progress) [S22]. Exit-intent and scroll triggers are **not described** on that page. In-app game mechanics
  (prize wheel, luck ticket) were added July 2026 [S18].
- **Chatbots:** sales, loyalty (registration without SMS, balance), support with routing to operators, order and
  bonus notifications; Mindbox builds the bot, about one month to launch [S8].
- **Ads:** push CDP segments to ad accounts, Yandex Direct optimization [S1][S4].
- **Surveys and quizzes:** "interactive AI quizzes" [S1]; details **unverified**.

## 6. Automation (Scenarios)

- **Start events**, a large catalogue [S9]: customer actions, product viewed, session ended, product list
  changed, product price drop / back in stock / out of stock, new order, order status changed, per-item order
  events, segment entered / left, new / registered customer, customer cards merged, email or phone changed or
  confirmed, custom field changed, subscription status changed, loyalty events (promo issued or applied, balance
  negative, bonus points available, discount card changed), mailing sent, and an API **operation call**.
  Historical events older than 7 days are not processed. Repeat limits per customer or per order exist.
- **Blocks:** start, condition (multi-branch), step group, splitter, delay, A/B test, limit, frequency per client
  and order [S23].
- **Step group, 12 step types** [S23]: send campaign (email / MobilePush / SMS / Viber / WebPush / messengers),
  issue action, issue promo code, change points balance, add to or remove from a static segment, edit client data,
  exclude or clear a product list, **call a webhook** (response drives branching), generate an auth code, link a
  discount card, issue action to an invited client (referral).
- **Webhook step semantics:** 2xx goes to Yes, 4xx except 429 to No, 5xx and 429 retry (3 retries, 5 or 6 minute
  gap, the help articles disagree); the response JSON can be saved into variables usable in scenarios and mails [S24].
- **Test mode** per scenario; canvas comments for the team (Oct 2025) [S25][S18].

## 7. Loyalty, promotions, coupons, bonuses

- **Bonus points** with accrual rules by product / price / segment, expiry, real-time balances across channels [S6].
- **Discounts and promotions**, mass or personal, by segment, channel, region; cascade discounts; happy-hour
  promotions on store local time (May 2026); bulk creation of up to 150 promotions from CSV (Sept 2026) [S6][S26][S18].
- **Promo codes:** one-time, multi-use and referral pools; bulk generation from a format (alphabet, max 20 chars)
  or import; per-customer usage limits; codes can come from a webhook response [S26][S23].
- **Gift cards** (virtual and physical, generated numbers, validity) and **discount cards** [S6][S18].
- **Order calculation at POS:** up to 500 ms calculation, 1 s order creation (claim); receipt printing of bonuses;
  tiers ("levels") mentioned without detail [S6].
- **Promotion diagnostics** ("why did my promo not apply") added Oct 2026 [S18].

## 8. Recommendations and ML

- **14 recommendation algorithm variants** in four groups (popular, similar, related, personal), applied in site,
  email, push, in-app, chatbots and at the cashier. Data thresholds: roughly 100 actions per catalog item for
  personal, 10 for related and popular [S5].
- **Best send time** prediction from open patterns by weekday [S5], **churn prediction**, next-best product and
  message [S5].
- **Mindbox AI:** analyses launched campaigns and generates A/B hypotheses [S1][S3].

## 9. A/B tests, control groups, analytics

- **A/B in scenarios:** up to 5 variants per test including a control, up to 10 tests per scenario; control branch
  receives no interaction; metrics: order conversion, average check, ARPU, action conversion; report within 24 h
  [S27]. A/B also exists for personalization and mobile apps [S27].
- **Global control group:** about 5% of the base is moved monthly into a group that receives no promotional mail
  (transactional unaffected); restrictable by a realtime segment [S28].
- **Frequency control:** daily cap per client per brand and channel (default 30 messages a day, transactional and
  test excluded, bypass with "high priority") [S29].
- **Attribution:** last-non-direct-touch within each channel group (mailings, site personalization, In-App), window
  1 to 30 days, excludable mailings; recalculated daily [S30].
- **Reports:** mailings, scenarios, business metrics (saved metric sets, goals), segments report (Feb 2026),
  personalization report, API export of reports [S30][S18][S15].

## 10. Deliverability and email tooling

- DKIM keys generated per sender in the UI; SPF set on Mindbox's side for the standard scheme; DMARC guidance [S31].
- **Dedicated Return-Path subdomain** (MX + SPF) for reputation isolation; by default all clients share one
  technical domain, so other clients' sending can affect reputation [S31][S32]. A dedicated sending subdomain was
  added Feb 2026 [S18].
- Email builder: drag-and-drop, 100+ template blocks, variables, recommendation blocks, per-segment block
  visibility; **no full-HTML upload** (markup must be split into blocks); raw HTML blocks added June 2026; shared
  (global) blocks Aug 2026; custom fonts, dark-mode display [S33][S18].
- Branded unsubscribe page, topic blocks on it so a subscriber opts out of a topic, not the channel [S18].
- Branded short-link domain for SMS [S18]. Dedicated IP pools: **unverified** (not found on official pages).

## 11. Consent and compliance

- **Subscription model:** states subscribed / unsubscribed / requires confirmation / no data; levels brand, channel
  (email, SMS, Viber, push, webpush) and topic; explicit vs implicit; transactional mail ignores subscription [S34].
- **Double opt-in** is a project setting, with a confirmation link that can confirm a channel or one topic [S34].
- **152-FZ:** Mindbox states compliance and listing in the Roskomnadzor operator register; ISO/IEC 27001:2022;
  data location is not stated on the page; infrastructure partners named (Yandex.Cloud, Oxygen, a Tier 3 data
  centre) [S7].
- Chatbots collect consents and manage frequency automatically [S8].

## 12. Roles, security, administration

- **Security groups** (permission bundles): eight standard (Owners, Marketers, Analysts, Client developers, Accounts,
  email template layout, Api, Security engineer) plus custom groups; per-brand and per-folder restriction; bulk
  staff import [S35].
- **Enhanced Security module** (enabled on all projects, not billed separately): personal-data masking with an
  unmask permission logged, IP allow-list for staff (and API access allow-list, June 2026), SIEM export of event
  logs, SSO [S36][S18]. 2FA is mandatory, passkeys supported [S7][S35].
- Event log of staff actions and security events [S7][S36].

## 13. API, webhooks, integrations

- V3 operations API (sync / async), scenario start by operation, customer data retrieval, discount-card issue,
  operation-log export, report export, ticket (link-in-mail) authentication [S15].
- Outbound webhooks exist **as a scenario step and as integration "Webhooks" doc section** [S2][S24]; no
  subscribe-to-event-type webhook catalogue was found (unverified).
- Integration catalogue is retail and CIS oriented (1C, Bitrix, POS, Russian messengers and SMS) [S4][S6].

## 14. Pricing and packaging

- **Subscription, billed monthly, quote by manager, excl. VAT** [S37].
  - **Light:** from 15,000 RUB/month, "up to 100,000 contacts", CDP, 7 mailing channels, on-site pop-ups, AI
    assistant, chat support.
  - **Standard:** from 150,000 RUB/month, 12 tools (ads, CRM, sales), consultant, tech specialist and success
    manager, anonymization, on-premise option; modules added individually.
  - Only volume signal is the Light contact guideline; no per-message prices are published. Free trial on request.
- Sales-led, implementation-heavy: Bitrix integration up to a month, loyalty migration one to three months, a
  consultant calculates payback [S37][S1].

---

## 15. Gap table

Status key: **has** = shipped, **partial** = some of it, **missing** = not built. "Deferred" means named in
`GLOSSARY.md` or an ADR as a future surface.

| Capability                                            | Mindbox (source)                                                                       | sphericon status                                                                                                                                            | Relevance for an open-core, email-first product                                                                                                     |
| ----------------------------------------------------- | -------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| Single contact profile across anon + known            | Unified profile, auto-merge on matching identifiers [S4][S13]                          | **has**: Contact + Visitor + Identify, alias keys email/phone/subject_id (GLOSSARY, ADR 0002); merge is deliberately an anti-term ("Identify: avoid merge") | **low** to copy: sphericon's model is intentionally stricter; an explicit manual merge of two Contacts is a smaller, separate gap (see below)       |
| Contact merge / dedup of two known Contacts           | Automatic, cannot be disabled, history of merged IDs, "cards merged" trigger [S13][S9] | **partial**: Identify stitches Visitors to a Contact; no two-Contact merge found in `typespec/site/resources/contacts.tsp`                                  | **med**: self-hosters importing from other tools create duplicates                                                                                  |
| Typed custom fields on more entities (order, product) | Nine types, ten entities, sensitive flag [S12]                                         | **partial**: typed Custom fields on Contact only (ADR 0006); no sensitive-field masking                                                                     | **med**: Contact-level is enough for email; sensitive masking is cheap and pairs with GDPR work (ADR 0021)                                          |
| Orders and products as data                           | First-class Orders, Products, YML feeds [S2][S14]                                      | **missing** (ROADMAP phase 7 e-commerce not started; only Events)                                                                                           | **med**: valuable only if e-commerce becomes a target; email-first can model orders as Events first                                                 |
| JS tracker + API ingestion                            | JS tracker, V3 sync/async API, SDKs [S15][S17]                                         | **has** web tracker, `/collect`, `/api`, ingest hooks (ADR 0037) ; **missing** mobile SDKs                                                                  | **low** for mobile SDK in the near term; **high** that the web path is already covered                                                              |
| Imports from files and other systems                  | Customers, orders, actions, products, cards, FTP/SFTP [S2][S18]                        | **partial**: ADR 0014 promises importers (Mautic, Mailchimp, Brevo, Keila, CSV) as free core; no contact import operation found in `typespec/site           | external`                                                                                                                                           | **high**: ADR 0014 names it the adoption lever; needs to exist |
| Dynamic rule segments                                 | Realtime, recalculated, static segmentations [S20]                                     | **partial**: one live rule segment kind, never materialized (ADR 0029); no static segment, no snapshot                                                      | **med**: static and "frozen at send time" segments help audit and A/B; the GLOSSARY rejects snapshots, so this is a design decision, not just a gap |
| Segment entered / left as automation trigger          | Yes [S9]                                                                               | **missing**: single event-action Trigger per Automation (GLOSSARY: Trigger)                                                                                 | **high**: top-requested automation entry (Drip has it too, see `drip-com-feature-analysis.md`)                                                      |
| RFM / lifecycle segments                              | Auto 13 RFM segments [S21]                                                             | **missing** (no order data; engagement rates exist)                                                                                                         | **low** until orders exist; an engagement-based lifecycle segment (active, lapsing, dormant) is **med**                                             |
| Predictive (churn, next-best, send time)              | ML features [S5]                                                                       | **missing**                                                                                                                                                 | **low**: needs data volume and ML ops; send-time optimization alone is **med** and tractable                                                        |
| Email channel                                         | Email with builder, dynamic blocks [S3][S33]                                           | **has** (Broadcast, Automation, Transactional, MJML-to-HTML embedded editor, ADR 0040, Liquid variables)                                                    | n/a                                                                                                                                                 |
| Block-level segment visibility in email               | Per-segment block visibility [S33]                                                     | **partial**: Liquid `{% if %}` works but the editor offers no block (ADR 0040 item 9)                                                                       | **med**: strong personalization win for email-first                                                                                                 |
| Shared / global email blocks                          | Shared blocks, Aug 2026 [S18]                                                          | **missing**: Templates are copied at author time, no reference (ADR 0003)                                                                                   | **low**: ADR 0003 chose copy deliberately                                                                                                           |
| SMS                                                   | Yes [S3]                                                                               | **missing** (reserved on Integration, Suppression, Unsubscribe; GLOSSARY "Channels")                                                                        | **med**: model is channel-aware already; ESP choice is regional, so lower priority for an AGPL core                                                 |
| Web push                                              | Yes, not iOS [S16]                                                                     | **missing**                                                                                                                                                 | **low**: browser-only, weak reach, maintenance cost                                                                                                 |
| Mobile push / In-App                                  | Yes, SDKs [S17]                                                                        | **missing**                                                                                                                                                 | **low**: needs SDKs, out of email-first scope                                                                                                       |
| Messengers / chatbots                                 | Telegram, VK, Viber, MAX; bots [S8]                                                    | **missing**; GLOSSARY says a bot is "just another channel"; on-site chatbot explicitly out of model                                                         | **low** to **med**: Telegram as a channel fits the model; two-way chatbot is out                                                                    |
| Cascade / fallback across channels                    | Free first, paid SMS if undelivered [S3]                                               | **missing** (one channel)                                                                                                                                   | **low** until a second channel exists                                                                                                               |
| On-site popups, banners, widgets                      | Pop-ups, banners, stories, embedded, 100 templates [S22]                               | **missing** (ROADMAP phase 5 not started; GLOSSARY deferred)                                                                                                | **high**: lead capture feeds the whole email funnel; ADR 0014 lists forms as core                                                                   |
| Surveys / quizzes                                     | AI quizzes [S1]                                                                        | **missing**                                                                                                                                                 | **low**: later, on top of forms                                                                                                                     |
| Automation: breadth of start events                   | Roughly 40 start events [S9]                                                           | **partial**: one event-action Trigger                                                                                                                       | **med**: the event model supports it; the number matters less than segment trigger and date trigger                                                 |
| Automation: branching, split, A/B, limit              | Condition (multi-branch), splitter, A/B, limit, delay [S23]                            | **missing**: strictly linear steps, branching deferred (GLOSSARY: Step)                                                                                     | **high**: branching is the biggest builder gap in both Drip and Mindbox                                                                             |
| Automation: step catalogue                            | 12 step kinds [S23]                                                                    | **partial**: send, wait, apply/remove tag, HTTP step (5 kinds)                                                                                              | **med**: add "set custom field", "add to segment" (not possible: segments are rules), "exit"                                                        |
| Webhook step with response branching                  | Webhook, branch on 2xx/4xx, variables from response [S24]                              | **partial**: HTTP step with signed request and retries, no response variables or branching (GLOSSARY: HTTP step)                                            | **med**: response variables make the HTTP step a real integration primitive                                                                         |
| Scenario test mode                                    | Test mode [S25]                                                                        | **partial**: test send (ADR 0041); no dry-run of an Automation found                                                                                        | **med**                                                                                                                                             |
| Re-entry                                              | Repeat limits per customer / order [S9]                                                | **missing**: enrolled at most once ever (GLOSSARY: Enrollment)                                                                                              | **high**: win-back and recurring flows need it                                                                                                      |
| A/B tests (email)                                     | Up to 5 variants, control, ARPU / conversion metrics [S27]                             | **missing** (ROADMAP "A/B" still open)                                                                                                                      | **high**: table stakes for email; start with subject / content split on Broadcast                                                                   |
| Global control group                                  | 5% monthly holdout, segment-restrictable [S28]                                         | **missing**                                                                                                                                                 | **med**: cheap, honest incrementality measurement; fits "workspace setting"                                                                         |
| Frequency cap                                         | Daily per client / brand / channel, default 30, priority bypass [S29]                  | **missing**: only Send rate limit per Integration (ADR 0023), which is a provider ceiling, not a per-recipient cap                                          | **high**: protects deliverability and reputation; small to build on the send chokepoint (ADR 0015)                                                  |
| Attribution                                           | Last-non-direct-touch, 1 to 30 d, per channel group [S30]                              | **missing** (no revenue / goals); engagement rollups exist                                                                                                  | **med**: needs orders or conversion events; a "goal event" attribution is smaller than full revenue                                                 |
| Reports                                               | Mailings, scenarios, business metrics, segments, API export [S30]                      | **partial**: workspace analytics (`analytics.tsp`), deliverability rates (ADR 0011)                                                                         | **med**: per-automation report and a segment report                                                                                                 |
| Deliverability: DKIM, DMARC, per-domain return path   | DKIM in UI, dedicated Return-Path [S31][S32]                                           | **has**: native DKIM, sending domains, tracking domains (ADR 0010, 0038, 0011, 0012)                                                                        | n/a: sphericon is ahead by self-serve DKIM plus complaint / bounce rates                                                                            |
| Dedicated IP / warmup                                 | **unverified**                                                                         | **missing** (EE scale-infra per ADR 0014)                                                                                                                   | **low** for core                                                                                                                                    |
| Unsubscribe page, topics                              | Branded page with topic blocks [S18]                                                   | **partial**: per-source unsubscribe scope (GLOSSARY: Sending source), one-click header (ADR 0012); topic-level preference page not confirmed                | **med**: a preference center is a compliance and retention feature                                                                                  |
| Consent model                                         | Brand / channel / topic, DOI [S34]                                                     | **has**: per-channel, per-source, confirmed opt-in (ADR 0001, 0013), Suppression                                                                            | n/a: the layered model is arguably stricter                                                                                                         |
| Privacy law                                           | 152-FZ, operator register [S7]                                                         | **partial**: GDPR erasure and export (ADR 0021), retention (ADR 0019); no 152-FZ specifics                                                                  | **low** to **med**: matters only for the Russian market; data-residency is EE per ADR 0014                                                          |
| Loyalty: bonus points, tiers, cards                   | Full engine, POS integrations [S6]                                                     | **missing**                                                                                                                                                 | **low**: retail loyalty is out of scope for an email-first core                                                                                     |
| Promotions and discount calculation                   | Promotions, cascade discounts, 500 ms calculation [S6][S26]                            | **missing**                                                                                                                                                 | **low**: a commerce engine, not marketing automation                                                                                                |
| Promo codes / coupons                                 | Pools, bulk generation, per-customer limits [S26]                                      | **missing**                                                                                                                                                 | **med**: unique codes in email are a common e-commerce request; a small "code pool" entity is much less than a promotion engine                     |
| Product recommendations                               | 14 algorithms, all channels [S5]                                                       | **missing**                                                                                                                                                 | **low** now; **med** if orders and products land                                                                                                    |
| Roles                                                 | Eight standard groups plus custom, per brand and folder [S35]                          | **partial**: Role per Membership (owner / admin / member), set "a follow-up" (GLOSSARY: Role); granular RBAC is EE per ADR 0014                             | **med**: matches the EE line                                                                                                                        |
| SSO, IP allow-list, masking, SIEM export              | Enhanced Security module on all projects [S36]                                         | **missing** SSO/SAML (EE, ADR 0014); audit log **has** (EE, ADR 0022); IP allow-list, PII masking, SIEM export missing                                      | **med** (EE): fits the "org-shape" gating rule                                                                                                      |
| API access control                                    | IP allow-list for API [S18]                                                            | **partial**: scoped API tokens (`tokens.tsp`), rate limits (ADR 0025)                                                                                       | **low**                                                                                                                                             |
| Staff actions log                                     | Event log [S7][S36]                                                                    | **has**: Audit log (EE, ADR 0022)                                                                                                                           | n/a                                                                                                                                                 |
| Multi-brand                                           | Brands inside one project [S14]                                                        | **has** differently: multiple Workspaces per User via Membership (ADR 0004); no cross-workspace shared settings                                             | **low**: Workspaces already cover the use case                                                                                                      |
| Outbound event webhooks                               | Scenario step plus "Webhooks" doc section [S2]                                         | **has**: Webhook endpoints with HMAC, self-disable (GLOSSARY), plus MCP surface (ADR 0016)                                                                  | n/a: sphericon is ahead                                                                                                                             |
| Self-hosting / on-premise                             | On-premise for Standard (enterprise) [S7][S37]                                         | **has**: single binary, open core                                                                                                                           | n/a: the main differentiator                                                                                                                        |
| Pricing model                                         | Quote-based, from 15,000 / 150,000 RUB [S37]                                           | AGPL core free; SaaS planned (ADR 0028)                                                                                                                     | n/a                                                                                                                                                 |

---

## 16. Gaps most worth closing (prioritized)

1. **Automation branching, segment-entry trigger and re-entry.** Mindbox has multi-branch conditions, a segment
   enter / leave start event and repeat limits [S9][S23]; sphericon is linear, single-trigger and enroll-once. These
   three changes unlock most real lifecycle flows (win-back, replenishment, nurture). The GLOSSARY already defers
   branching "until the sequence builder is real".
2. **Frequency cap and a global holdout.** A per-recipient daily cap [S29] and a rolling control group [S28] both sit
   on the existing single send chokepoint (ADR 0015) and the eligibility layers (ADR 0001). Cheap, protects
   deliverability, and gives honest measurement.
3. **Email A/B test with a control.** Subject and content variants on Broadcast first, then an automation split
   block; conversion metrics need a goal event [S27]. ROADMAP still lists A/B as open.
4. **Importers (CSV plus competitor formats).** ADR 0014 calls importers the adoption lever, and Mindbox documents
   contact, order and action imports [S2]. No contact import was found in the TypeSpec.
5. **On-site forms and popups.** ROADMAP phase 5 not started. Mindbox ships popups with frequency and segment
   targeting [S22]; for sphericon this is the lead-capture front door of the email funnel.
6. **Goal events and attribution.** A last-non-direct-touch model within a configurable window [S30] on a generic
   "conversion" event delivers revenue attribution without a full order model.
7. **Webhook response variables in the HTTP step** [S24], and a **static or frozen segment** option for audits.
8. **Preference center with topics** [S18] and **block-level segment visibility** in the email editor [S33].
9. **Promo code pools** as a small, standalone entity (generation, one-time use, per-contact limit) [S26].
10. **Manual contact merge**, with a history record [S13].
11. EE candidates that match ADR 0014: IP allow-list for staff, PII masking with logged unmask, SIEM export [S36].

## 17. Mindbox things deliberately NOT worth copying

- **Loyalty engine, discount calculation at the till, gift and discount cards, POS and cashier integrations** [S6]:
  a retail commerce engine with sub-second SLAs; out of scope for an email-first open-core product.
- **Price personalization (PDP) and Ads modules** (Yandex Direct, retargeting, Media) [S1]: separate products with
  separate buyers.
- **Mobile SDKs, mobile push and In-App** [S17]: platform work for a use case sphericon does not target.
- **Web push** [S16]: no iOS, per-browser subscriptions, low reach; revisit only on demand.
- **Two-way chatbots and operator routing** [S8]: GLOSSARY already excludes the on-site chatbot from the send model.
- **ML recommendation stack and churn prediction** [S5]: needs product catalog data and ML operations; no clear
  payoff before orders exist.
- **Sales-led, quote-only pricing and a mandatory implementation consultant** [S37]: opposite of the self-serve
  open-core adoption path.
- **No full-HTML upload in the email builder** [S33]: sphericon's embedded HTML editor (ADR 0040) is the better
  design for this audience.
- **Regional integration catalogue** (1C, Bitrix, SET Retail, Russian SMS and messengers) [S4]: a market choice;
  keep the generic HTTP step, Webhooks and ingest hooks instead.
- **Automatic, non-disableable identity merging** [S13]: sphericon's stricter Identify model avoids wrong merges;
  offer merge as an explicit, audited action only.

## 18. Uncertainty flags

1. API facts rely on search excerpts of developers.mindbox.ru (TLS error on fetch); operation names and limits
   are unverified.
2. Merge, dedup and segmentation articles are old per the search tool; behavior may have changed.
3. Dedicated IPs, a subscribe-to-event webhook catalogue, and details of surveys/quizzes were not found on official
   pages.
4. Throughput and latency figures (2M RPM, 300 ms, 500 ms order calculation, 2M emails or pushes per hour) are
   marketing claims, and two are not mutually consistent on Mindbox's own pages [S3].
5. The webhook retry gap is 5 minutes on one help page and 6 minutes on another [S24].
6. Data location for 152-FZ is not stated on the security page [S7].
7. Pain points are not researched here (no secondary sources collected).
8. A third-party directory snippet quoted about 150,000 RUB as the entry price; the official page says 15,000
   (Light) and 150,000 (Standard). The official page was used.

## Sources

- [S1] https://mindbox.ru/ (home: modules, channels, scale and ISO claims)
- [S2] https://help.mindbox.ru/docs (help center structure: Data, Filters, Integrations, Scenarios, Mailings, Personalization, Loyalty, Import, Analytics, Administration)
- [S3] https://mindbox.ru/products/mailings/
- [S4] https://mindbox.ru/products/cdp/
- [S5] https://mindbox.ru/products/ml/
- [S6] https://mindbox.ru/products/loyalty-program/
- [S7] https://mindbox.ru/security/
- [S8] https://mindbox.ru/products/chat-bots/
- [S9] https://help.mindbox.ru/docs/workflow-events
- [S10] https://help.mindbox.ru/docs/workflow-startup (via search excerpt) and https://developers.mindbox.ru/docs/workflow-startup
- [S12] https://help.mindbox.ru/docs/additional-data
- [S13] Dedup and merge articles: https://help.mindbox.ru/docs/%D0%BA%D0%B0%D0%BA%D0%B8%D0%B5-%D0%BD%D0%B0%D1%81%D1%82%D1%80%D0%BE%D0%B9%D0%BA%D0%B8-%D0%B5%D1%81%D1%82%D1%8C-%D1%83-%D0%B4%D0%B5%D0%B4%D1%83%D0%B1%D0%BB%D0%B8%D0%BA%D0%B0%D1%86%D0%B8%D0%B8 , https://help.mindbox.ru/docs/%D0%BC%D0%BE%D0%B6%D0%BD%D0%BE-%D0%BB%D0%B8-%D0%B2%D1%8B%D0%BA%D0%BB%D1%8E%D1%87%D0%B8%D1%82%D1%8C-%D0%B4%D0%B5%D0%B4%D1%83%D0%B1%D0%BB%D0%B8%D0%BA%D0%B0%D1%86%D0%B8%D1%8E (search excerpts, not fully read)
- [S14] https://help.mindbox.ru/docs/multibrand
- [S15] https://developers.mindbox.ru/docs/get-customer-data-xml and sibling pages (via search excerpts: workflow-startup, export-api-reports, ticket-authentication)
- [S16] https://developers.mindbox.ru/page/webpush and https://help.mindbox.ru/docs/%D0%BF%D0%BE%D0%B4%D0%BF%D0%B8%D1%81%D0%BA%D0%B0-%D0%BD%D0%B0-web-push-%D1%83%D0%B2%D0%B5%D0%B4%D0%BE%D0%BC%D0%BB%D0%B5%D0%BD%D0%B8%D1%8F (via search excerpts)
- [S17] https://mindbox.ru/products/mobile/
- [S18] https://help.mindbox.ru/docs/whats-new (dated documentation updates, Oct 2025 to Oct 2026; read up to character 100,000 of about 152,000)
- [S19] https://help.mindbox.ru/docs/send-leads-from-mindbox-via-albato
- [S20] https://help.mindbox.ru/docs/segmentations and https://help.mindbox.ru/docs/customer-lifecycle (via search excerpts)
- [S21] https://help.mindbox.ru/docs/%D0%BA%D0%B0%D0%BA-%D1%80%D0%B0%D0%B1%D0%BE%D1%82%D0%B0%D0%B5%D1%82-rfm-%D1%81%D0%B5%D0%B3%D0%BC%D0%B5%D0%BD%D1%82%D0%B8%D1%80%D0%BE%D0%B2%D0%B0%D0%BD%D0%B8%D0%B5 (via search excerpt)
- [S22] https://mindbox.ru/products/personalization/
- [S23] https://help.mindbox.ru/docs/workflow-steps
- [S24] https://help.mindbox.ru/docs/workflows-webhook-responses (via search excerpt)
- [S25] https://help.mindbox.ru/docs/workflow-test-mode (via search excerpt)
- [S26] https://help.mindbox.ru/docs/how-to-create-a-discount-promo and the promo-code pool articles listed in the same search (excerpts)
- [S27] https://help.mindbox.ru/docs/workflow-ab-tests and https://help.mindbox.ru/docs/what-is-ab-test (via search excerpts)
- [S28] Global control group article: https://help.mindbox.ru/docs/%D1%87%D1%82%D0%BE-%D1%82%D0%B0%D0%BA%D0%BE%D0%B5-%D0%B3%D0%BB%D0%BE%D0%B1%D0%B0%D0%BB%D1%8C%D0%BD%D0%B0%D1%8F-%D0%BA%D0%BE%D0%BD%D1%82%D1%80%D0%BE%D0%BB%D1%8C%D0%BD%D0%B0%D1%8F-%D0%B3%D1%80%D1%83%D0%BF%D0%BF%D0%B0 (via search excerpt)
- [S29] https://help.mindbox.ru/docs/frequency-policy (via search excerpt)
- [S30] https://help.mindbox.ru/docs/attribution , https://help.mindbox.ru/docs/mailings-dashboard , https://help.mindbox.ru/docs/goals (via search excerpts)
- [S31] https://help.mindbox.ru/docs/%D0%BA%D0%B0%D0%BA-%D0%BD%D0%B0%D1%81%D1%82%D1%80%D0%BE%D0%B8%D1%82%D1%8C-dkim-%D0%B8-dmarc (via search excerpt)
- [S32] https://help.mindbox.ru/docs/return-path (via search excerpt)
- [S33] https://mindbox.ru/products/mailings/email-builder/
- [S34] https://help.mindbox.ru/docs/subscriptions , https://help.mindbox.ru/docs/doi-subscription-change , https://help.mindbox.ru/docs/workflow-welcome-double-opt-in (via search excerpts)
- [S35] https://help.mindbox.ru/docs/system-staff-groups , https://help.mindbox.ru/docs/staff-create , https://help.mindbox.ru/docs/passkey (via search excerpts)
- [S36] https://help.mindbox.ru/docs/enterprise-security
- [S37] https://mindbox.ru/tariffs/
