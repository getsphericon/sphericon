---
status: accepted
---

# Operator console and abuse detector: one EE feature on the suspension mechanism

[[0007-workspace-suspension-in-core]] left the Operator console and the automated detector as
closed EE, and [[0008-operator-separate-identity]] fixed the Operator as a separate identity. This
ADR settles how both are built. It adds no new freeze path: both only set and clear the core
`suspended` state.

## Decisions

### One licensed feature

The Operator, its console and the detector ship under a single license feature, `operator`
(`ee/licensekey`). They are meaningful only together, for a SaaS operator.

### Operator identity and surface

- Own ent table `operator`, written and read only from `ee/`. No self-signup: the first Operator
  and every later one is created by `sphericon operator create <email>`.
- Password plus mandatory TOTP (two-step login of [[0020-session-expiry-epoch-and-two-step-login]]),
  a separate cookie and JWT secret, a shorter session than a customer's with no "remember me",
  attempts limited by [[0025-rate-limiting-hybrid-store-workspace-key]]. A lost TOTP is reset
  only through the CLI.
- Login hardening reuses the ADR 0025 machinery: failed password and TOTP attempts feed one
  `auth_attempt` counter per address under its own kind, `operator_login`, so an Operator and a
  User of the same address never share a counter. The delay answers the standard 429 even for a
  correct password or code, a started session resets the counter (the password step does not),
  and the per-IP login cap covers both `/operator` login steps. `sphericon operator reset-totp
  <email>` clears the TOTP and ends the Operator's sessions; the Operator re-enrols at next
  login. No endpoint resets a second factor.
- A fourth API surface, `/operator/*`: its own TypeSpec spec, ogen server and handler package.
  The console is a lazily loaded route subtree in the same SPA; without the license the surface
  answers 404.
- One Operator role. Roles wait for a second support person.
- Operator handlers need the raw `*ent.Client`, so `ee/operator` joins the closed list in AGENTS.md.

### Console v1 shows metadata only

Workspace list and search by slug, suspension state (actor, reason, time), Complaint and Bounce
rates, send volume, the Audit log. Actions: suspend with a reason, unsuspend, set or clear the
detector exemption. No Contacts, content or Events; impersonation stays deferred (ADR 0008).

### Actor attribution

`suspended_by` becomes a structured actor: `system`, `cli`, or an Operator id. The customer
sees an Operator action as "sphericon staff" (Audit, glossary); the internal record keeps the real
id. Operator becomes a kind of actor in `ee/audit`.

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

An Operator may exempt a Workspace from the detector (one flag, audited). It stays visible in the
console with its rates and can still be suspended by hand.

### Notices and appeal

Warning and freeze emails go from the platform sender (`SMTP_*`), not the Workspace's Integration
(which is held), to every owner. The `/site` banner shows the reason and a support contact
(`SUPPORT_EMAIL`). Appeal is an email; there is no in-app form or ticketing.

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
