import { useQuery } from '@tanstack/react-query'
import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  siteBroadcastsGetOptions,
  siteBroadcastsListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import {
  handleSiteBroadcastsCreate,
  handleSiteBroadcastsGet,
  handleSiteBroadcastsList,
  handleSiteBroadcastsSchedule,
  handleSiteBroadcastsSend,
  handleSiteBroadcastsTestSend,
  handleSiteBroadcastsUpdate,
  handleSiteIntegrationsList,
  handleSiteSegmentsList,
  handleSiteSegmentsPreview,
  handleSiteSendingDomainsList,
  handleSiteTemplatesList,
  handleSiteWorkspacesList,
} from '../../generated/site/msw.gen.ts'
import type { SiteIntegrationResource } from '../../generated/site/types.gen.ts'
import { Route as broadcastsEditRoute } from '../../routes/workspaces/$workspaceId/broadcasts/$broadcastId/edit.tsx'
import { Route as broadcastsReportRoute } from '../../routes/workspaces/$workspaceId/broadcasts/$broadcastId/report.tsx'
import { Route as broadcastsCreateRoute } from '../../routes/workspaces/$workspaceId/broadcasts/new.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { requestGate } from '../../test/requestGate.ts'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { BroadcastCreatePage, BroadcastEditPage } from './resource.tsx'

const CREATE_ROUTE = routeMount(broadcastsCreateRoute, { workspaceId: 'TestWs01' })

const EDIT_ROUTE = routeMount(broadcastsEditRoute, { workspaceId: 'TestWs01', broadcastId: '7' })

function ListProbe() {
  useQuery(siteBroadcastsListOptions({ path: { workspaceId: 'TestWs01' } }))
  return null
}

function DetailProbe() {
  useQuery(siteBroadcastsGetOptions({ path: { workspaceId: 'TestWs01', id: '7' } }))
  return null
}

// An email Broadcast; content overrides fields of its email content.
function broadcast(status: string, content: Record<string, unknown> = {}) {
  return {
    id: '7',
    name: 'Launch',
    channel: 'email',
    content: {
      channel: 'email',
      subject: 'Hello',
      fromName: 'Ada',
      fromEmail: 'ada@example.com',
      body: '<p></p>',
      ...content,
    },
    status,
  }
}

const integration = (id: string, name: string, isDefault = false): SiteIntegrationResource => ({
  id,
  name,
  channel: 'email',
  provider: 'smtp',
  enabled: true,
  isDefault,
  maxPerSecond: null,
  maxPerDay: null,
  providerLimit: {
    perSecond: { limit: null, source: null },
    perDay: { limit: null, source: null },
    sentLast24h: 0,
    warnings: ['unlimited'],
  },
  config: { kind: 'smtp', host: 'smtp.test', port: 587, from: 'a@acme.com' },
  preset: null,
  params: {},
  secretsRetiring: [],
  hookUrl: 'https://app.test/hooks/omik_one',
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
})

// The SMS Integration is disabled, so email is the one Channel a new Broadcast is offered; it is
// still listed, and never an email Broadcast's to send through.
const INTEGRATIONS = [
  integration('1', 'Primary SMTP', true),
  integration('21', 'Postmark'),
  { ...integration('30', 'SMS gateway'), channel: 'sms' as const, enabled: false },
]

const WORKSPACE = {
  id: 'TestWs01',
  name: 'Acme',
  collectKey: 'omck_test_key',
  postalAddress: '',
  timezone: 'UTC',
  accountLinkAvailable: false,
  permissions: [],
  mcpUrl: 'https://sphericon.test',
  trackerUrl: 'https://t.sphericon.test',
  shortLinkLength: 35,
  smsMaxLength: { gsm7: 1600, ucs2: 700 },
  holds: [],
  role: 'owner' as const,
  createdAt: '2026-01-01T00:00:00Z',
}

// The form reads the Workspace for its Send cap; this one has none, so nothing is counted.
const workspaces = handleSiteWorkspacesList({ body: [WORKSPACE] })

function empty() {
  return HttpResponse.json({ items: [], totalItems: 0 })
}

function sendingDomain(domain: string, verified: boolean) {
  return { id: domain, domain, dkimSelector: 'sphericon', verified }
}

