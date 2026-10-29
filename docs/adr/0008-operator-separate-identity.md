# Platform Staff member is a separate identity, not a User

> **Amended by [[0061-every-instance-has-a-staff-console-and-sign-up-is-always-open]]:** the Staff
> member is no longer EE or SaaS only; every Instance has one, the people who run it.

The platform needs staff who act _across_ all Workspaces (suspend an abusive sender,
impersonate for support). This collides with the model's founding invariant: every domain
entity is workspace-scoped and every query is scoped by a Workspace, reached through
`User → Membership(Role) → Workspace`. A global actor is exactly the thing that invariant
exists to forbid.

We model the **Staff member** as a **distinct identity** — its own store, its own auth surface
(a fourth API surface alongside `/site`, `/api`, `/collect`), holding **no** Membership — so
workspace-scoped code has _no path_ that can ever return a global actor. The invariant stays
enforceable by construction rather than by discipline. A person who is both staff and a
customer holds two separate identities, by design, for least-privilege and clean audit.

## Considered options

- **A global platform role/flag on User** (rejected): one login, but it bakes a
  workspace-scope exception into the core identity type — every scoped query path must then
  remember the global case exists, and forgetting it leaks cross-tenant.
- **A "platform" pseudo-workspace whose members are Staff members** (rejected): reuses Membership
  machinery but overloads Workspace with a non-tenant meaning — the same fused-concept leak
  the project rejects elsewhere (see the List anti-pattern).

## Consequences

- The Staff member, its auth surface, and the console are **EE/SaaS** (see ADR-0007); a plain
  self-hosted install has no Staff member concept and flips suspension via the core CLI.
- Impersonation (a global Staff member minting a temporary _scoped_ workspace session) is the one
  place these two worlds must touch; it is deferred until support load demands it, so that
  bridge is designed against a real need rather than speculatively.

- A platform **service** (not a person) is a second kind of machine identity on the same surface: a
  **Service credential**, also held by no Workspace and no Membership, created only from the command
  line, limited to named scopes and reached only through an internal listener. It is not a Staff member
  and logs in nowhere; see [[0026-operator-console-and-abuse-detector]].

See [[0026-operator-console-and-abuse-detector]] for the Staff member's credentials, surface and console.
