import "./permissions.tsp";

namespace Sphericon;

/** The API token scopes an /api operation needs (ADR 0061), as `x-required-scopes`: a token must carry all of them, `#[]` for none */
extern dec requiresScopes(
  target: Reflection.Operation,
  scopes: valueof ApiTokenScope[]
);

/**
 * A scope enum a User grants to a Connection (ADR 0061, "A Connection belongs to a User"): its
 * `x-grant-permissions` lists every member, with the Permission a User must hold to grant it
 * where the member carries @grantedWith, and none (any member's to grant) where it does not.
 */
extern dec grantable(target: Reflection.Enum);

/** The Permission a User must hold to grant this scope of a @grantable enum */
extern dec grantedWith(
  target: Reflection.EnumMember,
  permission: valueof Permission
);

// The grantable scopes (GrantableScope, SendScope, SetupScope) are @grantable; a member that only
// a User holding a Permission may grant carries @grantedWith.

/** Scopes an OAuth client may be granted without any extra opt-in: the external
 * API's read/write vocabulary. Token management scopes are deliberately absent,
 * so a connector can never mint further credentials. */
@grantable
enum GrantableScope {
  contactsRead: "contacts:read",
  contactsWrite: "contacts:write",

  @grantedWith(Permission.contactsExport)
  contactsExport: "contacts:export",

  @grantedWith(Permission.suppressionsManage)
  suppressionsWrite: "suppressions:write",

  eventsRead: "events:read",
  eventsWrite: "events:write",
  segmentsRead: "segments:read",
  segmentsWrite: "segments:write",
  broadcastsRead: "broadcasts:read",
  broadcastsWrite: "broadcasts:write",
  automationsRead: "automations:read",
  automationsWrite: "automations:write",
  enrollmentsRead: "enrollments:read",
  templatesRead: "templates:read",
  templatesWrite: "templates:write",

  @grantedWith(Permission.webhooksManage)
  webhooksRead: "webhooks:read",

  @grantedWith(Permission.webhooksManage)
  webhooksWrite: "webhooks:write",

  @grantedWith(Permission.webhooksManage)
  webhooksRegenerate: "webhooks:regenerate",

  @grantedWith(Permission.webhooksManage)
  webhooksTest: "webhooks:test",

  customFieldsRead: "custom_fields:read",
  sendingDomainsRead: "sending_domains:read",
  integrationsRead: "integrations:read",
}

/** Send-class scopes (ADR 0016, "Send is a second lock"). An OAuth client gets them
 * only when the user explicitly opts in. mcp:send is the MCP-specific key: send-class
 * tools are not listed over /mcp without it, so any send-class grant brings it along. */
@grantable
enum SendScope {
  emailsSend: "emails:send",
  broadcastsSend: "broadcasts:send",
  automationsPublish: "automations:publish",
  mcpSend: "mcp:send",
}

/** Setup scopes (ADR 0016): they change how the Workspace sends (the Sending domains it
 * authenticates, the Integrations and the provider credentials they hold). An OAuth client
 * gets them only when it asked for them and the user ticks a box of their own for them,
 * apart from the send class: they are no send-class scope and bring no mcp:send. */
@grantable
enum SetupScope {
  @grantedWith(Permission.sendingDomainsManage)
  sendingDomainsWrite: "sending_domains:write",

  @grantedWith(Permission.integrationsManage)
  integrationsWrite: "integrations:write",
}

/** Scopes only an API token can hold, never an OAuth grant */
enum TokenOnlyScope {
  contactsErase: "contacts:erase",
  capturedMessagesRead: "captured_messages:read",
  capturedMessagesWrite: "captured_messages:write",
  tokensRead: "tokens:read",
  tokensWrite: "tokens:write",
  auditRead: "audit:read",
  exportRead: "export:read",
}

/** Every API token scope: the one vocabulary of the external API */
enum ApiTokenScope {
  ...GrantableScope,
  ...SendScope,
  ...SetupScope,
  ...TokenOnlyScope,
}
