import { expect, test } from 'vitest'

import { Route as analyticsRoute } from '../routes/workspaces/$workspaceId/analytics/route.tsx'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { routeMount } from '../test/routeMount.ts'
import { AnalyticsLayout } from './AnalyticsLayout.tsx'

const mount = (range: string) => {
  const { initialPath, ...rest } = routeMount(analyticsRoute, { workspaceId: 'AcmeWs01' })
  return { ...rest, initialPath: `${initialPath}?range=${range}` }
}

test('choosing a window puts it in the URL', async () => {
  const { screen, navigate } = await renderWithRouter(<AnalyticsLayout />, mount('30d'))

  await screen.getByText('Last 90 days').click()

  expect(navigate).toHaveBeenCalledOnce()
})
