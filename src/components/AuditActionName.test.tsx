import { expect, test } from 'vitest'

import { renderWithProviders } from '../test/renderWithProviders.tsx'
import { AuditActionName, AuditActionOption } from './AuditActionName.tsx'

// Every action the backend records in a Workspace's Audit log (events.ExplicitAuditActions, and
// create, update and delete of each Audited entity) and in the Instance audit log
// (instanceaudit), with its words on screen.
const names: [string, string][] = [
  ['membership.update', "Changed a member's role"],
  ['membership.delete', 'Removed a member'],
  ['membership.domain_join', 'Added a member through a join domain'],
  ['invitation.create', 'Invited a person'],
  ['invitation.accept', 'Accepted an invitation'],
  ['invitation.revoke', 'Revoked an invitation'],
  ['user.login', 'Signed in'],
  ['user.identity_link', 'Linked a sign-in provider'],
  ['user.identity_unlink', 'Unlinked a sign-in provider'],
  ['user.email_verify', 'Verified their email'],
  ['user.password_change', 'Changed their password'],
  ['user.sign_out_everywhere', 'Signed out everywhere'],
  ['user.second_factor_enroll', 'Turned on two-factor authentication'],
  ['user.second_factor_disable', 'Turned off two-factor authentication'],
  ['user.second_factor_reset', 'Reset two-factor authentication'],
  ['user.recovery_codes_regenerate', 'Generated new recovery codes'],
  ['user.recovery_code_use', 'Used a recovery code'],
  ['workspace.create', 'Created the workspace'],
  ['workspace.update', 'Changed the workspace settings'],
  ['workspace.delete', 'Deleted the workspace'],
  ['workspace.restore', 'Restored the workspace'],
  ['workspace.suspend', 'Suspended the workspace'],
  ['workspace.unsuspend', 'Lifted the suspension of the workspace'],
  ['workspace.abuse_exempt', 'Exempted the workspace from the abuse detector'],
  ['workspace.abuse_unexempt', 'Ended the exemption from the abuse detector'],
  ['workspace.send_cap_change', 'Changed the Send cap'],
  ['contact.import', 'Imported contacts'],
  ['audit_log.export', 'Exported the audit log'],
  ['webhook_endpoint.test', 'Sent a test event to a webhook endpoint'],
  ['instance.default_send_cap_change', 'Changed the default Send cap'],
  ['api_token.create', 'Created an API token'],
  ['api_token.update', 'Changed an API token'],
  ['api_token.delete', 'Deleted an API token'],
  ['automation.create', 'Created an automation'],
  ['automation.update', 'Changed an automation'],
  ['automation.delete', 'Deleted an automation'],
  ['brand_kit.create', 'Created the brand kit'],
  ['brand_kit.update', 'Changed the brand kit'],
  ['brand_kit.delete', 'Deleted the brand kit'],
  ['broadcast.create', 'Created a broadcast'],
  ['broadcast.update', 'Changed a broadcast'],
  ['broadcast.delete', 'Deleted a broadcast'],
  ['contact.create', 'Created a contact'],
  ['contact.update', 'Changed a contact'],
  ['contact.delete', 'Deleted a contact'],
  ['custom_field.create', 'Created a custom field'],
  ['custom_field.update', 'Changed a custom field'],
  ['custom_field.delete', 'Deleted a custom field'],
  ['template.create', 'Created a template'],
  ['template.update', 'Changed a template'],
  ['template.delete', 'Deleted a template'],
  ['hold.create', 'Placed a Staff hold'],
  ['hold.update', 'Changed a Staff hold'],
  ['hold.delete', 'Lifted a Staff hold'],
  ['integration.create', 'Created an integration'],
  ['integration.update', 'Changed an integration'],
  ['integration.delete', 'Deleted an integration'],
  ['join_domain.create', 'Created a join domain'],
  ['join_domain.update', 'Changed a join domain'],
  ['join_domain.delete', 'Deleted a join domain'],
  ['rule.create', 'Created a rule'],
  ['rule.update', 'Changed a rule'],
  ['rule.delete', 'Deleted a rule'],
  ['segment.create', 'Created a segment'],
  ['segment.update', 'Changed a segment'],
  ['segment.delete', 'Deleted a segment'],
  ['sending_domain.create', 'Created a sending domain'],
  ['sending_domain.update', 'Changed a sending domain'],
  ['sending_domain.delete', 'Deleted a sending domain'],
  ['tag.create', 'Created a tag'],
  ['tag.update', 'Changed a tag'],
  ['tag.delete', 'Deleted a tag'],
  ['tracking_domain.create', 'Created a tracking domain'],
  ['tracking_domain.update', 'Changed a tracking domain'],
  ['tracking_domain.delete', 'Deleted a tracking domain'],
  ['webhook_endpoint.create', 'Created a webhook endpoint'],
  ['webhook_endpoint.update', 'Changed a webhook endpoint'],
  ['webhook_endpoint.delete', 'Deleted a webhook endpoint'],
]

test.each(names)('%s is shown as "%s"', async (action, name) => {
  const screen = await renderWithProviders(<AuditActionName action={action} />)

  await expect.element(screen.getByText(name, { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText(action)).not.toBeInTheDocument()
})

test('an action with no words yet is shown as its code', async () => {
  const screen = await renderWithProviders(<AuditActionName action="membership.transfer" />)

  await expect.element(screen.getByText('membership.transfer')).toBeInTheDocument()
})

test('an action keeps its code on hover', async () => {
  const screen = await renderWithProviders(<AuditActionName action="workspace.suspend" />)

  await screen.getByText('Suspended the workspace').hover()

  await expect.element(screen.getByText('workspace.suspend')).toBeInTheDocument()
})

test('an option names the action and shows the code it filters by', async () => {
  const screen = await renderWithProviders(<AuditActionOption action="tag.delete" />)

  await expect.element(screen.getByText('Deleted a tag')).toBeInTheDocument()
  await expect.element(screen.getByText('tag.delete')).toBeInTheDocument()
})

test('an option of an action with no words is its code alone', async () => {
  const screen = await renderWithProviders(<AuditActionOption action="membership.transfer" />)

  await expect.element(screen.getByText('membership.transfer')).toBeInTheDocument()
})
