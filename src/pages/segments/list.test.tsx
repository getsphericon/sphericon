import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteSegmentsArchive,
  handleSiteSegmentsList,
  handleSiteSegmentsUnarchive,
} from '../../generated/site/msw.gen.ts'
import { Route as segmentsRoute } from '../../routes/workspaces/$workspaceId/segments/index.tsx'
import { page, TIMESTAMPS } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { SegmentsListPage } from './list.tsx'

const WORKSPACE_ID = { workspaceId: 'TestWs01' }
const LIST_ROUTE = routeMount(segmentsRoute, WORKSPACE_ID)
const ARCHIVED_ROUTE = { ...LIST_ROUTE, initialPath: `${LIST_ROUTE.initialPath}?archived=true` }

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
    .toHaveAttribute('href', '/workspaces/TestWs01/segments/new')
})

test('Edit links to the segment edit page', async () => {
  worker.use(handleSiteSegmentsList({ body: page([VIPS], 1) }))

  const { screen } = await renderWithRouter(<SegmentsListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('VIP customers')).toBeInTheDocument()

  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await expect
    .element(screen.getByRole('menuitem', { name: 'Edit' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/segments/1/edit')
})

test('archiving a segment calls archive and refreshes the list', async () => {
  const archived: string[] = []
  let listFetches = 0
  worker.use(
    handleSiteSegmentsList(() => {
      listFetches++
      return HttpResponse.json({
        items: archived.length > 0 ? [TRIAL] : [VIPS, TRIAL],
        totalItems: 2,
      })
    }),
    handleSiteSegmentsArchive(({ params }) => {
      archived.push(params.id)
      return HttpResponse.json({ ...VIPS, archivedAt: '2026-01-01T00:00:00Z' })
    }),
  )

  const { screen } = await renderWithRouter(<SegmentsListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('VIP customers')).toBeInTheDocument()

  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await screen.getByRole('menuitem', { name: 'Archive' }).click()

  await expect.poll(() => archived).toEqual(['1'])
  await expect.element(screen.getByText('Segment archived')).toBeInTheDocument()
  await expect.poll(() => listFetches).toBe(2)
  await expect.element(screen.getByText('VIP customers')).not.toBeInTheDocument()
})

test('a segment a live broadcast uses is not archived and the broadcast is named', async () => {
  worker.use(
    handleSiteSegmentsList({ body: page([VIPS], 1) }),
    handleSiteSegmentsArchive(() =>
      problem(409, {
        usedBy: [{ kind: 'broadcast', id: '7', name: 'Summer sale teaser' }],
      }),
    ),
  )

  const { screen } = await renderWithRouter(<SegmentsListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('VIP customers')).toBeInTheDocument()

  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await screen.getByRole('menuitem', { name: 'Archive' }).click()

  await expect.element(screen.getByText('Failed to archive segment')).toBeInTheDocument()
  await expect.element(screen.getByText(/Broadcast "Summer sale teaser"/)).toBeInTheDocument()
  await expect.element(screen.getByText('VIP customers')).toBeInTheDocument()
})

test('the archived control opens the archived view through the URL', async () => {
  worker.use(handleSiteSegmentsList(() => HttpResponse.json({ items: [], totalItems: 0 })))

  const { screen, navigate } = await renderWithRouter(<SegmentsListPage />, LIST_ROUTE)
  await screen.getByText('Archived', { exact: true }).click()

  expect(navigate).toHaveBeenCalledWith(
    expect.objectContaining({ search: { archived: true }, replace: true }),
  )
})

test('the archived view lists archived segments and restores one', async () => {
  const flags: (string | null)[] = []
  const restored: string[] = []
  worker.use(
    handleSiteSegmentsList(({ request }) => {
      flags.push(new URL(request.url).searchParams.get('archived'))
      return HttpResponse.json({
        items: [{ ...VIPS, archivedAt: '2026-01-01T00:00:00Z' }],
        totalItems: 1,
      })
    }),
    handleSiteSegmentsUnarchive(({ params }) => {
      restored.push(params.id)
      return HttpResponse.json(VIPS)
    }),
  )

  const { screen } = await renderWithRouter(<SegmentsListPage />, ARCHIVED_ROUTE)
  await expect.poll(() => flags).toEqual(['true'])

  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await screen.getByRole('menuitem', { name: 'Restore' }).click()

  await expect.poll(() => restored).toEqual(['1'])
  await expect.element(screen.getByText('Segment restored')).toBeInTheDocument()
})
