import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSiteAnalyticsNewPeople } from '../../../generated/site/msw.gen.ts'
import { Route as newPeopleRoute } from '../../../routes/workspaces/$workspaceId/analytics/new-people.tsx'
import { problem } from '../../../test/problem.ts'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { routeMount } from '../../../test/routeMount.ts'
import { worker } from '../../../test/worker.ts'
import { NewPeoplePage } from './newPeople.tsx'

// The test router does not run the section's search validation, so the window is spelled out.
const base = routeMount(newPeopleRoute, { workspaceId: 'TestWs01' })
const mount = { ...base, initialPath: `${base.initialPath}?range=30d` }

const report = {
  total: 7,
  timeseries: [
    { date: '2026-03-07', manual: 3, api: 2, tracker: 1, unknown: 1 },
    { date: '2026-03-08', manual: 0, api: 0, tracker: 0, unknown: 0 },
  ],
}

test('shows how many people joined in the window and the legend of the sources', async () => {
  worker.use(handleSiteAnalyticsNewPeople({ body: report }))

  const { screen } = await renderWithRouter(<NewPeoplePage />, mount)

  await expect.element(screen.getByText('7', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Manual')).toBeInTheDocument()
  await expect.element(screen.getByText('Unknown')).toBeInTheDocument()
})

test('reads the section window from the URL', async () => {
  const ranges: (string | null)[] = []
  worker.use(
    handleSiteAnalyticsNewPeople(({ request }) => {
      ranges.push(new URL(request.url).searchParams.get('range'))
      return HttpResponse.json(report)
    }),
  )
  await renderWithRouter(<NewPeoplePage />, {
    ...base,
    initialPath: `${base.initialPath}?range=7d`,
  })

  await expect.poll(() => ranges).toEqual(['7d'])
})

test('a window without new people shows the empty state instead of the chart', async () => {
  worker.use(handleSiteAnalyticsNewPeople({ body: { total: 0, timeseries: [] } }))
  const { screen } = await renderWithRouter(<NewPeoplePage />, mount)

  await expect.element(screen.getByText('No new people in this window')).toBeInTheDocument()
})

test('shows an error alert when the report fails to load', async () => {
  worker.use(handleSiteAnalyticsNewPeople(() => problem(500, { detail: 'boom' })))
  const { screen } = await renderWithRouter(<NewPeoplePage />, mount)

  await expect.element(screen.getByText('Failed to load analytics').first()).toBeInTheDocument()
  await expect.element(screen.getByText('boom')).toBeInTheDocument()
})
