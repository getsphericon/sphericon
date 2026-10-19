# sphericon Roadmap — toward drip.com-level marketing automation

## Context

sphericon is an open-core marketing automation platform (Go backend + React/Vite frontend,
workspace-scoped multi-tenant). The goal is to incrementally bring the product to the level of
**drip.com**, but as a **general-purpose marketing automation platform** (not the e-commerce-specific
ECRM that drip is — e-commerce is designed-for architecturally and built later).

**Current state (the core is built):**

- Identity per the accepted ADRs: **Contact** is multi-key (subject_id / email / phone, all optional),
  has no `status`/`prospect` flag; **Visitor** resolves to a Contact via **Identify**, which stitches
  earlier anonymous Events; Events attach by stable `contact_id` resolved at ingest (never by email).
  Custom fields (auto-created, typed). Collect API (identify + events). [ADR 0002, 0006]
- **Segment engine** (`internal/segments`): react-querybuilder rules compiled to ent predicates —
  attributes, custom fields, and event-correlation conditions (NOT-EXISTS), with a live preview count.
- **Send-eligibility** (`internal/eligibility`, [ADR 0001]): (channel, destination)-keyed Suppression +
  per-source Unsubscribe, derived (never a contact flag); consulted in every send path; scoped
  unsubscribe links + an "unsubscribe from everything" path.
- **Sending — all three surfaces:** Broadcasts (river engine, open/click/unsub tracking, per-broadcast
  report), Automations (linear send/wait, enroll-once, event-bus triggers, visual xyflow builder),
  Transactional (`/api` send, references a Template, respects Suppression). Templates are MJML
  (compiled on send), copied-at-author-time for marketing [ADR 0003].
- **Deliverability so far:** bounce/complaint ingestion (SES-over-SNS hook → Suppression),
  workspace analytics dashboard.
- Infra: watermill domain-event bus (transactional outbox), river jobs — both with real handlers.
  Three API surfaces (site/external/collect), scoped API tokens. SMTP/SES integrations
  (encrypted). **Multi-user workspaces** [ADR 0004]: a User reaches a Workspace through a
  Membership (owner / admin / member), with invitations and a members UI; site handlers are
  scoped from the caller's Membership.

**Gaps to reach parity (the big rocks left):** automation branching / goals / per-step conditions,
onsite forms/popups, a visual MJML editor, A/B testing, an outbound
SES-compatible send API, more provider adapters (Yandex/SendGrid/…), SMS channel, e-commerce.

For a detailed competitive feature breakdown of drip.com, see
`research/drip-com-feature-analysis.md`. For an
analysis of the incumbent open-source competitor's pain points (Mautic) mapped to sphericon's
position — and the net-new items it surfaced — see
`research/mautic-pain-points-analysis.md`.

## Guiding principle: lean on maintained libraries

Minimize hand-rolled code. Where popular, **actively maintained** solutions exist, use them. Confirm a
library's maintenance status at `go get` / `pnpm add` time and pick the living variant.

| Task                             | Library (instead of hand-rolling)                                             |
| -------------------------------- | ----------------------------------------------------------------------------- |
| Email templating / merge tags    | **`github.com/osteele/liquid`** (Liquid, like drip); fallback `flosch/pongo2` |
| HTML → plaintext (text part)     | **`github.com/k3a/html2text`** or `jaytaylor/html2text`                       |
| Link rewriting + pixel injection | **`golang.org/x/net/html`** (official parser), not regexes                    |
| Signing tracking tokens          | **JWT** (already a dep via go-pkgz/auth) / `golang-jwt`, not hand-rolled HMAC |
| Send queue / retries             | **river** (already in the repo) — don't write an engine                       |
| Email HTML editor                | **`@mantine/tiptap`** (official Mantine package)                              |
| Responsive email (later)         | **`github.com/Boostport/mjml-go`** (MJML)                                     |
| Passkeys / WebAuthn              | **`github.com/go-webauthn/webauthn`** (go-pkgz/auth does not do WebAuthn)     |
| TOTP two-factor                  | **`github.com/pquerna/otp`** (TOTP + QR provisioning URIs)                    |

