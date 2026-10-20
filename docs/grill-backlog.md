# Grill backlog

A prioritized queue of **design decisions worth grilling** — each clears the bar of
_hard-to-reverse + surprising + real trade-off_, and each output is a glossary term
(`GLOSSARY.md`) and/or an ADR (`docs/adr/`). This is the working queue for
`/grill-with-docs` sessions; it is not itself authoritative (GLOSSARY.md + ADRs are).

Legend: 🔴 launch-blocker / "can't send otherwise" · 🟡 parity with drip.com · ⚪ later.

## Done (grilled)

- ✅ **Platform abuse control** — Workspace suspension (core mechanism) + Staff member identity.
  → ADR 0007, ADR 0008; CONTEXT: _Staff_, _Workspace suspension_.
- ✅ **Sending domains** — native DKIM, verified-domain-required-to-send.
  → ADR 0010; CONTEXT: _Sending domain_.
- ✅ **Complaint / bounce-rate metrics** — core/user-facing rate (thresholds are EE), complaint
  rate over `(sent − hard bounces)`, per-(workspace, Sending domain) flow rate, live-query now /
  rollup later. Blocked on Sending domains. → ADR 0011; CONTEXT: _Complaint rate_, _Bounce rate_.
- ✅ **Bulk-sender compliance** (Gmail/Yahoo 2024+) — RFC 8058 one-click (`List-Unsubscribe` +
  `List-Unsubscribe-Post`, HTTPS-only) on Broadcast/Automation via the footer's source-scoped
  token; endpoint split GET=confirm / POST=perform (fixes scanner auto-unsub); DMARC = warn-not-
  block bulk-readiness, send-gate stays DKIM-only. Complaint-rate pillar was ADR 0011. Rides on
  the ADR 0010 send-path rework. → ADR 0012; CONTEXT: _Unsubscribe_, _Sending domain_.
- ✅ **Double opt-in / confirmed subscription** — confirmed opt-in modeled as a positive
  destination-keyed Confirmation row + immutable `marketing.confirmed` Event (proof + signal),
  the mirror of Unsubscribe — no subscribed flag. Workspace-level policy (default off),
  forward-looking grandfather-backfill; import provenance (`double_opt_in`/`grandfathered`/
  `imported`); confirmation email respects Suppression + `everything`; GET=page/POST=confirm.
  → ADR 0013; CONTEXT: _Confirmation_, _Send-eligibility_.
- ✅ **`/metrics` exposure** — no longer on the public mux: opt-in internal listener on
  `SPHERICON_METRICS_ADDR` (off by default, no auth, network boundary is the control); metric labels
  limited to bounded technical dimensions, never tenant/personal identifiers. → ADR 0018.

## Queue

### 🟡 AI/MCP plane — agent-operable platform (no built-in chat)

The strategic AI-first bet: Sphericon is **operated by an agent through an MCP tool-surface**,
not through a built-in chatbox. The clean glossary (already a de-facto system prompt), the
`Event` spine, and **derived** Send-eligibility are what make the domain safely
agent-operable — the AI is a _client_ of the core invariants, never a bypass. One surface is
the substrate for two consumers: an external "bring-your-own-agent" (Claude Desktop, the
customer's own agent) and, later, in-app **inline intents** (NL→Segment in the builder,
"draft" in the composer, "explain" on analytics) — each producing a domain object in its
native editor under the existing `draft`/`inactive` approval gate. Open forks:

- **Surface generation** — grill _first_, it shapes the rest: MCP tools **derived from the
  contract** (a 4th TypeSpec spec `mcp`) vs. a projection over the existing `external` API.
  Hand-maintaining a parallel tool list that drifts from the contract is the rejected path
  (codegen ethos: generated over hand-rolled).
- **Tool granularity** — intent-level domain verbs (`create_segment_from_description`,
  `draft_broadcast`, `explain_broadcast_performance`) vs. a 1:1 CRUD mirror of REST
  endpoints. Few deep tools aligned to the glossary; a CRUD mirror drowns the agent.
