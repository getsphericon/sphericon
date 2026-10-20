import { expect, test } from 'vitest'

import { handleSiteEventsList, handleSiteWorkspacesList } from '../../../generated/site/msw.gen.ts'
import { Route as activityRoute } from '../../../routes/workspaces/$workspaceId/activity.tsx'
import { Route as trackingRoute } from '../../../routes/workspaces/$workspaceId/settings/tracking.tsx'
import { problem } from '../../../test/problem.ts'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { routeMount } from '../../../test/routeMount.ts'
import { worker } from '../../../test/worker.ts'
import { TrackingPage } from './tracking.tsx'

const TRACKING_ROUTE = routeMount(trackingRoute, { workspaceId: 'TestWs01' })

const workspace = {
  id: 'TestWs01',
  name: 'Acme',

  collectKey: 'omck_test_key',
  postalAddress: '',
  accountLinkAvailable: false,
  mcpUrl: 'https://sphericon.test',
  holds: [],
  role: 'owner' as const,
  createdAt: '2026-01-01T00:00:00Z',
}

// The page lists the workspaces and polls the events feed for the install status.
function serveWorkspace(totalItems = 0) {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteEventsList({ body: { items: [], page: 1, pageSize: 1, totalItems, totalPages: 0 } }),
  )
}

test('shows the tracking snippet and the test command with the collect key', async () => {
  serveWorkspace()

  const { screen } = await renderWithRouter(<TrackingPage />, TRACKING_ROUTE)

  await expect.element(screen.getByText(/omck_test_key/).first()).toBeInTheDocument()
  await expect.element(screen.getByText(/x-collect-key: omck_test_key/)).toBeInTheDocument()
  // No events yet → waiting state.
  await expect.element(screen.getByText('Waiting for the first event…')).toBeInTheDocument()
})

test('shows the connected install status once events arrive', async () => {
  serveWorkspace(7)

  const { screen } = await renderWithRouter(<TrackingPage />, TRACKING_ROUTE)

  await expect.element(screen.getByText(/Events are arriving/)).toBeInTheDocument()
})

test('the activity link points at the workspace activity feed', async () => {
  serveWorkspace()
  const { screen } = await renderWithRouter(<TrackingPage />, TRACKING_ROUTE)

  await expect
    .element(screen.getByRole('link', { name: 'Open activity feed' }))
    .toHaveAttribute('href', routeMount(activityRoute, { workspaceId: 'TestWs01' }).initialPath)
})

test('shows an error alert when the workspaces fail to load', async () => {
  worker.use(
    handleSiteWorkspacesList(() => problem(500, { detail: 'boom' })),
    handleSiteEventsList(() => problem(500, { detail: 'boom' })),
  )
  const { screen } = await renderWithRouter(<TrackingPage />, TRACKING_ROUTE)

  await expect.element(screen.getByRole('alert', { name: /Failed to load/ })).toBeInTheDocument()
})
