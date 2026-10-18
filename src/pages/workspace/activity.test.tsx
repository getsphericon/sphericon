import { expect, test } from 'vitest'

import { handleSiteEventsList } from '../../generated/site/msw.gen.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { ActivityPage } from './activity.tsx'

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

  const { screen } = await renderWithRouter(<ActivityPage />)

  await expect.element(screen.getByText('purchase')).toBeInTheDocument()
  await expect.element(screen.getByText('bob@example.com')).toBeInTheDocument()
  // Live mode is on by default.
  await expect.element(screen.getByLabelText('Live')).toBeChecked()
  await expect.element(screen.getByPlaceholder('Filter by action')).toBeInTheDocument()
})

test('expands a row to reveal the full event properties', async () => {
  worker.use(eventsPage())

  const { screen } = await renderWithRouter(<ActivityPage />)

  await screen.getByText('purchase').click()

  await expect.element(screen.getByText('user:bob@example.com')).toBeInTheDocument()
  await expect.element(screen.getByText(/"plan": "pro"/)).toBeInTheDocument()
})
