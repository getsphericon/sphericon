import { expect, test } from 'vitest'

import {
  handleSiteCustomFieldsList,
  handleSiteEventsActions,
} from '../../generated/site/msw.gen.ts'
import { Route as segmentsRoute } from '../../routes/workspaces/$workspaceId/segments/index.tsx'
import { Route as segmentsCreateRoute } from '../../routes/workspaces/$workspaceId/segments/new.tsx'
import { page } from '../../test/payloads.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { SegmentCreatePage } from './resource.tsx'

function serveCatalogue() {
  worker.use(
    handleSiteEventsActions({ body: { actions: [] } }),
    handleSiteCustomFieldsList({ body: page([], 0) }),
  )
}

test('Cancel links back to the segments list of the current workspace', async () => {
  serveCatalogue()
  const { screen } = await renderWithRouter(
    <SegmentCreatePage />,
    routeMount(segmentsCreateRoute, { workspaceId: 'TestWs01' }),
  )

  await expect
    .element(screen.getByRole('link', { name: 'Cancel' }))
    .toHaveAttribute('href', routeMount(segmentsRoute, { workspaceId: 'TestWs01' }).initialPath)
})