## Phased roadmap

Each phase builds on the previous ones. The order is chosen so that the first value (real email sending)
appears quickly, and the largest block (automations) lands on a ready foundation of sending + segments +
tracking.

| Phase     | Block                                | Status           | Depends on | Summary                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| --------- | ------------------------------------ | ---------------- | ---------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **1**     | **Broadcasts MVP**                   | ✅ Done          | —          | One-off email campaigns end-to-end + delivery tracking (opens/clicks/unsub) + per-campaign report. Audience = all active contacts (+ rule segment).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| **2**     | **Segment engine**                   | ✅ Done          | 1          | react-querybuilder rule definition compiled to an ent predicate (attributes + custom fields **+ event-based conditions** — "performed event X in last N days" via a correlated EXISTS), preview count, usable as broadcast audience.                                                                                                                                                                                                                                                                                                                                                                                                                              |
| **3**     | **Email templates + MJML**           | ✅ Done          | 1          | Reusable templates; single body format — **MJML** everywhere (liquid → MJML compile → text), test sends. A proper visual MJML editor is still to come (body is an MJML textarea for now).                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| **4**     | **Automations / Workflows**          | ✅ Done (linear) | 1, 2, 3    | Automation + AutomationRun schema, river-backed engine (trigger → email/wait steps, enroll-once), site CRUD API + UI (list, step editor, activate/deactivate). Triggers fire off the **domain-event bus** (`internal/events`). A visual branch/goal builder (@workflowbuilder/sdk, xyflow) is still to come — steps are a linear list for now.                                                                                                                                                                                                                                                                                                                    |
| **ADR**   | **Identity + eligibility refactors** | ✅ Done          | —          | ADR 0002 (unified Contact identity — multi-key, email optional, events by resolved id, anonymous-event stitching, `prospect` dropped) and ADR 0001 (send-eligibility — no `contact.status`; (channel, destination)-keyed Suppression + per-source Unsubscribe; derived eligibility) are built, wired into every send path, and tested.                                                                                                                                                                                                                                                                                                                            |
| 5         | Forms & onsite                       | ⬜               | 1, 2       | Signup forms/popups, embed, feeding into contacts/events (on top of Collect API + tracker).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| 6         | Analytics + deliverability           | 🟡 In progress   | 1, 4       | Dashboards **✅** (workspace analytics overview). Suppression list **✅** (do-not-send registry, consulted in the send loop). Bounce/complaint **ingestion ✅** — inbound hooks at `/hooks/{key}` (ADR 0037: the key belongs to one Integration, which reads its callbacks with a Preset; SES is the `ses` Preset: signature-verified, topic-bound and subscription-confirming) normalizes into typed `EmailBounce`/`EmailComplaint` events → suppression (permanent bounces + complaints). Sending domains + native DKIM **✅** (ADR 0010). Still ⬜: outbound SES-compatible endpoint (extend nikoksr/notify), A/B, more provider adapters (Yandex/SendGrid/…). |
| 7 (later) | E-commerce                           | ⬜               | 2, 4       | Shopify/Woo connectors, product catalog, purchase/cart events, revenue attribution. Enabled architecturally via the events model from Phase 1.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |

