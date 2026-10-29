import { HttpResponse } from 'msw'
import { beforeEach, expect, test } from 'vitest'

import {
  handleSiteBroadcastsList,
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
    sendingDomain: 'done',
    contact: 'done',
    sentEmail: 'done',
  },
  complete: true,
} as const

const mount = routeMount(overviewRoute, { workspaceId: 'TestWs01' })

beforeEach(() => {
  worker.use(handleSiteBroadcastsList(() => HttpResponse.json({ items: [], totalItems: 0 })))
})

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

test('a finished checklist leaves the shortcuts and the recent broadcasts', async () => {
  worker.use(
    handleSiteSetupChecklistGet({ body: finished }),
    handleSiteBroadcastsList(() =>
      HttpResponse.json({
        items: [
          {
            id: '1',
            name: 'Spring launch',
            status: 'sent',
            createdAt: '2026-10-27T08:09:00Z',
            stats: { recipientsTotal: 100, sentCount: 100, openedCount: 38 },
          },
        ],
        totalItems: 1,
      }),
    ),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('Create a new…')).toBeInTheDocument()
  await expect.element(screen.getByText('Spring launch')).toBeInTheDocument()
  await expect.element(screen.getByText('38%')).toBeInTheDocument()
})

test('the contact step offers every way to add one and no single page', async () => {
  worker.use(
    handleSiteSetupChecklistGet({
      body: {
        steps: {
          integration: 'done',
          sendingDomain: 'done',
          contact: 'todo',
          sentEmail: 'todo',
        },
        complete: false,
      },
    }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('Add your first Contact')).toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Install the tracker' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/settings/tracking')
  await expect
    .element(screen.getByRole('link', { name: 'Use the API' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/settings/api-keys')
  await expect
    .element(screen.getByRole('link', { name: 'Add one by hand' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/contacts')
})

test('a done contact step offers no action', async () => {
  worker.use(
    handleSiteSetupChecklistGet({
      body: {
        steps: {
          integration: 'todo',
          sendingDomain: 'todo',
          contact: 'done',
          sentEmail: 'todo',
        },
        complete: false,
      },
    }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('Add your first Contact')).toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Install the tracker' }))
    .not.toBeInTheDocument()
})
