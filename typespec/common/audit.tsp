// The actions of a Workspace's Audit log: /site, /staff and /api show the same, and filter by them.

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
  automationDiscard: "automation.discard",

  /** Restored an automation */
  automationRestore: "automation.restore",

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
  broadcastDiscard: "broadcast.discard",

  /** Restored a broadcast */
  broadcastRestore: "broadcast.restore",

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

  /** Created an integration */
  integrationCreate: "integration.create",

  /** Changed an integration */
  integrationUpdate: "integration.update",

  /** Deleted an integration */
  integrationDelete: "integration.delete",

  /** Archived an integration */
  integrationDiscard: "integration.discard",

  /** Restored an integration */
  integrationRestore: "integration.restore",

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

  /** Archived a rule */
  ruleDiscard: "rule.discard",

  /** Restored a rule */
  ruleRestore: "rule.restore",

  /** Created a segment */
  segmentCreate: "segment.create",

  /** Changed a segment */
  segmentUpdate: "segment.update",

  /** Deleted a segment */
  segmentDelete: "segment.delete",

  /** Archived a segment */
  segmentDiscard: "segment.discard",

  /** Restored a segment */
  segmentRestore: "segment.restore",

  /** Created a sending domain */
  sendingDomainCreate: "sending_domain.create",

  /** Changed a sending domain */
  sendingDomainUpdate: "sending_domain.update",

  /** Deleted a sending domain */
  sendingDomainDelete: "sending_domain.delete",

  /** Archived a sending domain */
  sendingDomainDiscard: "sending_domain.discard",

  /** Restored a sending domain */
  sendingDomainRestore: "sending_domain.restore",

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
