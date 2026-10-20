import "@typespec/openapi";

using TypeSpec.OpenAPI;

namespace Sphericon;

// MCP projection overrides (ADR 0016). Every /api operation becomes an MCP tool
// at /mcp unless it carries `x-mcp: { hidden: true }`; `x-mcp: { name }` overrides
// the default tool name (the operation id in snake_case, e.g. `contacts_list`).

// API token management is never an agent capability.
@@extension(Auth.tokensCreate, "x-mcp", #{ hidden: true });
@@extension(Auth.tokensList, "x-mcp", #{ hidden: true });
@@extension(Auth.tokensDelete, "x-mcp", #{ hidden: true });
@@extension(Auth.tokensBootstrap, "x-mcp", #{ hidden: true });

// Regenerating an Integration's hook key breaks a working URL and needs a person (ADR 0037).
@@extension(Integrations.regenerateKey, "x-mcp", #{ hidden: true });

// The Audit log is never an agent capability (ADR 0016, ADR 0022).
@@extension(AuditEntries.list, "x-mcp", #{ hidden: true });

// An Export stream is a bulk feed, never an agent capability (ADR 0016, ADR 0045).
@@extension(Export.events, "x-mcp", #{ hidden: true });
@@extension(Export.contacts, "x-mcp", #{ hidden: true });
@@extension(Export.messages, "x-mcp", #{ hidden: true });

// Names in sphericon vocabulary where the default would read poorly.
@@extension(Auth.meGet, "x-mcp", #{ name: "whoami" });
@@extension(Events.create, "x-mcp", #{ name: "events_record" });
@@extension(EventActions.list, "x-mcp", #{ name: "events_actions_list" });
@@extension(ContactsBatch.upsert, "x-mcp", #{ name: "contacts_upsert_batch" });
@@extension(EventsBatch.submit, "x-mcp", #{ name: "events_record_batch" });

@@extension(EmailLint.lint, "x-mcp", #{ name: "email_lint" });
@@extension(EmailSchema.get, "x-mcp", #{ name: "email_schema" });
@@extension(BrandKit.get, "x-mcp", #{ name: "brand_get" });

// Read-only reference data (ADR 0016).
@@extension(CustomFields.list, "x-mcp", #{ name: "custom_fields_list" });
@@extension(SendingDomains.list, "x-mcp", #{ name: "sending_domains_list" });
@@extension(Integrations.list, "x-mcp", #{ name: "integrations_list" });
@@extension(Presets.list, "x-mcp", #{ name: "presets_list" });
@@extension(
  SendingDomainRates.list,
  "x-mcp",
  #{ name: "sending_domains_rates" }
);

// Send is a second lock (ADR 0016): send-class tools are listed over MCP, and
// callable, only for tokens that also carry the `mcp:send` scope. Deactivating an
// automation only stops new enrollments, so it stays an authoring tool.
@@extension(Automations.activate, "x-mcp", #{ send: true });
@@extension(Emails.send, "x-mcp", #{ send: true });
@@extension(Broadcasts.schedule, "x-mcp", #{ send: true });
@@extension(Broadcasts.unschedule, "x-mcp", #{ send: true });
