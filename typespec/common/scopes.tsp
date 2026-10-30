import "@typespec/openapi";
import "./permissions.tsp";

using TypeSpec.OpenAPI;

namespace Sphericon;

// A grantable scope (GrantableScope, SendScope, SetupScope) declares, in its enum's
// `x-grant-permissions`, the Permission a User must hold to grant it to a Connection
// (ADR 0061, "A Connection belongs to a User"); a row without one is any member's to
// grant. Every member is listed, and generation fails on one that is not.

/** Scopes an OAuth client may be granted without any extra opt-in: the external
 * API's read/write vocabulary. Token management scopes are deliberately absent,
 * so a connector can never mint further credentials. */
@extension(
  "x-grant-permissions",
  #[
    #{ scope: GrantableScope.contactsRead },
    #{ scope: GrantableScope.contactsWrite },
    #{ scope: GrantableScope.eventsRead },
    #{ scope: GrantableScope.eventsWrite },
    #{ scope: GrantableScope.segmentsRead },
    #{ scope: GrantableScope.segmentsWrite },
    #{ scope: GrantableScope.broadcastsRead },
    #{ scope: GrantableScope.broadcastsWrite },
    #{ scope: GrantableScope.automationsRead },
    #{ scope: GrantableScope.automationsWrite },
    #{ scope: GrantableScope.enrollmentsRead },
    #{ scope: GrantableScope.templatesRead },
    #{ scope: GrantableScope.templatesWrite },
    #{
      scope: GrantableScope.webhooksRead,
      permission: Permission.webhooksManage,
    },
    #{
      scope: GrantableScope.webhooksWrite,
      permission: Permission.webhooksManage,
    },
    #{
      scope: GrantableScope.webhooksRegenerate,
      permission: Permission.webhooksManage,
    },
    #{
      scope: GrantableScope.webhooksTest,
      permission: Permission.webhooksManage,
    },
    #{ scope: GrantableScope.customFieldsRead },
    #{ scope: GrantableScope.sendingDomainsRead },
    #{ scope: GrantableScope.integrationsRead }
  ]
)
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
@extension(
  "x-grant-permissions",
  #[
    #{ scope: SendScope.emailsSend },
    #{ scope: SendScope.broadcastsSend },
    #{ scope: SendScope.automationsActivate },
    #{ scope: SendScope.mcpSend }
  ]
)
enum SendScope {
  emailsSend: "emails:send",
  broadcastsSend: "broadcasts:send",
  automationsActivate: "automations:activate",
  mcpSend: "mcp:send",
}

/** Setup scopes (ADR 0016): they change how the Workspace sends (the Sending domains it
 * authenticates, the Integrations and the provider credentials they hold). An OAuth client
 * gets them only when it asked for them and the user ticks a box of their own for them,
 * apart from the send class: they are no send-class scope and bring no mcp:send. */
@extension(
  "x-grant-permissions",
  #[
    #{
      scope: SetupScope.sendingDomainsWrite,
      permission: Permission.sendingDomainsManage,
    },
    #{
      scope: SetupScope.integrationsWrite,
      permission: Permission.integrationsManage,
    }
  ]
)
enum SetupScope {
  sendingDomainsWrite: "sending_domains:write",
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
