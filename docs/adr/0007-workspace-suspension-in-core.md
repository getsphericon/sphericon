# Workspace suspension: mechanism in core, policy and console in EE

For abuse control on SaaS, an abusive sender must be stoppable before it torches the shared
sending reputation. We model this as a reversible `suspended` state on Workspace that
**freezes all outbound sending** (Broadcast, Automation, Transactional) while preserving
login, dashboard/API reads, and `/collect` tracking — a reputation-protection measure, not a
lockout, so the owner can still see the notice and appeal.

The **enforcement** lives in the **AGPL core** because the send path (in the binary) is the
only reliable choke point — a SaaS-layer gate in front of core would be bypassed by anything
calling the send path directly. So the core ships the mechanism: the `suspended` state with
its attribution fields (actor + reason), the send-path refusal, an owner-facing notice on
`/site`, and a bare `sphericon workspace suspend/unsuspend` CLI. Everything above the mechanism —
the automated abuse detector, the Operator console, impersonation — is closed **EE/SaaS**.

## Considered options

- **SaaS-layer gate** (rejected): no choke point in the binary; bypassable.
- **An entitlement state** (rejected): suspension is an abuse fact, not an entitlement; conflating
  them couples abuse control to whatever decides entitlements.

## Consequences

- Self-hosted (typically one workspace, doesn't police itself) gets the full mechanism and a CLI
  toggle, and needs nothing more — no Operator, no console.
- The automated detector only fires above a minimum send volume (a complaint/bounce _rate_ is
  noise at low volume), notifies the owner, and is one-click reversible by an Operator — bounding
  the blast radius of a false positive on a legitimate sender.
- Impersonation and business dashboards are deferred out of the first version.

## Amendment: enforced as a source-level hold in Outbound send

The send-path refusal is the **`Held`** outcome of the single Outbound send module
([[0015-outbound-send-single-chokepoint]]), checked for every message on all three surfaces. It is
a reversible, source-level hold, not a per-recipient skip: a Broadcast in flight pauses with its
remaining recipients still pending and resumes on unsuspend, Automation Enrollments are held, and
Transactional requests get a 4xx. An Operator hold (ADR 0009) is a second freeze reason answered by
the same question, never merged with this one.

See [[0026-operator-console-and-abuse-detector]] for the Operator console and the detector.