function domainsPage(items: ReturnType<typeof sendingDomain>[]) {
  return HttpResponse.json({
    items,
    page: 1,
    pageSize: 100,
    totalItems: items.length,
    totalPages: 1,
  })
}

// Serves the broadcasts API: records write bodies and counts list/detail fetches. The Workspace
// has one verified Sending domain unless `domains` says otherwise.
function serve(
  status: string,
  detailGate?: Promise<void>,
  domains = [sendingDomain('example.com', true)],
) {
  const bodies: unknown[] = []
  const fetches = { list: 0, detail: 0 }
  worker.use(
    workspaces,
    handleSiteBroadcastsCreate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ ...broadcast('draft'), id: '42' }, { status: 201 })
    }),
    handleSiteBroadcastsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(broadcast(status))
    }),
    handleSiteBroadcastsSend(() => HttpResponse.json(broadcast(status))),
    handleSiteBroadcastsList(() => {
      fetches.list++
      return empty()
    }),
    handleSiteBroadcastsGet(async () => {
      fetches.detail++
      await detailGate
      return HttpResponse.json(broadcast(status))
    }),
    handleSiteSegmentsList(empty),
    handleSiteTemplatesList(empty),
    handleSiteSendingDomainsList(() => domainsPage(domains)),
    handleSiteIntegrationsList({ body: INTEGRATIONS }),
  )
  return { bodies, fetches }
}

test('creating a broadcast sends only the filled keys, refreshes the list and opens the edit page', async () => {
  const { bodies, fetches } = serve('draft')

  const { screen, navigate } = await renderWithRouter(
    <>
      <ListProbe />
      <BroadcastCreatePage />
    </>,
    CREATE_ROUTE,
  )
  await expect.poll(() => fetches.list).toBe(1)

  await screen.getByRole('button', { name: 'Continue' }).click()
  await screen.getByLabelText(/^Name/).fill('Launch')
  await screen.getByLabelText('Subject').fill('Hello')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([{ name: 'Launch', content: { channel: 'email', subject: 'Hello' } }])
  await expect.element(screen.getByText('Broadcast created')).toBeInTheDocument()
  await expect.poll(() => fetches.list).toBe(2)
  await expect.poll(() => navigate.mock.calls.length).toBe(1)
  expect(navigate.mock.calls[0]?.[0]).toMatchObject({
    params: { workspaceId: 'TestWs01', broadcastId: '42' },
  })
})

test('editing a draft; a cleared sender name is sent as null and the Segment is cleared too', async () => {
  const { bodies, fetches } = serve('draft')

  const { screen } = await renderWithRouter(
    <>
      <ListProbe />
      <DetailProbe />
      <BroadcastEditPage />
    </>,
    EDIT_ROUTE,
  )

  await expect.element(screen.getByLabelText(/^Name/)).toHaveValue('Launch')
  await expect.element(screen.getByLabelText('From name')).toHaveValue('Ada')

  await screen.getByLabelText('From name').fill('')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([
      {
        name: 'Launch',
        segmentId: null,
        integrationId: null,
        content: {
          channel: 'email',
          subject: 'Hello',
          fromName: null,
          fromEmail: 'ada@example.com',
          body: '<p></p>',
        },
      },
    ])
  await expect.element(screen.getByText('Broadcast updated')).toBeInTheDocument()
  await expect.poll(() => fetches.list).toBe(2)
  await expect.poll(() => fetches.detail).toBeGreaterThan(1)
})

