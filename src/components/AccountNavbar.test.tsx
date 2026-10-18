import { expect, test } from 'vitest'

import { Route as profileRoute } from '../routes/account/index.tsx'
import { Route as indexRoute } from '../routes/index.tsx'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { routeMount } from '../test/routeMount.ts'
import { AccountNavbar } from './AccountNavbar.tsx'

test('the back link points at the dashboard', async () => {
  const { screen } = await renderWithRouter(<AccountNavbar />)

  await expect
    .element(screen.getByRole('link', { name: 'Dashboard' }))
    .toHaveAttribute('href', routeMount(indexRoute).initialPath)
})

test('the profile link points at the profile page', async () => {
  const { screen } = await renderWithRouter(<AccountNavbar />)

  await expect
    .element(screen.getByRole('link', { name: 'Profile' }))
    .toHaveAttribute('href', routeMount(profileRoute).initialPath)
})