- **AI is an author by default, a sender only by opt-in (governing principle).** The AI plane's
  output is an _artifact_ — a Segment, a `draft` Broadcast, an `inactive` Automation, a
  Template, a Recommendation. `send`/`activate` is **not** an AI capability unless the human
  grants it per token: the default token gets `read` + `write`-drafts only, and the send-class
  scopes (`emails:send`, `broadcasts:send`, `automations:activate`) additionally need `mcp:send`
  to be reachable over MCP at all (ADR 0016, "Send is a second lock"). Pulling the trigger is
  otherwise a human action. The `draft`/`inactive` states are the _boundary of what the AI can touch_, not just a review
  gate. The core choke points (Send-eligibility, Suppression, verified domain,
  Workspace/Staff member freeze) still guard the human's eventual send.
- **AI-authorship attribution** — reuse the actor-attribution pattern (actor `system` /
  Staff member): entities an agent creates carry "who authored — AI vs User".
- **AI as a metered unit** — token/AI-call usage extends the Usage snapshot (ADR 0009)
  — a new metric, not a new mechanism.
- **Skills (operating playbooks) as a distinct layer** — tools are _capability_
  (`create_segment`), skills are _competence_ ("how to run a win-back well"): a skill is
  domain know-how + the sequence of tool calls. Delivered over the same MCP connection via
  the protocol-native **prompts** primitive (server-provided, user-invoked), so a BYO-agent
  gets them with no file distribution. Two authorship tiers: **sphericon-shipped** best-practice
  playbooks (welcome / cart-abandonment / win-back / list-hygiene — assets, encode correct
  use of _this_ model) vs. **workspace-authored** brand/voice playbooks (stateful,
  workspace-scoped, versioned → a candidate glossary term `Skill`, distinct from `Template`
  (email content) and `Automation` (runtime sequence)). Skills partly _resolve_ the
  tool-granularity fork — orchestration knowledge in the skill lets tools stay primitive
  without drowning the agent, so **grill skills together with tool granularity**. Liveness
  tension: a skill references tool names generated from the contract → needs a
  validity-against-current-surface check (same principle as goverter's completeness check).
  Ties to: External `/api` breadth, Usage metering, Audit log. **Rejected here:** a built-in
  **chat** as a primary UI — AI-first ≠ chat-first; a conversation is a poor home for outputs
  that are governed domain objects. Chat is _deferred and narrow_ — at most a later,
  optional **analytical panel** for open-ended "why/what-if" Q&A, never the main surface.

### 🔴 Prompt-injection from contact-supplied data

Split out from the AI/MCP plane because it touches the send path and the data model, not just
the tool layer. `Custom field`s and `Event` properties are **declared-by-use / auto-created
from untrusted tracker & API input** — so contact-supplied strings (e.g. an instruction
stuffed in `first_name`) flow into any prompt or into a BYO-agent's context the moment a
`read` tool returns them. Because the AI **only forms artifacts, never sends** (see AI/MCP
plane), a successful injection can't directly fire a send — but it can (a) corrupt an
artifact a human then approves and sends, or (b) exfiltrate contact data into a BYO-agent's
context. The human send-review is the backstop, not the primary defense. Open forks: where
untrusted-data boundaries are drawn (mark contact-origin fields as tainted?), whether an
AI-formed artifact surfaces its untrusted-data provenance at review time, and how tainting
flows through the `read` tools that feed a BYO-agent.

### ⚪ Proactive marketing intelligence (the "one person runs everything" bet)

The largest AI-first ambition: a system that **watches, recommends, and — under policy —
forms artifacts** — analyzing what's happening and drafting segments / automations /
broadcasts on the fly, so one marketer does the work of a marketing team. A shift from
_reactive_ (agent does what asked) to _proactive_ (agent forms hypotheses and drafts).
**The AI never sends** — its entire output is authoring-side artifacts (see AI/MCP plane:
"AI is an author, not a sender"); pulling the send/activate trigger stays a human action.
Deliberately **not** "no marketers": the human stays the accountable sender/approver; the
product is an AI copilot that proposes and _drafts_ **under a bounded mandate**, earning more
autonomy-to-draft as its accepted recommendations demonstrably move a metric. **Composes what
is already decided** — sensor = `Event` spine; hands = MCP tools (create drafts only);
competence = Skills; approval-and-send gate = `draft`/`inactive`. Runtime = a background
agentic loop on the domain-event bus (watermill) + river periodics, **triggered by
thresholds, not always-on** (runaway/token-cost is real → a new metered line). Open forks:

- **`Recommendation` as a domain object** — proposed change = rationale (grounded in
  Events/metrics) + proposed diff (which entities, in `draft`) + status (pending / accepted /
  dismissed / auto-applied) + AI-attribution. Anti-vocabulary: _not_ a notification, _not_ a
  task.
- **`Autonomy policy` / budget** — how "does it if the user agrees" is modeled, and the "it"
  is always _forming an artifact_, never sending: **per-action** consent (approve each draft)
  vs. **standing** consent bounded by a budget ("auto-draft win-back for cold segments, ≤ N
  drafts/week"). Bounds how many artifacts the AI may create unattended, not any send.
- **The outcome loop** — attributing whether a Recommendation (once a human sent it) actually
  improved the metric, back onto the Recommendation (reuses the Broadcast-recipient
  rollup/attribution pattern). **Without this it is a suggestion-spewer, not a marketer** —
  the make-or-break, and the hardest part.
- **Autonomy gradient** — over _artifact formation only_ (there is no auto-send tier): L0
  analyze/explain (read-only) · L1 propose drafts, per-action approve · L2 auto-form drafts
  within a policy/budget. The human always sends. Higher autonomy = more drafts formed
  unattended, never more sending.
  Depends on: ✅ Complaint/bounce-rate metrics (ADR 0011 — the analyzer reads the same rate the
  ADR 0007 auto-suspension detector does). Ties to: AI/MCP plane,
  Usage metering, Lead scoring, Audit log.

### 🟡 Automation action steps (beyond send/wait)

Step is only `send` | `wait` today. A whole class is unnamed: update custom field,
add/remove segment membership (tag), call a Webhook endpoint, notify team, enroll into
another Automation. Open forks: which actions are first-class Step kinds vs. deferred; how
"tagging" fits when the model rejects Lists; idempotency/side-effects on re-run.
Grill _before or with_ automation branching (they reshape the same Step/Enrollment model).

### 🟡 Automation branching / goals / per-step conditions

GLOSSARY.md deferred this "until the sequence builder is real" — the builder now is. Attacks
the stated invariant _"Enrollment points at exactly one current Step… no branching"_. Open
forks: branch/condition model, goals & exit conditions, whether the single-current-step
pointer survives, re-enrollment (currently out of scope).

### 🟡 Time/date-based & recurring triggers

Trigger is a single Event action today. Missing: date-based (birthday/anniversary),
scheduled/recurring (RSS-style newsletter), wait-until-time-of-day / send-time. Open forks:
is a time trigger a synthetic Event or a new Trigger kind; how it reconciles with enroll-once.

### 🟡 GDPR data tooling

Contact data export (portability) + right-to-erasure workflow + consent audit. The model
says the Unsubscribe survives contact deletion; the erasure/export _mechanism_ is undefined.

### 🟡 Contact import/export + dedup/merge

Bulk CSV import with field mapping and duplicate resolution; manual merge of duplicate
Contacts. Open fork: how merge interacts with the multi-key identity spine (subject_id /
email / phone) and Event re-attachment.

### 🟡 Lead / contact scoring

Marketing-automation staple, absent. Open forks: is a score a Custom field, a derived
value, or its own concept; what mutates it (Events? automation actions?).

### 🟡 Audit log (workspace-scoped)

Who changed what — team feature + compliance. Open forks: what's audited, retention,
whether Staff member actions (cross-workspace) log here or separately.

### 🟡 External `/api` breadth + rate limiting

The external surface is thin (broadcasts stub). Integrations need CRUD for
contacts/events/segments via `/api`, plus per-token rate limiting. Open forks: API shape
parity vs. site API, rate-limit unit (per token? per workspace?).

### 🟡 Deliverability observability

Google Postmaster Tools / feedback loops (FBL) ingestion; pre-send spam-score/render check
(mail-tester-like). Open forks: how FBL complaints reconcile with the SES-over-SNS ingest
path already feeding Suppression.

### ⚪ Later / lower-urgency

- Send-time optimization / timezone-aware sending.
- Dedicated IP pools + the warmup ramp (roadmap has send-rate control; pool model is open).
- Anonymous-Contact promotion policy — ADR 0002 left open (when/if a Visitor becomes a
  Contact row).
- RBAC roles beyond owner/admin/member (EE), SSO/SAML (EE).
- Impersonation (deferred in ADR 0008 until support load appears).
- Visual email editor (ADR 0040; roadmap Phase 3 remainder).
- Native CRM / Zapier / e-commerce connectors (roadmap Phase 7).
- Workspace configuration as code (idea from openstatus). Deliberately no remote CLI, `plan`/`apply`
  or Terraform provider now: the audience is marketers, and agents already operate the platform
  through MCP (ADR 0016). Revisit on a concrete ask (a Self-hoster wanting configuration in git, or
  moving a Workspace between environments). First step then: export/import of Tags, Custom fields,
  Segments, Templates and Automations as one file through the API and the UI, not a new binary.

## Candidate dependencies

Ready, popular, actively-maintained solutions per item, so the dependency surface is known
up front. **Confirm liveness at `go get` / `pnpm add` time** (roadmap principle). ⚠️ flags a
tension to resolve _before_ adopting: the **Postgres-only, single-binary self-host** constraint
(needs a Postgres-native path, not Redis/an extra service).

| Item                                                     | Candidate                                                                                                 | Kind    | Status / note                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| -------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- | ------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Sending domains — DKIM sign/verify, DMARC                | **emersion/go-msgauth** (`dkim`, `dmarc`, `authres`)                                                      | Go lib  | MIT, v1, active (Apr 2025). The pick for ADR 0010 signing.                                                                                                                                                                                                                                                                                                                                                                                                      |
| " — SPF check                                            | **blitiri.com.ar/go/spf** (albertito/spf)                                                                 | Go lib  | Standard Go SPF; slower-moving (last big update ~2022) but stable. SPF is advisory-only per ADR 0010.                                                                                                                                                                                                                                                                                                                                                           |
| " — DNS record lookups (verify TXT/CNAME)                | **miekg/dns**                                                                                             | Go lib  | De-facto Go DNS lib, very active.                                                                                                                                                                                                                                                                                                                                                                                                                               |
| Bulk-sender: one-click List-Unsubscribe (RFC 8058)       | _no lib_ — set `List-Unsubscribe(-Post)` headers in the raw MIME + reuse the existing **JWT** unsub token | build   | ⚠️ Rides on the ADR 0010 send-path rework: today's `nikoksr/notify` stack can set **no** custom headers and go-mail is **not** yet a dep. The header + endpoint design (ADR 0012) is settled; it lands when the raw-MIME/DKIM send stack does.                                                                                                                                                                                                                  |
| Complaint/bounce-rate metrics                            | _no lib_ — aggregate from `Event` rows in **Postgres**                                                    | build   | Source of truth already exists (`email.bounced`/complaint Events); no ClickHouse needed.                                                                                                                                                                                                                                                                                                                                                                        |
| Double opt-in                                            | _no lib_ — confirmation email + signed token (JWT, with `exp`), existing send path                        | build   | ✅ Grilled (ADR 0013): positive Confirmation row + `marketing.confirmed` event, no subscribed flag. Ties to Contact import (provenance dedup) + GDPR tooling (purge unconfirmed).                                                                                                                                                                                                                                                                               |
| AI/MCP surface — server                                  | **modelcontextprotocol/go-sdk** (official)                                                                | Go SDK  | Chosen. Official SDK, semver-stable 1.x (v1.6.x, Anthropic + Google), streamable-HTTP transport, JSON schema generated from Go structs via `jsonschema:` tags (fits the codegen ethos). Preferred over the community **mark3labs/mcp-go** (still 0.x, concentrated maintenance) for a long-lived open-core repo. Mount as a 4th surface next to the ogen servers; auth rides the existing scoped API-token model. Grill generated-vs-projected before adopting. |
| AI/MCP — LLM provider (for inline intents / later panel) | **anthropics/anthropic-sdk-go**                                                                           | Go SDK  | Provider-agnostic behind an `Integration`-style abstraction; default to Claude (Opus/Sonnet 4.x). Not needed for the BYO-agent path (client hosts the model).                                                                                                                                                                                                                                                                                                   |
| Prompt-injection defenses                                | _no lib_ — taint contact-origin fields + extra send-gate                                                  | build   | Mechanism is custom; the boundary + gate are the grill.                                                                                                                                                                                                                                                                                                                                                                                                         |
| Automation action steps / branching                      | **@xyflow/react** (already used) + **river** (already used) for execution                                 | in-repo | No workflow-engine dep needed; river drives steps.                                                                                                                                                                                                                                                                                                                                                                                                              |
| Time/date & recurring triggers                           | **river** periodic jobs / **robfig/cron**                                                                 | Go lib  | river (already in repo) has periodic jobs — prefer it over a new cron dep.                                                                                                                                                                                                                                                                                                                                                                                      |
| GDPR export / erasure                                    | _no lib_ — custom, stdlib serialization                                                                   | build   | Mechanism is custom; policy is the grill.                                                                                                                                                                                                                                                                                                                                                                                                                       |
| Contact CSV import                                       | **gocarina/gocsv** (or stdlib `encoding/csv`); FE parse **papaparse**                                     | Go + JS | Import mapping + dedup logic is custom.                                                                                                                                                                                                                                                                                                                                                                                                                         |
| Lead scoring                                             | _no lib_ — reuse the `internal/segments` rule engine                                                      | in-repo | Scoring rules ≈ segment predicates; build on what exists.                                                                                                                                                                                                                                                                                                                                                                                                       |
| Audit log                                                | _no lib_ — append-only Postgres table (optionally off the domain-event bus)                               | build   | No dominant Go lib; DB-level `pgaudit` is a different (ops) concern.                                                                                                                                                                                                                                                                                                                                                                                            |
| External `/api` rate limiting                            | **sethvargo/go-limiter**                                                                                  | Go lib  | ⚠️ Memory store = single-node; shared store needs Redis, which Sphericon avoids. Either a Postgres-backed limiter store or per-node limits + document it. Grill the unit (per token/workspace).                                                                                                                                                                                                                                                                 |
| Deliverability: Postmaster/FBL                           | **google.golang.org/api** (Postmaster Tools) + **jhillyerd/enmime** (parse ARF `message/feedback-report`) | Go lib  | FBL complaints reconcile into the existing SES-over-SNS → Suppression path.                                                                                                                                                                                                                                                                                                                                                                                     |
| Pre-send spam score / render                             | **rspamd** (self-run service) or mail-tester (SaaS)                                                       | service | Optional; heavier.                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| Send-time / timezone-aware                               | _no lib_ — stdlib `time` + IANA tz                                                                        | build   | —                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| IP warmup / send-rate                                    | **river** native rate/concurrency limiting                                                                | in-repo | Per roadmap; no hand-rolled throttler.                                                                                                                                                                                                                                                                                                                                                                                                                          |
| 2FA (TOTP) / passkeys                                    | **pquerna/otp** / **go-webauthn/webauthn**                                                                | Go lib  | Already named in the roadmap.                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| Visual email editor                                      | **@react-email/editor** (ADR 0040)                                                                        | JS      | Replaces the earlier easy-email-editor pick; the editor's document is the source and the server builds the HTML.                                                                                                                                                                                                                                                                                                                                                |
