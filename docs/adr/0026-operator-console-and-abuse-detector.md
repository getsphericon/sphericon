---
status: accepted
---

# Staff console and abuse detector: one EE feature on the suspension mechanism

[[0007-workspace-suspension-in-core]] left the Staff console and the automated detector as
closed EE, and [[0008-operator-separate-identity]] fixed the Staff member as a separate identity. This
ADR settles how both are built. It adds no new freeze path: both only set and clear the core
`suspended` state.

## Decisions

### One licensed feature

The Staff member, its console and the detector ship under a single license feature, `staff`
(`ee/licensekey`). They are meaningful only together, for the SaaS.

### Staff member identity and surface

- Own ent table `staff`, written and read only from `ee/`. No self-signup: the first Staff member
  and every later one is created by `sphericon staff create <email>`.
- Password plus mandatory TOTP (two-step login of [[0020-session-expiry-epoch-and-two-step-login]]),
  a separate cookie and JWT secret, a shorter session than a customer's with no "remember me",
  attempts limited by [[0025-rate-limiting-hybrid-store-workspace-key]]. A lost TOTP is reset
  only through the CLI.
- Login hardening reuses the ADR 0025 machinery: failed password and TOTP attempts feed one
  `auth_attempt` counter per address under its own kind, `staff_login`, so a Staff member and a
  User of the same address never share a counter. The delay answers the standard 429 even for a
  correct password or code, a started session resets the counter (the password step does not),
  and the per-IP login cap covers both `/staff` login steps. `sphericon staff reset-totp
  <email>` clears the TOTP and ends the Staff member's sessions; the Staff member re-enrols at next
  login. No endpoint resets a second factor.
- A fourth API surface, `/staff/*`: its own TypeSpec spec, ogen server and handler package.
  The console is a lazily loaded route subtree in the same SPA; without the license the surface
  answers 404.
- One Staff member role. Roles wait for a second support person.
- Staff member handlers need the raw `*ent.Client`, so `ee/staff` joins the closed list in AGENTS.md.

### Service credential: the machine identity of the Staff surface

A service that is not a person calls the Staff surface with a **Service credential**
([[0009-usage-metering-and-operator-holds]]). It sits next to the human Staff member, not inside it: no
password, no cookie, no TOTP, no console access.

- Own ent table `service_credential`, written and read only from `ee/staff`, with no Workspace and
  no Membership (like the Staff member). It is a bearer value `smsc_<prefix>_<secret>`; only the SHA-256 of
  the secret is stored (the secret is 40 random characters and is checked on every request, so a slow
  password hash would add cost and no strength).
- Created, rotated and revoked **only by the command line** (`sphericon service-credential
  create|rotate|revoke|list`). No endpoint can mint or change a credential. The value is shown once.
- Limited to named scopes: `usage:read`, `workspaces:read`, `holds:write`. Every service operation
  names the scopes it needs in one map, and an operation without an entry is refused to every
  credential, so a new endpoint cannot ship unguarded.
- A service may hold two active credentials at once (the name is not unique), so a rotation has no
  downtime: `rotate` issues a second one, `revoke` ends the old one. Each credential has an optional
  expiry and a last-used time, which `list` shows. A revoked or expired credential is refused.
- **Served only on an internal listener**, in the manner of the metrics listener
  ([[0018-metrics-opt-in-internal-listener]]): the service endpoints are a separate TypeSpec
  surface and ogen server (`/service`), bound to `SPHERICON_INTERNAL_ADDR`, empty by default and then no
  listener and no route anywhere; an address equal to the public `PORT` (or `SPHERICON_METRICS_ADDR`) is a
  configuration error and a bind failure is fatal. The public port answers 404 for `/service`. The
  network boundary is the first control and the credential the second.
- Rate limited per credential (`SPHERICON_RATE_LIMIT_SERVICE_PER_MINUTE`) and per client address on failed
  authentication, by the machinery of [[0025-rate-limiting-hybrid-store-workspace-key]].
- **Audit actor.** A write made under a credential is recorded with actor kind `service`, the
  credential's id and its name, in the manner of an API token. Like a Staff member it is staff of the
  platform: every customer-facing read shows it as "sphericon staff" with no id
  ([[0022-audit-log-ee-captured-at-scoped-client]]), and only the Staff console names it.
- All of it runs under the same `staff` license feature; unlicensed, nothing is served and the
  commands refuse.

### Console v1 shows metadata only

Workspace list and search by slug, suspension state (actor, reason, time), Complaint and Bounce
rates, send volume, the Audit log. Actions: suspend with a reason, unsuspend, set or clear the
detector exemption. No Contacts, content or Events; impersonation stays deferred (ADR 0008).

### Actor attribution

`suspended_by` becomes a structured actor: `system`, `cli`, or a Staff member id. The customer
sees a Staff member action as "sphericon staff" (Audit, glossary); the internal record keeps the real
id. Staff member becomes a kind of actor in `ee/audit`.

### Detector

An hourly river job registered through `Edition.Jobs()`, per (Workspace, Sending domain), over a
trailing 24-hour window and a minimum of 1000 sent in it (below that the rate is undefined,
[[0011-deliverability-rate-metrics]]). Two tiers, modelled on the review/pause split of Amazon SES:

| Tier   | Complaint rate | Bounce rate | Effect                                 |
| ------ | -------------- | ----------- | -------------------------------------- |
| Warn   | ≥ 0.1%         | ≥ 5%        | notify owners only, nothing frozen     |
| Freeze | ≥ 0.3%         | ≥ 10%       | suspend the whole Workspace (`system`) |

Any breach on any domain freezes the Workspace, because the hold is source-level (ADR 0007).
Limits and the volume floor are EE configuration with these defaults (env, one row each in
`docs/self-hosting.md`).

The warning is a computed status, not a stored state; the detector stores only when it last
notified, so each episode sends one email per tier, not one per hour.

### No loop after unsuspend

Unsuspend records `cleared_at`; the detector counts only events after it, and will not freeze the
Workspace again until 1000 new sends have accumulated. A detector never lifts a suspension.
Same idea as SES recounting only after the sender's fixes.

### Exemption

A Staff member may exempt a Workspace from the detector (one flag, audited). It stays visible in the
console with its rates and can still be suspended by hand.

### Notices and appeal

Warning and freeze emails go from the platform sender (`SMTP_*`), not the Workspace's Integration
(which is held), to every owner. The `/site` banner shows the reason and a support contact
(`SPHERICON_SUPPORT_EMAIL`). Appeal is an email; there is no in-app form or ticketing.

## Considered options

- **Detector writes a stored `warning` state** (rejected): a second Workspace state beside
  `suspended` for what is a function of the rates.
- **Detector auto-unsuspends once rates recover** (rejected): the unfreeze decision stays human,
  or one flapping sender oscillates and torches the shared reputation.
- **Separate license features for console and detector** (rejected): no customer wants one
  without the other.
- **Separate console host and app** (rejected for v1): infrastructure with no isolation gain
  that the separate cookie and license gate do not already give.

## Consequences

- Core keeps the mechanism and the CLI; a self-hoster is unaffected.
- The Audit schema and `suspended_by` change; `cleared_at`, the exemption flag and the last-notified
  marker are new Workspace fields (migration).
- Console UI code ships in the open bundle but is inert without the license; the closed part is the
  `ee/` backend.
