# Workspace overview prior art: what an email tool's home page shows

Researched November 2026 from primary sources only: the vendors' own help centers, developer docs, release notes and
product announcements (Klaviyo, Mailchimp, Customer.io, Brevo, Kit, Loops, ActiveCampaign, Omnisend, Braze, HubSpot,
Postmark, Resend). No review site or third-party blog is cited. Sphericon's side is read from the code and contracts at
`f56ec3dbb`, from [GLOSSARY.md](../../GLOSSARY.md) and from the ADRs linked inline. This note does not repeat
[bounce-and-suppression-ux.md](bounce-and-suppression-ux.md) (where vendors show bounces and suppressions),
[send-limits-prior-art.md](send-limits-prior-art.md) (who sets a send ceiling) or
[revenue-and-attribution-prior-art.md](revenue-and-attribution-prior-art.md) and
[email-attribution-and-store-refund-conventions.md](email-attribution-and-store-refund-conventions.md) (how revenue
is counted).

**Source quality caveat.** help.brevo.com and help.activecampaign.com answer automated fetchers with HTTP 403. Every
claim from those two hosts was read through a search-engine excerpt of the official page and is marked **(excerpt)**.
Mailchimp's help center does not describe its current home page, and Postmark and Resend do not document the page a
user lands on after sign-in; the rows for them say so rather than guess. The fetches were made through a tool that summarises a page, so a quoted
sentence is the tool's quotation, checked a second time for the claims the proposal rests on (Klaviyo's alerts and
conversion metric, Omnisend's lifecycle switch, Customer.io's Workspace performance page, HubSpot's Needs attention).
Vendor blocks about the vendor's own commercial terms are described neutrally ("account usage", "account notices")
and are not carried into the proposal.

## Question

The Workspace overview (`src/pages/workspace/overview.tsx`) is the landing page after sign-in. It shows the Setup
checklist until it is finished, a Revenue block, two "Create a new…" cards and "Recently created". Recent items and
links are not enough, and many Workspaces will never send an order, so revenue cannot be the centerpiece. What should
the page show?

