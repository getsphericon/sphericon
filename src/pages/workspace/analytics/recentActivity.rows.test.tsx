import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteAutomationsGet,
  handleSiteCustomFieldsList,
  handleSiteEventsActions,
  handleSiteEventsList,
  handleSiteTagsList,
} from '../../../generated/site/msw.gen.ts'
import { Route as recentActivityRoute } from '../../../routes/workspaces/$workspaceId/analytics/recent-activity.tsx'
import { page, TIMESTAMPS } from '../../../test/payloads.ts'
import { problem } from '../../../test/problem.ts'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { routeMount } from '../../../test/routeMount.ts'
import { worker } from '../../../test/worker.ts'
import { RecentActivityPage } from './recentActivity.tsx'

const WORKSPACE_ID = 'TestWs01'
const mount = routeMount(recentActivityRoute, { workspaceId: WORKSPACE_ID })

const event = (id: string, action: string, properties: Record<string, unknown>) => ({
  id,
  subjectId: `s:${id}`,
  email: null,
  action,
  properties,
  occurredAt: '2026-01-02T00:00:00Z',
  createdAt: '2026-01-02T00:00:00Z',
})

// The feed shows events, and the Workspace has the vip Tag, the plan field named Plan and the
// Onboarding Automation (id 7); Automation 8 is gone.
function serve(...items: ReturnType<typeof event>[]) {
  worker.use(
    handleSiteEventsActions({ body: { actions: [] } }),
    handleSiteEventsList({ body: page(items) }),
    handleSiteTagsList({ body: page([{ id: '1', name: 'vip', ...TIMESTAMPS }]) }),
    handleSiteCustomFieldsList({
      body: page([
        { id: '1', key: 'plan', name: 'Plan', type: 'string', ruleOperators: [], ...TIMESTAMPS },
      ]),
    }),
    handleSiteAutomationsGet(({ params }) =>
      params.id === '7'
        ? HttpResponse.json({
            id: '7',
            name: 'Onboarding',
            status: 'active',
            live: null,
            draft: null,
            ...TIMESTAMPS,
          })
        : problem(404, 'Not Found'),
    ),
  )
}

test('a Tag change names the Tag, linked while it exists', async () => {
  serve(
    event('1', 'contact.tagged', { tag: 'vip' }),
    event('2', 'contact.untagged', { tag: 'gone' }),
  )
  const { screen } = await renderWithRouter(<RecentActivityPage />, mount)

  await expect.element(screen.getByRole('link', { name: '"vip"' })).toBeVisible()
  await expect.element(screen.getByRole('row', { name: /Untagged "gone"/ })).toBeVisible()
  await expect.element(screen.getByRole('link', { name: '"gone"' })).not.toBeInTheDocument()
})

test('a Custom field change names the field by its name, and a clear says so', async () => {
  serve(
    event('1', 'contact.field_changed', { key: 'plan', value: 'pro' }),
    event('2', 'contact.field_changed', { key: 'plan' }),
  )
  const { screen } = await renderWithRouter(<RecentActivityPage />, mount)

  await expect.element(screen.getByText('Set Plan to "pro"')).toBeVisible()
  await expect.element(screen.getByText('Cleared Plan')).toBeVisible()
})

test('a Custom field no longer defined is named by its key, its value as JSON', async () => {
  serve(event('1', 'contact.field_changed', { key: 'seats', value: 4 }))
  const { screen } = await renderWithRouter(<RecentActivityPage />, mount)

  await expect.element(screen.getByText('Set seats to "4"')).toBeVisible()
})

test('an Enrollment names its Automation, and one that is gone by the words of its action', async () => {
  serve(
    event('1', 'automation.goal_reached', { automationId: 7 }),
    event('2', 'automation.exited', { automationId: 8 }),
    event('3', 'automation.enrolled', { automationId: 7 }),
    event('4', 'automation.completed', { automationId: 7 }),
  )
  const { screen } = await renderWithRouter(<RecentActivityPage />, mount)

  const linked = (row: RegExp) =>
    screen.getByRole('row', { name: row }).getByRole('link', { name: '"Onboarding"' })
  await expect.element(linked(/Entered "Onboarding"/)).toBeVisible()
  await expect.element(linked(/Completed "Onboarding"/)).toBeVisible()
  await expect.element(linked(/Reached the goal of "Onboarding"/)).toBeVisible()
  await expect.element(screen.getByText('Left an automation')).toBeVisible()
})

test('a page view names the page by its title, the path when it has none', async () => {
  serve(
    event('1', 'page.view', { title: 'Pricing', path: '/pricing', url: 'https://x.test/pricing' }),
    event('2', 'page.view', { path: '/docs' }),
  )
  const { screen } = await renderWithRouter(<RecentActivityPage />, mount)

  await expect.element(screen.getByText('Visited "Pricing"')).toBeVisible()
  await expect.element(screen.getByText('Visited "/docs"')).toBeVisible()
})

test('an Event without what its row names shows the words of its action', async () => {
  serve(
    event('1', 'contact.tagged', {}),
    event('2', 'contact.field_changed', { key: '' }),
    event('3', 'automation.enrolled', {}),
    event('4', 'page.view', { title: null }),
  )
  const { screen } = await renderWithRouter(<RecentActivityPage />, mount)

  await expect.element(screen.getByText('Was tagged')).toBeVisible()
  await expect.element(screen.getByText('Had a custom field changed')).toBeVisible()
  await expect.element(screen.getByText('Entered an automation')).toBeVisible()
  await expect.element(screen.getByText('Visited a page')).toBeVisible()
})
