# sphericon Enterprise Edition (`ee/`)

This directory holds **sphericon Enterprise** features. Everything under `ee/` is
**source-available but not open source** — it is governed by [`ee/LICENSE`](./LICENSE),
_not_ the AGPL-3.0 that covers the rest of the repository. See
[`../LICENSING.md`](../LICENSING.md) for the boundary.

## What lives here

Commercial, enterprise-targeted capabilities. Planned:

- **RBAC / roles** — fine-grained, role-based access control over a workspace
  (builds on the Membership model, ADR 0004).
- **SSO / SAML / OIDC** — enterprise single sign-on.

## How it ships

Enterprise code compiles into the **same single binary** as the open core. The
features stay locked until a valid **license key** is present at runtime — so we
distribute one artifact to everyone and unlock Enterprise functionality per
subscription (the Cal.com / GitLab model). There is no separate "enterprise build".

Implemented so far: the **Audit log** (`ee/audit`, ADR 0022), unlocked by the `audit`
feature of the license key (`ee/licensekey`, set through `SPHERICON_LICENSE_KEY`), and the
advanced-retention control (`ee/retention`, `retention` feature): a Workspace's
`retention_days` window, applied to the Audit log by one periodic job.
