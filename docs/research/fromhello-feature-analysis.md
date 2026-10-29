# fromHello Feature Analysis (for competitive gap analysis)

Researched October 2026 (fetched 29 Oct 2026) from fromHello's public GitHub repository, the SVG images in it,
and the pages of fromhello.io. Every fromHello claim carries a source tag `[Fn]`, which the Sources list at the end
resolves. sphericon's side comes from the code, with a path and line or an ADR for each claim.

fromHello (by the French studio Synapsr) is **open-source marketing automation for SaaS and product-led teams**:
first-party tracking, real-time segments, a journey builder, email, SMS, in-app, web push and ad audiences, and an
MCP server. The license is AGPL-3.0 and a hosted cloud is planned [F1][F3]. Its target matches sphericon's
(event-driven automation on an identity spine, self-hostable, agent-operable) more closely than Drip, Keila or
Mindbox did. Its center of gravity differs: B2B SaaS lifecycle (trials, plan limits, failed payments, cancellations)
instead of e-commerce or newsletters.

---

## 1. Source status

- **No source code is published.** The repository `Synapsr/fromHello` holds one commit (`597d3d7`, 7 Oct 2026,
  "Initial commit"): README.md, LICENSE (AGPL-3.0), CONTRIBUTING.md, SECURITY.md and images. The README says "The
  platform source isn't published here yet" [F1]. Nothing can be ported or read for implementation detail. This
  note is about product ideas only.
- **Stack, as the README and the self-hosting page state it:** TypeScript on Node.js 24, a pnpm/Turborepo
  monorepo with a Next.js dashboard (`apps/web`), a Hono API (`apps/api`), a BullMQ worker on Redis (`apps/worker`),
  an MCP server on the official TS SDK (`apps/mcp`), a JS SDK (`packages/sdk`), Postgres 18+ via Drizzle, Better Auth
  and LiquidJS [F1][F5].
- **The SVGs add little.** `cards`, `hub`, `tour`, `plays`, `ai` and `hosting.svg` hold no `<text>` or `<tspan>`.
  The glyphs are outlined into paths. Each SVG's `<title>` and `aria-label` repeat the README's alt text. Rendering
  them to PNG (rsvg-convert) showed only example labels, for instance "Viewed pricing 2+ times in 7 days" and
  "Goal reached: paid plan" [F2]. The MCP dial in `ai.svg` has 59 tick marks with no tool names [F2].
- **The website has the detail** [F3]. The homepage carries the journey node palette, the segment builder, the
  channel screens, the results screen, the full MCP tool list, the six plays with their event names, and pricing.
  The changelog [F4], the self-hosting page [F5] and the Customer.io comparison [F6] add dates and admitted gaps. All
  of these fetched with HTTP 200. The site has no docs, API reference or blog.
- **Every product claim is marketing copy.** The builder, segment, inbox and results screens on the homepage are
  mock-ups labelled "Example data" [F3]. No claim can be checked against running software or code. Claims that
  appear on only one surface are tagged **(one source)** below.
- **Other Synapsr repositories hold nothing relevant.** The organization has 19 public repositories [F7]. `docs` is
  an unmodified Mintlify starter kit. `hushletter` is a newsletter reader (TanStack Start, Convex). The others are
  unrelated products (Louez, Hovod, Parler, SaaS-Monitor and others).

## 2. fromHello feature inventory

### 2.1 Tracking and profiles

- **One-tag first-party SDK.** It auto-tracks page views and sessions, has a real-time install debugger, offers
  click, form and scroll-depth triggers matched by visible text rather than CSS selectors, and captures source,
  referrer and ad click IDs (changelog 10 Apr 2026) [F4]. The SDK queues events offline [F5]. It loads no
  third-party pixels [F1][F3].
- **Anonymous history joins the contact at signup** [F1][F3]. New signups merge by email (23 Jun 2026) [F4].
- **First touch.** Channel, UTM tags, referrer and landing page are captured at first touch and never
  overwritten, along with the latest attributed visit. People can be filtered and segmented by acquisition channel
  and UTM (7 Sep 2026) [F4].
- **Profile data.** A company field, typed custom fields (text, number, boolean, date, select), events, messages
  and replies [F5]. Free-text notes on profiles (24 Jun 2026) [F4]. CSV import with column mapping and duplicate
  detection, and a kanban view of people [F4].
- **Event mappings.** An event property is copied onto a profile field, for example "plan, on signed_up". This is
  visible in the MCP tools `create_event_mapping` and friends and in one example session [F3] **(one source)**.
- **Tracking opt-out switch** per browser, also for staff opening a customer's account (10 Sep 2026) [F4].
- **First-party open tracking**: their own pixel on every provider, a first open publishes an event, and no IP or
  user agent is stored (2 Oct 2026) [F4].

### 2.2 Segments

- **Dynamic and static segments.** The type is chosen at creation, and static members are managed by hand [F4][F5].
- **Rule builder.** It nests AND/OR groups (the labels read "all of these are true" and "any of these is true") and
  filters by event, profile field, message status or suppression [F1][F3].
- **Event rule with a count.** "Pricing Viewed **at least 2 times** in last 7 days", with windows of all time, last
  day, 7, 14 and 30 days, and a toggle "Exclude profiles who triggered this event" [F3]. The journey branch shows
  "did ≥ 2 in last 7 days" with an "Any property" event-property filter [F3] **(one source for the property filter)**.
