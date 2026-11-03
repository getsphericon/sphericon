import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteAutomationsGet,
  handleSiteBroadcastsGet,
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

// An email Event carries the subject its message kept; Broadcast 5 exists, Broadcast 6 is gone.
type EmailEvent = ReturnType<typeof event> & { emailSubject: string | null }

const emailEvent = (
  id: string,
  action: string,
  properties: Record<string, unknown>,
): EmailEvent => ({ ...event(id, action, properties), emailSubject: 'Welcome to Hexlet' })

// A message that kept no subject: sent before it was kept, or erased with its Contact.
const subjectless = (
  id: string,
  action: string,
  properties: Record<string, unknown>,
): EmailEvent => ({ ...emailEvent(id, action, properties), emailSubject: null })

function serveEmails(...items: EmailEvent[]) {
  serve(...items)
  worker.use(
    handleSiteBroadcastsGet(({ params }) =>
      params.id === '5'
        ? HttpResponse.json({
            id: '5',
            name: 'Launch',
            channel: 'email',
            content: { channel: 'email', subject: 'Welcome {{name}}', body: '<p></p>' },
            status: 'sent',
            ...TIMESTAMPS,
          })
        : problem(404, 'Not Found'),
    ),
  )
}

test('an email Event names the email by its subject, linked to the Broadcast while it exists', async () => {
  serveEmails(
    emailEvent('1', 'email.opened', { broadcastId: 5 }),
    emailEvent('2', 'email.delivered', { broadcastId: 6 }),
    emailEvent('3', 'email.sent', { automationId: 7 }),
    emailEvent('4', 'email.unsubscribed', { broadcastId: 5 }),
    emailEvent('5', 'email.complained', { broadcastId: 5 }),
  )
  const { screen } = await renderWithRouter(<RecentActivityPage />, mount)

  await expect
    .element(
      screen
        .getByRole('row', { name: /Opened "Welcome to Hexlet"/ })
        .getByRole('link', { name: '"Welcome to Hexlet"' }),
    )
    .toBeVisible()
  await expect
    .element(screen.getByRole('row', { name: /Unsubscribed from "Welcome to Hexlet"/ }))
    .toBeVisible()
  await expect
    .element(screen.getByRole('row', { name: /Marked "Welcome to Hexlet" as spam/ }))
    .toBeVisible()
  // The Broadcast is gone: the subject stays, plain.
  const delivered = screen.getByRole('row', { name: /Received "Welcome to Hexlet"/ })
  await expect.element(delivered).toBeVisible()
  await expect.element(delivered.getByRole('link')).not.toBeInTheDocument()
  // An Automation's email is linked to the Automation.
  await expect
    .element(
      screen
        .getByRole('row', { name: /Was sent "Welcome to Hexlet"/ })
        .getByRole('link', { name: '"Welcome to Hexlet"' }),
    )
    .toBeVisible()
})

test('a click names its link without the scheme, the whole one on hover, and the email', async () => {
  serveEmails(
    emailEvent('1', 'email.clicked', { broadcastId: 5, url: 'https://hexlet.io/courses' }),
    emailEvent('2', 'email.clicked', {
      broadcastId: 5,
      url: 'https://hexlet.io/courses/python/lessons/loops-and-conditions?utm=mail',
    }),
  )
  const { screen } = await renderWithRouter(<RecentActivityPage />, mount)

  await expect
    .element(
      screen.getByRole('row', { name: /Clicked hexlet\.io\/courses in "Welcome to Hexlet"/ }),
    )
    .toBeVisible()
  const long = screen.getByText(/hexlet\.io\/courses\/python.*…/)
  await expect.element(long).toBeVisible()
  await long.hover()
  await expect
    .element(
      screen.getByText('https://hexlet.io/courses/python/lessons/loops-and-conditions?utm=mail'),
    )
    .toBeVisible()
})

test('a bounce says whether it was hard or soft', async () => {
  serveEmails(
    emailEvent('1', 'email.bounced', { broadcastId: 5, bounceKind: 'hard' }),
    emailEvent('2', 'email.bounced', { broadcastId: 5, bounceKind: 'soft' }),
  )
  const { screen } = await renderWithRouter(<RecentActivityPage />, mount)

  await expect
    .element(screen.getByRole('row', { name: /Bounced \(hard\): "Welcome to Hexlet"/ }))
    .toBeVisible()
  await expect
    .element(screen.getByRole('row', { name: /Bounced \(soft\): "Welcome to Hexlet"/ }))
    .toBeVisible()
})

test('an email Event whose message kept no subject shows the words of its action', async () => {
  serveEmails(
    subjectless('1', 'email.opened', { broadcastId: 5 }),
    subjectless('2', 'email.clicked', { broadcastId: 5, url: 'https://hexlet.io/courses' }),
    subjectless('3', 'email.bounced', { broadcastId: 5, bounceKind: 'hard' }),
  )
  const { screen } = await renderWithRouter(<RecentActivityPage />, mount)

  await expect.element(screen.getByText('Opened an email')).toBeVisible()
  await expect.element(screen.getByText('Clicked a link in an email')).toBeVisible()
  await expect.element(screen.getByText('Bounced', { exact: true })).toBeVisible()
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