test('editing a Telegram draft loads its message content and clears what is emptied', async () => {
  const { bodies } = serve('draft')
  worker.use(
    handleSiteBroadcastsGet(() =>
      HttpResponse.json({
        id: '7',
        name: 'News',
        channel: 'telegram',
        status: 'draft',
        content: {
          channel: 'telegram',
          text: 'Hi',
          imageUrl: 'https://example.com/a.png',
          buttons: [{ label: 'Open', url: 'https://example.com/b' }],
          linkPreview: false,
          unsubscribeButton: 'Stop',
        },
      }),
    ),
  )
  const { screen } = await renderWithRouter(<BroadcastEditPage />, EDIT_ROUTE)

  await expect.element(screen.getByLabelText('Image URL')).toHaveValue('https://example.com/a.png')
  await expect.element(screen.getByRole('textbox', { name: 'Button 1 label' })).toHaveValue('Open')
  await expect
    .element(screen.getByRole('switch', { name: 'Show a preview of links in the text' }))
    .not.toBeChecked()
  await expect.element(screen.getByLabelText('Button label')).toHaveValue('Stop')

  await screen.getByLabelText('Image URL').fill('')
  await screen.getByRole('button', { name: 'Remove button 1' }).click()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([
      {
        name: 'News',
        segmentId: null,
        integrationId: null,
        content: {
          channel: 'telegram',
          text: 'Hi',
          imageUrl: '',
          buttons: [],
          linkPreview: false,
          unsubscribeButton: 'Stop',
        },
      },
    ])
})

test('a draft can be sent now', async () => {
  serve('draft')
  const { screen, navigate } = await renderWithRouter(<BroadcastEditPage />, EDIT_ROUTE)

  const send = screen.getByRole('button', { name: 'Send now' })
  await expect.element(send).toBeEnabled()
  await send.click()

  await expect.element(screen.getByText('Broadcast is sending')).toBeInTheDocument()
  await expect.poll(() => navigate.mock.calls.length).toBe(1)
})

test('a draft larger than the Send cap is warned about and can still be sent now', async () => {
  serve('draft')
  worker.use(
    handleSiteWorkspacesList({
      body: [{ ...WORKSPACE, sendCap: { perDay: 1000, sentLast24h: 0 } }],
    }),
    handleSiteSegmentsPreview({ body: { count: 2500 } }),
  )
  const { screen, navigate } = await renderWithRouter(<BroadcastEditPage />, EDIT_ROUTE)

  // 1000 go at once, 1000 more within the day and the last 500 on the second.
  await expect.element(screen.getByText(/2,500 contacts.*about 2 days/)).toBeInTheDocument()
  const send = screen.getByRole('button', { name: 'Send now' })
  await expect.element(send).toBeEnabled()
  await send.click()

  await expect.element(screen.getByText('Broadcast is sending')).toBeInTheDocument()
  await expect.poll(() => navigate.mock.calls.length).toBe(1)
})

test('send and schedule are disabled for a non-draft broadcast, test send stays available', async () => {
  serve('sent')
  const { screen } = await renderWithRouter(<BroadcastEditPage />, EDIT_ROUTE)

  await expect.element(screen.getByRole('button', { name: 'Send now' })).toBeDisabled()
  await expect.element(screen.getByRole('button', { name: 'Schedule' })).toBeDisabled()
  await screen.getByLabelText('Send a test to').fill('qa@example.com')
  await expect.element(screen.getByRole('button', { name: 'Send test' })).toBeEnabled()
})

test('editing shows no form while the broadcast loads, then the loaded values', async () => {
  const gate = Promise.withResolvers<undefined>()
  serve('draft', gate.promise)

  const { screen } = await renderWithRouter(<BroadcastEditPage />, EDIT_ROUTE)

  await expect.element(screen.getByLabelText(/^Name/)).not.toBeInTheDocument()
  gate.resolve(undefined)

  await expect.element(screen.getByLabelText(/^Name/)).toHaveValue('Launch')
})

test('editing shows an error alert when the broadcast cannot be loaded', async () => {
  worker.use(
    workspaces,
    handleSiteBroadcastsGet(() =>
      problem(404, { title: 'Not Found', detail: 'broadcast not found' }),
    ),
    handleSiteSegmentsList(empty),
    handleSiteTemplatesList(empty),
    handleSiteSendingDomainsList(() => domainsPage([])),
  )

  const { screen } = await renderWithRouter(<BroadcastEditPage />, EDIT_ROUTE)

  await expect.element(screen.getByText('Failed to load broadcasts')).toBeVisible()
  await expect.element(screen.getByText('broadcast not found')).toBeVisible()
})

const DOC = {
  type: 'doc',
  content: [{ type: 'paragraph', content: [{ type: 'text', text: 'Hello there' }] }],
}

