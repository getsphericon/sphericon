import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteAnalyticsOverview,
  handleSiteSendingDomainsList,
  handleSiteSetupChecklistGet,
  handleSiteWorkspacesList,
} from '../../generated/site/msw.gen.ts'
import { Route as overviewRoute } from '../../routes/workspaces/$slug/index.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { OverviewPage } from './overview.tsx'

const finished = {
  steps: { integration: 'done', sendingDomain: 'done', contact: 'done', sentEmail: 'done' },
  complete: true,
} as const

const overview = {
  contacts: { total: 42, active: 40, unsubscribed: 2, newInRange: 5 },
  email: {
    sentCount: 100,
    openedCount: 60,
    clickedCount: 20,
    openRate: 0.6,
    clickRate: 0.2,
    clickToOpenRate: 0.33,
  },
  automations: { total: 3, active: 2, runsActive: 1, runsCompleted: 9 },
  timeseries: [],
}

test('shows the workspace name and contacts count', async () => {
  worker.use(
    handleSiteSetupChecklistGet({ body: finished }),
    handleSiteAnalyticsOverview({ body: overview }),
    handleSiteWorkspacesList({
      body: [
        {
          id: '1',
          name: 'Acme',
          slug: 'test',
          publicId: 'AcmeWs01',
          collectKey: 'k',
          postalAddress: '',
          accountLinkAvailable: false,
          mcpUrl: 'https://sphericon.test',
          holds: [],
          role: 'owner' as const,
          createdAt: '2026-01-01T00:00:00Z',
        },
      ],
    }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, {
    path: '/workspaces/$slug/',
    initialPath: '/workspaces/test/',
  })

  await expect.element(screen.getByText('Acme')).toBeInTheDocument()
  await expect.element(screen.getByText('42', { exact: true })).toBeInTheDocument()
})

const mount = routeMount(overviewRoute, { slug: 'test' })

test('switching the range refetches the analytics for that window', async () => {
  const ranges: (string | null)[] = []
  worker.use(
    handleSiteSetupChecklistGet({ body: finished }),
    handleSiteWorkspacesList({ body: [] }),
    handleSiteAnalyticsOverview(({ request }) => {
      ranges.push(new URL(request.url).searchParams.get('range'))
      return HttpResponse.json(overview)
    }),
  )
  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.poll(() => ranges).toEqual(['30d'])
  await screen.getByText('Last 7 days').click()

  await expect.poll(() => ranges).toEqual(['30d', '7d'])
})

test('shows an error alert when analytics fail to load', async () => {
  worker.use(
    handleSiteSetupChecklistGet({ body: finished }),
    handleSiteWorkspacesList({ body: [] }),
    handleSiteAnalyticsOverview(() => problem(500, { detail: 'boom' })),
  )
  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('Failed to load analytics').first()).toBeInTheDocument()
  await expect.element(screen.getByText('boom')).toBeInTheDocument()
})

test('a workspace without contacts shows the empty hint instead of the chart', async () => {
  worker.use(
    handleSiteSetupChecklistGet({ body: finished }),
    handleSiteWorkspacesList({ body: [] }),
    handleSiteAnalyticsOverview({
      body: {
        ...overview,
        contacts: { total: 0, active: 0, unsubscribed: 0, newInRange: 0 },
      },
    }),
  )
  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('No contacts yet')).toBeInTheDocument()
})

test('a workspace in the warning tier sees which rate is over the limit', async () => {
  worker.use(
    handleSiteSetupChecklistGet({ body: finished }),
    handleSiteAnalyticsOverview({
      body: {
        ...overview,
        abuseWarning: { domain: 'mail.acme.test', metric: 'complaint', rate: 0.0015 },
      },
    }),
    handleSiteWorkspacesList({ body: [] }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect
    .element(screen.getByText('Your sending reputation needs attention'))
    .toBeInTheDocument()
  await expect
    .element(screen.getByText(/complaint rate on mail.acme.test is 0.15%/))
    .toBeInTheDocument()
})

test('no warning shows no alert', async () => {
  worker.use(
    handleSiteSetupChecklistGet({ body: finished }),
    handleSiteAnalyticsOverview({ body: overview }),
    handleSiteWorkspacesList({ body: [] }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('42', { exact: true })).toBeInTheDocument()
  await expect
    .element(screen.getByText('Your sending reputation needs attention'))
    .not.toBeInTheDocument()
})

test('before the first sent email only the checklist shows and no analytics are requested', async () => {
  let analyticsRequests = 0
  worker.use(
    handleSiteWorkspacesList({ body: [] }),
    handleSiteSendingDomainsList({
      body: { items: [], page: 1, pageSize: 20, totalItems: 0, totalPages: 1 },
    }),
    handleSiteSetupChecklistGet({
      body: {
        steps: {
          integration: 'done',
          sendingDomain: 'pending',
          contact: 'todo',
          sentEmail: 'todo',
        },
        complete: false,
      },
    }),
    handleSiteAnalyticsOverview(() => {
      analyticsRequests++
      return HttpResponse.json(overview)
    }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('Set up Sphericon')).toBeInTheDocument()
  await expect.element(screen.getByText('Waiting for DNS')).toBeInTheDocument()
  await expect.element(screen.getByText('Last 7 days')).not.toBeInTheDocument()
  expect(analyticsRequests).toBe(0)
})

test('after the first sent email the usual Overview shows with the checklist on top', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [] }),
    handleSiteSetupChecklistGet({
      body: {
        steps: { integration: 'done', sendingDomain: 'done', contact: 'todo', sentEmail: 'done' },
        complete: false,
      },
    }),
    handleSiteAnalyticsOverview({ body: overview }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('Set up Sphericon')).toBeInTheDocument()
  await expect.element(screen.getByText('42', { exact: true })).toBeInTheDocument()
})

test('a finished checklist is not shown', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [] }),
    handleSiteSetupChecklistGet({ body: finished }),
    handleSiteAnalyticsOverview({ body: overview }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('42', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Set up Sphericon')).not.toBeInTheDocument()
})
