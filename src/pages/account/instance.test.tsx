import { expect, test } from 'vitest'

import { handleSiteInstanceGetState } from '../../generated/site/msw.gen.ts'
import { Route as instanceRoute } from '../../routes/account/instance.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { InstancePage } from './instance.tsx'

const renderPage = () => renderWithRouter(<InstancePage />, routeMount(instanceRoute))

test('an owner of the oldest workspace sees the usage report setting', async () => {
  worker.use(
    handleSiteInstanceGetState({
      body: {
        telemetry: { enabled: true, locked: false },
        noticeAcknowledged: true,
        telemetrySchemaUrl: 'https://docs.example.test/telemetry-schema',
      },
    }),
  )
  const { screen } = await renderPage()

  await expect
    .element(screen.getByRole('switch', { name: 'Send the anonymous usage report' }))
    .toBeVisible()
})

test('anyone else is told only the owners of the oldest workspace can manage the instance', async () => {
  worker.use(handleSiteInstanceGetState(() => problem(403, 'Forbidden')))
  const { screen } = await renderPage()

  await expect
    .element(screen.getByText('Only the owners of the oldest workspace can manage the instance.'))
    .toBeVisible()
})
