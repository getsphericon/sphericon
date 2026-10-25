import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteIntegrationsCreate,
  handleSiteSendingDomainsList,
  handleSiteSetupChecklistGet,
} from '../../generated/site/msw.gen.ts'
import { Route as overviewRoute } from '../../routes/workspaces/$workspaceId/index.tsx'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { OverviewPage } from './overview.tsx'

const finished = {
  steps: {
    integration: 'done',
    testInbox: 'todo',
    sendingDomain: 'done',
    contact: 'done',
    sentEmail: 'done',
  },
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
          testInbox: 'done',
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

test('the Test inbox step creates the inbox without making it the default, and says so', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteIntegrationsCreate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({}, { status: 201 })
    }),
    handleSiteSetupChecklistGet({
      body: {
        steps: {
          integration: 'todo',
          testInbox: 'todo',
          sendingDomain: 'todo',
          contact: 'todo',
          sentEmail: 'todo',
        },
        complete: false,
      },
    }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('Try it with a Test inbox')).toBeInTheDocument()
  await expect.element(screen.getByText(/never made your default/)).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Create a Test inbox' }).click()

  await expect
    .poll(() => bodies)
    .toMatchObject([{ name: 'Test inbox', isDefault: false, config: { kind: 'inbox' } }])
})

test('a done Test inbox step offers no action', async () => {
  worker.use(
    handleSiteSetupChecklistGet({
      body: {
        steps: {
          integration: 'done',
          testInbox: 'done',
          sendingDomain: 'todo',
          contact: 'todo',
          sentEmail: 'todo',
        },
        complete: false,
      },
    }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('Try it with a Test inbox')).toBeInTheDocument()
  await expect
    .element(screen.getByRole('button', { name: 'Create a Test inbox' }))
    .not.toBeInTheDocument()
})