test('saving a broadcast written in the editor sends its document and no body', async () => {
  const { bodies } = serve('draft')
  worker.use(
    workspaces,
    handleSiteBroadcastsGet(() =>
      HttpResponse.json(broadcast('draft', { body: '<p>Hello there</p>', doc: DOC })),
    ),
  )

  const { screen } = await renderWithRouter(<BroadcastEditPage />, EDIT_ROUTE)
  await expect.element(screen.getByRole('button', { name: 'Insert variable' })).toBeEnabled()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([
      {
        name: 'Launch',
        segmentId: null,
        integrationId: null,
        content: {
          channel: 'email',
          subject: 'Hello',
          fromName: 'Ada',
          fromEmail: 'ada@example.com',
          doc: DOC,
        },
      },
    ])
})

test('a broadcast written in the editor opens clean: Send now is offered, and again after a Save', async () => {
  const written = broadcast('draft', { body: '<p>Hello there</p>', doc: DOC })
  serve('draft')
  worker.use(
    handleSiteBroadcastsGet(() => HttpResponse.json(written)),
    handleSiteBroadcastsUpdate(() => HttpResponse.json(written)),
  )

  const { screen } = await renderWithRouter(<BroadcastEditPage />, EDIT_ROUTE)
  await expect.element(screen.getByRole('button', { name: 'Insert variable' })).toBeEnabled()
  await expect.element(screen.getByRole('button', { name: 'Send now' })).toBeEnabled()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.element(screen.getByText('Broadcast updated')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Send now' })).toBeEnabled()
})

test('a broadcast with no document opens as editable source', async () => {
  serve('draft')
  worker.use(
    workspaces,
    handleSiteBroadcastsGet(() =>
      HttpResponse.json(broadcast('draft', { body: '<table><tr><td>Hi</td></tr></table>' })),
    ),
  )

  const { screen } = await renderWithRouter(<BroadcastEditPage />, EDIT_ROUTE)

  await expect
    .element(screen.getByLabelText('Body'))
    .toHaveValue('<table><tr><td>Hi</td></tr></table>')
})

test('a refused save shows the reason the server gave', async () => {
  serve('draft')
  worker.use(
    workspaces,
    handleSiteBroadcastsGet(() => HttpResponse.json(broadcast('draft', { doc: DOC }))),
    handleSiteBroadcastsUpdate(() => problem(422, { detail: 'heading level 9 is not allowed' })),
  )

  const { screen } = await renderWithRouter(<BroadcastEditPage />, EDIT_ROUTE)
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.element(screen.getByText('heading level 9 is not allowed')).toBeInTheDocument()
})

test('the From is a local part on a verified Sending domain, and an empty one is not sent', async () => {
  const { bodies } = serve('draft', undefined, [
    sendingDomain('news.example.com', true),
    sendingDomain('pending.example.com', false),
  ])

  const { screen } = await renderWithRouter(<BroadcastCreatePage />, CREATE_ROUTE)
  await screen.getByRole('button', { name: 'Continue' }).click()
  await screen.getByLabelText(/^Name/).fill('Launch')
  await screen.getByLabelText('From address').fill('news')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([
      { name: 'Launch', content: { channel: 'email', fromEmail: 'news@news.example.com' } },
    ])
})

test('a workspace with no verified Sending domain sees a hint with a link to set one up', async () => {
  serve('draft', undefined, [sendingDomain('pending.example.com', false)])

  const { screen } = await renderWithRouter(<BroadcastCreatePage />, CREATE_ROUTE)
  await screen.getByRole('button', { name: 'Continue' }).click()

  await expect.element(screen.getByText(/Verify a sending domain/)).toBeInTheDocument()
  await expect.element(screen.getByRole('link', { name: 'Sending domains' })).toBeInTheDocument()
  await expect.element(screen.getByLabelText('From address')).toBeDisabled()
})

test('a saved From whose domain lost verification stays visible, marked, and saves as it is', async () => {
  const { bodies } = serve('draft', undefined, [sendingDomain('news.example.com', true)])

  const { screen } = await renderWithRouter(<BroadcastEditPage />, EDIT_ROUTE)

  await expect.element(screen.getByLabelText('From address')).toHaveValue('ada')
  await expect
    .element(screen.getByRole('combobox', { name: 'From domain' }))
    .toHaveValue('example.com (not verified)')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => bodies).toMatchObject([{ content: { fromEmail: 'ada@example.com' } }])
})

