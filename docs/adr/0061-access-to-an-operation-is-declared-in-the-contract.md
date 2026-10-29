---
status: accepted
---

# Access to an operation is declared in the contract

Who may call an operation was decided in each handler: 102 `auth.HasScope` checks on the external
API and 34 `accounts.Can…` checks on `/site`. They had drifted (a missing scope answered 401 at 99
sites and 403 at 3), a new operation was open until someone remembered its check, and the frontend
kept its own copy of the Role table (`useCurrentRole.ts`). The comparison with six Go projects and
cal.com ([handler-and-module-organization-prior-art](../research/handler-and-module-organization-prior-art.md))
showed the same drift wherever checks are opt-in, and the `/service` surface already showed the
cure: its scopes are declared on each operation and enforced in one place
([[0026-operator-console-and-abuse-detector]]). Access is now declared in TypeSpec for every
surface, and refused unless declared.

## Decisions

- **Every operation declares what it needs.** An external operation declares the API token scopes
  it requires (`x-required-scopes`, as on `/service`), and `ExternalSecurityHandler` checks them for
  the operation it is given. A `/site` operation with a `workspaceId` in its path declares the
  Permissions it requires; an empty list means any member. A `/site` operation without a
  `workspaceId` must not declare any. A missing or misplaced declaration fails generation, and an
  operation absent from the generated table is refused to everyone.
- **A Role is a named set of Permissions** (the Gate). The table from Role to Permissions is one
  piece of Go in `accounts`, read through `accounts.Allows(role, permission)`; the twelve
  `Can…` functions are gone. The Permission vocabulary is an enum in TypeSpec, written
  `resource:verb` like the scopes.
- **A rule about one record is a Policy in its module.** "Only an owner changes an owner" and "a
  Workspace keeps an owner" are refused by `accounts` inside the operation, and so is any future
  rule that reads the record (an author editing only their own draft). No handler calls a Policy, so
  every surface gets it.
- **One middleware does the `/site` work of `membershipFor`.** An ogen middleware reads the
  operation and its `workspaceId`, loads the caller's Membership through `accounts.Accounts.Scope`,
  applies the Two-factor requirement ([[0020-session-expiry-epoch-and-two-step-login]]), checks the
  declared Permissions and puts the `*ent.Scoped` (with its actor,
  [[0022-audit-log-ee-captured-at-scoped-client]]) in the context. The one operation that must work
  while the requirement withholds the caller (switching the requirement off) says so in the contract.
- **Refusals answer one way.** 401: no credential, or one that is invalid, revoked or expired. 403:
  a valid credential that lacks a scope, or a Membership that lacks a Permission (`code:
  permission_required`). 404: the Workspace is unknown or the caller is not a member of it, so its
  existence does not leak.
- **The client reads Permissions, it does not compute them.** The Workspace resource of the current
  User carries its `permissions`; the UI asks `can(workspace, "tokens:manage")`.
- **A Connection belongs to a User, an API token to the Workspace.** Approving an OAuth client
  (Claude over MCP) creates a Connection tied to the approving User's Membership: any member may
  approve one. Each grantable scope declares in TypeSpec the Permission needed to grant it (the
  `webhooks:*` scopes need `webhooks:manage`, `integrations:write` needs `integrations:manage`,
  `sending_domains:write` needs `sending_domains:manage`); the consent screen offers only what the
  User may grant, and the server refuses the rest. On every request a Connection can do what its
  scopes **and** its User's current Role allow, the Two-factor requirement applies to it, and it ends
  with the Membership (removal, Leaving, User deletion). Each User sees and revokes their own
  Connections; `members:manage` sees and revokes anyone's. API tokens are minted by hand with
  `tokens:manage`, outlive the people who made them and are not shown with Connections.

## Permissions

| Permission               | Covers                                                | owner | admin | member |
| ------------------------ | ----------------------------------------------------- | :---: | :---: | :----: |
| `members:manage`         | Invitations, Roles, removal, others' Connections      |   ✓   |   ✓   |        |
| `audit:read`             | Audit log list and export                             |   ✓   |   ✓   |        |
| `audit:configure`        | Audit log retention                                   |   ✓   |   ✓   |        |
| `tokens:manage`          | API tokens                                            |   ✓   |   ✓   |        |
| `integrations:manage`    | writing Integrations, regenerating an ingest key      |   ✓   |   ✓   |        |
| `webhooks:manage`        | webhook endpoints, reads included                     |   ✓   |   ✓   |        |
| `sending_domains:manage` | adding, verifying, archiving Sending domains          |   ✓   |   ✓   |        |
| `contacts:erase`         | Erasure, Contact lookup by address                    |   ✓   |   ✓   |        |
| `contacts:export`        | exporting Contacts                                    |   ✓   |   ✓   |        |
| `suppressions:manage`    | adding and lifting Suppressions (reading stays open)  |   ✓   |   ✓   |        |
| `second_factor:manage`   | the Two-factor requirement, resetting a Second factor |   ✓   |   ✓   |        |
| `workspace:update`       | name and postal address                               |   ✓   |   ✓   |        |
| `workspace:timezone`     | the Workspace time zone                               |   ✓   |   ✓   |        |
| `brand_kit:manage`       | the Brand kit and its starter Templates               |   ✓   |   ✓   |        |
| `join_domains:manage`    | Join domains and the Domain join switch               |   ✓   |   ✓   |        |
| `account_link:open`      | the Account link, which proves who an Owner is        |   ✓   |       |        |
| `workspace:delete`       | deleting and restoring the Workspace                  |   ✓   |       |        |

An owner differs from an admin by ownership and the irreversible: deleting the Workspace, the
Account link ([[0009-usage-metering-and-operator-holds]]) and the owner Policies. Time zone, Brand
kit and Join domains move to admin: an admin already invites people as admins and holds the
provider secrets, so those were the weaker powers. A member loses exporting the Contacts and lifting
Suppressions, the two bulk actions on personal data and on consent that were open to everyone.
Sending, Automations, Rules and imports stay a member's work.

## Considered Options

- **Keep the checks in the handlers and only align them**: the drift and the forgotten check come
  back with the next operation; rejected.
- **A policy engine (Cedar, Casbin, OpenFGA, OPA)**: none of our rules has a condition, a
  customer-written policy or a per-object grant, so an engine adds a language, an entity load per
  request and a dependency for a table of three Roles. Revisit when custom Roles, per-object grants
  or attribute conditions appear; `accounts.Allows` is the seam an Enterprise implementation would
  replace. Rejected for now.
- **Declare Roles instead of Permissions on operations** (`@Roles("admin")`, as cal.com's API v2
  does): every change of the Role table would touch every operation; rejected.
- **Cache allow decisions** (cal.com caches them for five minutes): a demoted member would keep
  access; rejected, the Role comes with the Membership the request loads anyway.
- **Keep consent for owners and admins**: members cannot use an MCP client at all; rejected for a
  Connection bounded by the member's own Permissions.

## Consequences

- ADRs 0016, 0051 and 0057 are amended in place to point here.
- `docs/architecture.md`, list item 1 of the scoped-client construction points, moves from
  `Handlers.scopedFor` / `scopedWithRoleFor` to the middleware when it is built.
- A Connection needs its Membership in the schema; an API token does not get one.
