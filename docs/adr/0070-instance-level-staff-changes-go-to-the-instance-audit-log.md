---
status: accepted
---

# Instance-level Staff changes go to the Instance audit log

ADR 0067 records every Send cap change with its author. A Workspace's Send cap has a natural place
for it: the Workspace's Audit log, where suspension records its changes. The default Send cap of new
Workspaces has none. It belongs to the Instance, and nothing recorded who changed an Instance
setting:

- the Telemetry report switch updates the Instance state row and records nothing;
- a Legal document version keeps its publisher on its own row (ADR 0062), so the history of one
  table cannot hold another setting's.

The Workspace's Audit log cannot take an Instance change either:

- every Audit entry belongs to a Workspace and is erased with it (ADR 0043);
- it is an Enterprise feature that a plain core records nothing into (ADR 0022);
- its customers would see an entry that is not about their Workspace.

## Decision

An **Instance audit log** (GLOSSARY) records the changes a Staff member makes to the Instance's own
settings. Each entry says:

- which Staff member made the change;
- the action, as `instance.<what>_change`;
- the old and the new value of each changed field, with the same `from`/`to` shape as an Audit
  entry's diff;
- when the change was made.

- **Core, on every Instance.** The Staff console is on every Instance (ADR 0063), and so is this log:
  no License key gates it. What a setting is gated by (the default Send cap by the Mode, ADR 0060)
  gates its changes, not the log.
- **Owned by the Instance, not a Workspace.** The table `instance_audit_entries` has no Workspace
  edge and no scoped wrapper, and Workspace deletion never touches it. Its actor is a foreign key to
  the Staff member, who is never deleted (ADR 0026), so an entry always names who acted.
- **Written in the change's transaction.** `internal/instanceaudit.Record` takes the transaction
  client of the change, so a setting and its entry commit together or not at all, and a no-op
  records nothing. It is a plain insert, not the bus's `events.RecordAudit`: the Workspace's Audit
  log is a bus consumer of Enterprise, and this log needs neither.
- **Read in the console.** The core Staff API lists it newest first (`GET /staff/instance/audit-entries`),
  and the Instance page shows it.
- **The place for later Instance-level Staff changes.** The default Send cap is the first action.
  A later Staff change of an Instance setting names its action in `internal/instanceaudit` and
  records it the same way. The Telemetry report switch and Sign-up item publishing are not moved in
  this change.

## Considered options

- **A history table for the default Send cap alone**, like Legal document versions. It records the
  one setting but leaves the next Instance-level Staff change to invent its own.
- **A Workspace-less row in the Workspace's Audit log.** The table, its erasure, its retention and
  its customer-facing reads all assume a Workspace, and it is Enterprise: a core Instance setting
  would go unrecorded without a License key.