test('the Integration a broadcast sends through is chosen among the email ones and saved', async () => {
  const { bodies } = serve('draft')

  const { screen } = await renderWithRouter(<BroadcastCreatePage />, CREATE_ROUTE)
  await screen.getByRole('button', { name: 'Continue' }).click()
  await screen.getByLabelText(/^Name/).fill('Launch')
  const picker = screen.getByRole('combobox', { name: 'Send through' })
  await expect.element(picker).toHaveValue('Workspace default')
  await picker.click()
  expect(screen.getByRole('option', { name: 'SMS gateway' }).elements()).toHaveLength(0)
  await screen.getByRole('option', { name: 'Postmark' }).click()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([{ name: 'Launch', integrationId: '21', content: { channel: 'email' } }])
})

test('a saved Integration is shown, and switching back to the default clears it', async () => {
  const { bodies } = serve('draft')
  worker.use(
    workspaces,
    handleSiteBroadcastsGet(() =>
      HttpResponse.json({ ...broadcast('draft'), integrationId: '21' }),
    ),
  )

  const { screen } = await renderWithRouter(<BroadcastEditPage />, EDIT_ROUTE)
  const picker = screen.getByRole('combobox', { name: 'Send through' })
  await expect.element(picker).toHaveValue('Postmark')
  await picker.click()
  await screen.getByRole('option', { name: 'Workspace default' }).click()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => bodies).toMatchObject([{ integrationId: null }])
})

test("a new broadcast asks for its Channel, then shows that Channel's editor", async () => {
  serve('draft')

  const { screen } = await renderWithRouter(<BroadcastCreatePage />, CREATE_ROUTE)

  // Email is the one Channel with an enabled Integration (the SMS one is disabled), so it
  // is chosen already, and the editor waits for the person to go on.
  await expect.element(screen.getByRole('radio', { name: 'Email' })).toBeChecked()
  await expect.element(screen.getByLabelText('Subject')).not.toBeInTheDocument()
  await screen.getByRole('button', { name: 'Continue' }).click()

  await expect.element(screen.getByText('Channel: Email')).toBeInTheDocument()
  await expect.element(screen.getByLabelText('Subject')).toBeInTheDocument()
})

test('with no enabled Integration no Channel is offered, and the way to add one is shown', async () => {
  serve('draft')
  worker.use(
    handleSiteIntegrationsList({
      body: [{ ...integration('1', 'Primary SMTP', true), enabled: false }],
    }),
  )

  const { screen } = await renderWithRouter(<BroadcastCreatePage />, CREATE_ROUTE)

  await expect.element(screen.getByText(/No channel can send yet/)).toBeInTheDocument()
  await expect.element(screen.getByRole('radio', { name: 'Email' })).not.toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Continue' })).toBeDisabled()
})

// An archived Broadcast is read-only until it is unarchived (ADR 0100): its edit URL opens its
// report, which has no Edit, instead of a form whose Save would answer 409.
test('an archived broadcast opens its report instead of the form', async () => {
  serve('draft')
  worker.use(
    handleSiteBroadcastsGet(() =>
      HttpResponse.json({ ...broadcast('draft'), archivedAt: '2026-01-02T00:00:00Z' }),
    ),
  )

  const { screen, navigate } = await renderWithRouter(<BroadcastEditPage />, EDIT_ROUTE)

  await expect
    .poll(() => navigate)
    .toHaveBeenCalledWith({
      to: broadcastsReportRoute.to,
      params: { workspaceId: 'TestWs01', broadcastId: '7' },
      replace: true,
    })
  await expect.element(screen.getByRole('button', { name: 'Save' })).not.toBeInTheDocument()
})

const SEND_FAILED = 'Failed to send broadcast'
const REPORT = {
  to: broadcastsReportRoute.to,
  params: { workspaceId: 'TestWs01', broadcastId: '7' },
}

// The draft the edit page opens, of the Channel the test is about; `as` is it in another status.
function serveDraft(content?: Record<string, unknown>, channel = 'email') {
  serve('draft')
  const stored = content
    ? { ...broadcast('draft'), channel, content: { channel, ...content } }
    : broadcast('draft')
  worker.use(handleSiteBroadcastsGet(() => HttpResponse.json(stored)))
  return { as: (status: string) => ({ ...stored, status }) }
}