> **Progress:** Phases 1–4 **and** the two accepted ADR refactors (0001 send-eligibility,
> 0002 unified identity) are implemented, tested, and on `main`. Phase 2 added a
> standalone, domain-agnostic rule engine (`internal/segments`) compiling the
> react-querybuilder format to SQL; segment targeting + a deliverable-count preview
> are wired into broadcasts. Phase 3 made email bodies **MJML-only** (compiled via
> gomjml on send) with reusable templates and test-sends — one format, no dual editor.
> Phase 4 shipped linear automations (trigger → email/wait steps) end-to-end, with
> enrollment driven by an internal **domain-event bus** (`internal/events`, transactional
> outbox over watermill-sql; see docs/design/domain-events.md) and a **visual xyflow
> builder** (`@workflowbuilder/sdk`) that currently linearizes the graph (branches drawn
> in the canvas are dropped with a warning). The transactional send surface [ADR 0005]
> is live on `/api`. Send-eligibility (`internal/eligibility`) is consulted in the
> broadcast, automation, and transactional paths; identity resolution + anonymous-event
> stitching run at Collect ingest.
> (Done since: events-bus P0–P2 + typed union + webhooks; legacy email/pubsub
> retired with an inline jobs adapter; Phase 2 event-based segment conditions;
> ADR 0001/0002 refactors; transactional send [ADR 0005]; visual automation builder;
> multi-user workspaces [ADR 0004]; sending domains + native DKIM [ADR 0010]; complaint/bounce
> rates [ADR 0011]; RFC 8058 one-click unsubscribe [ADR 0012]; double opt-in [ADR 0013];
> one outbound-send chokepoint [ADR 0015]; MCP surface [ADR 0016]; generated scoped client
> [ADR 0017]; tags; OAuth server; health/readiness probes, Prometheus metrics and OTel.)

> **⚠️ Authoritative model — read `GLOSSARY.md` + `docs/adr/*` first.** This phased
> roadmap predates the domain model in `GLOSSARY.md` and the accepted ADRs, which
> supersede it where they disagree. Note in particular:
>
> - **"Snapshot / static segments" is rejected**, not deferred. Per `GLOSSARY.md`
>   ("a segment is always a rule; membership is never materialized"; anti-vocabulary
>   rejects List/snapshot), there is no second segment kind. Mentions of snapshot
>   below are obsolete.
> - The two ADR refactors that were once the near-term backlog — **ADR 0002**
>   (unified contact identity) and **ADR 0001** (send-eligibility) — are now **built
>   and wired** (see the ADR row in the table above). The event-segment conditions were
>   reworked to join by resolved `contact_id`, not email.

## Remaining backlog (the foundation is built)

The core model is in place; the open work is feature breadth on top of it:

- **Automation branching / goals / per-step conditions** — the visual builder exists but
  linearizes; the schema + engine model only ordered send/wait steps. Now that the builder
  is real, this is the natural next increment (GLOSSARY.md deferred it "until the sequence
  builder is real").
- **Anonymous-Contact promotion policy** — left open by ADR 0002: a Visitor resolves to a
  Contact only via Identify/API/import; whether/when an anonymous Visitor is promoted to a
  Contact row is undecided.
- **Multi-user workspaces [ADR 0004]** — built: Membership + Role (owner / admin / member),
  invitations, members UI, owner/admin-gated site handlers. Still open: the finer-grained
  RBAC roles and SSO/SAML, which are EE (`ee/` holds only the license and README so far;
  [ADR 0014]) and hang off this foundation.
- **Phase 5 — Forms & onsite popups** — not started.
- **Phase 6 remainder** — outbound SES-compatible send endpoint,
  A/B testing, more provider adapters (Yandex/SendGrid/…). See also **send-rate control &
  IP warmup** below — a deliverability prerequisite that pairs with the (built) sending domains.
- **Phase 3 — visual MJML editor** — the body is still an MJML textarea.
- **SMS channel** — reserved on Integration/Suppression/Unsubscribe; no implementation.
- **Phase 7 — e-commerce** — far future.

### Surfaced by the Mautic pain-point analysis

Three items below are **not** feature-breadth parity — they come from the incumbent
competitor analysis (`research/mautic-pain-points-analysis.md`),
which maps Mautic's most-repeated complaints to sphericon's position. Most of Mautic's systemic
pains (cron architecture, upgrade friction, Redis/multi-master deployment, dated UI) sphericon
already neutralises by design; these three are the ones still open for us:

- **Scale validation — highest value.** sphericon's core bet (live rule segments, membership
  _never materialized_ — `GLOSSARY.md`) structurally avoids Mautic's #1 abandonment cause
  (~1M contacts → 5-minute segment editor, hanging pages, ~4 contacts/sec import), **but it is
  unproven at that scale.** Load-test the segment engine (rule → SQL compile + live preview
  count) and the broadcast send-loop audience resolution at ~1M contacts; deliverable is a
  benchmark plus any indexes/query fixes it exposes. This is the single most impactful item
  for competing against Mautic head-to-head.
