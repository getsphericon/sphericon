import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteEventsList,
  handleSiteTrackingDomainsList,
  handleSiteTrackingDomainsRemove,
  handleSiteTrackingDomainsSet,
  handleSiteWorkspacesList,
} from '../../../generated/site/msw.gen.ts'
import type {
  Permission,
  SiteTrackingDomainResource,
  SiteWorkspaceResource,
} from '../../../generated/site/types.gen.ts'
import { Route as recentActivityRoute } from '../../../routes/workspaces/$workspaceId/analytics/recent-activity.tsx'
import { Route as trackingRoute } from '../../../routes/workspaces/$workspaceId/settings/tracking.tsx'
import { problem } from '../../../test/problem.ts'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { routeMount } from '../../../test/routeMount.ts'
import { worker } from '../../../test/worker.ts'
import { DOCS_URL, SDK_SOURCE_URL } from '../../../utils/docs.ts'
import { TrackingPage } from './tracking.tsx'

const TRACKING_ROUTE = routeMount(trackingRoute, { workspaceId: 'TestWs01' })

const workspace: SiteWorkspaceResource = {
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
  role: 'owner',
  createdAt: '2026-01-01T00:00:00Z',
}

// The page lists the workspaces and polls the events feed for the install status. Without a
// CNAME target the Instance offers no Tracking domains, so the page never asks for them: an
// unexpected request would fail the test.
function serveWorkspace(totalItems = 0) {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteEventsList({ body: { items: [], page: 1, pageSize: 1, totalItems, totalPages: 0 } }),
  )
}

// serveTrackingDomains serves a workspace on an Instance that offers Tracking domains, its member
// holding the given permissions, and the Workspace's Tracking domains.
function serveTrackingDomains(
  domains: SiteTrackingDomainResource[],
  permissions: Permission[] = ['tracking_domains:manage'],
) {
  worker.use(
    handleSiteWorkspacesList({
      body: [{ ...workspace, permissions, trackingTarget: 'edge.sphericon.test' }],
    }),
    handleSiteEventsList({
      body: { items: [], page: 1, pageSize: 1, totalItems: 0, totalPages: 0 },
    }),
    handleSiteTrackingDomainsList({ body: domains }),
  )
}

function trackingDomain(
  over: Partial<SiteTrackingDomainResource> = {},
): SiteTrackingDomainResource {
  const host = over.host ?? 't.acme.test'
  return {
    id: '1',
    host,
    role: 'current',
    state: 'pending',
    record: { type: 'CNAME', host, target: 'edge.sphericon.test' },
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...over,
  }
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

test('hides the Tracking domain section when the Instance offers none', async () => {
  serveWorkspace()

  const { screen } = await renderWithRouter(<TrackingPage />, TRACKING_ROUTE)

  await expect.element(screen.getByText(/s\.async = true/)).toBeInTheDocument()
  await expect
    .element(screen.getByRole('heading', { name: 'Tracking domain' }))
    .not.toBeInTheDocument()
})

test('builds the snippet and the test command on the current Tracking domain once it serves', async () => {
  serveTrackingDomains([trackingDomain({ state: 'serving', servedAt: '2026-01-02T00:00:00Z' })])

  const { screen } = await renderWithRouter(<TrackingPage />, TRACKING_ROUTE)

  await expect
    .element(screen.getByText(/s\.src = "https:\/\/t\.acme\.test\/t\.js"/))
    .toBeInTheDocument()
  await expect
    .element(screen.getByText(/curl -X POST "https:\/\/t\.acme\.test\/collect\/events"/))
    .toBeInTheDocument()
})

test('keeps the snippet on the tracker origin while the current Tracking domain does not serve', async () => {
  serveTrackingDomains([trackingDomain({ state: 'dns_verified' })])

  const { screen } = await renderWithRouter(<TrackingPage />, TRACKING_ROUTE)

  await expect.element(screen.getByText('DNS verified')).toBeInTheDocument()
  await expect
    .element(screen.getByText(/s\.src = "https:\/\/t\.sphericon\.test\/t\.js"/))
    .toBeInTheDocument()
})

test('sets the first Tracking domain', async () => {
  const bodies: unknown[] = []
  serveTrackingDomains([])
  worker.use(
    handleSiteTrackingDomainsSet(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(trackingDomain())
    }),
  )

  const { screen } = await renderWithRouter(<TrackingPage />, TRACKING_ROUTE)

  await screen.getByRole('textbox', { name: /^Host/ }).fill(' t.acme.test ')
  await screen.getByRole('button', { name: 'Set host' }).click()

  await expect.poll(() => bodies).toEqual([{ host: 't.acme.test' }])
})

