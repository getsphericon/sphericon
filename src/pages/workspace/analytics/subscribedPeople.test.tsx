import { expect, test } from 'vitest'

import { handleSiteAnalyticsSubscribedPeople } from '../../../generated/site/msw.gen.ts'
import { Route as subscribedPeopleRoute } from '../../../routes/workspaces/$workspaceId/analytics/subscribed-people.tsx'
import { problem } from '../../../test/problem.ts'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { routeMount } from '../../../test/routeMount.ts'
import { worker } from '../../../test/worker.ts'
import { SubscribedPeoplePage } from './subscribedPeople.tsx'

// The test router does not run the section's search validation, so the window is spelled out.
const base = routeMount(subscribedPeopleRoute, { workspaceId: 'TestWs01' })
const mount = { ...base, initialPath: `${base.initialPath}?range=7d` }

const report = {
  current: 1234,
  series: [
    { date: '2026-03-08', subscribed: 5, unsubscribed: 2, net: 3 },
    { date: '2026-03-09', subscribed: 0, unsubscribed: 4, net: -4 },
  ],
}

test('shows the current number and the net of each day', async () => {
  worker.use(handleSiteAnalyticsSubscribedPeople({ body: report }))
  const { screen } = await renderWithRouter(<SubscribedPeoplePage />, mount)

  await expect.element(screen.getByText('1234')).toBeInTheDocument()
  await expect.element(screen.getByText('+3')).toBeInTheDocument()
  await expect.element(screen.getByText('-4')).toBeInTheDocument()
})

test('a window without subscription changes shows the empty state, the current number stays', async () => {
  worker.use(
    handleSiteAnalyticsSubscribedPeople({
      body: {
        current: 7,
        series: [{ date: '2026-03-08', subscribed: 0, unsubscribed: 0, net: 0 }],
      },
    }),
  )
  const { screen } = await renderWithRouter(<SubscribedPeoplePage />, mount)

  await expect
    .element(screen.getByText('Nobody subscribed or unsubscribed in this window'))
    .toBeInTheDocument()
  await expect.element(screen.getByText('7', { exact: true })).toBeInTheDocument()
})

test('shows an error alert when the report fails to load', async () => {
  worker.use(handleSiteAnalyticsSubscribedPeople(() => problem(500, { detail: 'boom' })))
  const { screen } = await renderWithRouter(<SubscribedPeoplePage />, mount)

  await expect.element(screen.getByText('Failed to load analytics').first()).toBeInTheDocument()
  await expect.element(screen.getByText('boom')).toBeInTheDocument()
})
