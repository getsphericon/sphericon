import { expect, test } from 'vitest'

import { Route as unsubscribedRoute } from '../routes/unsubscribed.tsx'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { routeMount } from '../test/routeMount.ts'
import { UnsubscribedPage } from './unsubscribed.tsx'

const mount = routeMount(unsubscribedRoute)

test('confirms the opt-out without an escalation link by default', async () => {
  const { screen } = await renderWithRouter(<UnsubscribedPage />, mount)

  await expect.element(screen.getByText('Unsubscribed')).toBeInTheDocument()
  await expect
    .element(screen.getByText('You will no longer receive these emails.'))
    .toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Unsubscribe from all emails' }))
    .not.toBeInTheDocument()
})

test('offers the unsubscribe-from-all link the backend supplied', async () => {
  const all = 'https://example.com/e/u/all-token'
  const { screen } = await renderWithRouter(<UnsubscribedPage />, {
    ...mount,
    initialPath: `${mount.initialPath}?all=${encodeURIComponent(all)}`,
  })

  await expect
    .element(screen.getByRole('link', { name: 'Unsubscribe from all emails' }))
    .toHaveAttribute('href', all)
})
