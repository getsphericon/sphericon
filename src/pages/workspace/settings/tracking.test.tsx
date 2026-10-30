import { expect, test } from 'vitest'

import {
  handleSiteEventsList,
  handleSiteSendingDomainsList,
  handleSiteWorkspacesList,
} from '../../../generated/site/msw.gen.ts'
import type { SiteSendingDomainResource } from '../../../generated/site/types.gen.ts'
import { Route as recentActivityRoute } from '../../../routes/workspaces/$workspaceId/analytics/recent-activity.tsx'
import { Route as trackingRoute } from '../../../routes/workspaces/$workspaceId/settings/tracking.tsx'
import { problem } from '../../../test/problem.ts'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { routeMount } from '../../../test/routeMount.ts'
import { worker } from '../../../test/worker.ts'
import { DOCS_URL, SDK_SOURCE_URL } from '../../../utils/docs.ts'
import { TrackingPage } from './tracking.tsx'

const TRACKING_ROUTE = routeMount(trackingRoute, { workspaceId: 'TestWs01' })

const workspace = {
  id: 'TestWs01',
  name: 'Acme',

  collectKey: 'omck_test_key',
  postalAddress: '',
  timezone: 'UTC',
  accountLinkAvailable: false,
  permissions: [],
  mcpUrl: 'https://sphericon.test',
  trackerUrl: 'https://t.sphericon.test',
  holds: [],
  role: 'owner' as const,
  createdAt: '2026-01-01T00:00:00Z',
}

// The page lists the workspaces and polls the events feed for the install status.
function serveWorkspace(totalItems = 0, domains: SiteSendingDomainResource[] = []) {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteSendingDomainsList({
      body: { items: domains, page: 1, pageSize: 20, totalItems: domains.length, totalPages: 1 },
    }),
    handleSiteEventsList({ body: { items: [], page: 1, pageSize: 1, totalItems, totalPages: 0 } }),
  )
}

test('shows the tracking snippet and the test command with the collect key', async () => {
  serveWorkspace()

  const { screen } = await renderWithRouter(<TrackingPage />, TRACKING_ROUTE)

  await expect.element(screen.getByText(/omck_test_key/).first()).toBeInTheDocument()
  // The snippet is a queue stub that loads t.js asynchronously, not a bare script tag.
  await expect
    .element(screen.getByText(/_omq\.push\(\['init', \{ collectKey: "omck_test_key"/))
    .toBeInTheDocument()
  await expect.element(screen.getByText(/s\.async = true/)).toBeInTheDocument()
  await expect.element(screen.getByText(/x-collect-key: omck_test_key/)).toBeInTheDocument()
  // No events yet → waiting state.
  await expect.element(screen.getByText('Waiting for the first event…')).toBeInTheDocument()
})

test('the snippet and the test command use the tracker origin, not the app origin', async () => {
  serveWorkspace()

  const { screen } = await renderWithRouter(<TrackingPage />, TRACKING_ROUTE)

  await expect
    .element(screen.getByText(/s\.src = "https:\/\/t\.sphericon\.test\/t\.js"/))
    .toBeInTheDocument()
  await expect
    .element(screen.getByText(/baseUrl: "https:\/\/t\.sphericon\.test"/))
    .toBeInTheDocument()
  await expect
    .element(screen.getByText(/curl -X POST "https:\/\/t\.sphericon\.test\/collect\/events"/))
    .toBeInTheDocument()
})

const sendingDomain = (host: string, state: 'pending' | 'serving') =>
  ({
    id: host,
    domain: host.replace('track.', ''),
    dkimSelector: 'om1',
    verified: true,
    dkimRecord: { type: 'TXT', host: 'x', value: 'x' },
    spfRecord: { type: 'TXT', host: 'x', value: 'x' },
    dmarcRecord: { type: 'TXT', host: 'x', value: 'x' },
    trackingRecord: { type: 'CNAME', host, target: 'edge.sphericon.test', state },
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
  }) satisfies SiteSendingDomainResource

test('offers the Tracking host of a serving Sending domain and builds the snippet on it', async () => {
  serveWorkspace(0, [
    sendingDomain('track.acme.test', 'serving'),
    sendingDomain('track.pending.test', 'pending'),
  ])

  const { screen } = await renderWithRouter(<TrackingPage />, TRACKING_ROUTE)

  // Only the serving host is offered, beside the Instance's tracker origin.
  await screen.getByRole('combobox', { name: 'Tracker host' }).click()
  await expect
    .element(screen.getByRole('option', { name: 'https://track.acme.test' }))
    .toBeInTheDocument()
  await expect
    .element(screen.getByRole('option', { name: 'https://track.pending.test' }))
    .not.toBeInTheDocument()

  await screen.getByRole('option', { name: 'https://track.acme.test' }).click()
  await expect
    .element(screen.getByText(/s\.src = "https:\/\/track\.acme\.test\/t\.js"/))
    .toBeInTheDocument()
  await expect
    .element(screen.getByText(/curl -X POST "https:\/\/track\.acme\.test\/collect\/events"/))
    .toBeInTheDocument()
})

test('links to the guide on sending events and to the SDK sources', async () => {
  serveWorkspace()

  const { screen } = await renderWithRouter(<TrackingPage />, TRACKING_ROUTE)

  await expect
    .element(screen.getByRole('link', { name: 'How to send events' }))
    .toHaveAttribute('href', `${DOCS_URL}/guide/tracking`)
  await expect
    .element(screen.getByRole('link', { name: /Browser SDK/ }))
    .toHaveAttribute('href', `${SDK_SOURCE_URL}/analytics`)
  await expect
    .element(screen.getByRole('link', { name: /Node SDK/ }))
    .toHaveAttribute('href', `${SDK_SOURCE_URL}/node`)
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
    .toHaveAttribute(
      'href',
      routeMount(recentActivityRoute, { workspaceId: 'TestWs01' }).initialPath,
    )
})

test('shows an error alert when the workspaces fail to load', async () => {
  worker.use(
    handleSiteWorkspacesList(() => problem(500, { detail: 'boom' })),
    handleSiteEventsList(() => problem(500, { detail: 'boom' })),
  )
  const { screen } = await renderWithRouter(<TrackingPage />, TRACKING_ROUTE)

  await expect.element(screen.getByRole('alert', { name: /Failed to load/ })).toBeInTheDocument()
})
