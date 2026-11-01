import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSiteAnalyticsEvents } from '../../../generated/site/msw.gen.ts'
import { Route as eventsRoute } from '../../../routes/workspaces/$workspaceId/analytics/events.tsx'
import { Route as recentActivityRoute } from '../../../routes/workspaces/$workspaceId/analytics/recent-activity.tsx'
import { problem } from '../../../test/problem.ts'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { routeMount } from '../../../test/routeMount.ts'
import { worker } from '../../../test/worker.ts'
import { EventsReportPage } from './eventsReport.tsx'

// The test router does not run the section's search validation, so the window is spelled out.
const base = routeMount(eventsRoute, { workspaceId: 'TestWs01' })
const mount = { ...base, initialPath: `${base.initialPath}?range=7d` }
const feedPath = routeMount(recentActivityRoute, { workspaceId: 'TestWs01' }).initialPath

test('lists each action with its counts and last occurrence', async () => {
  worker.use(
    handleSiteAnalyticsEvents({
      body: {
        actions: [
          { action: 'purchase', count: 12, contacts: 7, lastAt: '2026-01-02T03:04:00Z' },
          { action: 'email.opened', count: 3, contacts: 2, lastAt: '2026-01-01T00:00:00Z' },
        ],
      },
    }),
  )

  const { screen } = await renderWithRouter(<EventsReportPage />, mount)

  await expect.element(screen.getByText('purchase')).toBeInTheDocument()
  await expect.element(screen.getByText('Opened an email')).toBeInTheDocument()
  await expect.element(screen.getByText('12', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('7', { exact: true })).toBeInTheDocument()
})

test('an action opens Recent activity filtered by it, keeping the window', async () => {
  worker.use(
    handleSiteAnalyticsEvents({
      body: {
        actions: [{ action: 'purchase', count: 12, contacts: 7, lastAt: '2026-01-02T03:04:00Z' }],
      },
    }),
  )

  const { screen } = await renderWithRouter(<EventsReportPage />, mount)

  await expect
    .element(screen.getByRole('link', { name: 'purchase' }))
    .toHaveAttribute('href', `${feedPath}?range=7d&action=purchase`)
})

test('reads the section window from the URL', async () => {
  const ranges: (string | null)[] = []
  worker.use(
    handleSiteAnalyticsEvents(({ request }) => {
      ranges.push(new URL(request.url).searchParams.get('range'))
      return HttpResponse.json({ actions: [] })
    }),
  )

  const { screen } = await renderWithRouter(<EventsReportPage />, mount)

  await expect.element(screen.getByText('No events in this window')).toBeInTheDocument()
  expect(ranges).toEqual(['7d'])
})

test('shows an empty state when the window holds no events', async () => {
  worker.use(handleSiteAnalyticsEvents({ body: { actions: [] } }))

  const { screen } = await renderWithRouter(<EventsReportPage />, mount)

  await expect.element(screen.getByText('No events in this window')).toBeInTheDocument()
})

test('shows an error when the report fails to load', async () => {
  worker.use(handleSiteAnalyticsEvents(() => problem(500, { detail: 'boom' })))

  const { screen } = await renderWithRouter(<EventsReportPage />, mount)

  await expect.element(screen.getByText('Failed to load events').first()).toBeInTheDocument()
  await expect.element(screen.getByText('boom')).toBeInTheDocument()
})