- **Message status leaf:** "Has never had status Replied" [F3].
- **Match count while editing.** The homepage says outright that this is "a preview of the rules, not live
  membership" [F3], the same model as sphericon's preview.
- **Recomputed on every event**, and entering or leaving a segment can trigger a journey (4 May 2026) [F4][F5].
  The Sales play fires a webhook "the first time someone matches" [F3].
- **Member list**: all members, paginated and searchable (7 Sep 2026) [F4].

### 2.3 Journeys

- **14 node types in four groups** [F3][F5]:
  - Triggers: Entry (trigger event, optional event property, attribute or segment entry).
  - Actions: Send email, Send SMS, Webhook, Update profile, Add to segment, Remove from segment, Enroll in journey.
  - Flow control: Wait, Wait for event (labelled edges "on event" and "on timeout"), Branch (ordered, first match
    wins, ends with "Everyone else"), A/B split (percent per variant, two edges).
  - Endings: Exit, Goal (a name plus an optional conversion event).
- **Versions with a draft.** The builder header shows "Live: v3", "Save draft", "Publish v4" [F3]. A journey an
  agent builds stays a draft until it is published [F1][F3].
- **Email step** loads subject and body from a template at send time, with "Override sender" [F3].
- **Liquid** with conditions, loops, filters, profile fields and **the event that triggered the journey**
  (29 Jul 2026) [F4]. The Expansion and Payments plays quote the limit and the amount from the triggering event [F3].
- **Who is in each journey**: one tab per status (still in, completed, exited, failed), the current step, what
  the person waits for, and whether a step is overdue (22 Sep 2026) [F4].
- **Exit on event.** In the Expansion, Payments and Retention plays a later event (`subscription_expanded`,
  `payment_succeeded`, a reply) stops the journey and cancels later steps [F3].

### 2.4 Channels and inbox

- **Email** through Resend, Postmark, SendGrid, SMTP or Microsoft 365 (OAuth sign-in, sends from the mailbox,
  reads replies including the junk folder, and adds bounces read from the mailbox to suppression) [F1][F3][F4].
- **Replies.** Mail goes out with a Reply-To address at a reply mailbox, polled over IMAP ("checking every 30s"),
  through Microsoft 365, or by webhooks [F3][F5]. A reply shows on the profile, threaded per customer, and can be
  answered inline (26 Jun 2026) [F4]. It publishes an `email_replied` event that can start a journey or wake one
  that waits for it [F1][F3]. Several reply mailboxes are possible, and each reply lands in the mailbox the message
  went out from [F4].
- **SMS** through Twilio, SMSPartner, MessageBird or a custom HTTP API, with a segment counter that is aware of
  Unicode, and STOP/HELP honoured [F3][F4]. **In-app** banners and modals for identified visitors [F3]. **Web push**
  with browser opt-in [F3]. **Ad audiences**: Meta, LinkedIn and Google, linked to a segment, synced on demand,
  contacts hashed before upload [F3].
- **Branded unsubscribe page** and the one-click header (5 Sep 2026) [F4].

### 2.5 Results

- **Per journey**: entered, completed, failed and average duration over 7, 14, 30 or 90 days, each compared with
  the previous period [F1][F3].
- **Goal funnel**: the share of enrollments that reached each goal node [F1][F3].
- **A/B**: a winner is called "only with enough data and a significant lift". The example shows "97% confidence,
  +34%", with the success metric "reached downstream node" and email engagement tracked beside it [F1][F3]. The
  sample-size and significance rules are not published.
- **Workspace analytics** as a funnel with per-stage drop-off (30 May 2026) [F4].

### 2.6 AI and MCP

- **Hosted endpoint** `https://mcp.fromhello.io/v1`, or self-hosted over stdio or HTTP [F1][F3].
- **Tool list.** The page claims 59 tools but lists **57 names** [F3]: `lookup_skill`; segments (`list_`, `get_`,
  `preview_`, `create_`, `update_`, `delete_segment`, `add_to_segment`, `remove_from_segment`); journeys (`list_`,
  `get_journey`, `get_journey_draft`, `create_journey_draft`, `save_journey_draft`, `update_`, `publish_`, `pause_`,
  `enroll_in_`, `delete_journey`); templates (list, get, create, preview, update, delete); tracking
  (`list_event_properties`, `list_event_property_values`, CRUD on custom fields and event mappings); **15 analytics
  reads** (`get_overview_kpis`, `get_message_volume`, `get_channel_detail`, `get_template_performance`,
  `get_journey_performance`, `get_journey_insights`, `get_audience_tiers`, `get_segment_growth`,
  `get_lifecycle_conversion`, `get_engagement_trend`, `get_retention_curve`, `get_top_events`,
  `get_engagement_funnel`, `get_audience_recommendations`, `get_message_stats`); people (`search_profiles`,
  `get_profile`, `set_profile_ai_context`, profile notes); approval (`confirm_pending_action`).
- **Approval model.** No tool sends a message. Publishing, pausing or deleting a journey "takes a second,
  confirming tool call". **Other edits, including a live journey's exit condition, apply when saved**, so AI
  "can change live journeys without a confirmation step" [F1][F3].