The maintainer's ranking rule for the answer: start from what **every** Workspace needs and what is **most urgent**.
Rank each candidate block by universality (does it apply to every Workspace, shop or not, Broadcasts-only or
Automations-heavy?) and urgency (does it need the user's action now?). Things that are broken or about to break come
first and show only when true; then the figures every sender has; a block that applies only to some Workspaces
(revenue) appears only when it applies, never as a placeholder.

## Summary

- **Most vendors lead with shortcuts or setup, not with problems.** Of twelve tools, one leads with a conditional
  alerts block on the home page (Klaviyo); HubSpot's legacy home had a dismissible "Needs attention" widget that its
  new home dropped. Braze, Customer.io and Loops lead with recent or editable items, Omnisend and Brevo with setup,
  Kit with a "Start writing" action and revenue, Postmark and Resend with sending figures.
- **Problems usually live elsewhere**: a separate health page (Customer.io, HubSpot email Health, Brevo
  Deliverability Center), a banner inside a tool (HubSpot probation, ActiveCampaign limits), an email (ActiveCampaign
  account review, Braze alerts) or the moment of sending (Omnisend). The one strong precedent for "issues only when
  they occur, each with an action button" is Customer.io's Workspace performance page, and Klaviyo's home alerts.
- **Revenue is rarely forced on a non-shop**: Klaviyo turns it into a conversion count, Brevo makes it an optional
  widget, Mailchimp and ActiveCampaign show store figures only with a connected store, and Braze, Customer.io, HubSpot,
  Loops, Postmark and Resend have no revenue on home. The exception is Kit, whose creators sell through Kit itself: its
  Revenue widget comes second, and what it shows before a first sale is not documented. Omnisend is ecommerce-only.
- **Period with comparison is the norm where there is a period**: Klaviyo, Braze, Omnisend and Mailchimp's Marketing
  dashboard compare with the previous period of the same length. Defaults cluster on 30 days.
- **Onboarding is keyed to activity**, not to time: Omnisend switches its default view after the first campaign is
  sent; Braze hides its first block until something is edited.
- **Proposal** (section 4): a fixed page, no widgets, six blocks ordered by urgency, then universality: Needs
  attention (only when something is true), Setup checklist (until first completed, then never again), Sending and
  deliverability, Upcoming and recent sends, Audience and engagement, and last Revenue (only once the Workspace has
  had an Order). One period control (7/30/90, default 30) with the previous period beside every period figure. Seven
  API gaps (section 5).

## 1. Prior art

The table gives one row per tool. A cell says "not documented" when no primary page was found, and links point to the
page that owns the claim. Details per tool follow the table.

| Tool           | First block, leads with                                                        | Blocks in order                                                                                                                                              | New vs mature account                                                                                             | Revenue for a non-shop                                                                | Problems on the home page                                                                                                                            | Period and comparison                                                                     | Customizable                                         |
| -------------- | ------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- | ---------------------------------------------------- |
| Klaviyo        | Alerts, conditional ([K1])                                                     | Alerts, Business performance summary, Top performing flows, Recent campaign messages ([K1], [K2])                                                            | Recommendations and "Pick up where you left off" "depending on your setup"; when they go is not documented ([K1]) | Configurable: a non-monetary conversion metric shows conversion counts instead ([K1]) | Account notices, flow trigger anomalies, delivery drops; hidden when none ([K1])                                                                     | Selector with comparison period, custom up to 180 days, Home only ([K1])                  | Conversion metric and period only ([K1])             |
| Mailchimp      | Current web home not documented; 2016: suggestions ([M1])                      | 2016: Explore Mailchimp, Store (only with a store), Audience, Campaign engagement ([M1])                                                                     | Personalized recommended next steps after activation ([M2])                                                       | Store block / Marketing conversions only with a connected store ([M1], [M3])          | Not documented on home; suspension on the Account page ([M4])                                                                                        | Marketing dashboard: range plus "Compare with" previous period ([M3])                     | Mobile home only ([M5])                              |
| Omnisend       | Goals (setup) for a new account, then Overview (metrics) ([O1])                | Tabs: Goals, Overview (Sales performance, Audience growth), Live view ([O1])                                                                                 | Switches to Overview after the first campaign is sent ([O1])                                                      | Ecommerce-only product; not documented ([O1])                                         | None on the dashboard; paused sending shown when the user sends ([O2])                                                                               | Range, daily/weekly/monthly, compare with previous period, previous year or custom ([O1]) | Toggle chart metrics only ([O1])                     |
| Brevo          | Onboarding steps in the default layout (excerpt) ([B1])                        | User-chosen widgets: contacts, account usage, deliverability score, last campaigns, automation and transactional sent, conversions revenue… (excerpt) ([B1]) | Default layout "with no data" until used (excerpt) ([B1])                                                         | Optional "Conversions revenue" widget (excerpt) ([B1])                                | Deliverability score widget; suspension in the Deliverability Center (excerpt) ([B1], [B2])                                                          | Fixed per widget (7 or 30 days) (excerpt) ([B1])                                          | Yes: add or remove widgets per user (excerpt) ([B1]) |
| Customer.io    | "Continue where you left off" (shortcuts) ([C1])                               | Continue where you left off, message performance (30 days), Ask AI, suggested actions, team activity ([C1], [C2])                                            | Setup list during setup; first block becomes getting-started actions after 90 days without edits ([C1], [C3])     | No block ([C1])                                                                       | Not on home; Workspace performance page lists Active issues only when they occur, each with an action ([C4])                                         | Fixed 30 days, no deltas on home ([C1])                                                   | Not documented                                       |
| Braze          | "Pick up where you left off" (shortcuts) ([Z1])                                | Pick up where you left off, Performance overview ([Z1])                                                                                                      | First block hidden until something is edited or created ([Z1])                                                    | No block ([Z1])                                                                       | Not on home; alerts by email or webhook ([Z2])                                                                                                       | Range, default 30 days, % change against the previous range ([Z1])                        | KPI formulas and breakdowns; no block layout ([Z1])  |
| HubSpot        | New home: meetings; legacy home: meetings, tasks, then Needs attention ([H1])  | Legacy: Meetings, Tasks, Needs attention, Frequently visited, Recent activity, Insights… ([H1])                                                              | Onboarding guide with a progress bar in some products ([H1])                                                      | Not documented on home                                                                | Legacy Needs attention: workflow errors, disconnected accounts, dismissible; probation banner in the email tool; email Health tab ([H1], [H2], [H3]) | Not documented on home                                                                    | Yes: widgets and side panel ([H1])                   |
| ActiveCampaign | AI prompt and insight cards (excerpt) ([A1])                                   | AI home; legacy Marketing dashboard widgets (excerpt) ([A1], [A2])                                                                                           | Smart Start guide for new accounts (excerpt) ([A3])                                                               | Ecommerce dashboard in Reports, needs an integration (excerpt) ([A4])                 | Account review by email; limit banners on every page (excerpt) ([A5], [A6])                                                                          | Not documented                                                                            | Legacy widgets drag, add, hide (excerpt) ([A2])      |
| Kit            | "Start writing" action, then Revenue (metrics) ([T1], [T2])                    | Start writing, Revenue, Subscribers, Recommendations, Email performance, From Kit ([T1])                                                                     | No in-app checklist documented; parts show only with data ([T1], [T4])                                            | Revenue widget always listed; empty case not documented ([T1])                        | None on the dashboard; an app-wide bar while the account is not approved ([T5])                                                                      | Fixed: today, 7, 30 days, all time, % change for 7 and 30; email over 90 days ([T1])      | One widget preference only ([T3])                    |
| Loops          | A list of content with a Create button (shortcuts) ([L1], [L3])                | One sortable list of campaigns, loops and transactional emails ([L2], [L3])                                                                                  | Not documented (quickstart is a docs page) ([L1])                                                                 | No revenue view documented                                                            | None on home; warnings inside a campaign or workflow ([L5], [L6])                                                                                    | Not documented                                                                            | Sort and drag to order items ([L3], [L4])            |
| Postmark       | Landing page not documented; per-Server stats lead with sending figures ([P2]) | Per Server: sent, bounces, spam complaints by day ([P2], [P3])                                                                                               | Pending approval restricts recipients ([P5])                                                                      | Not applicable                                                                        | Bounce and complaint rates turn orange near and red over the limit, on an always-shown figure ([P2]); pause announced by email ([P4])                | Custom range (2013); no comparison documented ([P3])                                      | Not documented                                       |
| Resend         | Landing page not documented; Metrics page leads with volume ([R1], [R2])       | Sent chart, then Bounces and complaints with risk indicators, per-domain breakdown ([R1], [R2])                                                              | No approval; a domain is "Pending" until DNS ([R4])                                                               | Not applicable                                                                        | Permanent bounce and complaint block with risk indicators; limits on a docs page ([R2], [R3])                                                        | Presets and custom up to 30 days; no comparison documented ([R2])                         | Filters only ([R2])                                  |

### Klaviyo

Source: "Getting started with your Home dashboard" ([K1], updated 7 July 2025), and the 2023 redesign post ([K2]).

- Order: "The alerts section at the top of the Home dashboard", then Business performance summary, Top performing
  flows (up to ten) and Recent campaign messages (up to ten) ([K1]). The 2023 post gives the same order and says every
  notification is still under the bell ([K2]).
- Alerts are conditional: "you may not see the alerts section if you are not experiencing these issues", and a role
  without access does not see them. They cover account notices "or issues with specific flows or campaigns": flow
  trigger-metric anomalies and drops in a flow message's delivery volume ([K1]). Deliverability
  score, bounce or spam rates and domain authentication are not documented on Home.
- Revenue: "If you do not choose a conversion metric with an associated monetary value … you will instead see the
  total number of conversions"; the summary then shows "Total conversions and Attributed conversions" ([K1]).
- Period: "Select a date range and comparison period in the Time period selector"; custom ranges up to 180 days; "The
  selected time period applies only to the Home tab"; flows show whether revenue or conversions rose or fell against
  the comparison period ([K1]).

### Mailchimp

- The current desktop home page is not described in the help center. The only description of a home dashboard is the
  2016 announcement: Explore Mailchimp (suggestions), Store (only if a store is connected), Audience, Campaign
  engagement ([M1]). What's New mentions bot-filtered automation data "on your home screen and Marketing Dashboard"
  without describing the home screen ([M6]).
- New account: "After you activate your account, your homepage will show a personalized list of recommended next
  steps" ([M2]); when it ends is not documented.
- The Marketing dashboard (under Analytics, not the home page) shows "Marketing conversions" only with a connected
  store, has a date range and "Compare with" the previous period or audience averages ([M3]).
- Suspension details sit on the Account page and go by email to the primary contact ([M4]).

### Omnisend

Source: "Navigate your Omnisend dashboard" ([O1], dated 30 September 2026).

- Three tabs: Goals (a personalized setup checklist), Overview (Sales performance, Audience growth), Live view.
- Lifecycle switch: "After you send your first Email Campaign or SMS Campaign, the Dashboard switches to the Overview
  section as the default view." The same article elsewhere ties the switch to "connect your store and complete
  initial setup tasks"; the two triggers disagree. Goals stays reachable and keeps adding recommendations ([O1]).
- Sales performance is always store revenue (Omnisend-attributed, campaigns vs automations, total store revenue,
  orders). Audience growth shows new subscriptions and unsubscribes per channel ([O1]).
- Comparison defaults to the previous period of the same length; previous year or a custom period can be chosen and
  the choice is remembered separately from Reports ([O1]).
- Paused sending (complaints over 0.1%, bounces over 7%, unsubscribes over 2.5%) is not on the dashboard: the user sees
  "a message that email sending was paused" when sending, and the campaign gets an Error status ([O2]).

### Brevo (all excerpt)

- The home page is a set of widgets each user turns on or off under "Customize page > Manage widgets"; at least one
  stays. Widgets: calendar, contacts (total, last 30 days), account usage, email deliverability score, last campaigns,
  automation and transactional sent in the last 7 days, popular products, conversions revenue, call history, tasks
  ([B1]).
- First login: a "default home page layout with no data" of Onboarding steps, add-contacts and create-campaign and
  create-automation shortcuts, account usage and call history; data "will automatically appear" ([B1]).
- The deliverability score widget compares account health with previous months ([B1]). Suspension is explained in the
  Deliverability Center, not on the home page ([B2]).

### Customer.io

Sources: Dashboard ([C1], redesigned in the 8 April 2026 release [C2]); quick start guide ([C3]); Workspace
performance dashboard ([C4], updated 27 October 2026).

- Order: "Continue where you left off" (a carousel of pages last edited), message performance over 30 days with a
  channel switch, Ask AI, AI-suggested actions, teammates' activity over the past week ([C1]).
- A Setup list appears on the dashboard during setup, and not every item must be done ([C3]). After 90 days without
  changes, the first block becomes "a list of actions you can take to get started" ([C1]).
- Problems are on a separate page, "Workspace performance": a status ("Healthy", "Busy", "Slow performance"), and
  "Active issues lists problems affecting your workspace", each with an action such as "Review sending settings" or
  "Report to support" ([C4]). It compares today with yesterday only.

### Braze

Sources: Home dashboard ([Z1]); notification preferences ([Z2]).

- "Pick up where you left off" (recent campaigns, Canvases and segments with type and status) "appears after you have
  edited or created a campaign, Canvas, or segment"; then "Performance overview" (users, MAU, DAU, new users,
  stickiness, sessions) ([Z1]).