test('Send now and Schedule wait for unsaved edits to be saved', async () => {
  serveDraft()
  const { screen } = await renderWithRouter(<BroadcastEditPage />, EDIT_ROUTE)
  await expect.element(screen.getByRole('button', { name: 'Send now' })).toBeEnabled()

  await screen.getByLabelText(/^Name/).fill('Launch, edited')
  await screen.getByLabelText('Schedule for').fill('2030-05-06T07:08')

  await expect.element(screen.getByRole('button', { name: 'Send now' })).toBeDisabled()
  await expect.element(screen.getByRole('button', { name: 'Schedule', exact: true })).toBeDisabled()
  await expect
    .element(screen.getByText('Save your changes first', { exact: false }))
    .toBeInTheDocument()
})

test('Send now and Schedule wait while Save is pending, then deliver what was saved', async () => {
  const gate = requestGate()
  const sent: string[] = []
  serveDraft()
  worker.use(
    handleSiteBroadcastsUpdate(async () => {
      await gate.hold()
      return HttpResponse.json({ ...broadcast('draft'), name: 'Launch, edited' })
    }),
    handleSiteBroadcastsSend(({ params }) => {
      sent.push(params.id)
      return HttpResponse.json(broadcast('sending'))
    }),
  )
  const { screen, navigate } = await renderWithRouter(<BroadcastEditPage />, EDIT_ROUTE)
  await screen.getByLabelText(/^Name/).fill('Launch, edited')
  await screen.getByLabelText('Schedule for').fill('2030-05-06T07:08')
  await screen.getByRole('button', { name: 'Save' }).click()
  await gate.received

  await expect.element(screen.getByRole('button', { name: 'Send now' })).toBeDisabled()
  await expect.element(screen.getByRole('button', { name: 'Schedule', exact: true })).toBeDisabled()
  gate.release()

  await expect.element(screen.getByText('Broadcast updated')).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Send now' }).click()
  await expect.poll(() => sent).toEqual(['7'])
  await expect.poll(() => navigate.mock.calls).toContainEqual([REPORT])
})

test('scheduling a draft posts the chosen time as an ISO timestamp and opens the report', async () => {
  const bodies: unknown[] = []
  serveDraft()
  worker.use(
    handleSiteBroadcastsSchedule(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(broadcast('scheduled'))
    }),
  )
  const { screen, navigate } = await renderWithRouter(<BroadcastEditPage />, EDIT_ROUTE)

  await screen.getByLabelText('Schedule for').fill('2030-05-06T07:08')
  await screen.getByRole('button', { name: 'Schedule', exact: true }).click()

  await expect.element(screen.getByText('Broadcast scheduled')).toBeInTheDocument()
  expect(bodies).toEqual([{ scheduledAt: new Date('2030-05-06T07:08').toISOString() }])
  await expect.poll(() => navigate.mock.calls).toContainEqual([REPORT])
})

test('a refused Send now shows the error toast and stays on the page', async () => {
  serveDraft()
  worker.use(
    handleSiteBroadcastsSend(() => problem(409, { title: 'Conflict', detail: 'Already sending' })),
  )
  const { screen, navigate } = await renderWithRouter(<BroadcastEditPage />, EDIT_ROUTE)

  await screen.getByRole('button', { name: 'Send now' }).click()

  await expect.element(screen.getByText(SEND_FAILED)).toBeInTheDocument()
  await expect.element(screen.getByText('Already sending')).toBeInTheDocument()
  expect(navigate).not.toHaveBeenCalled()
})

test('a test send posts the typed address and confirms without navigating', async () => {
  const bodies: unknown[] = []
  serveDraft()
  worker.use(
    handleSiteBroadcastsTestSend(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ from: 'noreply@sphericon.test', captured: false })
    }),
  )
  const { screen, navigate } = await renderWithRouter(<BroadcastEditPage />, EDIT_ROUTE)

  await screen.getByLabelText('Send a test to').fill('qa@example.com')
  await screen.getByRole('button', { name: 'Send test' }).click()

  await expect.element(screen.getByText('Test email sent')).toBeInTheDocument()
  await expect.element(screen.getByText(/Sent from noreply@sphericon\.test/)).toBeInTheDocument()
  expect(bodies).toEqual([{ email: 'qa@example.com' }])
  expect(navigate).not.toHaveBeenCalled()
})