- **Send-rate control & IP warmup** — configurable per-provider send rate (emails/sec) and a
  warmup ramp for new IPs/domains. Today only fixed per-queue concurrency exists
  (`internal/jobs/worker.go` — `QueueBroadcasts: {MaxWorkers: 10}`); there is no throttle.
  Lean on **river**'s native rate/concurrency limiting, not a hand-rolled throttler. Builds
  on the sending domains (ADR 0010).
- **Resource archiving** — archive (soft-hide) old Broadcasts / Automations / Segments so
  busy workspaces stay navigable. No `archived_at`/soft-delete exists today; Mautic's clutter
  complaint (no way to archive unused resources) applies to us too.
- **Independently-scalable worker tier (process roles)** — deployment topology, _distinct_
  from scale-validation above (that is query cost at 1M contacts; this is throughput
  scaling). **Horizontal scaling already works today:** river distributes job _work_ across
  all clients via `SELECT … FOR UPDATE SKIP LOCKED` (leader only does maintenance), and the
  watermill-sql event subscribers use **stable, role-named consumer groups** (`"persist"`,
  `"automations"`, `"webhooks"`, `"suppression"` — `internal/events/subscriber.go`) with an
  offset adapter, so N replicas compete per group — **no double enrollment / duplicate
  webhook / duplicate send.** Verified, and it needs no Redis (the coordination lives in
  Postgres — the same anti-Mautic simplicity). The _gap_ is that every replica is a full
  stack (HTTP + events + jobs), so the **worker tier cannot be scaled independently of the
  web tier** — a big broadcast forces more full binaries behind the LB. Fix is a **run-mode
  split**, cheap because `App.RunEvents` / `App.RunJobs` / `App.Server` are already separable
  methods (`internal/app/app.go`) and it mirrors the existing `migrate` subcommand:
  - `serve` (default, all-in-one) — HTTP + events + jobs. **Keep this the default** — the
    single-binary + single-Postgres self-host story is a core positioning advantage; splitting
    is opt-in for scale, never required.
  - `worker` — events + jobs, no HTTP; scale this deployment for send/automation throughput.
  - `web` — HTTP only, workers not started.
  - Also make river's per-queue `MaxWorkers` (hardcoded 5/10/5 in `internal/jobs/worker.go`)
    config-driven so a worker box can be sized.

  Implementation gotchas to carry into the design (not a distributed-systems project — a
  process-role flag + config):
  - A `web`-only node must build an **insert-capable** river client (`NewClient` +
    `Enqueue*`/`OnEvent`) but **not** call `Start()`. Confirm `app.New`/`register` can wire the
    jobs client without starting workers — startup is currently coupled inside `RunJobs`; that
    decoupling is the one real refactor.
  - **Deploy constraint:** at least one `serve`/`worker` node must run — river needs a started
    client for the leader/scheduler, or enqueued jobs never process (document it).
  - Per-process `MaxWorkers` × replicas is **not** a fleet-wide rate limit. A global emails/sec
    cap needs river's global rate limiting — ties back to the **send-rate control** item above.

### Account security — core, not EE

Authentication hardening is table-stakes, not an enterprise upsell, so these ship in the
AGPL core (only **SSO/SAML** stays the EE line — see the open-core split). Both extend the
current go-pkgz/auth email+password flow, which does neither natively.

- **Two-factor authentication (TOTP)** — opt-in second factor per User: enrol (QR provisioning
  URI + secret), verify at login, recovery codes, disable. Lib: `github.com/pquerna/otp`.
  A User-level concern (2FA travels with the human across every Workspace they reach via
  Membership), not Workspace-scoped.
- **Passkeys (WebAuthn)** — passwordless / phishing-resistant login as an additional
  credential on the same User. Lib: `github.com/go-webauthn/webauthn` (needs a
  `credential`-style table keyed to User; `SPHERICON_APP_URL` origin becomes the WebAuthn RP ID).
  Sequence 2FA first — it establishes the "extra User credential" schema passkeys build on.

