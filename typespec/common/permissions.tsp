namespace Sphericon;

/** A named action in a Workspace that a Role allows (ADR 0061, GLOSSARY "Permission"),
 * written `resource:verb` like the API token scopes. A Role is a set of them; the table
 * from Role to Permissions lives in the server, and the client reads a Membership's
 * Permissions from its Workspace instead of computing them. */
enum Permission {
  /** Invitations, Roles, removal, others' Connections */
  membersManage: "members:manage",

  /** The Audit log list and export */
  auditRead: "audit:read",

  /** The Audit log retention */
  auditConfigure: "audit:configure",

  /** API tokens */
  tokensManage: "tokens:manage",

  /** Writing Integrations, regenerating an ingest key */
  integrationsManage: "integrations:manage",

  /** Webhook endpoints, reads included */
  webhooksManage: "webhooks:manage",

  /** Adding, verifying, archiving Sending domains */
  sendingDomainsManage: "sending_domains:manage",

  /** Erasure, Contact lookup by address */
  contactsErase: "contacts:erase",

  /** Exporting Contacts */
  contactsExport: "contacts:export",

  /** Adding and lifting Suppressions (reading them stays open) */
  suppressionsManage: "suppressions:manage",

  /** The Two-factor requirement, resetting a Second factor */
  secondFactorManage: "second_factor:manage",

  /** The Workspace name and postal address */
  workspaceUpdate: "workspace:update",

  /** The Workspace time zone */
  workspaceTimezone: "workspace:timezone",

  /** The Reporting currency and the Attribution window */
  revenueConfigure: "revenue:configure",

  /** The Brand kit and its starter Templates */
  brandKitManage: "brand_kit:manage",

  /** Join domains and the Domain join switch */
  joinDomainsManage: "join_domains:manage",

  /** Following a Hold's action link to resolve it (a member is told to ask an owner or admin) */
  holdsResolve: "holds:resolve",

  /** The Account link, which proves who an Owner is */
  accountLinkOpen: "account_link:open",

  /** Deleting and restoring the Workspace */
  workspaceDelete: "workspace:delete",

  /** Granting the owner Role, changing or removing an owner, resetting an owner's Second factor */
  ownersManage: "owners:manage",

  /** Reading and changing the Instance state (Telemetry, the notice). Every owner holds it; only
   * the holders in the oldest Workspace decide about the Instance */
  instanceManage: "instance:manage",
}
