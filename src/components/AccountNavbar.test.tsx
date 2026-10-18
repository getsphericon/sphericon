import { beforeEach, expect, test } from 'vitest'

import { handleSiteInstanceGetState } from '../generated/site/msw.gen.ts'
import { Route as profileRoute } from '../routes/account/index.tsx'
import { Route as instanceRoute } from '../routes/account/instance.tsx'
import { Route as indexRoute } from '../routes/index.tsx'
import { problem } from '../test/problem.ts'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { routeMount } from '../test/routeMount.ts'
import { worker } from '../test/worker.ts'
import { AccountNavbar } from './AccountNavbar.tsx'

// Most users may not manage the Instance: its state is refused to them.
beforeEach(() => {
  worker.use(handleSiteInstanceGetState(() => problem(403)))
})

test('an owner of the oldest workspace is offered the instance page', async () => {
  worker.use(
    handleSiteInstanceGetState({
      body: {
        telemetry: { enabled: true, locked: false },
        noticeAcknowledged: true,
        telemetrySchemaUrl: 'https://docs.example.test/telemetry-schema',
      },
    }),
  )
  const { screen } = await renderWithRouter(<AccountNavbar />)

  await expect
    .element(screen.getByRole('link', { name: 'Instance' }))
    .toHaveAttribute('href', routeMount(instanceRoute).initialPath)
})

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
