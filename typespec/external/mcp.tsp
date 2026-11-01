import "../lib/main.tsp";

namespace Sphericon;

// MCP projection overrides (ADR 0016), declared with `@mcp` (typespec/lib), which writes the
// operation's `x-mcp` extension. Every /api operation becomes an MCP tool at /mcp unless it
// carries `@mcp(#{ hidden: true })`; `name` overrides the default tool name (the operation id
// in snake_case, e.g. `contacts_list`).

// API token management is never an agent capability.
@@mcp(Auth.tokensCreate, #{ hidden: true });
@@mcp(Auth.tokensList, #{ hidden: true });
@@mcp(Auth.tokensDelete, #{ hidden: true });
@@mcp(Auth.tokensBootstrap, #{ hidden: true });

// Regenerating an Integration's hook key breaks a working URL and needs a person (ADR 0037).
@@mcp(Integrations.regenerateKey, #{ hidden: true });

// Removing a retired Tracking domain breaks the links in mail already sent on it and needs a
// person (ADR 0071).
@@mcp(TrackingDomains.remove, #{ hidden: true });

// Deleting every Captured message in one call needs a person (ADR 0058).
@@mcp(CapturedMessages.deleteAll, #{ hidden: true });

// The Audit log is never an agent capability (ADR 0016, ADR 0022).
@@mcp(AuditEntries.list, #{ hidden: true });

// An Export stream is a bulk feed, never an agent capability (ADR 0016, ADR 0045).
@@mcp(Export.events, #{ hidden: true });
@@mcp(Export.contacts, #{ hidden: true });
@@mcp(Export.messages, #{ hidden: true });
@@mcp(Export.audit, #{ hidden: true });
@@mcp(Export.erasures, #{ hidden: true });

// Names in sphericon vocabulary where the default would read poorly.
@@mcp(Auth.meGet, #{ name: "whoami" });
@@mcp(Events.create, #{ name: "events_record" });
@@mcp(EventActions.list, #{ name: "events_actions_list" });
@@mcp(ContactsBatch.upsert, #{ name: "contacts_upsert_batch" });
@@mcp(EventsBatch.submit, #{ name: "events_record_batch" });

@@mcp(EmailLint.lint, #{ name: "email_lint" });
@@mcp(EmailSchema.get, #{ name: "email_schema" });
@@mcp(BrandKit.get, #{ name: "brand_get" });

// Read-only reference data (ADR 0016).
@@mcp(CustomFields.list, #{ name: "custom_fields_list" });
@@mcp(SendingDomains.list, #{ name: "sending_domains_list" });
@@mcp(Integrations.list, #{ name: "integrations_list" });
@@mcp(Presets.list, #{ name: "presets_list" });
@@mcp(SendingDomainRates.list, #{ name: "sending_domains_rates" });

// Send is a second lock (ADR 0016): send-class tools are listed over MCP, and
// callable, only for tokens that also carry the `mcp:send` scope (ADR 0065). Deactivating
// an Automation or a Rule only stops new Enrollments or Firings, so it stays an authoring tool.
@@mcp(Automations.publish, #{ send: true });
@@mcp(Rules.publish, #{ send: true });
@@mcp(Emails.send, #{ send: true });
@@mcp(Broadcasts.schedule, #{ send: true });
@@mcp(Broadcasts.unschedule, #{ send: true });
