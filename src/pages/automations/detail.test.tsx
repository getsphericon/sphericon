import { expect, test } from 'vitest'

import {
  handleSiteAutomationsGet,
  handleSiteEnrollmentsList,
} from '../../generated/site/msw.gen.ts'
import { Route as automationsDetailRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/index.tsx'
import { page, TIMESTAMPS } from '../../test/payloads.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { AutomationDetailPage } from './detail.tsx'

const MOUNT = routeMount(automationsDetailRoute, { workspaceId: 'TestWs01', automationId: '1' })

test('shows the automation and the contacts enrolled in it', async () => {
  worker.use(
    handleSiteAutomationsGet({
      body: {
        id: '1',
        name: 'Welcome series',
        status: 'active',
        triggerEvent: 'contact.created',
        steps: [],
        ...TIMESTAMPS,
      },
    }),
    handleSiteEnrollmentsList({
      body: page(
        [
          {
            id: '5',
            automationId: '1',
            contactId: '42',
            status: 'completed',
            currentStep: 1,
            ...TIMESTAMPS,
          },
        ],
        1,
      ),
    }),
  )

  const { screen } = await renderWithRouter(<AutomationDetailPage />, MOUNT)

  await expect.element(screen.getByText('Welcome series')).toBeInTheDocument()
  await expect.element(screen.getByText('42')).toBeInTheDocument()
  await expect.element(screen.getByText('Completed')).toBeInTheDocument()
})
