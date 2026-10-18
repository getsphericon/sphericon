import { expect, test } from 'vitest'

import { Route as accountRoute } from '../routes/account/route.tsx'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { routeMount } from '../test/routeMount.ts'
import { AccountLayout } from './AccountLayout.tsx'

test('renders the brand, the account sidebar and the user menu', async () => {
  const { screen, navigate } = await renderWithRouter(<AccountLayout />, routeMount(accountRoute))

  await expect.element(screen.getByText('sphericon')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Toggle color scheme' })).toBeVisible()

  await expect.element(screen.getByText('Dashboard')).toBeInTheDocument()
  await expect.element(screen.getByText('Profile')).toBeInTheDocument()
  await expect.element(screen.getByText('Security')).toBeInTheDocument()
  expect(navigate).not.toHaveBeenCalled()
})
