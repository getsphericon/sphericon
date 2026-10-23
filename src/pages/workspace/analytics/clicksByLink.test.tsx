import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSiteAnalyticsClicksByLink } from '../../../generated/site/msw.gen.ts'
import type { SiteAnalyticsClicksByLink } from '../../../generated/site/types.gen.ts'
import { Route as clicksByLinkRoute } from '../../../routes/workspaces/$workspaceId/analytics/clicks-by-link.tsx'
import { problem } from '../../../test/problem.ts'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { routeMount } from '../../../test/routeMount.ts'
import { worker } from '../../../test/worker.ts'
import { ClicksByLinkPage } from './clicksByLink.tsx'

// The test router does not run the section's search validation, so the window is spelled out.
const base = routeMount(clicksByLinkRoute, { workspaceId: 'TestWs01' })
const mountFor = (range: string) => ({ ...base, initialPath: `${base.initialPath}?range=${range}` })

const report: SiteAnalyticsClicksByLink = {
  eventsRetentionDays: 400,
  links: [
    {
      url: 'https://shop.test/pricing',
      clicks: 3,
      owners: [
        { kind: 'broadcast', id: '200', name: 'Spring sale', clicks: 2 },
        { kind: 'automation', id: '100', name: 'Welcome series', clicks: 1 },
      ],
    },
  ],
}

test('lists each link with its clicks and the Broadcast or Automation that carried it, linked to its report', async () => {
  worker.use(handleSiteAnalyticsClicksByLink({ body: report }))

  const { screen } = await renderWithRouter(<ClicksByLinkPage />, mountFor('30d'))

  await expect.element(screen.getByText('https://shop.test/pricing')).toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Spring sale' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/broadcasts/200/report')
  await expect
    .element(screen.getByRole('link', { name: 'Welcome series' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/automations/100')
})

test('reads the section window from the URL', async () => {
  const ranges: (string | null)[] = []
  worker.use(
    handleSiteAnalyticsClicksByLink(({ request }) => {
      ranges.push(new URL(request.url).searchParams.get('range'))
      return HttpResponse.json(report)
    }),
  )
  await renderWithRouter(<ClicksByLinkPage />, mountFor('7d'))

  await expect.poll(() => ranges).toEqual(['7d'])
})

test('a window without clicks shows the empty state', async () => {
  worker.use(handleSiteAnalyticsClicksByLink({ body: { ...report, links: [] } }))
  const { screen } = await renderWithRouter(<ClicksByLinkPage />, mountFor('30d'))

  await expect.element(screen.getByText('No links were clicked in this window')).toBeInTheDocument()
})

test('says so when the Event retention is shorter than the window', async () => {
  worker.use(handleSiteAnalyticsClicksByLink({ body: { ...report, eventsRetentionDays: 30 } }))
  const { screen } = await renderWithRouter(<ClicksByLinkPage />, mountFor('90d'))

  await expect.element(screen.getByText(/only the last 30 days/)).toBeInTheDocument()
})

test('shows no retention note when the window fits the retention', async () => {
  worker.use(handleSiteAnalyticsClicksByLink({ body: { ...report, eventsRetentionDays: 30 } }))
  const { screen } = await renderWithRouter(<ClicksByLinkPage />, mountFor('30d'))

  await expect.element(screen.getByText('https://shop.test/pricing')).toBeInTheDocument()
  await expect.element(screen.getByText(/only the last/)).not.toBeInTheDocument()
})

test('shows no retention note when Events are kept forever', async () => {
  worker.use(handleSiteAnalyticsClicksByLink({ body: { ...report, eventsRetentionDays: 0 } }))
  const { screen } = await renderWithRouter(<ClicksByLinkPage />, mountFor('90d'))

  await expect.element(screen.getByText('https://shop.test/pricing')).toBeInTheDocument()
  await expect.element(screen.getByText(/only the last/)).not.toBeInTheDocument()
})

test('shows an error alert when the report fails to load', async () => {
  worker.use(handleSiteAnalyticsClicksByLink(() => problem(500, { detail: 'boom' })))
  const { screen } = await renderWithRouter(<ClicksByLinkPage />, mountFor('30d'))

  await expect.element(screen.getByText('boom')).toBeInTheDocument()
})
