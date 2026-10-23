import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSiteAnalyticsOpensByHour } from '../../../generated/site/msw.gen.ts'
import { Route as opensByHourRoute } from '../../../routes/workspaces/$workspaceId/analytics/opens-by-hour.tsx'
import { problem } from '../../../test/problem.ts'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { routeMount } from '../../../test/routeMount.ts'
import { worker } from '../../../test/worker.ts'
import { OpensByHourPage } from './opensByHour.tsx'

// The test router does not run the section's search validation, so the window is spelled out.
const base = routeMount(opensByHourRoute, { workspaceId: 'TestWs01' })
const mount = { ...base, initialPath: `${base.initialPath}?range=30d` }

const report = (opensAt: Record<string, number>) => ({
  total: Object.values(opensAt).reduce((a, b) => a + b, 0),
  cells: Array.from({ length: 7 * 24 }, (_, i) => {
    const weekday = Math.floor(i / 24) + 1
    const hour = i % 24
    return { weekday, hour, opens: opensAt[`${weekday}-${hour}`] ?? 0 }
  }),
})

test('names the opens of each hour by weekday and hour', async () => {
  worker.use(handleSiteAnalyticsOpensByHour({ body: report({ '2-22': 7 }) }))
  const { screen } = await renderWithRouter(<OpensByHourPage />, mount)

  await expect.element(screen.getByLabelText('Tuesday 22:00, 7 opens')).toBeInTheDocument()
  await expect.element(screen.getByLabelText('Tuesday 21:00, 0 opens')).toBeInTheDocument()
})

test('reads the section window from the URL', async () => {
  const ranges: (string | null)[] = []
  worker.use(
    handleSiteAnalyticsOpensByHour(({ request }) => {
      ranges.push(new URL(request.url).searchParams.get('range'))
      return HttpResponse.json(report({}))
    }),
  )
  await renderWithRouter(<OpensByHourPage />, {
    ...base,
    initialPath: `${base.initialPath}?range=7d`,
  })

  await expect.poll(() => ranges).toEqual(['7d'])
})

test('a window without opens shows the empty state instead of the grid', async () => {
  worker.use(handleSiteAnalyticsOpensByHour({ body: report({}) }))
  const { screen } = await renderWithRouter(<OpensByHourPage />, mount)

  await expect.element(screen.getByText('No opens in this window')).toBeInTheDocument()
})

test('shows an error alert when the report fails to load', async () => {
  worker.use(handleSiteAnalyticsOpensByHour(() => problem(500, { detail: 'boom' })))
  const { screen } = await renderWithRouter(<OpensByHourPage />, mount)

  await expect.element(screen.getByText('boom')).toBeInTheDocument()
})