test('a test send captured by a Test inbox says it was not sent to the address named', async () => {
  serveDraft()
  worker.use(
    handleSiteBroadcastsTestSend(() =>
      HttpResponse.json({ from: 'hello@acme.test', captured: true }),
    ),
  )
  const { screen } = await renderWithRouter(<BroadcastEditPage />, EDIT_ROUTE)

  await screen.getByLabelText('Send a test to').fill('qa@example.com')
  await screen.getByRole('button', { name: 'Send test' }).click()

  await expect
    .element(screen.getByText('Captured in your Test inbox, not sent to the address you named.'))
    .toBeInTheDocument()
  await expect.element(screen.getByText(/Sent from/)).not.toBeInTheDocument()
})

test('a rejected test send shows the error toast', async () => {
  serveDraft()
  worker.use(
    handleSiteBroadcastsTestSend(() => problem(422, { title: 'Bad', detail: 'No integration' })),
  )
  const { screen } = await renderWithRouter(<BroadcastEditPage />, EDIT_ROUTE)

  await screen.getByLabelText('Send a test to').fill('qa@example.com')
  await screen.getByRole('button', { name: 'Send test' }).click()

  await expect.element(screen.getByText(SEND_FAILED)).toBeInTheDocument()
})

// Telegram has no Test send (ADR 0080): the action is not offered, and scheduling is.
test('a Telegram broadcast offers no test send and can be scheduled', async () => {
  const bodies: unknown[] = []
  const draft = serveDraft({ text: 'Hello', buttons: [], linkPreview: true }, 'telegram')
  worker.use(
    handleSiteBroadcastsSchedule(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(draft.as('scheduled'))
    }),
  )
  const { screen } = await renderWithRouter(<BroadcastEditPage />, EDIT_ROUTE)

  await screen.getByLabelText('Schedule for').fill('2030-05-06T07:08')
  await screen.getByRole('button', { name: 'Schedule', exact: true }).click()

  await expect.element(screen.getByText('Broadcast scheduled')).toBeInTheDocument()
  expect(bodies).toEqual([{ scheduledAt: new Date('2030-05-06T07:08').toISOString() }])
  expect(screen.getByLabelText('Send a test to').elements()).toHaveLength(0)
  expect(screen.getByRole('button', { name: 'Send test' }).elements()).toHaveLength(0)
})

// An SMS Broadcast is tested on a phone number the author types (ADR 0080), then scheduled.
test('an SMS broadcast is test-sent to the typed number, then scheduled', async () => {
  const bodies: unknown[] = []
  const schedules: unknown[] = []
  const draft = serveDraft({ text: 'Hello', linkTracking: true }, 'sms')
  worker.use(
    handleSiteBroadcastsTestSend(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ captured: false })
    }),
    handleSiteBroadcastsSchedule(async ({ request }) => {
      schedules.push(await request.json())
      return HttpResponse.json(draft.as('scheduled'))
    }),
  )
  const { screen } = await renderWithRouter(<BroadcastEditPage />, EDIT_ROUTE)

  await screen.getByLabelText('Send a test text to').fill('+1 415 555 0100')
  await screen.getByRole('button', { name: 'Send test' }).click()

  await expect.element(screen.getByText('Test text sent')).toBeInTheDocument()
  expect(bodies).toEqual([{ phone: '+1 415 555 0100' }])
  expect(screen.getByLabelText('Send a test to').elements()).toHaveLength(0)
  await expect.element(screen.getByText(/Sent from/)).not.toBeInTheDocument()

  await screen.getByLabelText('Schedule for').fill('2030-05-06T07:08')
  await screen.getByRole('button', { name: 'Schedule', exact: true }).click()

  await expect.element(screen.getByText('Broadcast scheduled')).toBeInTheDocument()
  expect(schedules).toEqual([{ scheduledAt: new Date('2030-05-06T07:08').toISOString() }])
})
