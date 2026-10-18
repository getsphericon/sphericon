import { expect, test } from 'vitest'

import { renderWithProviders } from '../../test/renderWithProviders.tsx'
import { BroadcastHoldAlert, BroadcastHoldBadge } from './BroadcastHold.tsx'

test.each([
  ['no_integration', /No email provider is configured/],
  ['unverified_domain', /sender domain is not verified/],
  ['workspace_suspended', /Sending is suspended for this workspace/],
])('the alert explains the %s hold', async (reason, explanation) => {
  const screen = await renderWithProviders(<BroadcastHoldAlert reason={reason} />)

  await expect.element(screen.getByText('Sending is on hold')).toBeInTheDocument()
  await expect.element(screen.getByText(explanation)).toBeInTheDocument()
  await expect
    .element(screen.getByText(/the remaining recipients are sent automatically/))
    .toBeInTheDocument()
})

test('the alert falls back to the generic explanation for an unknown reason', async () => {
  const screen = await renderWithProviders(<BroadcastHoldAlert reason="something_new" />)

  await expect.element(screen.getByText('Sending is on hold')).toBeInTheDocument()
  await expect.element(screen.getByText(/Sending is paused\./)).toBeInTheDocument()
  await expect.element(screen.getByText(/No email provider/)).not.toBeInTheDocument()
})

test('the badge marks a row as on hold', async () => {
  const screen = await renderWithProviders(<BroadcastHoldBadge />)

  await expect.element(screen.getByText('On hold')).toBeInTheDocument()
})
