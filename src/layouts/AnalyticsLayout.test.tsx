import { expect, test } from 'vitest'

import { Route as recentActivityRoute } from '../routes/workspaces/$workspaceId/analytics/recent-activity.tsx'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { routeMount } from '../test/routeMount.ts'
import { AnalyticsLayout } from './AnalyticsLayout.tsx'

// The layout is mounted on a report other than the section's first (Email metrics), where a
// navigation to the section itself would land through its redirect.
const mount = (range: string) => {
  const { initialPath, ...rest } = routeMount(recentActivityRoute, { workspaceId: 'AcmeWs01' })
  return { ...rest, initialPath: `${initialPath}?range=${range}` }
}

test('choosing a window keeps the report and puts the window in the URL', async () => {
  const { screen, navigate, router } = await renderWithRouter(<AnalyticsLayout />, mount('30d'))

  await screen.getByText('Last 90 days').click()

  expect(navigate).toHaveBeenCalledOnce()
  const target = router.buildLocation(navigate.mock.calls[0]![0])
  expect(target.pathname).toBe('/workspaces/AcmeWs01/analytics/recent-activity')
  expect(target.search).toEqual({ range: '90d' })
})