- Range defaults to the last 30 days; "Percentages are calculated based on the current date range as compared to the
  previous date range" ([Z1]).
- No revenue and no alerts on home; error alerts go by email or webhook ([Z2]).

### HubSpot

Sources: "Use your home page" ([H1], updated 11 September 2026); sending probation ([H2]); email Health ([H3]).

- Each user (or an admin preset) chooses the default landing page; a dashboard can be it ([H1]).
- New home (beta): meetings to prepare for, contacts to follow up with, and a side panel "Your day at a glance" ([H1]).
- Legacy home: Meetings, Tasks, **Needs attention**, Frequently visited, Recent activity, Insights, Team
  collaboration, Lessons. "Needs attention: review urgent tasks within your account, such as workflow errors,
  disconnected social accounts, and more"; "To dismiss an alert, click the X icon" ([H1]). The new home does not
  mention it.
- A sending probation shows "a banner … at the top of the marketing email tool" ([H2]); the email Health tool (score,
  bounces, spam reports) is under Marketing > Email ([H3]).

### ActiveCampaign (all excerpt)

- Accounts created after 23 July 2025 land on the Active Intelligence home: an AI prompt and insight cards; older
  accounts can switch back to the legacy Marketing dashboard (Contact trend, Campaigns, Interaction rate widgets)
  ([A1], [A2]).
- New accounts get a "Smart Start Guide" with three tasks ([A3]); insight cards need an account at least 30 days
  old with one campaign sent ([A7]).
- The Ecommerce dashboard is in Reports and needs a Deep Data integration ([A4]).
- Account review is announced by email and holds campaigns ([A5]); a limit banner shows "on each page in your
  account" ([A6]).

### Kit

Source: "Understanding the Kit dashboard" ([T1], "updated this week" when read).

- Order: "Start writing" (a template picker for a new Broadcast), Revenue, Subscribers (top sources, the most recent
  subscribers, engagement scoring), Recommendations, Email performance (average open and click rate and emails sent
  over 90 days, the last five Broadcasts), "From Kit" (news and tips) ([T1]). Another article places Revenue "at the
  top of the welcome screen" ([T2]). It leads with an action and metrics, not with alerts.
- Conditional parts: the revenue breakdown "If you earned revenue over the past 30 days", Top sources "If you gained
  subscribers over the past seven days", Recommendations only for Creator Network members ([T1]). Whether the Revenue
  widget hides for an account that never sold is not documented. Revenue in another currency than the account's
  default is left out of the widget's figures ([T1]).
- Periods are fixed: "today, the past seven days, the past 30 days, and all time", with percentage changes over seven
  and 30 days for revenue and subscribers ([T1]).
- Problems: an account not yet approved sees a yellow bar "across the top of your Kit account": "Your account won't be
  able to send emails until it is approved" ([T5]). Sequences that do not send are diagnosed by hand ([T6]).

### Loops

- Home is a list of content (campaigns, loops, transactional emails) with a Create button ([L1], [L2]); the 2024
  redesign made it a compact list sortable "by top level metrics like last updated and opens" ([L3]) and kept manual
  drag-and-drop ordering ([L4]). No account-level metrics or alerts block is documented.
- Problems are shown where they occur: bounce-rate warnings in campaign metrics ([L5]); Guardian puts "a colored
  notification dot" on a campaign, loop or transactional email when it detects an issue ([L6]).

### Postmark

- The page after sign-in is not documented. Each Server gives "high-level email performance stats" ([P2]); the
  statistics page shows sent, bounces and spam complaints by day and opens a day's activity on click ([P3], 2013).