- **Audit.** MCP tool calls are in the audit log [F1][F3]. The comparison page says "every call is written to your
  audit log" [F6] **(one source for "every")**.
- **Agent settings tab**: one-click key, setup per client, and the time each agent last called (8 Sep 2026) [F4].
- **Built-in assistant** in the app (29 May 2026) [F4]. **Opt-in AI send-time personalization** rewrites each email
  per recipient at a light, medium or deep level, with no per-message review, and the send waits if the AI fails
  (7 Jul 2026) [F1][F4].

### 2.7 The six plays [F3]

Each play lists its trigger event and what it needs:

| Play             | Mechanism fromHello describes                                                                                                              |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| Paid acquisition | Anonymous page views are passed to the Meta, LinkedIn or Google pixel. The ad brings the visitor back, and at signup the history joins.    |
| Sales            | A segment such as "on a trial, 3 teammates invited and pricing viewed in last 7 days". First match posts a Slack webhook, once per person. |
| Expansion        | `plan_limit_reached` shows an in-app offer, then an email the next day quoting the limit. `subscription_expanded` stops it.                |
| Payments         | `payment_failed` with an amount sends an email, an in-app banner and an SMS on day 3. `payment_succeeded` stops everything.                |
| Retention        | `subscription_cancelled` sends an email from your own address and one follow-up. A reply stops the journey and lands on the profile.       |
| Reviews          | `nps_submitted` with a score of 9 or 10 sends an email asking for a review and one follow-up. A low score alerts Slack instead.            |

The Paid acquisition play passes page views to third-party ad pixels. The same page says "Our tracking loads no
third-party pixels". The page does not reconcile the two.

### 2.8 Data, control and what fromHello lacks

- Export a contact as JSON or erase profile and messages through the API, with each erasure audited. A suppression
  list fed by unsubscribes and bounces. The audit log covers invites, API keys, sending settings, consent changes,
  erasures and MCP calls. **Four roles, from owner to viewer.** API keys are scoped to one workspace [F1][F3][F5].
- **Admitted gaps** [F6]: "No newsletters or one-off broadcasts: messages to many people go out through
  journeys", "No store, cart or order integration", no mobile push, no WhatsApp, not generally available.
- Pricing exists, per reachable contact, with tiers [F3]. It is noted for completeness only. The open app stays
  neutral about the hosted offering (AGENTS.md), so nothing here is a recommendation on it.

## 3. Gap table

Verdicts: **adopt** (take the idea as is), **adapt** (take it in sphericon's own model), **have** (already
built), **out** (out of scope).

