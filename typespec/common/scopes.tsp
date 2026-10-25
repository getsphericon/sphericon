namespace Sphericon;

/** Scopes an OAuth client may be granted without any extra opt-in: the external
 * API's read/write vocabulary. Token management scopes are deliberately absent,
 * so a connector can never mint further credentials. */
enum GrantableScope {
  contactsRead: "contacts:read",
  contactsWrite: "contacts:write",
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
  webhooksRead: "webhooks:read",
  webhooksWrite: "webhooks:write",
  webhooksRegenerate: "webhooks:regenerate",
  webhooksTest: "webhooks:test",
  customFieldsRead: "custom_fields:read",
  sendingDomainsRead: "sending_domains:read",
  integrationsRead: "integrations:read",
}

/** Send-class scopes (ADR 0016, "Send is a second lock"). An OAuth client gets them
 * only when the user explicitly opts in. mcp:send is the MCP-specific key: send-class
 * tools are not listed over /mcp without it, so any send-class grant brings it along. */
enum SendScope {
  emailsSend: "emails:send",
  broadcastsSend: "broadcasts:send",
  automationsActivate: "automations:activate",
  mcpSend: "mcp:send",
}

/** Scopes only an API token can hold, never an OAuth grant */
enum TokenOnlyScope {
  contactsErase: "contacts:erase",
  sendingDomainsWrite: "sending_domains:write",
  integrationsWrite: "integrations:write",
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
  ...TokenOnlyScope,
}