- The limits are a bounce rate under 10% and a spam complaint rate under 0.1%; a Server's rate "will appear" orange as
  it nears a limit and red when over it ([P2]). Over the limits, the account is "contacted (via email)" and "paused,
  with messages queued" ([P4]). The warning is colour on a figure always shown, not a separate block.
- An account pending approval may only send to its own domain (API errors 412 and 413) ([P5]).

### Resend

- The Metrics page, which a changelog calls the "Overview page", shows one graph of delivered, opened, clicked,
  bounced and complained, then a breakdown per domain ([R1]); the 2025 update added date ranges, a domain filter and a
  "Bounces and Complaints" bar chart "with risk indicators" ([R2]).
- The account limits (bounce under 4%, complaint under 0.08%) are on a docs page, which names the Metrics page as
  where to watch them; going over "may result in a temporary pause in sending" ([R3]).
- There is "no sandbox mode, no approval process"; a domain is "Pending" until its DNS records are found ([R4]).

## 2. Patterns and outliers

### What recurs

- **A setup list for a new account, keyed to activity.** Omnisend (Goals until the first send), Customer.io (Setup
  list), Brevo (Onboarding steps), ActiveCampaign (Smart Start), Mailchimp (recommended next steps), HubSpot
  (onboarding guide). Where the end is documented it is an event, not a date: Omnisend's first send, Braze's first edit.
  Sphericon's Setup checklist already follows this, with one difference (section 3).
- **Recent or "continue" items near the top.** Klaviyo (Pick up where you left off, recent campaigns), Braze and
  Customer.io (first block), HubSpot (Frequently visited, Recent activity), Brevo (last campaigns). They list what the
  user touched or what went out, not what was created.
- **A period selector with the previous period beside each figure.** Klaviyo, Braze, Omnisend, Mailchimp's Marketing
  dashboard. Customer.io and Brevo use fixed windows instead.
- **Revenue only where there is a shop.** No vendor documents an empty revenue figure for an account without orders:
  it is configurable (Klaviyo), optional (Brevo), conditional on a store (Mailchimp, Omnisend's whole product,
  ActiveCampaign) or absent (Braze, Customer.io, HubSpot home, Loops, Postmark, Resend). Kit, where selling happens
  inside the product, puts it second and does not say what an account with no sale sees ([T1]). Kit also leaves revenue
  in another currency out of its figures, as Sphericon does ([T1]).
- **Account-level freezes are not a home-page block.** Suspension, review and probation go to an account page, a
  banner across the app or inside the email tool, or an email ([M4], [B2], [H2], [A5], [A6]).

### Deliberate outliers

- **Klaviyo leads with a conditional alerts block** and hides it when nothing is wrong ([K1]). It is the closest match
  to the maintainer's rule, but its alerts are about flows and payments, not deliverability.
- **Customer.io puts every operational problem on a separate page** with a health status and actionable issues ([C4]),
  keeping the home page about work in progress.
- **Brevo and HubSpot let each user build the page** from widgets ([B1], [H1]); everyone else ships a fixed layout.
- **ActiveCampaign, Customer.io and HubSpot moved the home page to an AI prompt or AI-suggested actions** in
  2025-2026 ([A1], [C1], [H1]).
- **The transactional senders lead with sending figures and rates**: Postmark colours an always-shown rate orange
  near its limit and red over it ([P2]); Resend keeps a permanent bounce and complaint chart with risk indicators
  ([R2]). They are the only tools whose first figures are deliverability.
- **Loops has no dashboard at all**: its home is a sortable list of content ([L3]).

**Inference.** The maintainer's "needs attention first, only when true" has two precedents (Klaviyo's alerts, HubSpot's
legacy widget) and one strong pattern beside it (Customer.io's issues page). Most marketing tools keep problems off the
home page; for Sphericon that would be a mistake, because Sphericon has more of them to show than a hosted ESP does: a
bring-your-own provider can stop the account (SES `SHUTDOWN`), a self-hosted Instance has no vendor watching its
reputation, and a Sending domain's DKIM can disappear.

## 3. What Sphericon already has

Read at `f56ec3dbb`. Every line is a fact about the code or the contract unless it says **Inference**.

### The page today

- [`overview.tsx`](../../src/pages/workspace/overview.tsx): the Setup checklist while `complete` is false, the
  Revenue block, two "Create a new…" cards (Broadcast, Automation) and "Recently created".
- [`SetupChecklist.tsx`](../../src/pages/workspace/SetupChecklist.tsx) reads `SiteSetupProgress`
  ([`setup-checklist.tsp`](../../typespec/site/resources/setup-checklist.tsp)): four steps (a real Integration, a
  verified Sending domain, a Contact, a sent email). The read model is point-in-time
  ([`internal/setupchecklist`](../../internal/setupchecklist/setupchecklist.go)): it is "now", not "ever", so a
  mature Workspace whose only Sending domain loses its DKIM record, or whose only Integration is archived, gets the
  whole checklist back on top of its overview.
- [`RevenueBlock.tsx`](../../src/pages/workspace/RevenueBlock.tsx) reads `SiteAnalyticsRevenue` with its own
  7/30/90 control (30 by default) and shows total, credited share and orders; `series`, `creditedOrders` and
  `otherCurrencyOrders` are returned and not read.
- [`RecentlyCreated.tsx`](../../src/pages/workspace/RecentlyCreated.tsx) merges the first page of the Broadcast and
  Automation lists by `createdAt`. `SiteBroadcastsListFilter` has only `archived`: no status filter and no order by
  `scheduledAt` or `sentAt`, so "what is scheduled" or "what went out last" cannot be asked today.
- [`WorkspaceLayout.tsx`](../../src/layouts/WorkspaceLayout.tsx) already shows, on every page, the Workspace
  suspension banner (`suspendedAt`, `suspensionReason`, `supportEmail`), the Staff hold banner (`holds[]`, each
  with its `message` and optional `actionUrl`), the Two-factor grace banner and the release banner.

### What the API already returns that the overview does not use

- `SiteAnalyticsOverview` ([`analytics.tsp`](../../typespec/site/resources/analytics.tsp)) is read only by the Email
  metrics report ([`emailMetrics.tsx`](../../src/pages/workspace/analytics/emailMetrics.tsx)), with the Analytics
  section's own range. It carries contacts (total, active, unsubscribed, new in range), the send cohort (sent,
  opened, clicked and the three rates), automations (total, active, runs active, runs completed), the daily series,
  and `abuseWarning`.
  - The send cohort is **Broadcast recipients only**: `emailCounts` and `timeSeries` in
    [`internal/analytics`](../../internal/analytics/analytics.go) query `BroadcastRecipient`. A Workspace that only
    runs Automations shows 0 sent.
  - `abuseWarning` is "absent … without the `staff` license": a self-hosted Instance without the Enterprise
    features never gets it, although [ADR 0011](../adr/0011-deliverability-rate-metrics.md) puts the rate
    computation in the core and says it "is shown on the workspace dashboard".
  - `SiteAnalyticsPoint` documents its day as a "UTC send-day"; the code cuts Report days in the Workspace time
    zone ([ADR 0057](../adr/0057-a-report-day-is-a-day-in-the-workspace-time-zone.md)). The doc comment is stale.
