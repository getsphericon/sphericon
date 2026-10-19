---
status: accepted
---

# Audit log: Enterprise, captured at the scoped client, delivered over the outbox

A Workspace keeps an append-only **Audit log** of **Audit entries** (GLOSSARY): who changed a Workspace's configuration or access, what, and when. ADR 0014 already puts audit logs in EE (security/governance, gated by the license key); this ADR records how they are captured and delivered. The core binary stores and shows nothing; it only publishes the facts an EE subscriber persists.

- **Edition.** The log table, the subscriber, the `/site` page, the CSV export and the `/api` read live in `ee/`. A plain self-host has no audit log. Core carries only the `audit.entry` domain-event type and the publish seam.
- **Actors.** User (site), API token (external and MCP), Staff member (shown to the customer as "sphericon staff"; ADR 0008) and `system` (jobs, automations, automatic suspension).
- **Scope: control plane only.** Membership and roles, invitations, API tokens and keys, sending domains, integrations, webhook endpoints, Workspace settings and suspension, broadcasts, automations, segments, templates, tags, custom fields, Contact edits, deletes and imports, data exports, and logins. Never the data plane (`/collect`, ingested Events, Outbound sends, tracking): those facts already are Events and Outbound messages. A login belongs to a User, not a Workspace, so it is written into the log of every Workspace the User holds a Membership in; there is no account-level log.
- **Capture is a hybrid.**
  1. _Automatic_ in the generated scoped-client wrappers (ADR 0017: `Create`, `UpdateOneID`, `Update`, `DeleteOneID`, `Delete`). An entity opts in with a schema annotation. The actor (kind and id) is part of `*ent.Scoped`, set at the same closed list of construction points. Rows a job writes in the normal course of work (broadcast claim, outbound lease, `AutomationRun` state) are not annotated and so are not audited.
  2. _Explicit_ calls to the same seam from the raw-client packages (`accounts` for Membership and Invitation, `api/auth`) and for actions that are not a row write (login, password change, export, suspension).
- **No actor, no entry.** The scopes built from a secret rather than a login (collect key, Integration ingest key, signed unsubscribe/confirm token, bootstrap token) carry the actor kind `ingest`, and capture skips it, so ingest traffic (Identify creating Contacts and Custom fields) never reaches the log. The same Contact edit through `/api` under an API token is audited.
- **Delivery.** The seam publishes `audit.entry` through the transactional outbox in the mutation's own transaction (`events.Bus.WithinScopedTx`); an EE subscriber persists it, deduplicated by event key (at-least-once). For an audited entity the generated wrapper opens a transaction itself, runs the mutation and publishes in it, and joins an already open `WithinScopedTx` instead of nesting. A rolled-back mutation leaves no entry; a committed one cannot lose it. Without a license the event is published and nobody subscribes.
- **Entry content.** Time, actor kind and id, action `<entity>.<verb>`, target type and id plus a snapshot of the target's name (for a Contact, the id only: an email or name is personal data), a before/after diff of changed fields, request id, IP and user agent. Fields marked `sensitive` in the schema (token hashes, passwords, Integration credentials, webhook signing secrets) record only that they changed. `contact.*` entries name the changed fields and never carry values, so erasure of a Contact needs no rewrite of an immutable log.
- **Not an Event.** The persist consumer writes an Event row for every registered bus type, and the automations and webhooks consumers see every type. `audit.entry` opts out of all three (it implements an "unprojected" capability that persist, the automation trigger and the all-events webhook match skip), so an administrative action never becomes a Contact Event or an automation trigger.
- **Access.** Append-only (no edit or delete in any surface). Owner and admin read it on `/site` with filters (period, actor, action, target type, IP, request id) and cursor pagination. CSV export from the UI and a read on external `/api` under a token scope (`audit:read`). Not exposed over MCP (ADR 0016). Retention is the EE advanced-retention control (ADR 0014), not a second engine.
- **Forwarding.** `audit.entry` is an event type a Webhook endpoint can subscribe to (EE only, signed deliveries). An empty subscription list still means "all customer-facing events" but never includes `audit.entry`; it must be chosen explicitly.

## Considered options

- **Explicit audit calls in every handler.** Rejected: it repeats the "do not forget" rule that ADR 0017 removed for scoping, and a new endpoint silently has no entry.
- **ent mutation hooks reading the actor from `context`.** Rejected for the reason ADR 0017 rejected an interceptor: hidden state in `ctx`, and legitimate unscoped actors need bypass flags.
- **Capture every Scoped write.** Rejected: job bookkeeping would flood the log with `system` noise. Opt-in by annotation keeps it to configuration and access.
- **Write the entry synchronously into a core table.** Rejected: puts an EE feature's schema in the AGPL core and breaks "no feature born free then moved" (ADR 0014).
- **Reuse the Event machinery.** Rejected: an Event is a Contact/data-plane fact that feeds segmentation; an Audit entry is an actor's action on settings. Mixing them would put administrative actions into customers' segments.
- **A per-User account-level log** (as Customer.io has). Rejected: the Workspace is the tenant boundary (ADR 0004, 0017); duplicating login entries into each Workspace keeps every read scoped.

## Consequences

- The annotation and the actor on `*ent.Scoped` change the scoped template (`ent/template/scoped*.tmpl`), so a new Workspace-owned entity is covered by opting in at its schema, nothing hand-written per handler.
- Raw-client paths and non-CRUD actions need explicit calls; a test lists them so a new one is not forgotten.
- `ee/` gets its first table and migration.
- The outbox carries one more event type, which Webhook endpoints must treat as opt-in.
