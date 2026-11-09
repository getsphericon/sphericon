# Enterprise Edition

[Architecture](../architecture.md) → Enterprise Edition

## License gates and audit capture

**Enterprise Edition (`ee/`)**: same binary, gated by the runtime license key (`SPHERICON_LICENSE_KEY`,
verified by `ee/licensekey`; empty = plain core, a bad key fails boot). `ee.New` builds the
`Edition` (extra event consumers + read seams) in `internal/app` and in `testhelper.Setup`
(`WithoutLicense()` for the unlicensed case); core reaches EE only through interfaces
(`auditlog.Reader`, which `auditlog.Log` serves the Audit log over and refuses as unlicensed
without it, `events.Consumer`). The audit table's ent schema sits in `ent/schema`
(ent has one schema package) but is written and read only by `ee/domain/insights/auditentries` (the
Workspace erase deletes a deleted Workspace's entries too, below). An Audit entry is
an event that is not Projected (`events.Projected`): it has no Event row, automations skip it,
and a Webhook endpoint receives it only when it names `audit.entry`.
The Audit table's rows are removed only through `ee/domain/insights/auditentries`, by `ee/usecase/auditlogretention`'s prune (the
`retention` license feature, a Workspace `retention_days` window, one hourly river job
of the Enterprise job runner `ee/jobs`, which calls `auditentries.PruneBefore`), and by the Workspace erase
(`internal/usecase/workspacedeletion`), which deletes a deleted Workspace's entries directly (ADR 0110).
An explicit entry, one no audited write produces (a Workspace setting, a login, a membership
change, the Audit log export `auditentries.Log.Export`), is recorded one way: `events.RecordAudit`
inside the transaction of the change (`bus.WithinTx`, or `bus.WithinScopedTx` with the scope's
`Actor()` when there is a scope); `events.ExplicitAuditActions` lists its actions. Many of them
have no scope to take the actor from (a User's action goes to each of their Workspaces, a
suspension names its Staff actor), so the scoped client has no recorder of its own. `ee.New`
takes the bus for the export: the bus is built first, with the Edition's hooks from `ee.Hooks`.
The Enterprise consumers of the domain events (`ee/jobs.EventConsumers`: the Audit log's and the
Erasures stream's) are registered on the bus beside core's; their workers come with the
Enterprise job runner's `Extension`. In tests no River runtime works the queue, so call
`env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)` to work the jobs a publish inserted.
**Audit capture in the scoped client (ADR 0022):** an `*ent.Scoped` carries its actor
(`s.As(actor, opener)`, set at the construction points: `Accounts.Scope` user,
`HandleBearerAuth` api_token, collect/SES/consent/bootstrap `events.Ingest`; `bus.Act` for
system). An entity opts in with `schema.Audited{Action, NameField}` (fields: `schema.Sensitive{}`);
its generated wrappers then run in their own transaction, or join the one the scope is bound
to (`WithinScopedTx` keeps the actor), and publish the entry. Ingest and actor-less scopes take
the plain path. Upserts of an audited entity must be `DoNothing`. Add the annotation, nothing
per handler.
Note `ee/licensekey`, not `ee/license`: the path is case-insensitive on macOS and would
collide with `ee/LICENSE`.

## Enterprise Edition layout

**Enterprise Edition layout (ADR 0102).** `ee/` has the core's shape: its domain modules in
`ee/domain/<context>/<module>`, in the core's contexts and order, its use cases in
`ee/usecase/<name>`, its surfaces in `ee/api`, its job runner in `ee/jobs` (the workers, the
periodic schedules and the consumers of the domain events, ADR 0111), the Edition `ee/ee.go`,
which builds the modules and surfaces and schedules nothing, and beside them the
License key's gate `ee/licensekey` (component `ee-licensekey`), which imports nothing of ours.
The same rule places a package: one that owns a table is a domain module in the context of what
it targets, an operation belongs to the module that owns its subject and is a use case only by
the test of ADR 0112, and a package that is both is split.
Identity: `servicecredentials` (the Service credentials, with their scope vocabulary generated
from the service contract). Insights: `auditentries` (the Audit entries), `erasures` (the Erasures
rows), `usages` (the Usage rows) and `abusedetector` (the Abuse detector: its limits, its reading
of a Workspace's standing and its hourly pass, which warns and freezes through `workspaces`,
`memberships` and `suspension` across every Workspace, ADR 0112), which hold the raw client
(`ee-domain-insights-raw`), and the Workspace oversight (`workspaceoversight`, `ee-domain-insights-raw`:
the Staff console's Enterprise sections and the service endpoints over suspension, holds, the
Audit log, Usage, the Abuse detector and the service feeds; a module of the top of insights, since
each of its operations calls one module and what it owns is the rules of looking across
Workspaces). The use case (`ee-usecase-raw`, an isolated instance holding the raw client):
`auditlogretention` (the advanced-retention prune over `workspaces` and the Audit log). The `export` license gate is the Edition's own (`ee.ExportStreams`).
An Enterprise domain module imports the core's modules of its context and the contexts below
and the Enterprise modules of those contexts, never a module above nor a use case; an Enterprise
use case imports domain modules only, never an unchecked operation of the core (`domain-unchecked`
admits only the core's use cases; the Edition's own would be `ee-domain-unchecked`, ADR 0110) and is imported only by `ee/api`, the Edition and the wiring. The boundary with the core is
unchanged: no core component may import an `ee` component, and each `ee` component names its
importers; the command line (`cli`) may import the Service credentials, which only it issues,
rotates and revokes (ADR 0026).
