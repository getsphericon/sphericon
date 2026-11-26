// The actions, targets and entries of a Workspace's Audit log: /site, /staff and /api show the
// same, and filter by action. An entry is a template, as the Staff console names one more kind of
// actor; the actor /site and /api share is in audit-log.tsp.

namespace Sphericon;

/** What an Audit entry says was done, as `<entity>.<verb>`: an action the server records on its
 * own, or the create, update or delete of an audited object */
enum AuditAction {
  /** Changed a member's role */
  membershipUpdate: "membership.update",

  /** Removed a member */
  membershipDelete: "membership.delete",

  /** Added a member through a join domain */
  membershipDomainJoin: "membership.domain_join",

  /** Invited a person */
  invitationCreate: "invitation.create",

  /** Accepted an invitation */
  invitationAccept: "invitation.accept",

  /** Revoked an invitation */
  invitationRevoke: "invitation.revoke",

  /** Signed in */
  userLogin: "user.login",

  /** Linked a sign-in provider */
  userIdentityLink: "user.identity_link",

  /** Unlinked a sign-in provider */
  userIdentityUnlink: "user.identity_unlink",

  /** Verified their email */
  userEmailVerify: "user.email_verify",

  /** Changed their password */
  userPasswordChange: "user.password_change",

  /** Signed out everywhere */
  userSignOutEverywhere: "user.sign_out_everywhere",

  /** Turned on two-factor authentication */
  userSecondFactorEnroll: "user.second_factor_enroll",

  /** Turned off two-factor authentication */
  userSecondFactorDisable: "user.second_factor_disable",

  /** Reset two-factor authentication */
  userSecondFactorReset: "user.second_factor_reset",

  /** Generated new recovery codes */
  userRecoveryCodesRegenerate: "user.recovery_codes_regenerate",

  /** Used a recovery code */
  userRecoveryCodeUse: "user.recovery_code_use",

  /** Created the workspace */
  workspaceCreate: "workspace.create",

  /** Changed the workspace settings */
  workspaceUpdate: "workspace.update",

  /** Deleted the workspace */
  workspaceDelete: "workspace.delete",

  /** Restored the workspace */
  workspaceRestore: "workspace.restore",

  /** Suspended the workspace */
  workspaceSuspend: "workspace.suspend",

  /** Lifted the suspension of the workspace */
  workspaceUnsuspend: "workspace.unsuspend",

  /** Exempted the workspace from the abuse detector */
  workspaceAbuseExempt: "workspace.abuse_exempt",

  /** Ended the exemption from the abuse detector */
  workspaceAbuseUnexempt: "workspace.abuse_unexempt",

  /** Changed the Send cap */
  workspaceSendCapChange: "workspace.send_cap_change",

  /** Changed the sending rules: the Frequency caps and the Sending windows */
  workspaceSendingRulesChange: "workspace.sending_rules_change",

  /** Imported contacts */
  contactImport: "contact.import",

  /** Exported the audit log */
  auditLogExport: "audit_log.export",

  /** Sent a test event to a webhook endpoint */
  webhookEndpointTest: "webhook_endpoint.test",

  /** Created an API token */
  apiTokenCreate: "api_token.create",

  /** Changed an API token */
  apiTokenUpdate: "api_token.update",

  /** Deleted an API token */
  apiTokenDelete: "api_token.delete",

  /** Created an automation */
  automationCreate: "automation.create",

  /** Changed an automation */
  automationUpdate: "automation.update",

  /** Deleted an automation */
  automationDelete: "automation.delete",

  /** Archived an automation */
  automationArchive: "automation.archive",

  /** Unarchived an automation */
  automationUnarchive: "automation.unarchive",

  /** Created the brand kit */
  brandKitCreate: "brand_kit.create",

  /** Changed the brand kit */
  brandKitUpdate: "brand_kit.update",

  /** Deleted the brand kit */
  brandKitDelete: "brand_kit.delete",

  /** Created a broadcast */
  broadcastCreate: "broadcast.create",

  /** Changed a broadcast */
  broadcastUpdate: "broadcast.update",

  /** Deleted a broadcast */
  broadcastDelete: "broadcast.delete",

  /** Archived a broadcast */
  broadcastArchive: "broadcast.archive",

  /** Unarchived a broadcast */
  broadcastUnarchive: "broadcast.unarchive",

