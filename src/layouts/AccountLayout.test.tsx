import { beforeEach, expect, test } from 'vitest'

import { handleSiteInstanceGetState } from '../generated/site/msw.gen.ts'
import { Route as accountRoute } from '../routes/account/route.tsx'
import { problem } from '../test/problem.ts'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { routeMount } from '../test/routeMount.ts'
import { worker } from '../test/worker.ts'
import { AccountLayout } from './AccountLayout.tsx'

// Most users may not manage the Instance: its state is refused to them.
beforeEach(() => {
  worker.use(handleSiteInstanceGetState(() => problem(403)))
})

test('renders the brand, the account sidebar and the user menu', async () => {
  const { screen, navigate } = await renderWithRouter(<AccountLayout />, routeMount(accountRoute))

  await expect.element(screen.getByText('Sphericon')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Toggle color scheme' })).toBeVisible()

  await expect.element(screen.getByText('Dashboard')).toBeInTheDocument()
  await expect.element(screen.getByText('Profile')).toBeInTheDocument()
  await expect.element(screen.getByText('Security')).toBeInTheDocument()
  expect(navigate).not.toHaveBeenCalled()
})
