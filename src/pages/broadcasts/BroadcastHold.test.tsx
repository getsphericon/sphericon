import { expect, test } from 'vitest'

import { renderWithProviders } from '../../test/renderWithProviders.tsx'
import { BroadcastHoldAlert, BroadcastHoldBadge } from './BroadcastHold.tsx'

test.each([
  ['no_integration', /No email provider is configured/],
  ['unverified_domain', /sender domain is not verified/],
  ['workspace_suspended', /Sending is suspended for this workspace/],
  ['provider_stopped', /email provider has stopped sending from your account/],
  ['staff_hold', /Contact an Owner or Admin of this workspace/],
  ['workspace_pending_deletion', /Restore the workspace to resume/],
] as const)('the alert explains the %s hold', async (reason, explanation) => {
  const screen = await renderWithProviders(<BroadcastHoldAlert reason={reason} />)

  await expect.element(screen.getByText('Sending is on hold')).toBeInTheDocument()
  await expect.element(screen.getByText(explanation)).toBeInTheDocument()
  await expect
    .element(screen.getByText(/the remaining recipients are sent automatically/))
    .toBeInTheDocument()
})

test('the badge marks a row as on hold', async () => {
  const screen = await renderWithProviders(<BroadcastHoldBadge />)

  await expect.element(screen.getByText('On hold')).toBeInTheDocument()
})