### Self-hosting lifecycle — core, not EE

Operating a self-hosted instance over time: knowing an update exists, applying it safely, and
(optionally) telling us how the fleet uses the product. The release check and the telemetry
report are **two channels with two switches** (ADR 0032): refusing telemetry must not hide a
security release.

- **Release-update notification** (built) — the running instance checks for a newer published release
  and surfaces an admin-only in-app banner ("vX.Y is available"). This is the solid MVP
  deliverable. **Auto-update is a separate, hedged follow-up** and is deployment-specific:
  reasonable for the single **binary** install, wrong for Docker/k8s (you roll a new image —
  that's watchtower/the orchestrator's job, not ours), and risky when a release carries
  migrations. Scope the banner as the feature; treat auto-apply as binary-only, maybe.
- **Update / migration / Postgres runbook** — a documented, versioned upgrade path (doc home:
  [Operations: Upgrading](./operations/upgrading), linked from the migrations section of
  `docs/self-hosting.md`). Keep two runbooks distinct — the user lumps them, the plan must not:
  - _App upgrade_ — pull the new image/binary, run `./sphericon migrate` (or `SPHERICON_DB_AUTO_MIGRATE` on a
    single replica), roll servers. Migrations are embedded and forward-only.
  - _Postgres major upgrade_ — a DBA operation (`pg_upgrade` or dump/restore across majors),
    independent of the app release. The doc says "14+".
- **Usage telemetry (opt-out)** — decided in
  [ADR 0032](https://github.com/getsphericon/sphericon/blob/main/docs/adr/0032-instance-telemetry-report-and-release-check.md): a daily anonymous Telemetry
  report on by default, switchable off by `SPHERICON_TELEMETRY` or by the owner in the app. The
  release check is a separate channel with its own switch. Built: see
  [Telemetry and the Release check](./operations/telemetry).

---

## Phase 1 — Broadcasts MVP (detailed)

> **Historical plan.** Kept as the record of how Phase 1 was scoped; it is not maintained as a
> description of the code. Where it disagrees with `GLOSSARY.md`, the ADRs or the code, those
> win. Known drift: there is no `Contact.status` (opt-out is an `Unsubscribe` row, ADR 0001);
> `GET /e/u/{token}` renders the confirm page and only `POST` opts out (ADR 0012); bodies are
> MJML in a textarea, not `@mantine/tiptap`; river is started by `App.RunJobs`; the goverter
> converters live in `internal/api/site/resources`; Campaigns is a real navbar section.

Goal of the MVP: a user can **create a broadcast, pick an audience, write an email, send (or schedule) it,
and see a report** (sent / opened / clicked / unsubscribed). This is the first "sellable" value.

### MVP scope (in / out)

- **In:** a single email to an audience; HTML editor; merge tags (Liquid); immediate and scheduled
  sending via the queue; open/click/unsubscribe tracking; per-campaign report.
- **Audience (MVP):** "all active contacts in the workspace" + optionally a **rule segment**.
  (Historical note: this originally read "snapshot segment"; that concept was later **rejected**
  — see `GLOSSARY.md`. A segment is always a live rule; there is no static/snapshot kind.)
- **Out (later phases):** visual drag-and-drop builder, reusable templates, full Liquid feature set,
  A/B, bounce/complaint handling, dedicated domains.

### 1. Data model (ent — `ent/schema/`)

Follow the pattern in `ent/schema/segment.go` (workspace edge, `id` Int64 immutable, timestamps).

- **`Broadcast`** — `workspace_id`, `name`, `subject`, `from_name`, `from_email` (or taken from the
  default integration), `body_html`, `body_text` (auto-generated), `segment_id` (nullable → null = all
  active contacts), `integration_id` (nullable → default), `status`
  (enum: `draft`/`scheduled`/`sending`/`sent`/`failed`), `scheduled_at` (nullable), `sent_at` (nullable),
  aggregate counters (`recipients_total`, `sent_count`, `opened_count`, `clicked_count`,
  `unsubscribed_count`, `failed_count`), timestamps.
- **`BroadcastRecipient`** (per-recipient delivery log — the basis for tracking) — `broadcast_id`,
  `contact_id`, `workspace_id`, `status` (enum: `pending`/`sent`/`failed`), `opened_at`/`clicked_at`
  (nullable), `error` (nullable), `sent_at` (nullable), timestamps. Unique index on
  (`broadcast_id`, `contact_id`) for idempotency.

Register the new edges on `Workspace` (`ent/schema/workspace.go`).
**Order matters** (Atlas diffs migrations from generated `ent/`, not from `ent/schema/`):

1. edit `ent/schema/*.go` → 2) `mise run generate:backend` (regenerates `ent/`) →
2. `mise run db:generate name=add_broadcasts` → 4) `mise run db:migrate`.