| Feature                                           | fromHello                                   | sphericon today                                                                                                                                                                                                                                                                                                                                                                                                                                               | Verdict         | Why                                                                                                                                    |
| ------------------------------------------------- | ------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| Anonymous history joins at signup                 | [F1][F4]                                    | Contact + Visitor + Identify (GLOSSARY.md:62-68, ADR 0002); events of the Visitor attach by `contact_id` (`internal/condition/condition.go:244-246`)                                                                                                                                                                                                                                                                                                          | have            | Same capability. sphericon's stitching is stricter and has no "merge"                                                                  |
| Event leaf "at least N times in a window"         | [F3]                                        | Event leaf is "performed / notPerformed, optionally within N days" only (`segments.tsp:42-43`, `condition.go:247-279`)                                                                                                                                                                                                                                                                                                                                        | adopt           | Needed by the Sales and "high intent" segments. Small compiler change                                                                  |
| Event-property filter on an event leaf            | [F3] (one source)                           | None in a Segment. `trigger:<key>` exists only in a Rule (`segments.tsp:33-36`, ADR 0054)                                                                                                                                                                                                                                                                                                                                                                     | adapt           | Pairs with the count leaf. Property keys have no catalogue (ADR 0054)                                                                  |
| Numeric and date comparisons                      | implied by "score of 9 or 10" [F3]          | Custom fields are typed number and datetime (`custom-fields.tsp:8-13`) but take only eq, neq, contains and notNull (`segments.tsp:13-15`). A non-text `trigger:` value makes the leaf false (`segments.tsp:35-36`)                                                                                                                                                                                                                                            | adopt           | Blocks the Reviews play. Already recommended in `keila-feature-analysis.md` §9.2                                                       |
| Message-status and suppression leaves             | [F1][F3]                                    | Engagement is an Event (`email.opened` …) usable in a leaf, human-only (`condition.go:258-261`). No suppression leaf. Preview counts only deliverable contacts (`segments.tsp:118-122`)                                                                                                                                                                                                                                                                       | have / low      | Message status is covered by Events. A suppression leaf is a small add if asked for                                                    |
| Live match count while editing                    | [F3]                                        | `Segments_preview` on `/site` and `/api`, so on MCP (`segments.tsp:131-141`)                                                                                                                                                                                                                                                                                                                                                                                  | have            | Same "preview, not membership" model                                                                                                   |
| Static segment                                    | [F4][F5]                                    | Rejected: "There is no other segment kind" (GLOSSARY.md:138-142)                                                                                                                                                                                                                                                                                                                                                                                              | out             | A deliberate model decision. A Tag covers a hand-managed set                                                                           |
| Segment enter or leave starts a journey           | [F4][F5]                                    | Missing. Segments are never materialized (GLOSSARY.md:139-140, ADR 0029). A Rule on an Event with a Condition plus apply-Tag gives "first time matched" (ADR 0054)                                                                                                                                                                                                                                                                                            | adapt (grill)   | Recommended before: `mindbox-feature-analysis.md` §16 #1, Drip "Entered a Segment"                                                     |
| Journey branch (first match wins)                 | [F3][F5]                                    | Missing. Steps are strictly linear (`automations.tsp:14-20`, GLOSSARY.md:387-394)                                                                                                                                                                                                                                                                                                                                                                             | adapt (grill)   | `docs/grill-backlog.md:143-148`, mindbox §16 #1                                                                                        |
| Wait for event with timeout                       | [F1][F3]                                    | Missing. Only a `wait` of fixed seconds (`automations.tsp:78-79`)                                                                                                                                                                                                                                                                                                                                                                                             | adapt (grill)   | Needs two outgoing paths, so it comes with branching                                                                                   |
| A/B split with significance gate                  | [F1][F3]                                    | Missing in Automations and Broadcasts                                                                                                                                                                                                                                                                                                                                                                                                                         | adapt (grill)   | mindbox §16 #3, drip §11. Depends on branching and goals                                                                               |
| Exit on event / goal                              | [F3][F5]                                    | `exited` exists and is reached by unsubscribe, suppression or hard bounce (`enrollments.tsp:6-11`, GLOSSARY.md:377-385, `internal/automations/enrollment.go:70-91`). No exit or goal event                                                                                                                                                                                                                                                                    | adopt           | Works on a linear Automation, no branching needed. Unblocks Payments, Expansion and Retention                                          |
| Trigger event's properties in the message         | [F4] (29 Jul)                               | Automation email and HTTP step bind Contact fields only (`internal/outbound/outbound.go:712-729`, `internal/automations/httpstep.go:57`). A Rule's HTTP action binds `event` (`internal/jobs/rules_http.go:99`)                                                                                                                                                                                                                                               | adopt           | "Your payment of €49 failed" needs it. The Rule path shows the shape                                                                   |
| Update profile, enroll in journey actions         | [F3][F5]                                    | Rule actions: apply/remove Tag, HTTP, enroll in Automation (`rules.tsp:14-19`). Automation steps: no set-field and no enroll                                                                                                                                                                                                                                                                                                                                  | adapt           | `docs/grill-backlog.md:135-141` already names them                                                                                     |
| Re-entry                                          | not stated                                  | Enrollment once ever (GLOSSARY.md:377-379, `enrollment.go:14-17`)                                                                                                                                                                                                                                                                                                                                                                                             | adapt (grill)   | Payments fails monthly, so a once-ever Enrollment covers the first failure only. mindbox §16 #1                                        |
| Draft and live version, publish step              | [F3]                                        | Automation versions exist (ADR 0049), but `Update` has no status gate: saving steps of an active Automation changes them for every later Enrollment at once (`internal/automations/automations.go:352-391,411-415`). Rule `Update` leaves the status untouched, so an active Rule's Condition and Actions change live (`internal/rules/rules.go:217-219`). A Broadcast, by contrast, is editable only as a draft (`internal/broadcasts/authoring_test.go:72`) | adapt (grill)   | See §4 item 6: a hole in ADR 0016's "send is a second lock"                                                                            |
| Who is in each Automation                         | [F4]                                        | `Enrollments_list` with a status filter, current step and `resumeAt`, on `/site` and `/api` (`enrollments.tsp:38-89`)                                                                                                                                                                                                                                                                                                                                         | have            | Overdue flag and search are UI polish                                                                                                  |
| Results per Automation over a period              | [F1][F3]                                    | Workspace totals (`analytics.tsp:53-65`). Per-Automation engagement report: opens, clicks, delivered, bounced, complained, links (`automations.tsp:229-260`). No entered, completed, failed or duration per period. Revenue per Automation is planned (#719, ADR 0064)                                                                                                                                                                                        | adopt           | Cheap from `AutomationRun` rows. Report day is ADR 0057                                                                                |
| Goal funnel                                       | [F1][F3]                                    | Missing                                                                                                                                                                                                                                                                                                                                                                                                                                                       | adapt           | Follows exit-on-goal                                                                                                                   |
| Analytics readable by agents                      | 15 `get_*` tools [F3]                       | `/api` exposes `Broadcasts_report`, `Enrollments_*` and `SendingDomainRates_list` only. No Automation report and no workspace analytics on `/api` (`openapi/external.openapi.json`), so none on MCP (ADR 0016)                                                                                                                                                                                                                                                | adopt           | Projection is automatic, so the cost is contract work                                                                                  |
| MCP tool surface                                  | 57 listed, 59 claimed [F3]                  | About 95 tools: 108 `/api` operations (`openapi/external.openapi.json`) minus 13 hidden (`typespec/external/mcp.tsp`)                                                                                                                                                                                                                                                                                                                                         | have            | Parity in breadth. Analytics is the gap (row above)                                                                                    |
| Playbooks over MCP (`lookup_skill`)               | [F3]                                        | MCP prompts `welcome`, `win-back`, `list-hygiene`, `set-up-sending-domain` (`internal/mcpserver/prompts/`)                                                                                                                                                                                                                                                                                                                                                    | have            | Add prompts for the plays once their primitives exist                                                                                  |
| Agent cannot send                                 | [F1][F3]                                    | Send-class tools need `mcp:send` and are not listed without it (`typespec/external/mcp.tsp:58-62`, ADR 0016)                                                                                                                                                                                                                                                                                                                                                  | have            | Stricter than fromHello's per-call confirmation, but see the draft/live row                                                            |
| Second confirming call for publish, pause, delete | [F1][F3]                                    | None. Delete on an Automation with Enrollments is refused (409) and archive is reversible (`typespec/external/resources/automations.tsp:165-200`)                                                                                                                                                                                                                                                                                                             | low             | Scope gating covers send. MCP elicitation could add confirmation later                                                                 |
| MCP calls in the audit log                        | [F1][F3][F6]                                | Writes through MCP are audited under the API-token actor, with user agent (ADR 0022). Reads are not, and an entry does not say it came through MCP                                                                                                                                                                                                                                                                                                            | adapt (small)   | A surface marker on the entry is cheap                                                                                                 |
| Connected agents and last call                    | [F4]                                        | API tokens have `lastUsedAt` (`typespec/site/resources/tokens.tsp:26`). OAuth (ADR 0016) mints ordinary scoped tokens                                                                                                                                                                                                                                                                                                                                         | have / low      | UI grouping only                                                                                                                       |
| Reply capture as an Event                         | [F1][F3][F4]                                | Missing. No Reply-To setting anywhere in the contracts. Inbound presets handle delivery callbacks only: `generic`, `mailgun`, `postmark`, `ses` (`internal/preset/builtin/`). ADR 0037:37 rejects a bare MTA (VERP, a mailbox parsing DSNs) for bounces; it does not address replies                                                                                                                                                                          | adapt           | B2B retention and sales plays rest on it. `provider_message_id` is stored and indexed (`ent/schema/outbound_message.go:76-78,159-160`) |
| Two-way inbox (answer replies in the app)         | [F4]                                        | Missing                                                                                                                                                                                                                                                                                                                                                                                                                                                       | out (for now)   | A two-way surface. Out by analogy with the on-site chatbot entry (GLOSSARY.md:1017-1018), which does not decide it: a maintainer call  |
| First-touch acquisition (UTM, referrer, landing)  | [F4]                                        | `page.view` carries URL and referrer (`packages/analytics/src/index.ts:338-342`). Nothing derived onto the Contact                                                                                                                                                                                                                                                                                                                                            | adapt (later)   | Paid-acquisition reporting. Not urgent without ad channels                                                                             |
| Email providers                                   | Resend, Postmark, SendGrid, SMTP, M365 [F1] | `smtp`, `ses`, test `inbox` (`integrations.tsp:14-20`). Resend, Postmark and SendGrid work over SMTP relay                                                                                                                                                                                                                                                                                                                                                    | have / out      | Microsoft 365 OAuth sending is a mailbox, not a bulk provider                                                                          |
| SMS, in-app, web push, ad audiences               | [F1][F3]                                    | SMS reserved (`integrations.tsp:8-11`). Popups and the like deferred (GLOSSARY.md:1003-1018)                                                                                                                                                                                                                                                                                                                                                                  | out             | Per GLOSSARY. Ad audiences can be fed by Webhooks or an HTTP step                                                                      |
| Viewer role                                       | four roles [F1]                             | `owner`, `admin`, `member` (`memberships.tsp:8-12`). No read-only role (ADR 0004, ADR 0061)                                                                                                                                                                                                                                                                                                                                                                   | adapt (later)   | ADR 0061 makes a Role a named set of Permissions, so a viewer is a table row                                                           |
| Profile notes, AI context                         | [F3][F4]                                    | Missing                                                                                                                                                                                                                                                                                                                                                                                                                                                       | out (for now)   | Low value in an agent-first product. Revisit with a CRM-like contact page                                                              |
| AI send-time personalization                      | [F1][F4] (marketing)                        | Contrary to the model                                                                                                                                                                                                                                                                                                                                                                                                                                         | out             | Breaks the author-time copy rule (GLOSSARY.md:387-390) and "AI is an author" (ADR 0016:36-40)                                          |
| Built-in assistant, AI credits                    | [F3][F4]                                    | No built-in chat by design (`docs/grill-backlog.md:36`)                                                                                                                                                                                                                                                                                                                                                                                                       | out             | BYO agent over MCP is the sphericon line                                                                                               |
| Broadcasts                                        | none [F6]                                   | Broadcast with schedule, report, pacing (ADR 0016, ADR 0023)                                                                                                                                                                                                                                                                                                                                                                                                  | sphericon ahead | fromHello admits the gap                                                                                                               |
| Orders and revenue                                | none [F6]                                   | Order and Revenue read models, last-click attribution (GLOSSARY.md:153-162, ADR 0064)                                                                                                                                                                                                                                                                                                                                                                         | sphericon ahead | fromHello admits the gap                                                                                                               |
| Export, erase, suppression, audit, own domain     | [F1]                                        | ADR 0021, ADR 0045, ADR 0022 (EE), ADR 0010, ADR 0012                                                                                                                                                                                                                                                                                                                                                                                                         | have            | Parity                                                                                                                                 |

## 4. Recommended plan

Ranked by value per unit of cost. The cost is a rough estimate from the code paths named.
"Grill" means a model decision that should go through the `grilling` skill before a spec.

### How the six plays map onto sphericon

| Play             | What sphericon can do today                                                                   | What blocks it                                                                                   |
| ---------------- | --------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| Sales            | A Rule on `team_member_invited`, Condition "lacks Tag `pql`", HTTP action to Slack, apply Tag | Count leaf (item 1)                                                                              |
| Reviews          | A Rule on `nps_submitted`, Condition `trigger:score`, enroll in an Automation                 | Numeric operators (item 2)                                                                       |
| Payments         | An Automation triggered by `payment_failed`: email, wait, email                               | Exit on `payment_succeeded` (3), `{{ event.amount }}` (4), re-entry (10). SMS and in-app are out |
| Expansion        | An Automation triggered by `plan_limit_reached`                                               | Exit on `subscription_expanded` (3), the limit in the email (4). In-app is out                   |
| Retention        | An Automation triggered by `subscription_cancelled`, email plus follow-up                     | Reply capture (9). Exit on reply (3)                                                             |
| Paid acquisition | Anonymous history joins at Identify                                                           | Ad pixels and audiences are out of scope. First touch is item 11                                 |

### 1. Event-count leaf in the Condition (adopt; low cost)

The event leaf today packs a day window into `value` and compiles to a `contact.HasEventsWith` predicate on the
action and `occurred_at` no older than N days (`internal/condition/condition.go:247-279`). Add "at least N times".
The leaf gets explicit fields (for example `times` and `withinDays`) instead of overloading `value` further. The
project is greenfield, so the contract's `SiteConditionLeaf` (`segments.tsp:30-45`) is reshaped, not extended with a
compatibility branch. It compiles to a correlated `COUNT(*)` subquery on `events` compared with N, keeping the
human-only filter for engagement (`condition.go:258-261`). Segment, Rule and preview pick it up at once, because
they share the one compiler (ADR 0054). The Go-side evaluation of `trigger:` leaves is untouched. Open question:
does "at most N" or "exactly N" earn its place, or only "at least"? fromHello shows only "at least".

### 2. Numeric and date operators for typed fields and `trigger:` (adopt; low cost)

Add `gt`, `gte`, `lt` and `lte` for `number` Custom fields, before and after (or "within N days") for `datetime`
Custom fields (`custom-fields.tsp:8-13`), and the numeric family for `trigger:<key>`, where a JSON number today makes
the leaf false (`segments.tsp:33-36`). ADR 0054 says one operator table serves the SQL and the Go evaluation, and a
test runs the same cases through each, so the operator lands once. This closes the Keila checklist
(`keila-feature-analysis.md` §9.2). Depends on nothing.

### 3. Exit and goal events on an Automation (adopt; low to medium cost)

An Automation gets a list of **exit events**. When one is recorded for a Contact with an active Enrollment, the
Enrollment ends through the existing `automations.Exit` (`internal/automations/enrollment.go:70-91`), the path the
unsubscribe already takes. The automations consumer already sees every Event type (ADR 0022:18), so this is a second lookup in the same consumer, not a new subscriber. A
**goal** is the same mechanism with an outcome: the Enrollment ends as "reached goal" rather than "exited", and
the report counts it. It works on today's linear Steps and covers what fromHello's Payments, Expansion and Retention
plays use exits for. Open questions: (a) a new terminal status, or `exited` plus a reason (the Enrollment
lifecycle is a compare-and-swap table, ADR 0046)? (b) Do exit events belong to the Automation, like the Trigger, or
to the version (GLOSSARY.md:365-369)? (c) Should an exit Condition (not only an event) be allowed, using the shared
Condition? Cross-reference: `docs/grill-backlog.md:143-148` lists "goals & exit conditions". This item takes the
linear half of that entry without settling branching.

### 4. The triggering Event's properties in Automation messages (adopt; low to medium cost)

An Automation email or HTTP step renders with `outbound.Bindings(contact, nil)`, Contact fields only
(`internal/outbound/outbound.go:712-729`, `internal/automations/httpstep.go:57`). The Rule HTTP action already binds
`event` (`internal/jobs/rules_http.go:99`). Store the triggering Event's id (or a bounded copy of its properties) on
the Enrollment (`AutomationRun`) when it is created, and bind it as `event` in every step. The values come from
ingest, so they are `x-untrusted` in the MCP projection (ADR 0016) and HTML-escaped like Contact values. Open
question: is a reference enough, given Event retention (ADR 0019) and erasure (ADR 0021), or does the Enrollment keep
a copy that expires with it?

### 5. Results per Automation over a period (adopt; low cost)

Extend `SiteAutomationReport` (`automations.tsp:229-260`) with entered, completed, failed and exited counts and
the average duration for a period of 7, 14, 30 or 90 report days (ADR 0057), each with the previous period. All of
it comes from `AutomationRun` rows (created and updated timestamps, status). Add "reached goal" once item 3 lands,
and attributed revenue once #719 lands (ADR 0064). Mirror the report on `/api` (item 7).

### 6. Draft and live versions, with publish as send-class (adapt; medium cost; grill first)

Finding from the code: saving steps on an **active** Automation applies them to every later Enrollment at once, with
no status check in `Update` (`internal/automations/automations.go:352-391`; `saveSteps` writes a new version,
`:411-415`). Rule `Update` "leaves the status untouched", so an active Rule's Condition and Actions change live
(`internal/rules/rules.go:217-219`). Neither needs `mcp:send`. The send-class list is activate, send, schedule and
unschedule only (`typespec/external/mcp.tsp:58-62`). So a token without `mcp:send` can probably rewrite the email an
active Automation sends to new Enrollments, or add an `enroll_automation` action to an active Rule. A Broadcast, by
contrast, is editable only as a draft (`internal/broadcasts/authoring_test.go:72`). This is not verified by a test.
It reads as a gap in ADR 0016's "send is a second lock" (ADR 0016:36-40) rather than a vulnerability. fromHello has
the same gap and says so [F1].

Sketch: an edit of an active Automation writes a **draft version** beside the live one (ADR 0049 already keeps
immutable versions and rewrites only an unused latest one). **Publish** makes the draft live and is send-class, so
it is gated by `automations:activate` and `mcp:send`. The same applies to a Rule's Condition and Actions. Open
questions: does every edit of a live object go through publish, or only the fields that change who gets mail and
what they get? Does publish need an ADR amending ADR 0016 and ADR 0049? Decide this before branching (item 10),
because branching reshapes the same versions.

### 7. Analytics on `/api`, and so on MCP (adopt; low to medium cost)

`/api` lacks the workspace analytics (`typespec/site/resources/analytics.tsp:262-318`: overview, new people,
subscribed people, clicks by link, opens by hour, events) and the Automation report, so an agent over MCP cannot
answer "where do signups drop off?". That question is fromHello's main MCP pitch (15 `get_*` tools [F3]). The MCP
surface is a projection of `/api` (ADR 0016), so adding external operations with an `analytics:read` scope (or
reusing the resource read scopes) makes them tools with no MCP code. Segment members are not on `/api` either (the
`/site` route is `segments.tsp:144-150`). Add MCP prompts for the plays (`internal/mcpserver/prompts/`) once items
1 to 4 exist.

### 8. Small action additions: set a Custom field, and enroll from a Step (adapt; low cost)

fromHello's "event mappings" (`create_event_mapping`, "plan, on signed_up" [F3]) is a stateless reaction. In
sphericon it is a Rule action "set Custom field `<key>` from `trigger:<prop>` (or a constant)", next to apply-Tag
(`rules.tsp:14-19`). It is a new implementation of the Action seam (ADR 0054), and the Custom field type governs
conversion (ADR 0006). The same "set Custom field" and "enroll in Automation" as Automation Step kinds are on
`docs/grill-backlog.md:135-141`. Open question: does setting a field publish an Event (and so can it chain Rules)?
ADR 0054 says an Action that starts to publish must stamp the causation marker.

### 9. Reply capture as an `email.replied` Event (adapt; medium to high cost)

Three parts. (a) A **Reply-To** on the Integration or the message. No contract has one today (`fromName` is the only
sender field, `typespec/site/resources/integrations.tsp:83`). (b) **Inbound presets**: Postmark inbound, SES
receiving through SNS and Mailgun routes post the reply to an Integration hook. This is the ADR 0037 Preset
mechanism, with a new kind of payload rather than a new subsystem. IMAP polling and Microsoft 365 are what fromHello
does. ADR 0037:37 rejects a bare MTA with an inbound mailbox for bounces (DSNs) only and says nothing about
replies, so mailbox polling for replies is undecided and needs its own decision. (c) **Correlation**: match
`In-Reply-To` and `References` against the stored `provider_message_id` (`ent/schema/outbound_message.go:76-78`,
indexed at `:159-160`). Fall back to the sender's address as an Identify-style lookup. Publish `email.replied` on the
Contact, with the message and its Broadcast or Automation. It then works as a Trigger, a Rule trigger, a Condition
leaf and an exit event (item 3). Keep the body out of the Event, or keep only a bounded excerpt, because of erasure
(ADR 0021). An inbox UI that answers in the app is left out by analogy with the on-site chatbot entry (GLOSSARY.md:1017-1018),
which is about the chatbot and does not decide this; it is a maintainer call. Open questions: store the reply body
at all? Is a reply an engagement Event for the deliverability rates (ADR 0011)?

### 10. Branching, wait for event, A/B split, re-entry, segment-entry trigger (adapt; high cost; grill)

These all change the Step and Enrollment model ("an Enrollment points at exactly one current Step", GLOSSARY.md:
387-394), and `docs/grill-backlog.md:143-148` already holds the open forks. Mindbox (§16 #1 and #3) and Drip (§2, §11)
rank them highest too. Not repeated here. What fromHello adds to that grilling:

- **Branch** = an ordered list of Conditions, first match wins, with a closing "everyone else". It reuses the shared
  Condition (ADR 0054). Branching on a Segment needs a `segment:<id>` leaf with a cycle check.
- **Wait for event** = a wait with an event action and a timeout. The existing `resumeAt` (`enrollments.tsp:55`)
  is the timeout, and the trigger consumer wakes the run on the event. Two labelled edges.
- **A/B split** = percent per variant. The success metric is "reached the next node" or a goal (item 3). A winner is
  called only above a minimum sample and a significance threshold, which fromHello does not publish. sphericon
  should write its own down.
- **Re-entry**: a policy per Automation (never, after the last Enrollment ended, at most once per N days). Payments
  needs it.
- **Segment enter or leave**: GLOSSARY.md:139-140 rules out materialized membership. Use the Rule approximation
  (an Event, a Condition, "lacks Tag", then apply Tag and enroll), or decide to materialize membership for Segments
  used as triggers only.

### 11. Lower priority (adapt later)

- **First-touch acquisition**: derive channel, UTM, referrer and landing page from the first `page.view` of a
  Contact's Visitors into a read model, the way Order is a read model of order Events (GLOSSARY.md:153-157), and
  expose it as Condition fields. Useful for acquisition reports. Low urgency while ad channels are out.
- **Viewer role**: a read-only Role is one row in the Role-to-Permission table (ADR 0061).
- **Audit surface marker**: record `api` or `mcp` on audit entries written through an API token (ADR 0022 already
  stores the user agent). Logging MCP reads is a bigger decision, since ADR 0022 keeps the log to the control plane.

### Dependencies

- Item 1 and item 2 are independent and precede the Sales and Reviews prompts (item 7).
- Item 3 precedes the goal half of item 5 and the A/B metric of item 10.
- Item 4 is independent. Item 9 benefits from item 3 (exit on reply).
- Item 6 should be decided before item 10.
- Item 7 is independent, and grows as items 1 to 5 add data.

### Open questions for the maintainer

1. Is item 6's reading of ADR 0016 right: should changing a live Automation or Rule need the send lock?
2. For goals and exits (item 3): a new terminal Enrollment status, or `exited` with a reason?
3. Is reply capture (item 9) in scope for the core? Is a provider-webhook-only version enough to start, and is
   polling a mailbox for replies acceptable, given that ADR 0037:37 rejects an inbound mailbox for bounces only?
4. Should the Condition grow event-property filters on Segment event leaves now (with item 1), or wait for an
   event-property catalogue?
5. Which plays should ship as MCP prompts next to `welcome` and `win-back`?

## 5. Things not worth copying

- **SMS, in-app banners and modals, web push, ad-audience sync** [F3]: channels GLOSSARY.md:1003-1018 defers. Ad
  audiences can be fed by Webhooks or an HTTP step to a third-party sync tool.
- **AI send-time personalization** [F1][F4]: it rewrites content after authoring and without review, against the
  author-time copy rule and "AI is an author" (ADR 0016:36-40, `docs/grill-backlog.md:54`).
- **A built-in assistant with AI credits** [F3][F4]: sphericon's line is the user's own agent over MCP
  (`docs/grill-backlog.md:36`). Credits are a hosted-offering matter, so the neutrality rule (AGENTS.md) applies.
- **Static segments** [F4]: rejected by GLOSSARY.md:138-142. A Tag covers the hand-managed case.
- **Pricing by reachable contact** [F3]: a hosted-offering decision, not for this repository.
- **Per-call confirmation in place of scopes** [F1]: sphericon's scope gate is stronger. Keep it and close the
  live-edit gap instead (item 6).

## 6. Uncertainty flags

1. Every fromHello claim comes from marketing pages and mock-ups with "Example data". No code is published.
2. The MCP tool count: 59 is claimed and 57 names are listed [F3]. The other two are unknown.
3. The A/B rule ("enough data", "significant lift") has no published threshold. The 97% figure is example data.
4. The event-property filter on segment event rules appears only in the journey branch mock-up [F3].
5. "Every call" audited [F6] versus "MCP tool calls" audited [F1]: whether reads are logged is unclear.
6. Paid acquisition passes page views to ad pixels while the same page says the tracking loads no third-party pixels
   [F3]. How the two fit together is not explained.
7. sphericon item 6 (live edits without the send lock) comes from reading `Update` in `internal/automations` and
   `internal/rules`. No test confirms or rules it out.

---

## Sources

- [F1] README.md at commit `597d3d7`: https://github.com/Synapsr/fromHello/blob/597d3d77a6dffb0d3c148e41ef462629fc2f6f3f/README.md
- [F2] `assets/{cards,hub,tour,plays,ai,hosting}.svg` at the same commit: https://github.com/Synapsr/fromHello/tree/597d3d77a6dffb0d3c148e41ef462629fc2f6f3f/assets
- [F3] Homepage: https://fromhello.io/en (fetched 29 Oct 2026)
- [F4] Changelog: https://fromhello.io/en/changelog (fetched 29 Oct 2026, latest entry 2 Oct 2026)
- [F5] Self-hosting: https://fromhello.io/en/self-hosting (page dated 7 Oct 2026)
- [F6] fromHello vs Customer.io: https://fromhello.io/en/compare/customer-io (page dated 13 Oct 2026)
- [F7] Synapsr repositories: https://github.com/Synapsr (via `gh api users/Synapsr/repos`); https://github.com/Synapsr/docs ; https://github.com/Synapsr/hushletter
- Prior sphericon notes cross-referenced: `docs/research/drip-com-feature-analysis.md`, `docs/research/keila-feature-analysis.md`, `docs/research/mindbox-feature-analysis.md`, `docs/research/revenue-and-attribution-prior-art.md`
