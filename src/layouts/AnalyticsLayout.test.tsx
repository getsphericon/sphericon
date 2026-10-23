import type { AnyRoute } from '@tanstack/react-router'
import { expect, test } from 'vitest'

import { Route as emailMetricsRoute } from '../routes/workspaces/$workspaceId/analytics/email-metrics.tsx'
import { Route as recentActivityRoute } from '../routes/workspaces/$workspaceId/analytics/recent-activity.tsx'
import { Route as analyticsRoute } from '../routes/workspaces/$workspaceId/analytics/route.tsx'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { routeMount } from '../test/routeMount.ts'
import { AnalyticsLayout } from './AnalyticsLayout.tsx'

const mount = (range: string) => {
  const { initialPath, ...rest } = routeMount(analyticsRoute, { workspaceId: 'AcmeWs01' })
  return { ...rest, initialPath: `${initialPath}?range=${range}` }
}

const reportPath = (route: AnyRoute) => routeMount(route, { workspaceId: 'AcmeWs01' }).initialPath

test('the menu lists the existing reports under their groups, each on its own URL', async () => {
  const { screen } = await renderWithRouter(<AnalyticsLayout />, mount('30d'))

  await expect.element(screen.getByText('Email', { exact: true })).toBeVisible()
  await expect.element(screen.getByText('Tracking', { exact: true })).toBeVisible()
  await expect
    .element(screen.getByRole('link', { name: 'Email metrics' }))
    .toHaveAttribute('href', `${reportPath(emailMetricsRoute)}?range=30d`)
  await expect
    .element(screen.getByRole('link', { name: 'Recent activity' }))
    .toHaveAttribute('href', `${reportPath(recentActivityRoute)}?range=30d`)
})

test('a group without a report is not shown', async () => {
  const { screen } = await renderWithRouter(<AnalyticsLayout />, mount('30d'))

  await expect.element(screen.getByText('Email', { exact: true })).toBeVisible()
  await expect.element(screen.getByText('Audience', { exact: true })).not.toBeInTheDocument()
})

test('the chosen window is carried to every report link', async () => {
  const { screen } = await renderWithRouter(<AnalyticsLayout />, mount('7d'))

  await expect
    .element(screen.getByRole('link', { name: 'Email metrics' }))
    .toHaveAttribute('href', `${reportPath(emailMetricsRoute)}?range=7d`)
  await expect
    .element(screen.getByRole('link', { name: 'Recent activity' }))
    .toHaveAttribute('href', `${reportPath(recentActivityRoute)}?range=7d`)
})

test('choosing a window puts it in the URL', async () => {
  const { screen, navigate } = await renderWithRouter(<AnalyticsLayout />, mount('30d'))

  await screen.getByText('Last 90 days').click()

  expect(navigate).toHaveBeenCalledOnce()
})
