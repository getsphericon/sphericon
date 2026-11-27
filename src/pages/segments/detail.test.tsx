import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteSegmentContactsList,
  handleSiteSegmentsGet,
} from '../../generated/site/msw.gen.ts'
import type { SiteContactResource } from '../../generated/site/types.gen.ts'
import { Route as segmentsDetailRoute } from '../../routes/workspaces/$workspaceId/segments/$segmentId/index.tsx'
import { EMPTY_CONDITION, testId, TIMESTAMPS } from '../../test/payloads.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { SegmentDetailPage } from './detail.tsx'

const MOUNT = routeMount(segmentsDetailRoute, { workspaceId: 'TestWs01', segmentId: testId(1) })

function contact(id: number, email: string): SiteContactResource {
  return { id: testId(id), email, tags: [], revision: 1, ...TIMESTAMPS }
}

const SEGMENT = handleSiteSegmentsGet({
  body: {
    id: testId(1),
    name: 'VIP customers',
    condition: EMPTY_CONDITION,
    revision: 1,
    ...TIMESTAMPS,
  },
})

test('shows the segment name and the contacts it matches', async () => {
  worker.use(
    SEGMENT,
    handleSiteSegmentContactsList({ body: { items: [contact(7, 'vip@example.com')] } }),
  )

  const { screen } = await renderWithRouter(<SegmentDetailPage />, MOUNT)

  await expect.element(screen.getByText('VIP customers')).toBeInTheDocument()
  await expect.element(screen.getByText('vip@example.com')).toBeInTheDocument()
  await expect
    .element(screen.getByRole('button', { name: 'Load more contacts' }))
    .not.toBeInTheDocument()
})

test('loading more reads the next page by its cursor, below the first', async () => {
  const cursors: (string | null)[] = []
  worker.use(
    SEGMENT,
    handleSiteSegmentContactsList(({ request }) => {
      const cursor = new URL(request.url).searchParams.get('cursor')
      cursors.push(cursor)
      return HttpResponse.json(
        cursor
          ? { items: [contact(8, 'second@example.com')] }
          : { items: [contact(7, 'vip@example.com')], nextCursor: 'c7' },
      )
    }),
  )
  const { screen } = await renderWithRouter(<SegmentDetailPage />, MOUNT)

  await screen.getByRole('button', { name: 'Load more contacts' }).click()

  await expect.element(screen.getByText('second@example.com')).toBeInTheDocument()
  await expect.element(screen.getByText('vip@example.com')).toBeInTheDocument()
  expect(cursors).toContain('c7')
})

test('says so when no contact matches', async () => {
  worker.use(SEGMENT, handleSiteSegmentContactsList({ body: { items: [] } }))

  const { screen } = await renderWithRouter(<SegmentDetailPage />, MOUNT)

  await expect.element(screen.getByText('No contacts match this segment')).toBeInTheDocument()
})
