import { expect, test } from 'vitest'

import {
  handleSiteSegmentContactsList,
  handleSiteSegmentsGet,
} from '../../generated/site/msw.gen.ts'
import { Route as segmentsDetailRoute } from '../../routes/workspaces/$workspaceId/segments/$segmentId/index.tsx'
import { page, TIMESTAMPS } from '../../test/payloads.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { SegmentDetailPage } from './detail.tsx'

const MOUNT = routeMount(segmentsDetailRoute, { workspaceId: 'TestWs01', segmentId: '1' })

test('shows the segment name and the contacts it matches', async () => {
  worker.use(
    handleSiteSegmentsGet({ body: { id: '1', name: 'VIP customers', ...TIMESTAMPS } }),
    handleSiteSegmentContactsList({
      body: page([{ id: '7', email: 'vip@example.com', tags: [], ...TIMESTAMPS }], 1),
    }),
  )

  const { screen } = await renderWithRouter(<SegmentDetailPage />, MOUNT)

  await expect.element(screen.getByText('VIP customers')).toBeInTheDocument()
  await expect.element(screen.getByText('vip@example.com')).toBeInTheDocument()
})