- The other reports (New people, Subscribed people with `current` and the daily net flow, Clicks by link, Opens by
  hour, Events) each have their own operation and page under `src/pages/workspace/analytics/`.
- The per-Sending-domain Complaint and Bounce rates as `(numerator, denominator, rate)` with a volume floor exist
  only on the Staff surface (`StaffDeliverability`, `StaffDomainRates` in
  [`staffee/resources/workspaces.tsp`](../../typespec/staffee/resources/workspaces.tsp)).
- Problems a user must act on are spread over separate resources, each shown on its own page:
  - Broadcast `holdReason` (`workspace_suspended`, `unverified_domain`, `no_integration`, `provider_stopped`) and
    status `failed` ([`broadcasts.tsp`](../../typespec/site/resources/broadcasts.tsp));
  - Integration `providerLimit.warnings` (`unlimited`, `providerQuotaUnavailable`, `providerStopped`,
    `providerProbation`, `providerSandbox`) ([`integrations.tsp`](../../typespec/site/resources/integrations.tsp));
  - Sending domain `verified` false with `verificationLostAt` set
    ([`sending-domains.tsp`](../../typespec/site/resources/sending-domains.tsp)); DMARC is returned as a suggested
    record, with no field saying whether it is published (the bulk-ready signal of ADR 0012);
  - Webhook endpoint `disabledAt` and `disabledReason` ([`webhooks.tsp`](../../typespec/site/resources/webhooks.tsp));
  - failed Enrollments, per Automation only (`SiteAutomationResults.current.failed`,
    [`automations.tsp`](../../typespec/site/resources/automations.tsp));
  - `sendCap.sentLast24h` against `sendCap.perDay` on the Workspace (`saas` only,
    [ADR 0067](../adr/0067-the-send-cap-is-the-platforms-limit-the-provider-limit-the-customers.md)).
- A period with the one before it already exists once: `SiteAutomationResults` returns `current` and `previous`
  over the same number of Report days. No Workspace-level operation does.

## 4. Proposal: one design for the overview

Everything in this section is **inference** unless it cites a fact above.

### Ranking

Each candidate block gets a universality verdict (does every Workspace have it?) and an urgency verdict (does it ask
for an action now?). The order on the page follows urgency first, then universality.

| Block                                                  | Universality                     | Urgency                            | Shown when                                             |
| ------------------------------------------------------ | -------------------------------- | ---------------------------------- | ------------------------------------------------------ |
| Suspension and Staff holds (layout banners, unchanged) | Any Workspace                    | Sending is stopped                 | While true, on every page (already)                    |
| 1. Needs attention                                     | Any Workspace that sends         | Broken or about to break           | Only while at least one item is true                   |
| 2. Setup checklist                                     | Every new Workspace              | Nothing sends until done           | Until first completed, never after                     |
| 3. Sending and deliverability                          | Every Workspace that has sent    | Medium: the context of item 1      | From the first sent message                            |
| 4. Upcoming and recent sends                           | Every Workspace that sends       | Low to medium (what goes out next) | Always; empty state carries the create actions         |
| 5. Audience and engagement                             | Every Workspace with Contacts    | Low                                | From the first Contact                                 |
| 6. Revenue                                             | Only Workspaces that send Orders | Low                                | From the first Order Completed, never as a placeholder |

Dropped: the two "Create a new…" cards (the actions move into block 4's header and the checklist's steps) and
"Recently created" (creation order says nothing about what needs a look; block 4 lists what is sending, scheduled and
sent).

### Page header and period

