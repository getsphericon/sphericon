# Library choices — minimize hand-rolled code

Guiding principle: where a popular, **actively maintained** Go/TS library does the job,
use it instead of hand-rolling. Maintenance status below was verified directly via the
GitHub API / npm on **2026-06-28** (push date, latest tag, archived flag, stars).
Re-verify before adopting — these numbers age.

Already in use (don't re-introduce): ent, ogen, river, watermill, samber/do, go-pkgz/auth,
golang-jwt, tink-go, go-crypt/crypt, osteele/liquid, k3a/html2text, wneessen/go-mail,
nikoksr/notify, golang.org/x/net, spf13/viper, ariga.io/atlas (the diff library in cmd/db); frontend: React 19, Mantine,
TanStack Router/Query, react-querybuilder + @react-querybuilder/mantine, @mantine/tiptap.

## Quick wins — replace current hand-rolled utilities

| Hand-rolled today                             | Adopt                     | Status (2026-06-28)        | Note                                            |
| --------------------------------------------- | ------------------------- | -------------------------- | ----------------------------------------------- |
| CORS middleware (`internal/server/server.go`) | **rs/cors**               | ✅ 2026-06, v1.11.1, 2.9k★ | drop-in `func(http.Handler)`, no router lock-in |
| requestID / recoverer / timeout middleware    | **go-chi/chi/middleware** | ✅ 2026-06, v5.3.0, 22k★   | usable without adopting the chi router          |

Keep hand-rolled (no worthwhile library): pagination math, RFC 7807 `problem+json` renderer
(every candidate lib is abandoned / sub-50★), ogen Opt→pointer converters, the messaging
provider catalog, and the `internal/segments` rule compiler (domain-specific, already
library-grade and extractable).

## Phase 3 — email templates

| Need                              | Pick                      | Status                    | Note                                                                         |
| --------------------------------- | ------------------------- | ------------------------- | ---------------------------------------------------------------------------- |
| Email body                        | **HTML** (ADR 0040)       | accepted                  | Liquid merge tags over email-ready HTML; parsed with `golang.org/x/net/html` |
| CSS inlining (required for email) | **vanng822/go-premailer** | ✅ 2026-06, v1.34.0, 202★ | de-facto Go CSS inliner                                                      |

## Phase 4 — automations / workflows

Backend engine: **do not** run Temporal (separate server/cluster — breaks minimal self-host).
Build on what we already run:

| Piece                                        | Pick                                    | Status                                                            | Role                                                                                                                                                                                                                                 |
| -------------------------------------------- | --------------------------------------- | ----------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Delays / timers / step execution             | **river** (already in repo)             | ✅                                                                | durable retries/backoff/scheduling on Postgres                                                                                                                                                                                       |
| State machine (trigger→state→action, guards) | **qmuntal/stateless**                   | ✅ 2026-02, v1.8.0, 1.4k★                                         | fresher than looplab/fsm (🟡 2025-05); we own persistence (ent)                                                                                                                                                                      |
| Event-driven transitions                     | **watermill** (already)                 | ✅                                                                |                                                                                                                                                                                                                                      |
| Frontend visual builder                      | **@workflowbuilder/sdk** (synergycodes) | ✅ npm 2.1.0 (2026-06-16), repo push 2026-06-26, Apache-2.0, 241★ | React Flow (xyflow) based, custom node types + theming; emits a workflow graph we execute with our river engine (their reference backend is Temporal — we ignore that). New peer deps: xyflow, JSON Forms (i18next already present). |

Temporal stays a roadmap escape hatch only if orchestration glue becomes the dominant
bug source (and ideally SaaS-tier only).

## Phases 5–6 — onsite, deliverability, analytics

| Need                                       | Pick                                                                | Status                                      | Note                                                                                                                                                                               |
| ------------------------------------------ | ------------------------------------------------------------------- | ------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Email syntax + MX + disposable             | **AfterShip/email-verifier**                                        | ✅ push 2026-02, v1.4.1 tag (2024), 1.6k★   | tag old but repo active (list updates)                                                                                                                                             |
| Request-body validation                    | **go-playground/validator**                                         | ✅ 2026-06, v10.30.3, 20k★                  | syntax only; pair with email-verifier for deliverability                                                                                                                           |
| SES bounce/complaint (via SNS)             | **aws/aws-lambda-go** (`events`)                                    | ✅ 2026-05, v1.54.0, 3.8k★                  | official structs for SES notification JSON                                                                                                                                         |
| Parse raw bounce / DSN / ARF emails        | **jhillyerd/enmime**                                                | ✅ 2026-06, v2.4.1, 518★                    | replaces emersion/go-message (🟡 2025-02) — fresher, purpose-built for email parsing. ⚠️ No dedicated maintained DSN/ARF parser exists; build the report-parsing on top of enmime. |
| Recurring triggers (birthdays, date-based) | **adhocore/gronx**                                                  | ✅ 2026-05, v1.20.0, 509★                   | `IsDue`/`NextTick`; firing via river. NOT robfig/cron (⚠️ 2024-07, ~2 yr quiet — duplicates river anyway)                                                                          |
| Rate limiting (external API, no Redis)     | **golang.org/x/time/rate**                                          | ✅ 2026-03, v0.15.0, official               | per-workspace token bucket; sethvargo/go-limiter (🟡 2025-11) if ready-made middleware wanted                                                                                      |
| CSV contact import/export                  | **stdlib `encoding/csv`** (+ **jszwec/csvutil** for struct mapping) | csvutil 🟡 2025-03, v1.10.0, tagged         | prefer csvutil over gocarina/gocsv (✅ pushed but **no release tags** → reproducibility risk)                                                                                      |
| Outgoing webhooks                          | **river + stdlib `crypto/hmac`**, **Standard Webhooks** format      | standard-webhooks ✅ 2026-06, v1.0.2, 1.7k★ | river owns retries; sign per the Standard Webhooks scheme so customers verify with off-the-shelf libs                                                                              |

## Skip (premature / nothing worth adopting)

- **RFC 7807 libraries** — all abandoned / sub-50★; hand-rolled renderer is correct.
- **Feature-flag engines** (Unleash, go-feature-flag, open-feature) — plan/tier gating is a
  `workspace.plan` attribute + checks in our domain layer, not a flag engine. Revisit only if
  we need gradual rollouts / A-B kill-switches.
- **A Markdown linter** (markdownlint-cli2, rumdl) — oxfmt already formats the Markdown, `check:adr`
  guards the one structural rule that matters (ADR numbers) and `check:links` resolves links and
  `#fragments`; the structure errors left (heading levels, duplicate headings) have not cost a
  review. Revisit if a docs structure rule appears in review twice.
