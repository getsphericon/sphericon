import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSiteSegmentsDelete, handleSiteSegmentsList } from '../../generated/site/msw.gen.ts'
import { Route as segmentsRoute } from '../../routes/workspaces/$slug/segments/index.tsx'
import { page, TIMESTAMPS } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { SegmentsListPage } from './list.tsx'

const SLUG = { slug: 'test' }
const LIST_ROUTE = routeMount(segmentsRoute, SLUG)

const VIPS = { id: '1', name: 'VIP customers', ...TIMESTAMPS }
const TRIAL = { id: '2', name: 'Trial users', ...TIMESTAMPS }

test('lists the workspace segments', async () => {
  worker.use(handleSiteSegmentsList({ body: page([VIPS, TRIAL], 2) }))

  const { screen } = await renderWithRouter(<SegmentsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('VIP customers')).toBeInTheDocument()
  await expect.element(screen.getByText('Trial users')).toBeInTheDocument()
})

test('shows the empty state when there are no segments', async () => {
  worker.use(handleSiteSegmentsList({ body: page([], 0) }))

  const { screen } = await renderWithRouter(<SegmentsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Create your first segment.')).toBeInTheDocument()
})

test('shows an error alert when the list fails to load', async () => {
  worker.use(handleSiteSegmentsList(() => problem(500)))

  const { screen } = await renderWithRouter(<SegmentsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Failed to load segments')).toBeInTheDocument()
})

test('Add segment links to the create page', async () => {
  worker.use(handleSiteSegmentsList({ body: page([], 0) }))

  const { screen } = await renderWithRouter(<SegmentsListPage />, LIST_ROUTE)

  await expect
    .element(screen.getByRole('link', { name: 'Add segment' }))
    .toHaveAttribute('href', '/workspaces/test/segments/new')
})

test('Edit links to the segment edit page', async () => {
  worker.use(handleSiteSegmentsList({ body: page([VIPS], 1) }))

  const { screen } = await renderWithRouter(<SegmentsListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('VIP customers')).toBeInTheDocument()

  await expect
    .element(screen.getByRole('link', { name: 'Edit' }))
    .toHaveAttribute('href', '/workspaces/test/segments/1/edit')
})

test('deleting a segment asks for confirmation, deletes it and refreshes the list', async () => {
  const deleted: string[] = []
  let listFetches = 0
  worker.use(
    handleSiteSegmentsList(() => {
      listFetches++
      return HttpResponse.json({
        items: deleted.length ? [TRIAL] : [VIPS, TRIAL],
        totalItems: 2,
      })
    }),
    handleSiteSegmentsDelete(({ params }) => {
      deleted.push(params.id)
      return new HttpResponse(null, { status: 204 })
    }),
  )

  const { screen } = await renderWithRouter(<SegmentsListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('VIP customers')).toBeInTheDocument()

  await screen.getByRole('button', { name: 'Delete' }).first().click()
  expect(deleted).toEqual([])
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete' }).click()

  await expect.poll(() => deleted).toEqual(['1'])
  await expect.element(screen.getByText('Segment deleted')).toBeInTheDocument()
  await expect.poll(() => listFetches).toBe(2)
  await expect.element(screen.getByText('VIP customers')).not.toBeInTheDocument()
})
