import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteBroadcastsDelete,
  handleSiteBroadcastsList,
} from '../../generated/site/msw.gen.ts'
import { Route as broadcastsRoute } from '../../routes/workspaces/$workspaceId/broadcasts/index.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { BroadcastsListPage } from './list.tsx'

const LIST_ROUTE = routeMount(broadcastsRoute, { workspaceId: 'TestWs01' })

const STATS = { recipientsTotal: 120, openedCount: 45 }
const DRAFT = {
  id: '1',
  name: 'Spring launch',
  status: 'draft',
  stats: STATS,
}
const HELD = {
  id: '2',
  name: 'Autumn digest',
  status: 'sending',
  holdReason: 'unverified_domain',
  stats: STATS,
}

test('lists broadcasts with status, hold badge and counters', async () => {
  worker.use(
    handleSiteBroadcastsList(() => HttpResponse.json({ items: [DRAFT, HELD], totalItems: 2 })),
  )

  const { screen } = await renderWithRouter(<BroadcastsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Spring launch')).toBeInTheDocument()
  await expect.element(screen.getByText('Autumn digest')).toBeInTheDocument()
  await expect.element(screen.getByText('Draft')).toBeInTheDocument()
  await expect.element(screen.getByText('Sending', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('On hold')).toBeInTheDocument()
  await expect.element(screen.getByText('45').first()).toBeInTheDocument()
})

test('shows the empty state when there are no broadcasts', async () => {
  worker.use(handleSiteBroadcastsList(() => HttpResponse.json({ items: [], totalItems: 0 })))

  const { screen } = await renderWithRouter(<BroadcastsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Create your first broadcast.')).toBeInTheDocument()
})

test('shows an error alert when the list fails to load', async () => {
  worker.use(handleSiteBroadcastsList(() => problem(500)))

  const { screen } = await renderWithRouter(<BroadcastsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Failed to load broadcasts')).toBeInTheDocument()
})

test('New broadcast links to the create page', async () => {
  worker.use(handleSiteBroadcastsList(() => HttpResponse.json({ items: [], totalItems: 0 })))

  const { screen } = await renderWithRouter(<BroadcastsListPage />, LIST_ROUTE)

  await expect
    .element(screen.getByRole('link', { name: 'New broadcast' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/broadcasts/new')
})

test('Report and Edit link with the broadcast id; Edit is only enabled for drafts', async () => {
  worker.use(
    handleSiteBroadcastsList(() => HttpResponse.json({ items: [DRAFT, HELD], totalItems: 2 })),
  )

  const { screen } = await renderWithRouter(<BroadcastsListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('Spring launch')).toBeInTheDocument()

  await expect
    .element(screen.getByRole('link', { name: 'Edit' }).nth(1))
    .toHaveAttribute('data-disabled')
  await expect
    .element(screen.getByRole('link', { name: 'Report' }).first())
    .toHaveAttribute('href', '/workspaces/TestWs01/broadcasts/1/report')
  await expect
    .element(screen.getByRole('link', { name: 'Edit' }).first())
    .toHaveAttribute('href', '/workspaces/TestWs01/broadcasts/1/edit')
})

test('deleting a broadcast asks for confirmation, deletes it and refreshes the list', async () => {
  const deleted: string[] = []
  let listFetches = 0
  worker.use(
    handleSiteBroadcastsList(() => {
      listFetches++
      return HttpResponse.json({
        items: deleted.length > 0 ? [HELD] : [DRAFT, HELD],
        totalItems: 2,
      })
    }),
    handleSiteBroadcastsDelete(({ params }) => {
      deleted.push(params.id)
      return new HttpResponse(null, { status: 204 })
    }),
  )

  const { screen } = await renderWithRouter(<BroadcastsListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('Spring launch')).toBeInTheDocument()

  await screen.getByRole('button', { name: 'Delete' }).first().click()
  expect(deleted).toEqual([])
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete' }).click()

  await expect.poll(() => deleted).toEqual(['1'])
  await expect.element(screen.getByText('Broadcast deleted')).toBeInTheDocument()
  await expect.poll(() => listFetches).toBe(2)
  await expect.element(screen.getByText('Spring launch')).not.toBeInTheDocument()
})

test('changing the page requests page 2 of the broadcasts', async () => {
  const pages: (string | null)[] = []
  worker.use(
    handleSiteBroadcastsList(({ request }) => {
      pages.push(new URL(request.url).searchParams.get('page'))
      return HttpResponse.json({ items: [DRAFT], totalItems: 25 })
    }),
  )

  const { screen } = await renderWithRouter(<BroadcastsListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('Spring launch')).toBeInTheDocument()
  await screen.getByRole('button', { name: '2', exact: true }).click()

  await expect.poll(() => pages).toEqual(['1', '2'])
})

test('a failed delete shows the error toast and keeps the broadcast', async () => {
  worker.use(
    handleSiteBroadcastsList(() => HttpResponse.json({ items: [DRAFT], totalItems: 1 })),
    handleSiteBroadcastsDelete(() => problem(500)),
  )

  const { screen } = await renderWithRouter(<BroadcastsListPage />, LIST_ROUTE)
  await screen.getByRole('button', { name: 'Delete' }).first().click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete' }).click()

  await expect.element(screen.getByText('Failed to delete broadcast')).toBeInTheDocument()
  await expect.element(screen.getByText('Spring launch')).toBeInTheDocument()
})
