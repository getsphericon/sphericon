import { expect, test } from 'vitest'

import { Route as profileRoute } from '../routes/account/index.tsx'
import { Route as indexRoute } from '../routes/index.tsx'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { AccountNavbar } from './AccountNavbar.tsx'

test('the back link navigates to the dashboard', async () => {
  const { screen, navigate } = await renderWithRouter(<AccountNavbar />)

  await screen.getByText('Dashboard').click()

  expect(navigate).toHaveBeenCalledWith({ to: indexRoute.to })
})

test('the profile link navigates to the profile page', async () => {
  const { screen, navigate } = await renderWithRouter(<AccountNavbar />)

  await screen.getByText('Profile').click()

  expect(navigate).toHaveBeenCalledWith({ to: profileRoute.to })
})
