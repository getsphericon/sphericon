import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSiteEventsActions, handleSiteEventsList } from '../../../generated/site/msw.gen.ts'
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
    handleSiteEventsActions({ body: { actions: ['email.opened', 'purchase'] } }),
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

test('picking a System event by its name puts its action in the URL, on the first page, in the same window', async () => {
  serveEvents([], 60)
  const { screen, navigate, router } = await renderWithRouter(<RecentActivityPage />, {
    ...mount,
    initialPath: `${mount.initialPath}?range=30d&page=2`,
  })

  await screen.getByPlaceholder('Filter by action').click()
  await screen.getByRole('option', { name: /Opened an email/ }).click()

  await expect.poll(() => navigate.mock.calls.at(-1)?.[0]).toMatchObject({ replace: true })
  const target = router.buildLocation(navigate.mock.calls.at(-1)![0])
  expect(target.search).toEqual({ range: '30d', action: 'email.opened' })
})

test('the page in the URL is the page of the feed requested', async () => {
  const queries: Query[] = []
  serveEvents(queries, 60)
  await renderWithRouter(<RecentActivityPage />, {
    ...mount,
    initialPath: `${mount.initialPath}?page=2&action=purchase`,
  })

  await expect.poll(() => queries.at(-1)).toEqual({ page: '2', action: 'purchase' })
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

test('an action in the URL the Workspace has not recorded stays the filter', async () => {
  const queries: Query[] = []
  serveEvents(queries)
  const { screen } = await renderWithRouter(<RecentActivityPage />, {
    ...mount,
    initialPath: `${mount.initialPath}?action=signup`,
  })

  await expect.element(screen.getByPlaceholder('Filter by action')).toHaveValue('signup')
  await expect.poll(() => queries.at(-1)).toEqual({ page: '1', action: 'signup' })
})

test('shows an error alert when the feed fails to load', async () => {
  worker.use(
    handleSiteEventsActions({ body: { actions: [] } }),
    handleSiteEventsList(() => problem(500)),
  )
  const { screen } = await renderWithRouter(<RecentActivityPage />, mount)

  await expect.element(screen.getByText('Failed to load activity').first()).toBeInTheDocument()
})
