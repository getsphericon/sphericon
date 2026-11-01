import type { OptionsFilter } from '@mantine/core'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { actionFilter } from '../utils/actionFilter.ts'

// useAuditActionLabel names the action of an Audit entry, in a Workspace's Audit log or in the
// Instance audit log, by its words: what the actor did. The code stays what is stored, filtered on,
// sent and exported; an action that has no words yet is shown as its code.
export function useAuditActionLabel() {
  const { t } = useTranslation()

  return useMemo(() => {
    const words: Record<string, string> = {
      'membership.update': t(($) => $.auditActions.membershipUpdate),
      'membership.delete': t(($) => $.auditActions.membershipDelete),
      'membership.domain_join': t(($) => $.auditActions.membershipDomainJoin),
      'invitation.create': t(($) => $.auditActions.invitationCreate),
      'invitation.accept': t(($) => $.auditActions.invitationAccept),
      'invitation.revoke': t(($) => $.auditActions.invitationRevoke),
      'user.login': t(($) => $.auditActions.userLogin),
      'user.identity_link': t(($) => $.auditActions.userIdentityLink),
      'user.identity_unlink': t(($) => $.auditActions.userIdentityUnlink),
      'user.email_verify': t(($) => $.auditActions.userEmailVerify),
      'user.password_change': t(($) => $.auditActions.userPasswordChange),
      'user.sign_out_everywhere': t(($) => $.auditActions.userSignOutEverywhere),
      'user.second_factor_enroll': t(($) => $.auditActions.userSecondFactorEnroll),
      'user.second_factor_disable': t(($) => $.auditActions.userSecondFactorDisable),
      'user.second_factor_reset': t(($) => $.auditActions.userSecondFactorReset),
      'user.recovery_codes_regenerate': t(($) => $.auditActions.userRecoveryCodesRegenerate),
      'user.recovery_code_use': t(($) => $.auditActions.userRecoveryCodeUse),
      'workspace.create': t(($) => $.auditActions.workspaceCreate),
      'workspace.update': t(($) => $.auditActions.workspaceUpdate),
      'workspace.delete': t(($) => $.auditActions.workspaceDelete),
      'workspace.restore': t(($) => $.auditActions.workspaceRestore),
      'workspace.suspend': t(($) => $.auditActions.workspaceSuspend),
      'workspace.unsuspend': t(($) => $.auditActions.workspaceUnsuspend),
      'workspace.abuse_exempt': t(($) => $.auditActions.workspaceAbuseExempt),
      'workspace.abuse_unexempt': t(($) => $.auditActions.workspaceAbuseUnexempt),
      'workspace.send_cap_change': t(($) => $.auditActions.workspaceSendCapChange),
      'contact.import': t(($) => $.auditActions.contactImport),
      'audit_log.export': t(($) => $.auditActions.auditLogExport),
      'webhook_endpoint.test': t(($) => $.auditActions.webhookEndpointTest),
      'instance.default_send_cap_change': t(($) => $.auditActions.instanceDefaultSendCapChange),
      // The create, update and delete of each Audited entity (ent/schema, Audited.Action).
      'api_token.create': t(($) => $.auditActions.apiTokenCreate),
      'api_token.update': t(($) => $.auditActions.apiTokenUpdate),
      'api_token.delete': t(($) => $.auditActions.apiTokenDelete),
      'automation.create': t(($) => $.auditActions.automationCreate),
      'automation.update': t(($) => $.auditActions.automationUpdate),
      'automation.delete': t(($) => $.auditActions.automationDelete),
      'brand_kit.create': t(($) => $.auditActions.brandKitCreate),
      'brand_kit.update': t(($) => $.auditActions.brandKitUpdate),
      'brand_kit.delete': t(($) => $.auditActions.brandKitDelete),
      'broadcast.create': t(($) => $.auditActions.broadcastCreate),
      'broadcast.update': t(($) => $.auditActions.broadcastUpdate),
      'broadcast.delete': t(($) => $.auditActions.broadcastDelete),
      'contact.create': t(($) => $.auditActions.contactCreate),
      'contact.update': t(($) => $.auditActions.contactUpdate),
      'contact.delete': t(($) => $.auditActions.contactDelete),
      'custom_field.create': t(($) => $.auditActions.customFieldCreate),
      'custom_field.update': t(($) => $.auditActions.customFieldUpdate),
      'custom_field.delete': t(($) => $.auditActions.customFieldDelete),
      'template.create': t(($) => $.auditActions.templateCreate),
      'template.update': t(($) => $.auditActions.templateUpdate),
      'template.delete': t(($) => $.auditActions.templateDelete),
      'hold.create': t(($) => $.auditActions.holdCreate),
      'hold.update': t(($) => $.auditActions.holdUpdate),
      'hold.delete': t(($) => $.auditActions.holdDelete),
      'integration.create': t(($) => $.auditActions.integrationCreate),
      'integration.update': t(($) => $.auditActions.integrationUpdate),
      'integration.delete': t(($) => $.auditActions.integrationDelete),
      'join_domain.create': t(($) => $.auditActions.joinDomainCreate),
      'join_domain.update': t(($) => $.auditActions.joinDomainUpdate),
      'join_domain.delete': t(($) => $.auditActions.joinDomainDelete),
      'rule.create': t(($) => $.auditActions.ruleCreate),
      'rule.update': t(($) => $.auditActions.ruleUpdate),
      'rule.delete': t(($) => $.auditActions.ruleDelete),
      'segment.create': t(($) => $.auditActions.segmentCreate),
      'segment.update': t(($) => $.auditActions.segmentUpdate),
      'segment.delete': t(($) => $.auditActions.segmentDelete),
      'sending_domain.create': t(($) => $.auditActions.sendingDomainCreate),
      'sending_domain.update': t(($) => $.auditActions.sendingDomainUpdate),
      'sending_domain.delete': t(($) => $.auditActions.sendingDomainDelete),
      'tag.create': t(($) => $.auditActions.tagCreate),
      'tag.update': t(($) => $.auditActions.tagUpdate),
      'tag.delete': t(($) => $.auditActions.tagDelete),
      'tracking_domain.create': t(($) => $.auditActions.trackingDomainCreate),
      'tracking_domain.update': t(($) => $.auditActions.trackingDomainUpdate),
      'tracking_domain.delete': t(($) => $.auditActions.trackingDomainDelete),
      'webhook_endpoint.create': t(($) => $.auditActions.webhookEndpointCreate),
      'webhook_endpoint.update': t(($) => $.auditActions.webhookEndpointUpdate),
      'webhook_endpoint.delete': t(($) => $.auditActions.webhookEndpointDelete),
    }
    return (action: string): string => words[action] ?? action
  }, [t])
}

// useAuditActionFilter is the search of a picker of audit actions: an option matches by its words
// or by its code.
export function useAuditActionFilter(): OptionsFilter {
  const labelOf = useAuditActionLabel()
  return useMemo(() => actionFilter(labelOf), [labelOf])
}
