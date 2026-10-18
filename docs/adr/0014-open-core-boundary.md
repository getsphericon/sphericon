---
status: accepted
---

# Open-core boundary: gate on org-shape, not product value

The free/paid line is drawn along **who the buyer is**, not **how much product they get**.
Everything one company needs to run marketing to its own audience is AGPL core and free —
automation, `/collect` tracking, unified identity, segments, broadcasts, transactional, forms,
double opt-in, self-serve DKIM/DNS verification, per-domain deliverability metrics. What is
gated is the **organizational shape** around that product: security/governance for a company
with compliance obligations, embedding for a platform reselling us, and scale-infra for a
high-volume operator. A solo self-hoster hits **no** paywall on product value; a small company
hits one only on governance it wouldn't have without a security team.

This is a **greenfield** decision: nothing is shipped yet, so the boundary is _established here_,
not retrofitted onto existing code. The trap it avoids from the start is treating "enterprise
features" as a single bucket. Monetization runs on **three distinct mechanisms** that must not be
conflated — conflating them is what produces a boundary that feels weak and arbitrary later.

## Three mechanisms, kept separate

- **(a) License-gated features** — code in `ee/`, unlocked by the runtime EE license key (the
  offline key mechanism, distinct from the hosted offering). Only
  org-shape features belong here.
- **(b) Managed service** — the planned SaaS: managed hosting and deliverability-as-a-service
  (shared/warm sending domains, pooled/dedicated IP). Not a feature; a separate revenue line,
  run outside this repository and reaching the core only through the Staff surface
  (ADR 0009). Never a `license key` gate.
- **(c) Professional services** — done-for-you migration, deliverability setup, compliance
  documentation, support/SLA. Sold by people, bundled with enterprise or cloud contracts.
  **Never code-gated.**

Migration has two halves that split across mechanisms. The **import tooling** — importers from
Mautic, Mailchimp, Brevo, Keila, and CSV — is **product code and belongs in the free core**: low
switching cost is a direct adoption lever ("came from Mautic, imported in ten minutes"), and
hiding importers behind EE would throttle the exact inflow the open core exists to capture. The
**done-for-you migration** — hand-moving a complex instance, mapping custom fields, porting
automations — is a professional service (c).

Support and compliance docs are mechanism (c), not features — they cannot "live in `ee/`".
Managed hosting is (b). Only (a) is compiled behind the license key. Collapsing all three into
"enterprise features" is what produced an under-specified boundary.

## What is free (AGPL core)

The entire product a single company uses to send to its own audience:

- Event-driven **Automation** (Trigger / Enrollment / Step), the whole thesis of the product.
- On-site behavioral tracking (`/collect`), unified **Contact + Visitor + Identify** identity,
  the CDP spine.
- **Segments** (rule builder over fields, custom fields, and Events), **Broadcasts**,
  **Transactional**, **Forms**, double opt-in (ADR 0013).
- Self-serve sending-domain setup with live DKIM/SPF/DMARC checks (ADR 0010) and per-domain
  deliverability-rate metrics (ADR 0011).
- **Import tooling** — importers from Mautic, Mailchimp, Brevo, Keila, and CSV. Low switching
  cost is an adoption lever; the code is free. (Done-for-you migration is a service, mechanism c.)
- Basic auth and the workspace/membership model (ADR 0004).

Gating any of the above — or capping contacts, workspaces, API, or webhooks in self-host — is
forbidden (see Consequences). Product value is the adoption engine and the open-source promise;
it is not the paywall.

## What is EE — mechanism (a) only

Org-shape features that a small self-hoster does not need, so gating them does not dent adoption:

- **Security/governance**: SSO/SAML/OIDC, SCIM provisioning, RBAC / granular roles / multiple
  teams (extends the membership model, ADR 0004), audit logs.
- **Embedding**: white-label, multi-tenant, and embeddable components — the platform-reseller
  surface (agencies, SaaS embedding marketing automation). This is the strongest and most
  defensible lane for a CDP+automation product, and it is in EE by design from the start.
- **Scale-infra** (gated by _form_, not product value): dedicated-IP / warmup automation and
  data-residency / advanced-retention controls. These matter only to a high-volume or regulated
  operator; the send path itself stays free.

Because this is greenfield, the split is designed in from day one: `ee/` exists as a separate
directory from the first commit, and no feature is ever born free-then-moved. The list above is
where new EE features are _created_, not a migration target for core code.

## Considered options

- **Feature-gate product value** (rejected): the weakest, riskiest lever. Capping audience size,
  automations, tracking, or API in self-host kills adoption and invites a fork of an AGPL
  codebase. Product value stays free; we monetize org-shape, hosting, and services instead.
- **Put migration / support / compliance docs in `ee/`** (rejected): done-for-you migration,
  support, and compliance docs are professional services (c), not code — bundling them into a
  license key is a category error and gates nothing enforceable. (Import _tooling_ is the
  separate, free-core half of migration; see above.)
- **Gate the import tooling behind EE to monetize switchers** (rejected): the highest-value
  inflow is people leaving Mautic/Mailchimp; taxing the moment they arrive throttles adoption for
  a one-time fee. Importers are free; only the hands-on migration of a complex instance is paid.
- **Scope EE to only SSO + RBAC** (the default assumption; rejected as too thin): captures the
  governance slice but omits the white-label/embedding lane — the primary B2B2C revenue lever
  for a CDP+automation tool (cf. Dittofeed's self-host enterprise, packaged around multi-tenant
  auth, white-labeling, and embedded components). Including embedding in EE from the start is the
  deliberate choice here.
- **Unify the EE license key and the hosted offering into one "entitlement"** (rejected,
  consistent with ADR 0009): an offline runtime key and an external contract have different lifecycles
  and failure modes.
- **Gate the CDP core (advanced segmentation, identity resolution) behind EE** (rejected): that
  _is_ the product thesis; gating it contradicts "product value is free" and removes the reason to
  adopt at all. Only infra-shaped scale controls around the core are gateable.

## Consequences

- **No-retro-relicensing rule**: a feature shipped free in the AGPL core is never moved into
  `ee/`. New EE features may be born in `ee/`; existing free ones stay free. This is the standing
  guard against an open-core "rug pull" (the BSL/SSPL backlash pattern) and is what makes the
  AGPL promise credible.
- **No self-host caps**: contacts, workspaces, API, and webhooks are uncapped in self-host.
  Volume/seat limits are a _cloud pricing_ instrument (mechanism b), never a self-host license
  gate.
- EE is scoped from day one beyond the default SSO/RBAC assumption: **white-label / multi-tenant
  / embedding** is a first-class EE surface, not a later add-on.
- The three mechanisms stay architecturally separate: `ee/` behind the license key (a); the hosted
  offering run outside this repository (b, ADR 0009); services as contracts (c). No single "entitlement" abstraction
  unifies them.
- This ADR sets the _policy_; it does not enumerate every future EE feature. New candidates are
  tested against one question: **does a single company need this to send to its own audience?**
  If yes → core. If it only matters at org-scale, for embedding, or for regulated/high-volume
  operation → EE.
- **Login security is core.** A TOTP Second factor with Recovery codes, the Workspace-level
  Two-factor requirement, and the Login throttle (ADR 0025, rate limiting; ADR 0020) pass the test above: a single company
  needs them to protect its own sign-in. Only org-scale governance (SSO/SAML, an audit-log UI)
  stays EE; core records second-factor changes as domain events.
