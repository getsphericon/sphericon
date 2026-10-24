import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSiteEventsList } from '../../../generated/site/msw.gen.ts'
import { Route as recentActivityRoute } from '../../../routes/workspaces/$workspaceId/analytics/recent-activity.tsx'
import { problem } from '../../../test/problem.ts'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { routeMount } from '../../../test/routeMount.ts'
import { worker } from '../../../test/worker.ts'
import { RecentActivityPage } from './recentActivity.tsx'

const WORKSPACE_ID = 'TestWs01'
const mount = routeMount(recentActivityRoute, { workspaceId: WORKSPACE_ID })

const event = (id: string, over: Record<string, unknown> = {}) => ({
  id,
  subjectId: `anon:${id}`,
  email: null,
  action: 'page_view',
  properties: {},
  occurredAt: '2026-01-02T00:00:00Z',
  createdAt: '2026-01-02T00:00:00Z',
  ...over,
})

interface Query {
  page: string | null
  action: string | null
}

function serveEvents(queries: Query[], totalItems = 1) {
  worker.use(
    handleSiteEventsList(({ request }) => {
      const params = new URL(request.url).searchParams
      queries.push({ page: params.get('page'), action: params.get('action') })
      return HttpResponse.json({
        items: [event('1')],
        page: Number(params.get('page') ?? 1),
        pageSize: 25,
        totalItems,
        totalPages: Math.ceil(totalItems / 25),
      })
    }),
  )
}

test('an event without an email shows its subject id', async () => {
  serveEvents([])
  const { screen } = await renderWithRouter(<RecentActivityPage />, mount)

  await expect.element(screen.getByText('anon:1')).toBeInTheDocument()
})

test('filtering by action refetches with the action and resets to page one', async () => {
  const queries: Query[] = []
  serveEvents(queries, 60)
  const { screen } = await renderWithRouter(<RecentActivityPage />, mount)

  await expect.element(screen.getByText('anon:1')).toBeInTheDocument()
  await screen.getByRole('button', { name: '2', exact: true }).click()
  await expect.poll(() => queries.some((q) => q.page === '2')).toBe(true)

  await screen.getByPlaceholder('Filter by action').fill('purchase')

  await expect.poll(() => queries.at(-1)).toEqual({ page: '1', action: 'purchase' })
})

test('the action search param is the starting filter', async () => {
  const queries: Query[] = []
  serveEvents(queries)
  const { screen } = await renderWithRouter(<RecentActivityPage />, {
    ...mount,
    initialPath: `${mount.initialPath}?action=purchase`,
  })

  await expect.element(screen.getByPlaceholder('Filter by action')).toHaveValue('purchase')
  await expect.poll(() => queries.at(-1)).toEqual({ page: '1', action: 'purchase' })
})

test('shows an error alert when the feed fails to load', async () => {
  worker.use(handleSiteEventsList(() => problem(500)))
  const { screen } = await renderWithRouter(<RecentActivityPage />, mount)

  await expect.element(screen.getByText('Failed to load activity').first()).toBeInTheDocument()
})
