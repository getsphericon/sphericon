import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteAutomationsGet,
  handleSiteBroadcastsGet,
  handleSiteCustomFieldsList,
  handleSiteEventsActions,
  handleSiteEventsList,
  handleSiteTagsList,
} from '../../generated/site/msw.gen.ts'
import { Route as activityRoute } from '../../routes/workspaces/$workspaceId/activity.tsx'
import { page, TIMESTAMPS, testId } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { ActivityPage } from './page.tsx'

const WORKSPACE_ID = 'TestWs01'
const mount = routeMount(activityRoute, { workspaceId: WORKSPACE_ID })

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
    handleSiteTagsList({ body: page([{ id: testId(1), name: 'vip', ...TIMESTAMPS }]) }),
    handleSiteCustomFieldsList({
      body: page([
        {
          id: testId(1),
          key: 'plan',
          name: 'Plan',
          type: 'string',
          ruleOperators: [],
          revision: 1,
          ...TIMESTAMPS,
        },
      ]),
    }),
    handleSiteAutomationsGet(({ params }) =>
      params.id === testId(7)
        ? HttpResponse.json({
            id: testId(7),
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
    event(testId(1), 'contact.tagged', { tag: 'vip' }),
    event(testId(2), 'contact.untagged', { tag: 'gone' }),
  )
  const { screen } = await renderWithRouter(<ActivityPage />, mount)

  await expect.element(screen.getByRole('link', { name: '"vip"' })).toBeVisible()
  await expect.element(screen.getByRole('row', { name: /Untagged "gone"/ })).toBeVisible()
  await expect.element(screen.getByRole('link', { name: '"gone"' })).not.toBeInTheDocument()
})

test('a Custom field change names the field by its name, and a clear says so', async () => {
  serve(
    event(testId(1), 'contact.field_changed', { key: 'plan', value: 'pro' }),
    event(testId(2), 'contact.field_changed', { key: 'plan' }),
  )
  const { screen } = await renderWithRouter(<ActivityPage />, mount)

  await expect.element(screen.getByText('Set Plan to "pro"')).toBeVisible()
  await expect.element(screen.getByText('Cleared Plan')).toBeVisible()
})

test('a Custom field no longer defined is named by its key, its value as JSON', async () => {
  serve(event(testId(1), 'contact.field_changed', { key: 'seats', value: 4 }))
  const { screen } = await renderWithRouter(<ActivityPage />, mount)

  await expect.element(screen.getByText('Set seats to "4"')).toBeVisible()
})

test('an Enrollment names its Automation, and one that is gone by the words of its action', async () => {
  serve(
    event(testId(1), 'automation.goal_reached', { automationId: testId(7) }),
    event(testId(2), 'automation.exited', { automationId: testId(8) }),
    event(testId(3), 'automation.enrolled', { automationId: testId(7) }),
    event(testId(4), 'automation.completed', { automationId: testId(7) }),
  )
  const { screen } = await renderWithRouter(<ActivityPage />, mount)

  const linked = (row: RegExp) =>
    screen.getByRole('row', { name: row }).getByRole('link', { name: '"Onboarding"' })
  await expect.element(linked(/Entered "Onboarding"/)).toBeVisible()
  await expect.element(linked(/Completed "Onboarding"/)).toBeVisible()
  await expect.element(linked(/Reached the goal of "Onboarding"/)).toBeVisible()
  await expect.element(screen.getByText('Left an automation')).toBeVisible()
})

