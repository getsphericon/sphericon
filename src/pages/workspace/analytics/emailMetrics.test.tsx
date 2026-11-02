import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSiteAnalyticsOverview } from '../../../generated/site/msw.gen.ts'
import type {
  SiteAnalyticsOverview,
  SiteAnalyticsPeriod,
} from '../../../generated/site/types.gen.ts'
import { Route as emailMetricsRoute } from '../../../routes/workspaces/$workspaceId/analytics/email-metrics.tsx'
import { problem } from '../../../test/problem.ts'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { routeMount } from '../../../test/routeMount.ts'
import { worker } from '../../../test/worker.ts'
import { EmailMetricsPage } from './emailMetrics.tsx'

// The test router does not run the section's search validation, so the window is spelled out.
const base = routeMount(emailMetricsRoute, { workspaceId: 'TestWs01' })
const mount = { ...base, initialPath: `${base.initialPath}?range=30d` }

const period: SiteAnalyticsPeriod = {
  from: '2026-10-02',
  to: '2026-10-31',
  sending: {
    broadcast: 100,
    automation: 0,
    transactional: 0,
    delivered: 98,
    bounced: 1,
    complained: 0,
  },
  email: {
    sentCount: 100,
    openedCount: 60,
    clickedCount: 20,
    openRate: 0.6,
    clickRate: 0.2,
    clickToOpenRate: 0.33,
  },
  newContacts: 5,
}

const overview: SiteAnalyticsOverview = {
  contacts: { total: 42, active: 40, unsubscribed: 2 },
  automations: { total: 3, active: 2, runsActive: 1, runsCompleted: 9 },
  current: period,
  previous: { ...period, from: '2026-09-02', to: '2026-10-01' },
  timeseries: [],
}

test('shows the contacts count', async () => {
  worker.use(handleSiteAnalyticsOverview({ body: overview }))

  const { screen } = await renderWithRouter(<EmailMetricsPage />, mount)

  await expect.element(screen.getByText('42', { exact: true })).toBeInTheDocument()
})

test('reads the section window from the URL', async () => {
  const ranges: (string | null)[] = []
  worker.use(
    handleSiteAnalyticsOverview(({ request }) => {
      ranges.push(new URL(request.url).searchParams.get('range'))
      return HttpResponse.json(overview)
    }),
  )
  await renderWithRouter(<EmailMetricsPage />, {
    ...base,
    initialPath: `${base.initialPath}?range=7d`,
  })

  await expect.poll(() => ranges).toEqual(['7d'])
})

test('a window without sent email shows the empty state instead of the chart', async () => {
  worker.use(
    handleSiteAnalyticsOverview({
      body: { ...overview, current: { ...period, email: { ...period.email, sentCount: 0 } } },
    }),
  )
  const { screen } = await renderWithRouter(<EmailMetricsPage />, mount)

  await expect.element(screen.getByText('No email sent in this window')).toBeInTheDocument()
})

test('shows an error alert when analytics fail to load', async () => {
  worker.use(handleSiteAnalyticsOverview(() => problem(500, { detail: 'boom' })))
  const { screen } = await renderWithRouter(<EmailMetricsPage />, mount)

  await expect.element(screen.getByText('Failed to load analytics').first()).toBeInTheDocument()
  await expect.element(screen.getByText('boom')).toBeInTheDocument()
})

test('a workspace without contacts shows the empty hint instead of the chart', async () => {
  worker.use(
    handleSiteAnalyticsOverview({
      body: {
        ...overview,
        contacts: { total: 0, active: 0, unsubscribed: 0 },
      },
    }),
  )
  const { screen } = await renderWithRouter(<EmailMetricsPage />, mount)

  await expect.element(screen.getByText('No contacts yet')).toBeInTheDocument()
})
