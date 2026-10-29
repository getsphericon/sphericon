---
status: accepted
---

# Every Instance has a Staff console, and a Staff member opens sign-up

What belongs to an Instance rather than to a Workspace had no owner of its own: sign-up followed the
Mode ([[0060-an-instance-runs-in-one-mode]]), and the Telemetry report was set by the Owners of the
oldest Workspace ([[0032-instance-telemetry-report-and-release-check]]), a rule that depends on who
happened to create a Workspace first. Sign-up is about to carry what the Instance asks a person to
accept ([[0062-sign-up-items-the-operator-sets-and-every-decision-is-recorded]]), which is the
operator's to decide in every market, on a self-hosted Instance as much as on the SaaS. The people who
run an Instance already have an identity, the Staff member ([[0008-operator-separate-identity]]), but
its console was Enterprise and off in `selfhosted`. We bring the Staff member and the console into the
open core, on every Instance, and make it the place where the Instance's own settings live.

## Decisions

- **The Staff member, its sign-in and the Staff console run on every Instance**, in both Modes and
  without a License key. ADR 0008 stands: a Staff member is not a User and holds no Membership.
  `sphericon staff create` makes the first one, as before.
- **The console has core sections and Enterprise sections.** The core holds the Instance's own
  settings: sign-up (open or closed) and its Sign-up items with their Legal documents (ADR 0062). The
  cross-Workspace views and actions of [[0026-operator-console-and-abuse-detector]] (Workspace metadata,
  suspension, the Abuse detector, Staff holds) and the Service credentials stay under the `staff`
  license feature, shown in the same console when the key has it.
- **Sign-up is closed on a new Instance until a Staff member opens it**, in both Modes. The Mode no
  longer decides it. A closed `signup` answers as it does today (`403`, `signup_closed`).
- **A Staff member invites the first Users from the console** while sign-up is closed: the invited
  person creates their account and their first Workspace, through the same Sign-up items as a public
  sign-up. `sphericon user create-owner` stays for automation.
- **The Telemetry report setting moves to the console too**, and the "Owners of the oldest Workspace"
  rule goes with it. That is a separate change from this one.

## Considered options

- **An Instance-admin flag on a User**: one login, but it is the global role on User that ADR 0008
  rejects; rejected.
- **A CLI for the Instance's settings on a self-hosted Instance, the console only on the SaaS**: two
  ways to do one thing, and a self-hoster writing legal texts from a shell; rejected.
- **Keep the console Enterprise**: a self-hoster without the key could not set what their market
  requires before opening sign-up; rejected.

## Consequences

- A first run needs `sphericon staff create` before anyone can sign up: the first-run documentation
  says so. The open-until-the-first-Owner rule of ADR 0060 is gone.
- The Staff member's store and sign-in leave `ee/staff` for the core; `docs/architecture.md` and its
  closed list of packages that hold the raw client follow.
- The `/staff` surface is no longer a 404 without the license: its core routes answer on every
  Instance, its Enterprise routes keep answering 404 without the `staff` feature.