test('shows a refused host on the field', async () => {
  serveTrackingDomains([])
  worker.use(
    handleSiteTrackingDomainsSet(() =>
      problem(422, { title: 'Unprocessable', errors: { host: ['Use a host below your domain'] } }),
    ),
  )

  const { screen } = await renderWithRouter(<TrackingPage />, TRACKING_ROUTE)

  await screen.getByRole('textbox', { name: /^Host/ }).fill('example.com')
  await screen.getByRole('button', { name: 'Set host' }).click()

  await expect.element(screen.getByText('Use a host below your domain')).toBeInTheDocument()
  await expect
    .element(screen.getByText('Could not set the tracking domain'))
    .not.toBeInTheDocument()
})

test('shows the CNAME, a pending replacement and the retired hosts', async () => {
  serveTrackingDomains([
    trackingDomain({ state: 'serving', servedAt: '2026-01-02T00:00:00Z' }),
    trackingDomain({ id: '2', host: 'links.acme.test', role: 'replacement', reason: 'host_taken' }),
    trackingDomain({ id: '3', host: 'old.acme.test', role: 'retired', state: 'serving' }),
  ])

  const { screen } = await renderWithRouter(<TrackingPage />, TRACKING_ROUTE)

  await expect.element(screen.getByText('t.acme.test', { exact: true }).first()).toBeInTheDocument()
  await expect
    .element(screen.getByText('edge.sphericon.test', { exact: true }).first())
    .toBeInTheDocument()
  await expect
    .element(
      screen.getByText(
        't.acme.test keeps serving until links.acme.test serves; then links.acme.test takes over and t.acme.test is retired.',
      ),
    )
    .toBeInTheDocument()
  await expect
    .element(screen.getByText(/Another workspace has already verified/))
    .toBeInTheDocument()
  await expect.element(screen.getByText('old.acme.test', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Forget' })).toBeInTheDocument()
})

test('removing a host that served retires it after saying sent links keep working', async () => {
  const removed: string[] = []
  serveTrackingDomains([trackingDomain({ state: 'serving', servedAt: '2026-01-02T00:00:00Z' })])
  worker.use(
    handleSiteTrackingDomainsRemove(({ params }) => {
      removed.push(params.id)
      return new HttpResponse(null, { status: 204 })
    }),
  )

  const { screen } = await renderWithRouter(<TrackingPage />, TRACKING_ROUTE)

  await screen.getByRole('button', { name: 'Remove' }).click()
  await expect
    .element(screen.getByRole('dialog').getByText(/t\.acme\.test is retired/))
    .toBeInTheDocument()
  expect(removed).toEqual([])
  await screen.getByRole('dialog').getByRole('button', { name: 'Remove' }).click()

  await expect.poll(() => removed).toEqual(['1'])
})

test('forgetting a retired host warns that its links stop working', async () => {
  const removed: string[] = []
  serveTrackingDomains([
    trackingDomain({ id: '3', host: 'old.acme.test', role: 'retired', state: 'serving' }),
  ])
  worker.use(
    handleSiteTrackingDomainsRemove(({ params }) => {
      removed.push(params.id)
      return new HttpResponse(null, { status: 204 })
    }),
  )

  const { screen } = await renderWithRouter(<TrackingPage />, TRACKING_ROUTE)

  await screen.getByRole('button', { name: 'Forget' }).click()
  await expect
    .element(screen.getByRole('dialog').getByText(/sent on old\.acme\.test .* will stop working/))
    .toBeInTheDocument()
  await screen.getByRole('dialog').getByRole('button', { name: 'Forget' }).click()

  await expect.poll(() => removed).toEqual(['3'])
})

test('a member without tracking_domains:manage sees the hosts but cannot change them', async () => {
  serveTrackingDomains([trackingDomain({ state: 'pending' })], [])

  const { screen } = await renderWithRouter(<TrackingPage />, TRACKING_ROUTE)

  await expect.element(screen.getByText('Pending', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Verify' })).not.toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Remove' })).not.toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Change' })).not.toBeInTheDocument()
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