test('a page view names the page by its title, the path when it has none', async () => {
  serve(
    event(testId(1), 'page.view', {
      title: 'Pricing',
      path: '/pricing',
      url: 'https://x.test/pricing',
    }),
    event(testId(2), 'page.view', { path: '/docs' }),
  )
  const { screen } = await renderWithRouter(<ActivityPage />, mount)

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
      params.id === testId(5)
        ? HttpResponse.json({
            id: testId(5),
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
    emailEvent(testId(1), 'email.opened', { broadcastId: testId(5) }),
    emailEvent(testId(2), 'email.delivered', { broadcastId: testId(6) }),
    emailEvent(testId(3), 'email.sent', { automationId: testId(7) }),
    emailEvent(testId(4), 'email.unsubscribed', { broadcastId: testId(5) }),
    emailEvent(testId(5), 'email.complained', { broadcastId: testId(5) }),
  )
  const { screen } = await renderWithRouter(<ActivityPage />, mount)

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
    emailEvent(testId(1), 'email.clicked', {
      broadcastId: testId(5),
      url: 'https://hexlet.io/courses',
    }),
    emailEvent(testId(2), 'email.clicked', {
      broadcastId: testId(5),
      url: 'https://hexlet.io/courses/python/lessons/loops-and-conditions?utm=mail',
    }),
  )
  const { screen } = await renderWithRouter(<ActivityPage />, mount)

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

test('a bounce says whether it was hard or soft, and one of no known kind only that it bounced', async () => {
  serveEmails(
    emailEvent(testId(1), 'email.bounced', { broadcastId: testId(5), bounceKind: 'hard' }),
    emailEvent(testId(2), 'email.bounced', { broadcastId: testId(5), bounceKind: 'soft' }),
    emailEvent(testId(3), 'email.bounced', { broadcastId: testId(5) }),
  )
  const { screen } = await renderWithRouter(<ActivityPage />, mount)

  await expect
    .element(screen.getByRole('row', { name: /Bounced \(hard\): "Welcome to Hexlet"/ }))
    .toBeVisible()
  await expect
    .element(screen.getByRole('row', { name: /Bounced \(soft\): "Welcome to Hexlet"/ }))
    .toBeVisible()
  await expect
    .element(screen.getByRole('row', { name: /Bounced: "Welcome to Hexlet"/ }))
    .toBeVisible()
})

test('a delayed and a dropped email say so', async () => {
  serveEmails(
    emailEvent(testId(1), 'email.delayed', { broadcastId: testId(5) }),
    emailEvent(testId(2), 'email.dropped', { broadcastId: testId(5) }),
  )
  const { screen } = await renderWithRouter(<ActivityPage />, mount)

  await expect
    .element(screen.getByRole('row', { name: /Delayed "Welcome to Hexlet"/ }))
    .toBeVisible()
  await expect
    .element(screen.getByRole('row', { name: /Dropped "Welcome to Hexlet"/ }))
    .toBeVisible()
})

test('an email Event whose message kept no subject shows the words of its action', async () => {
  serveEmails(
    subjectless('1', 'email.opened', { broadcastId: testId(5) }),
    subjectless('2', 'email.clicked', { broadcastId: testId(5), url: 'https://hexlet.io/courses' }),
    subjectless('3', 'email.bounced', { broadcastId: testId(5), bounceKind: 'hard' }),
  )
  const { screen } = await renderWithRouter(<ActivityPage />, mount)

  await expect.element(screen.getByText('Opened an email')).toBeVisible()
  await expect.element(screen.getByText('Clicked a link in an email')).toBeVisible()
  await expect.element(screen.getByText('Bounced', { exact: true })).toBeVisible()
})

test('an Event without what its row names shows the words of its action', async () => {
  serve(
    event(testId(1), 'contact.tagged', {}),
    event(testId(2), 'contact.field_changed', { key: '' }),
    event(testId(3), 'automation.enrolled', {}),
    event(testId(4), 'page.view', { title: null }),
  )
  const { screen } = await renderWithRouter(<ActivityPage />, mount)

  await expect.element(screen.getByText('Was tagged')).toBeVisible()
  await expect.element(screen.getByText('Had a custom field changed')).toBeVisible()
  await expect.element(screen.getByText('Entered an automation')).toBeVisible()
  await expect.element(screen.getByText('Visited a page')).toBeVisible()
})

test("an SMS message's delivery state shows, an undelivered one with the provider's error code", async () => {
  serve(
    event(testId(1), 'sms.sent', { destination: '+14155550177' }),
    event(testId(2), 'sms.delivered', { destination: '+14155550177' }),
    event(testId(3), 'sms.undelivered', { destination: '+14155550177', errorCode: '30003' }),
    event(testId(4), 'sms.undelivered', { destination: '+14155550177' }),
  )
  const { screen } = await renderWithRouter(<ActivityPage />, mount)

  await expect.element(screen.getByText('Was sent a text message')).toBeVisible()
  await expect.element(screen.getByText('Text message delivered')).toBeVisible()
  await expect
    .element(screen.getByText('Text message not delivered (error 30003)', { exact: true }))
    .toBeVisible()
  await expect
    .element(screen.getByText('Text message not delivered', { exact: true }))
    .toBeVisible()
})
