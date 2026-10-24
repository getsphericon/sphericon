---
status: accepted
---

# A Workspace admits Users by a verified Join domain

A company that runs its marketing in sphericon invited every colleague by hand. An Owner wants
"everyone at `@acme.com` can join our Workspace" without managing a list, on a self-hosted Instance
and a hosted one alike, and without anyone being able to claim a domain they do not own. This is a
membership rule over one DNS-verified domain, not federation, so it lives in the core and takes no
license key (ADR 0014: SAML, OIDC and SCIM stay Enterprise). Core against Enterprise is a one-way
door, and this lands in the core for good.

## Decisions

- **Join domain and Domain join.** A **Join domain** is a Workspace-scoped domain an Owner proves
  by publishing a TXT record at `_sphericon-join.<domain>` (the table and its verification are
  `internal/joindomains`). A **Domain join** is a Membership created because its member proved an
  address on a verified Join domain; the Membership's `source` is `domain`. A Workspace has a
  switch, `workspaces.domain_join_enabled`, off by default, that an Owner turns on and off on its
  own (adding a domain never gives anyone access by surprise). A verified domain stays reserved to
  its Workspace while the switch is off.
- **When a User joins.** Only at the moment an address becomes a Proven email (ADR 0048,
  `email_proven_at` stamped from empty), and in the transaction that stamps it, so the Membership
  and the proof are atomic. An address proven before the switch was turned on never joins
  afterwards, an Invitation accepted for an address that is already proven is not a trigger, and
  turning the switch on adds nobody and invites nobody: no endpoint reveals whether an address has
  a User (ADR 0031), and a mass Invitation would. An Owner invites such a person by hand, as before.
- **Which Workspace.** The address's domain, normalized as a Join domain is (lower case, IDN as
  punycode), must equal a verified Join domain exactly: `acme.com` does not cover `eu.acme.com`.
  The verified-only unique index makes at most one Workspace hold it, so a person joins at most one
  Workspace. The join reads the stored verification and never DNS inside the transaction: a domain
  that lost its record stops admitting people once the periodic re-check has un-verified it (the
  re-check and its mail to the Owners are the Sending domain pattern; existing Memberships stay).
- **One decision, one apply step.** `accounts.DecideJoin` is a pure function of the facts (the
  address was proven now, a verified domain matches, the switch, the Workspace's state, an existing
  Membership, the sign-up gate) with the outcomes join and nothing plus the reason, tested branch
  by branch and by properties like `DecideSignin` (ADR 0048). `applyDomainJoin` in `accounts` is the
  one place that applies it, called explicitly from each step that proves an address. It is not an
  ent hook: a hook would hide a Membership insert inside a User update. The Membership is `member`
  with source `domain`, never `owner` or `admin`; nothing happens for a suspended Workspace or one
  in its deletion window (ADR 0043), and a Staff hold is not a join gate. Nobody is mailed, neither
  the joiner nor the Owner.
- **The sign-up gate stays shut.** A domain match never admits a new User on a closed Instance
  (ADR 0042): the gate is a fact of the decision (an existing User, or sign-up open). Accepting an
  Invitation is untouched by it.
- **Roles.** An Invitation accepted by a member raises `member` to `admin` and never lowers a Role,
  so an explicit Invitation beats the domain rule.
- **Landing.** The verify-link response carries the Workspace to open, `landingWorkspaceId`: the
  joined Workspace, else the first one made for the person. The join runs before the first
  Workspace is created inside the same transaction, which still happens, so the joiner has a
  personal Workspace as well, and the SPA goes straight to the one named instead of guessing from
  the order of its list.
- **Audit.** The join is an explicit `membership.domain_join` entry (ADR 0022) with the system as
  the actor, the joiner as the target and the domain and Role in the diff. The switch is an explicit
  `workspace.update` entry with a `domain_join_enabled` diff, like the Two-factor requirement.
  The Join domain rows are Audited entities.
- **Scoping (ADR 0017).** The join builds a `*ent.Scoped` for a Workspace the User has no session
  in, right after the Join domain row named it: a new entry in the closed list in `docs/agents/workspace-scoping.md`. The
  Instance-wide lookup of a domain to its verified row runs before any scope exists, so it sits in
  the new `internal/joindomains` package, which joins the raw-client list.
- **Operations.** Listing, adding, verifying and removing Join domains and reading and setting the
  switch are `/site` operations, owner only; nothing is added to the external API or MCP. The
  members list shows how each member got in (`source`: `creator`, `invitation`, `domain`).

## Which proof points trigger it

The rule is the spec's: every place the Proven email is stamped from empty, and the confirmed email
change. That covers the verify link (including a reset link followed by a still-Unverified User,
which completes the same registration), a provider sign-in's create, link and take-over, a
confirmed email change, and an Invitation accepted by a User whose address was not yet proven.

- **Wired:** the verify link and a reset link that completes a registration (`VerifyEmail`), the
  provider sign-in's create, link and take-over, the confirmed email change, the Invitation accept
  that proves the address for the first time, and a reset link followed by a Verified User whose
  address was never proven (`updateProfile(..., proven=true)`). Each calls `applyDomainJoin` on the
  transaction that stamps the proof. A write that may find the column already set (the provider link, the
  reset and the Invitation accept) is guarded by `email_proven_at IS NULL` and joins only when its
  rows-changed count says it stamped from empty; a reset link followed by a User whose address was
  already proven is a plain password change and joins nothing. A new User from a provider obeys the
  sign-up gate (a closed Instance stays closed); the Invitation accept does not. The explicit
  Invitation's Membership is made first, so an `admin` Invitation is never lowered by the join.
- `RegisterVerified` and `CreateOwner` (an Instance with verification off) stamp only
  `email_verified_at`: nothing is proven, so they never join.

## Consequences

- An Invitation to Workspace B for a not-yet-proven address on Workspace A's domain also joins that
  person to A, and a removed member who later confirms another address on the domain rejoins. Both
  are the cost of "proving the address is the event".
- ADR 0048's sentence "The Instance has no other registration gate (no domain restriction)" is
  amended: a Workspace may now admit by domain, while the Instance itself still has no registration
  gate beyond `SPHERICON_SIGNUP_OPEN`, so provider sign-up has none either.
- Out of scope, on purpose: approval-based joining, a Role other than `member`, subdomain or
  wildcard domains, adding existing Users when the switch is turned on, blocking people with a
  matching address from creating their own Workspaces, and any exposure through the external API.
