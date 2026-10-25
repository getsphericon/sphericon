import { expect, test } from 'vitest'

import { handleSiteEventsList } from '../../../generated/site/msw.gen.ts'
import { Route as recentActivityRoute } from '../../../routes/workspaces/$workspaceId/analytics/recent-activity.tsx'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { routeMount } from '../../../test/routeMount.ts'
import { worker } from '../../../test/worker.ts'
import { RecentActivityPage } from './recentActivity.tsx'

const mount = routeMount(recentActivityRoute, { workspaceId: 'TestWs01' })

const eventsPage = () =>
  handleSiteEventsList({
    body: {
      items: [
        {
          id: '2',
          subjectId: 'user:bob@example.com',
          email: 'bob@example.com',
          action: 'purchase',
          properties: { plan: 'pro', amount: 42 },
          occurredAt: '2026-01-02T00:00:00Z',
          createdAt: '2026-01-02T00:00:00Z',
        },
      ],
      page: 1,
      pageSize: 25,
      totalItems: 1,
      totalPages: 1,
    },
  })

test('renders the event feed with a live toggle and action filter', async () => {
  worker.use(eventsPage())

  const { screen } = await renderWithRouter(<RecentActivityPage />, mount)

  await expect.element(screen.getByText('purchase')).toBeInTheDocument()
  await expect.element(screen.getByText('bob@example.com')).toBeInTheDocument()
  // Live mode is on by default.
  await expect.element(screen.getByLabelText('Live')).toBeChecked()
  await expect.element(screen.getByPlaceholder('Filter by action')).toBeInTheDocument()
})

test('expands a row to reveal the full event properties', async () => {
  worker.use(eventsPage())

  const { screen } = await renderWithRouter(<RecentActivityPage />, mount)

  await screen.getByText('purchase').click()

  await expect.element(screen.getByText('user:bob@example.com')).toBeInTheDocument()
  await expect.element(screen.getByText(/"plan": "pro"/)).toBeInTheDocument()
})

test('says an email event of a send through a Test inbox was captured, not delivered', async () => {
  worker.use(
    handleSiteEventsList({
      body: {
        items: [
          {
            id: '3',
            subjectId: 'user:cy@example.com',
            email: 'cy@example.com',
            action: 'email.delivered',
            properties: { captured: true },
            createdAt: '2026-01-03T00:00:00Z',
          },
        ],
        page: 1,
        pageSize: 25,
        totalItems: 1,
        totalPages: 1,
      },
    }),
  )

  const { screen } = await renderWithRouter(<RecentActivityPage />, mount)

  await expect.element(screen.getByText('email.delivered')).toBeInTheDocument()
  await expect.element(screen.getByText('Captured, not delivered')).toBeInTheDocument()
})
