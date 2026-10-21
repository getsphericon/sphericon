import { expect, test } from 'vitest'

import {
  handleSiteSendingDomainsList,
  handleSiteSetupChecklistGet,
} from '../../generated/site/msw.gen.ts'
import { Route as overviewRoute } from '../../routes/workspaces/$workspaceId/index.tsx'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { OverviewPage } from './overview.tsx'

const finished = {
  steps: { integration: 'done', sendingDomain: 'done', contact: 'done', sentEmail: 'done' },
  complete: true,
} as const

const mount = routeMount(overviewRoute, { workspaceId: 'TestWs01' })

test('an unfinished checklist is shown with its steps', async () => {
  worker.use(
    handleSiteSendingDomainsList({
      body: { items: [], page: 1, pageSize: 20, totalItems: 0, totalPages: 1 },
    }),
    handleSiteSetupChecklistGet({
      body: {
        steps: {
          integration: 'done',
          sendingDomain: 'pending',
          contact: 'todo',
          sentEmail: 'todo',
        },
        complete: false,
      },
    }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('Set up Sphericon')).toBeInTheDocument()
  await expect.element(screen.getByText('Waiting for DNS')).toBeInTheDocument()
})

test('a finished checklist is not shown', async () => {
  worker.use(handleSiteSetupChecklistGet({ body: finished }))

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('Set up Sphericon')).not.toBeInTheDocument()
})