### 2. API contract (TypeSpec → ogen → TS)

The frontend talks to the **site API**, so the primary contract goes there.

- New `typespec/site/resources/broadcasts.tsp` (pattern: `typespec/site/resources/segments.tsp`):
  a resource under `/w/{slug}/broadcasts`, CRUD + actions **`POST .../{id}/send`** and
  **`POST .../{id}/schedule`**, a `stats` field on the resource. Wire it into `typespec/site/main.tsp`.
- The existing `typespec/external/resources/broadcasts.tsp` — extend to the richer model later; for the
  MVP touch only the site spec.
- `mise run generate:typespec` → `mise run generate:openapi` (TS client + react-query hooks in `src/generated/site`).

### 3. Backend — handlers + converters

- `internal/api/site/broadcasts.go` — handler methods (pattern: `internal/api/site/segments.go`):
  CRUD + `Send`/`Schedule`. Scope every query by workspace.
- goverter mapping for `BroadcastResource` in `internal/api/site/sitemap/` (add methods to the
  hand-written interface next to `sitemap.go`; `converter_gen.go` is regenerated).

### 4. Send engine (river — `internal/jobs/`)

river is chosen for the actual sending (retries/concurrency); worker registration pattern is in
`internal/jobs/worker.go` (currently only `ExampleWorker`).

> **Start river first.** Verified: in `cmd/server/main.go` only watermill runs in a goroutine
> (`application.RunPubSub`); `river.Client.Start()` is NOT called, and `jobs.NewClient` only constructs
> the client. Without this, jobs enqueue and never run. The first step of this block is to wire river
> worker startup (mirroring `RunPubSub` in `app.go`: provide the client via `do.Provide`, run
> `client.Start(ctx)` in a goroutine from `main.go`, stop it in `Shutdown`).

- **`SendBroadcastJob{broadcast_id}`** — moves the broadcast to `sending`, resolves the audience (all
  active contacts or live-segment members), creates `BroadcastRecipient` rows (status=pending), and
  enqueues a `SendMessageJob` per recipient. On completion → `sent`.