  /** Created a contact */
  contactCreate: "contact.create",

  /** Changed a contact */
  contactUpdate: "contact.update",

  /** Deleted a contact */
  contactDelete: "contact.delete",

  /** Created a custom field */
  customFieldCreate: "custom_field.create",

  /** Changed a custom field */
  customFieldUpdate: "custom_field.update",

  /** Deleted a custom field */
  customFieldDelete: "custom_field.delete",

  /** Created a template */
  templateCreate: "template.create",

  /** Changed a template */
  templateUpdate: "template.update",

  /** Deleted a template */
  templateDelete: "template.delete",

  /** Placed a Staff hold */
  holdCreate: "hold.create",

  /** Changed a Staff hold */
  holdUpdate: "hold.update",

  /** Lifted a Staff hold */
  holdDelete: "hold.delete",

  /** Generated the identity secret */
  identitySecretCreate: "identity_secret.create",

  /** Regenerated the identity secret */
  identitySecretUpdate: "identity_secret.update",

  /** Deleted the identity secret */
  identitySecretDelete: "identity_secret.delete",

  /** Created an integration */
  integrationCreate: "integration.create",

  /** Changed an integration */
  integrationUpdate: "integration.update",

  /** Deleted an integration */
  integrationDelete: "integration.delete",

  /** Deleted an integration for good (it stays as history) */
  integrationDiscard: "integration.discard",

  /** Created a join domain */
  joinDomainCreate: "join_domain.create",

  /** Changed a join domain */
  joinDomainUpdate: "join_domain.update",

  /** Deleted a join domain */
  joinDomainDelete: "join_domain.delete",

  /** Created a rule */
  ruleCreate: "rule.create",

  /** Changed a rule */
  ruleUpdate: "rule.update",

  /** Deleted a rule */
  ruleDelete: "rule.delete",

  /** Deleted a rule for good (its firings stay as history) */
  ruleDiscard: "rule.discard",

  /** Created a segment */
  segmentCreate: "segment.create",

  /** Changed a segment */
  segmentUpdate: "segment.update",

  /** Deleted a segment */
  segmentDelete: "segment.delete",

  /** Archived a segment */
  segmentArchive: "segment.archive",

  /** Unarchived a segment */
  segmentUnarchive: "segment.unarchive",

  /** Created a sending domain */
  sendingDomainCreate: "sending_domain.create",

  /** Changed a sending domain */
  sendingDomainUpdate: "sending_domain.update",

  /** Deleted a sending domain */
  sendingDomainDelete: "sending_domain.delete",

  /** Deleted a sending domain for good (it stays as history) */
  sendingDomainDiscard: "sending_domain.discard",

  /** Created a tag */
  tagCreate: "tag.create",

  /** Changed a tag */
  tagUpdate: "tag.update",

  /** Deleted a tag */
  tagDelete: "tag.delete",

  /** Created a tracking domain */
  trackingDomainCreate: "tracking_domain.create",

  /** Changed a tracking domain */
  trackingDomainUpdate: "tracking_domain.update",

  /** Deleted a tracking domain */
  trackingDomainDelete: "tracking_domain.delete",

  /** Created a webhook endpoint */
  webhookEndpointCreate: "webhook_endpoint.create",

  /** Changed a webhook endpoint */
  webhookEndpointUpdate: "webhook_endpoint.update",

  /** Deleted a webhook endpoint */
  webhookEndpointDelete: "webhook_endpoint.delete",
}

/** What an audited change was done to */
model AuditTarget {
  /** Entity type, e.g. membership */
  type: string;

  /** The target's id */
  id?: string | null;

  /** Name snapshot taken when the entry was written; absent for a Contact */
  name?: string | null;
}

/** One immutable Audit entry, by the actor model of the surface that shows it */
model AuditEntryOf<Actor> {
  /** Unique identifier */
  id: EntityId;

  /** When the change happened */
  occurredAt: Timestamp;

  /** Who did it */
  actor: Actor;

  /** What was done */
  action: AuditAction;

  /** What it was done to */
  target: AuditTarget;

  /** Before/after of the changed fields; secrets appear as changed only */
  diff?: JsonObject | null;

  /** Request id of the change */
  requestId?: string | null;

  /** Client address of the change */
  ip?: string | null;

  /** Client user agent of the change */
  userAgent?: string | null;
}