- One period control in the page header: 7, 30 or 90 Report days (the existing `SiteAnalyticsRange`), **30 by
  default** (Braze's and Customer.io's default window, [Z1], [C1]; ADR 0064's revenue default). It is the `range`
  search parameter, like the Analytics section's, so a figure that links to its report keeps the period.
- Every period figure in blocks 3, 5 and 6 shows the previous period of the same length beside it as a signed change
  (Klaviyo, Braze, Omnisend, [K1], [Z1], [O1]; Sphericon's own `SiteAutomationResults.previous`). A change is not
  shown when the previous period had nothing to compare with. The trailing-window deliverability rates and the
  24-hour ceilings are exempt: they are not period figures.
- Blocks 1 and 2 are live, not period-bound. The deliverability rates in block 3 use the abuse detector's trailing
  window (24 hours, [ADR 0026](../adr/0026-operator-console-and-abuse-detector.md)) and say so, because that is the
  window that freezes sending.
- No customization: a fixed layout whose blocks appear by condition. Widgets (Brevo, HubSpot) are the outlier, and a
  condition-driven page already adapts to a shop and a non-shop.

### 1. Needs attention (universal, urgent)

A list of items, each one sentence with the object it is about and one action link, ordered by severity, then by
age. Shown only while non-empty; no "all good" box (Klaviyo hides its alerts the same way, [K1]). It does not repeat
the suspension and Staff hold banners the layout shows on every page.

Severity **blocked** (something does not send now):

- A Sending domain lost its DKIM record (`verificationLostAt`): mail from it is held. Link: the domain's records.
- An Integration the provider has stopped (`providerStopped`): its sends are held. Link: Integrations.
- Broadcasts held for another cause than suspension (`holdReason` `unverified_domain`, `no_integration`,
  `provider_stopped`), as one item with a count. Link: the Broadcast, or the list when several.
- Active Automations with an email step and no Integration that can send.

Severity **degraded** (it sends, slower or at risk):

- A Sending domain in the abuse detector's warning tier: its Complaint or Bounce rate and the limit at which sending
  freezes. The tier exists only where the detector runs (its thresholds are Enterprise configuration, ADR 0011 and
  ADR 0026). On an Instance without it, block 3 shows the rates with no limits and this item never appears; whether
  the core should warn on fixed limits of its own is an open decision against ADR 0011, not part of this
  proposal.
- The Send cap is spent (`sentLast24h` ≥ `perDay`, `saas` only) or a scheduled Broadcast is larger than what is left
  of it: sending is deferred, not stopped (Deferral, [ADR 0067](../adr/0067-the-send-cap-is-the-platforms-limit-the-provider-limit-the-customers.md)).
- An Integration's Provider limit is spent for the day, or the provider has it on probation or in its sandbox
  (`providerProbation`, `providerSandbox`).

Severity **notice** (something failed and was not retried, or a setting costs delivery):

- Enrollments that failed in the last 7 days, per Automation, with the count. Link: the Automation's report.
- A Broadcast with status `failed`.
- A Webhook endpoint that disabled itself (`disabledAt`).
- A verified Sending domain without a published DMARC record (not bulk-ready, ADR 0012).
- Orders completed in another currency than the Reporting currency: "N orders in EUR are not counted". Link: the
  Reporting currency setting. This is the actionable revenue warning; see the note on the known defect below.

The Setup checklist's steps never appear here before the checklist is first completed: until then the checklist is
the list of what to do.

### 2. Setup checklist (every new Workspace, urgent for it)

Unchanged in content. One change in lifetime: it shows until the Workspace **first** completes it and never again.
Today it is recomputed "now" ([`internal/setupchecklist`](../../internal/setupchecklist/setupchecklist.go)), so a
mature Workspace whose only Sending domain loses its record gets the whole onboarding list back; after the change
that case is a **blocked** item in block 1. Omnisend's switch after the first send is the precedent ([O1]).

For a new Workspace the page is then: the checklist, block 4 with its empty state and create actions, and nothing
else, because blocks 3, 5 and 6 have no data yet and are hidden rather than shown as zeros.

### 3. Sending and deliverability (every sender)

Hidden until the first message is sent (the checklist's last step). Then, for the period:

- Messages sent, split Broadcast / Automation / Transactional, with the change against the previous period and a
  small daily series.
- Delivered, hard bounces and complaints as counts, from the delivery Events
  ([ADR 0053](../adr/0053-delivery-and-engagement-events-cover-every-send.md)).
- Per Sending domain, the Complaint rate and the Bounce rate as the detector reads them (trailing 24 hours,
  numerator / denominator / rate, the rate shown as "not enough mail yet" below the volume floor, never as 0), with
  the warning and freeze limits when the Instance runs the detector ([ADR 0011](../adr/0011-deliverability-rate-metrics.md),
  [ADR 0026](../adr/0026-operator-console-and-abuse-detector.md)).
- Where a ceiling applies, the last 24 hours against it: the Send cap on `saas`, and each Integration's Provider
  limit per day.

### 4. Upcoming and recent sends (every Workspace)

Replaces "Recently created" and the two create cards. Three short lists, each with its own "New broadcast" or "New
automation" button in the header:

- **Sending and scheduled**: Broadcasts `sending` (with progress and its estimated completion, and what limits it,
  `SiteBroadcastProgress`) and `scheduled` (with `scheduledAt`), soonest first.
- **Last sent**: the last five Broadcasts by `sentAt`, with their open and click rates and, when block 6 is shown,
  their revenue.
- **Automations**: the active ones with entered and failed enrollments in the period.

For a new Workspace the lists are empty and the block shows one empty state with both create actions.

### 5. Audience and engagement (every Workspace with Contacts)

Hidden until the first Contact. For the period, each with the previous period beside it:

- Subscribed now (`SiteAnalyticsSubscribedPeople.current`) and the net change in the period (subscribed minus
  unsubscribed), with new Contacts.
- Open rate, click rate and click-to-open of the send cohort, Broadcasts and Automations together, opens labelled as
  including machine opens or not as the reports already do.
- Each figure links to its Analytics report (Subscribed people, New people, Email metrics).

### 6. Revenue (only Workspaces that send Orders)

- **Not rendered at all** until the Workspace has read its first Order Completed. No placeholder, no "connect a
  store" alert: a Workspace that never sends orders never sees the word. How to send orders is documented with the
  Revenue settings, not advertised on the overview. A Reporting currency chosen by an Owner or Admin before any
  Order does not make the block appear.
- From the first Order: total revenue, the share credited to messages, orders, each with the previous period, and the
  daily series the endpoint already returns (`series`). Labelled, as now, as the revenue of the orders the Workspace
  sent in ([ADR 0064](../adr/0064-revenue-is-read-from-order-events-and-credited-to-the-last-clicked-message.md)).
- Orders in another currency go to block 1 as a notice (above), not into the block's figures.

**The known defect, re-read against the code.** The brief describes orders that "arrived without a `currency`" and wait
until one is chosen. The amendment to ADR 0064 removed that state: "An Order Completed must name its `currency`. One
without it is refused at ingest", "The first Order Completed a Workspace reads sets the Reporting currency", "Nothing
waits for a currency any more". The code agrees: `internal/orders/event.go` refuses an Order Completed with no
currency ("currency is required, an ISO 4217 code such as USD") and `accounts.AdoptReportingCurrency` sets the
currency from the first Order. So the states the page must tell apart are:

1. No Order ever, no currency: no block (today: the blue "No orders yet" alert, which is the placeholder to remove).
2. No Order ever, currency chosen by hand: no block (today: "0 / – / 0" with no hint, the half of the defect that is
   real).
3. Orders, some in another currency (`otherCurrencyOrders` > 0, which the UI does not read today): figures, plus the
   notice in block 1. This is the actionable "orders waiting" case that exists.
4. Order changes (Updated, Refunded, Cancelled) held because their Order Completed has not arrived: they count
   nothing yet. Not shown on the overview; the Order Completed that is missing is the shop's to send.

The maintainer's direction (no block before the first Order, a warning when orders wait, figures otherwise) holds;
only the trigger of the warning changes from "orders without a currency" to "orders in another currency". The doc
comments of `internal/orders/revenue.go` ("nil until an Owner or Admin chooses it") predate the amendment.

## 5. What the API needs

Named against the existing TypeSpec. None of it exists today.

1. **One read of what needs attention.** A new operation `GET /workspaces/{workspaceId}/attention` returning
   `SiteAttention { items: SiteAttentionItem[] }`. Each item: `kind` (a closed enum: `sendingDomainUnverified`,
   `integrationProviderStopped`, `broadcastsHeld`, `automationCannotSend`, `deliverabilityWarning`, `sendCapSpent`,
   `providerLimitSpent`, `integrationProviderProbation`, `integrationProviderSandbox`, `enrollmentsFailed`,
   `broadcastFailed`, `webhookDisabled`, `sendingDomainNotBulkReady`, `ordersInOtherCurrency`), `severity`
   (`blocked`, `degraded`, `notice`), the subject (kind, id, name), an optional `count`, `since`, and the
   kind-specific figures (`SiteAbuseWarning`'s domain, metric and rate; a currency and order count). Today these
   facts are on six resources, the Broadcast list has no status filter, and failed Enrollments are only per
   Automation, so the page would need many calls and could not see all of them. It leaves out Workspace suspension
   and Staff holds, which `SiteWorkspaceResource` already carries for the layout; a Staff hold's `reasonCode` is
   never interpreted. `SiteAnalyticsOverview.abuseWarning` moves here. `sendingDomainNotBulkReady` needs a check
   that does not exist: the DMARC record is generated and shown (`dmarcRecord`, `sending.DMARCRecord`), but no code
   looks it up, although GLOSSARY ("Sending domain") says SPF and DMARC are "generated, shown, and checked".
2. **Deliverability rates on the site surface, in the core.** A `SiteDeliverability` shaped like
   `StaffDeliverability` (`windowHours`, `volumeFloor`, `sendVolume`, `domains[]` of `complaintRate` and `bounceRate`
   as `(numerator, denominator, rate | null)`), returned without the `staff` license, as ADR 0011 already decided
   ("shown on the workspace dashboard"), plus optional `warnAt` and `freezeAt` limits present only where the detector
   runs. Without the detector the rates come with no limits and no attention item is derived from them. Either a
   field of the overview or its own operation.
3. **A send cohort that counts every marketing send.** `SiteAnalyticsEmail` and `SiteAnalyticsPoint` count Broadcast
   recipients only (`internal/analytics`), so an Automations-only Workspace shows 0 sent. They need Automation sends,
   and `sentCount` split by surface (`broadcast`, `automation`, `transactional`), with delivered, hard-bounced and
   complained counts. The `SiteAnalyticsPoint` doc comment ("UTC send-day") should say Report day.
4. **The previous period.** `SiteAnalyticsOverview`, `SiteAnalyticsSubscribedPeople` and `SiteAnalyticsRevenue` gain
   the period before the range, in the `current` / `previous` shape `SiteAutomationResults` already uses, so the page
   does not issue every call twice with shifted dates (the range enum cannot express a shifted window anyway).
5. **"Has this Workspace ever had an Order".** `SiteAnalyticsRevenue.firstOrderAt: Timestamp | null` (the first Order
   Completed read, in any currency), so the page hides the block on null rather than on `reportingCurrency`. The
   `otherCurrencyOrders` count already exists but is bound to the range; the attention item needs it over a fixed
   recent window, with the currencies it is in (`otherCurrencies: CurrencyCode[]`) so the notice can name them. The
   doc comments of `orders.Figures`, `orders.Revenue` and `WorkspaceRevenue` should drop "until an Owner or Admin
   chooses it".
6. **A Setup checklist that stays done.** `SiteSetupProgress.completedAt: Timestamp | null`, stored the first time
   every step is done, with `complete` read from it; the live step statuses stay for the checklist itself. Needs a
   column on the Workspace (a migration).
7. **Lists for block 4.** `SiteBroadcastsListFilter` gains `status` (several values) and a sort by `scheduledAt` or
   `sentAt`; the list returns `progress` for a `sending` Broadcast (today "returned by get and send only, never by
   the list"); `SiteAnalyticsAutomations` gains `entered` and `failed` enrollments in the range (today it carries
   lifetime `runsActive` and `runsCompleted`).

## Sources

Reference links used above. A host marked "excerpt" was read only through search-engine excerpts.

- K1: <https://help.klaviyo.com/hc/en-us/articles/9974064152347>
- K2: <https://www.klaviyo.com/blog/redesigned-homepage-dashboard>
- M1: <https://mailchimp.com/resources/how-our-new-dashboard-makes-your-data-actionable/>
- M2: <https://mailchimp.com/help/create-an-account/>
- M3: <https://mailchimp.com/help/about-email-analytics/>
- M4: <https://mailchimp.com/help/about-suspension/>
- M5: <https://mailchimp.com/help/about-mailchimp-for-ios/>
- M6: <https://mailchimp.com/whats-new/>
- O1: <https://support.omnisend.com/en/articles/2901036-navigate-your-omnisend-dashboard>
- O2: <https://support.omnisend.com/en/articles/1061893-troubleshoot-paused-email-sending>
- B1: <https://help.brevo.com/hc/en-us/articles/26524401876754-Customize-your-Brevo-home-page> (excerpt)
- B2: <https://help.brevo.com/hc/en-us/articles/360017299259-Why-have-my-account-or-email-campaigns-been-suspended> (excerpt)
- C1: <https://docs.customer.io/journeys/metrics/dashboard/>
- C2: <https://docs.customer.io/release-notes/2026-04-08-ui-refresh/>
- C3: <https://docs.customer.io/get-started/quick-start-guide/>
- C4: <https://docs.customer.io/messaging/metrics/health/>
- Z1: <https://www.braze.com/docs/user_guide/analytics/dashboard/home_dashboard>
- Z2: <https://braze.com/docs/user_guide/administer/global/admin_settings/notification_preferences>
- H1: <https://knowledge.hubspot.com/help-and-resources/use-your-home-page>
- H2: <https://knowledge.hubspot.com/email/resolve-an-email-sending-probation>
- H3: <https://knowledge.hubspot.com/marketing-email/monitor-your-email-sending-reputation-with-the-email-health-tool>
- A1: <https://help.activecampaign.com/hc/en-us/articles/20843404001436-Active-Intelligence-overview> (excerpt)
- A2: <https://help.activecampaign.com/hc/en-us/articles/360019364059-About-the-ActiveCampaign-Marketing-Dashboard> (excerpt)
- A3: <https://help.activecampaign.com/hc/en-us/articles/26213073117212-Getting-started-with-the-ActiveCampaign-Smart-Start-Guide> (excerpt)
- A4: <https://help.activecampaign.com/hc/en-us/articles/360010543559-About-the-ActiveCampaign-Ecommerce-Dashboard> (excerpt)
- A5: <https://help.activecampaign.com/hc/en-us/articles/221362648> (excerpt)
- A6: <https://help.activecampaign.com/hc/en-us/articles/115000364264> (excerpt)
- A7: <https://help.activecampaign.com/hc/en-us/articles/25147980442140-Active-Intelligence-Autonomous-Insights> (excerpt)
- T1: <https://help.kit.com/en/articles/15358051-understanding-the-kit-dashboard>
- T2: <https://help.kit.com/en/articles/6085844-sell-your-products-in-different-currencies>
- T3: <https://help.kit.com/en/articles/15460643-subscriber-signals-getting-started>
- T4: <https://help.kit.com/en/articles/14005977-how-to-set-up-your-new-kit-account-a-complete-checklist>
- T5: <https://help.kit.com/en/articles/4327425-troubleshooting-incentive-email-not-sending>
- T6: <https://help.kit.com/en/articles/2502568-troubleshooting-why-isn-t-my-email-sequence-sending>
- L1: <https://loops.so/docs/quickstart>
- L2: <https://loops.so/>
- L3: <https://loops.so/changelog/redesign>
- L4: <https://loops.so/updates/our-first-redesign>
- L5: <https://loops.so/changelog/goals-are-live>
- L6: <https://loops.so/changelog/guardian>
- P2: <https://postmarkapp.com/support/article/1137-servers-faq>
- P3: <https://postmarkapp.com/blog/redesigned-statistics-and-activity-updates>
- P4: <https://postmarkapp.com/support/article/does-postmark-have-a-daily-send-limit>
- P5: <https://postmarkapp.com/developer/api/overview>
- R1: <https://resend.com/changelog/improved-overview-metrics>
- R2: <https://resend.com/changelog/enhanced-metrics-dashboard>
- R3: <https://resend.com/docs/knowledge-base/account-quotas-and-limits>
- R4: <https://resend.com/docs/knowledge-base/does-resend-require-production-approval>

[K1]: https://help.klaviyo.com/hc/en-us/articles/9974064152347
[K2]: https://www.klaviyo.com/blog/redesigned-homepage-dashboard
[M1]: https://mailchimp.com/resources/how-our-new-dashboard-makes-your-data-actionable/
[M2]: https://mailchimp.com/help/create-an-account/
[M3]: https://mailchimp.com/help/about-email-analytics/
[M4]: https://mailchimp.com/help/about-suspension/
[M5]: https://mailchimp.com/help/about-mailchimp-for-ios/
[M6]: https://mailchimp.com/whats-new/
[O1]: https://support.omnisend.com/en/articles/2901036-navigate-your-omnisend-dashboard
[O2]: https://support.omnisend.com/en/articles/1061893-troubleshoot-paused-email-sending
[B1]: https://help.brevo.com/hc/en-us/articles/26524401876754-Customize-your-Brevo-home-page
[B2]: https://help.brevo.com/hc/en-us/articles/360017299259-Why-have-my-account-or-email-campaigns-been-suspended
[C1]: https://docs.customer.io/journeys/metrics/dashboard/
[C2]: https://docs.customer.io/release-notes/2026-04-08-ui-refresh/
[C3]: https://docs.customer.io/get-started/quick-start-guide/
[C4]: https://docs.customer.io/messaging/metrics/health/
[Z1]: https://www.braze.com/docs/user_guide/analytics/dashboard/home_dashboard
[Z2]: https://braze.com/docs/user_guide/administer/global/admin_settings/notification_preferences
[H1]: https://knowledge.hubspot.com/help-and-resources/use-your-home-page
[H2]: https://knowledge.hubspot.com/email/resolve-an-email-sending-probation
[H3]: https://knowledge.hubspot.com/marketing-email/monitor-your-email-sending-reputation-with-the-email-health-tool
[A1]: https://help.activecampaign.com/hc/en-us/articles/20843404001436-Active-Intelligence-overview
[A2]: https://help.activecampaign.com/hc/en-us/articles/360019364059-About-the-ActiveCampaign-Marketing-Dashboard
[A3]: https://help.activecampaign.com/hc/en-us/articles/26213073117212-Getting-started-with-the-ActiveCampaign-Smart-Start-Guide
[A4]: https://help.activecampaign.com/hc/en-us/articles/360010543559-About-the-ActiveCampaign-Ecommerce-Dashboard
[A5]: https://help.activecampaign.com/hc/en-us/articles/221362648
[A6]: https://help.activecampaign.com/hc/en-us/articles/115000364264
[A7]: https://help.activecampaign.com/hc/en-us/articles/25147980442140-Active-Intelligence-Autonomous-Insights
[T1]: https://help.kit.com/en/articles/15358051-understanding-the-kit-dashboard
[T2]: https://help.kit.com/en/articles/6085844-sell-your-products-in-different-currencies
[T3]: https://help.kit.com/en/articles/15460643-subscriber-signals-getting-started
[T4]: https://help.kit.com/en/articles/14005977-how-to-set-up-your-new-kit-account-a-complete-checklist
[T5]: https://help.kit.com/en/articles/4327425-troubleshooting-incentive-email-not-sending
[T6]: https://help.kit.com/en/articles/2502568-troubleshooting-why-isn-t-my-email-sequence-sending
[L1]: https://loops.so/docs/quickstart
[L2]: https://loops.so/
[L3]: https://loops.so/changelog/redesign
[L4]: https://loops.so/updates/our-first-redesign
[L5]: https://loops.so/changelog/goals-are-live
[L6]: https://loops.so/changelog/guardian
[P2]: https://postmarkapp.com/support/article/1137-servers-faq
[P3]: https://postmarkapp.com/blog/redesigned-statistics-and-activity-updates
[P4]: https://postmarkapp.com/support/article/does-postmark-have-a-daily-send-limit
[P5]: https://postmarkapp.com/developer/api/overview
[R1]: https://resend.com/changelog/improved-overview-metrics
[R2]: https://resend.com/changelog/enhanced-metrics-dashboard
[R3]: https://resend.com/docs/knowledge-base/account-quotas-and-limits
[R4]: https://resend.com/docs/knowledge-base/does-resend-require-production-approval