- **`SendMessageJob{recipient_id}`** — renders html/text (merge tags via Liquid), rewrites links +
  injects the open pixel + unsubscribe link, resolves the sender via `internal/messaging` (the
  workspace's default enabled integration), sends, writes the result to `BroadcastRecipient`, increments
  the broadcast counters.
- Scheduled sending: `scheduled_at` → river `ScheduledAt` (native delayed enqueue).

### 5. Delivery tracking (public endpoints — `internal/server/`)

Pattern: the public tracker `internal/server/tracker.go` (serves `/t.js`, ingests events).

- A signed token encodes `recipient_id`. Note: `internal/secrets/cipher.go` is **symmetric encryption**
  (for integration configs), not HMAC. Either add a small JWT/HMAC helper (key from `cfg.EncryptionKey`)
  or encrypt `recipient_id` with the existing cipher — don't assume `internal/secrets` provides a signing
  primitive. Endpoints:
  - `GET /e/o/{token}` — 1×1 gif pixel → `opened_at`, increment `opened_count`.
  - `GET /e/c/{token}?u=<url>` — record `clicked_at`, increment, 302-redirect to the original URL.
  - `GET /e/u/{token}` — unsubscribe action → `Contact.status = unsubscribed`, increment
    `unsubscribed_count` (required for compliance — mandatory in the MVP).
- When rendering the email in `SendMessageJob`: rewrite all `<a href>` to `/e/c/...`, add the pixel and
  an unsubscribe footer.
- **Hook for Phase 4 (automations):** record opens/clicks not only as columns on `BroadcastRecipient`
  but also as **first-class rows in `Event`** (`email.opened` / `email.clicked`, with `subject_id` = the
  contact and `properties.broadcast_id`). The recipient columns remain a denormalized convenience. This
  gives automations (the "performed event" trigger) an engagement-event source from day one, so Phase 4
  needs no backfill.

### 6. Frontend (React — `src/`)

- Enable the **Campaigns** item in `src/components/AppNavbar.tsx` (currently disabled "Coming soon").
- Routes (pattern: `src/routes/segments/` and `src/routes/contacts/`): list, create/edit (composer),
  report. Register them in `src/router.tsx`.
- **Composer:** name, audience selector (all active / segment), subject, from, and an
  **HTML editor** using **`@mantine/tiptap`** (official Mantine package — no custom CSS, Mantine only).
- **Send/Schedule:** buttons calling the generated `siteBroadcastsSend/Schedule` hooks from
  `src/generated/site`.
- **Campaign report:** stat cards (recipients/sent/opened/clicked/unsubscribed) from `stats`.
- i18n strings in `locales/`, then `mise run generate:i18n-types`.

### 7. Tests (pattern: `internal/api/site/contacts_test.go`)

- Handler tests for broadcasts CRUD + auth/workspace isolation (testhelper.Setup + typed ogen client).
- Engine test: `SendBroadcastJob` creates recipients and sets statuses (with a fake sender).
- Tracking test: hitting the open/click/unsub endpoints updates the recipient/contact and counters.
- Frontend: composer-form tests (`mise run test:watch`).

### Execution order (commits, directly on `main`, Conventional Commits)

1. `feat`: ent schemas Broadcast + BroadcastRecipient + migration.
2. `feat`: TypeSpec site/broadcasts + regeneration (`mise run generate`).
3. `feat`: site handlers broadcasts (CRUD) + goverter mapping + tests.
4. `feat`: river worker startup in main.go + SendBroadcastJob/SendMessageJob + render/merge tags + sender resolve.
5. `feat`: public tracking endpoints (open/click/unsub) + link rewrite/pixel/footer.
6. `feat`: frontend — Campaigns navbar + list + composer (@mantine/tiptap) + send/schedule.
7. `feat`: frontend — campaign report (stats) + i18n.

### End-to-end verification (Phase 1)

1. `mise run setup` / `mise run dev` — bring up the stack (https://sphericon.localhost), mailpit on :8025.
2. Create an SMTP integration (point it at mailpit) in Settings.
3. Add a few active contacts.
4. Create a broadcast → audience "all active" → write an email with <code v-pre>{{ first_name }}</code> → **Send**.
5. Verify the emails arrive in **mailpit** (http://localhost:8025), merge tags are substituted, and there
   is an unsubscribe link and a pixel.
6. Open the email / click a link / hit unsubscribe → confirm the **campaign report** shows
   opened/clicked/unsubscribed increasing, and the contact becomes `unsubscribed`.
7. Scheduled send: set `scheduled_at` in the future → river sends it on time.
8. `mise run test` (backend) and `mise run check` (tsc + oxlint/oxfmt + golangci-lint) are green.

### Open questions / later

- ~~**Snapshot segments:**~~ **Resolved: rejected.** A segment is always a live rule (no
  `SegmentMember` membership table). See `GLOSSARY.md` anti-vocabulary.
- ~~**Bounce/complaint handling**~~ **Done:** SES-over-SNS ingestion → Suppression (Phase 6);
  SMTP DSN is still open.
- ~~**External API broadcasts**~~ **Done:** `typespec/external/resources/broadcasts.tsp`.
