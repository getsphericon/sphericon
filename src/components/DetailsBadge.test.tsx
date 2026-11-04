import { expect, test } from 'vitest'
import { userEvent } from 'vitest/browser'

import { renderWithProviders } from '../test/renderWithProviders.tsx'
import { DetailsBadge } from './DetailsBadge.tsx'

test('opens the details when the badge is chosen and closes them on Escape', async () => {
  const screen = await renderWithProviders(
    <DetailsBadge details={['Receiver answered 410 Gone', null]}>{'Disabled'}</DetailsBadge>,
  )

  await expect.element(screen.getByText('Receiver answered 410 Gone')).not.toBeInTheDocument()
  await screen.getByRole('button', { name: 'Disabled' }).click()
  await expect.element(screen.getByText('Receiver answered 410 Gone')).toBeInTheDocument()

  await userEvent.keyboard('{Escape}')
  await expect.element(screen.getByText('Receiver answered 410 Gone')).not.toBeInTheDocument()
})

test('is a plain badge when there are no details', async () => {
  const screen = await renderWithProviders(
    <DetailsBadge details={[null, undefined, '']}>{'Enabled'}</DetailsBadge>,
  )

  await expect.element(screen.getByText('Enabled')).toBeInTheDocument()
  await expect.element(screen.getByRole('button')).not.toBeInTheDocument()
})
