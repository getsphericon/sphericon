import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteBroadcastsCreate,
  handleSiteIntegrationsList,
  handleSiteSegmentsList,
  handleSiteSegmentsPreview,
  handleSiteSendingDomainsList,
  handleSiteTemplatesList,
  handleSiteWorkspacesList,
} from '../../generated/site/msw.gen.ts'
import type { SiteSendCap } from '../../generated/site/types.gen.ts'
import { Route as broadcastsRoute } from '../../routes/workspaces/$workspaceId/broadcasts/index.tsx'
import { Route as broadcastsCreateRoute } from '../../routes/workspaces/$workspaceId/broadcasts/new.tsx'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { BroadcastCreatePage } from './resource.tsx'

const mount = routeMount(broadcastsCreateRoute, { workspaceId: 'TestWs01' })

const DOC = {
  type: 'doc',
  content: [{ type: 'paragraph', content: [{ type: 'text', text: 'Hello there' }] }],
}

const template = (id: string, subject: string, doc: object | null = null) => ({
  id,
  name: `Template ${id}`,
  subject,
  body: `<p>${id}</p>`,
  doc,
  editorVersion: doc ? '1.7.14' : null,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
})

const workspace = {
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

const VIPS = {
  kind: 'group' as const,
  combinator: 'and' as const,
  rules: [{ kind: 'rule' as const, field: 'email', operator: 'contains' as const, value: 'vip' }],
}

// The Workspace's one Integration: email is the Channel a new Broadcast is offered.
const EMAIL_INTEGRATION = {
  id: '1',
  name: 'Primary SMTP',
  channel: 'email' as const,
  provider: 'smtp' as const,
  enabled: true,
  isDefault: true,
  maxPerSecond: null,
  maxPerDay: null,
  providerLimit: {
    perSecond: { limit: null, source: null },
    perDay: { limit: null, source: null },
    sentLast24h: 0,
    warnings: ['unlimited' as const],
  },
  config: { kind: 'smtp' as const, host: 'smtp.test', port: 587, from: 'a@acme.com' },
  preset: null,
  params: {},
  secretsRetiring: [],
  hookUrl: 'https://app.test/hooks/omik_one',
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

// The new-Broadcast page past its Channel step: the email editor.
async function openEmailEditor() {
  const { screen } = await renderWithRouter(<BroadcastCreatePage />, mount)
  await screen.getByRole('button', { name: 'Continue' }).click()
  return screen
}

function serve(
  templates: ReturnType<typeof template>[],
  bodies: unknown[] = [],
  sendCap: SiteSendCap | null = null,
) {
  worker.use(
    handleSiteWorkspacesList({ body: [{ ...workspace, sendCap }] }),
    handleSiteSegmentsList(() =>
      HttpResponse.json({ items: [{ id: '9', name: 'VIPs', condition: VIPS }], totalItems: 1 }),
    ),
    handleSiteTemplatesList(() =>
      HttpResponse.json({ items: templates, totalItems: templates.length }),
    ),
    handleSiteSendingDomainsList(() => HttpResponse.json({ items: [], totalItems: 0 })),
    handleSiteIntegrationsList({ body: [EMAIL_INTEGRATION] }),
    handleSiteBroadcastsCreate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ id: '42', name: 'x' }, { status: 201 })
    }),
  )
}

test('picking a template copies its subject and document, and saves the document only', async () => {
  const bodies: unknown[] = []
  serve([template('1', 'Welcome aboard', DOC)], bodies)
  const screen = await openEmailEditor()

  await screen.getByLabelText(/^Name/).fill('Launch')
  await screen.getByRole('combobox', { name: 'Start from template' }).click()
  await screen.getByRole('option', { name: 'Template 1' }).click()
  await expect.element(screen.getByLabelText(/^Subject/)).toHaveValue('Welcome aboard')
  await expect.element(screen.getByRole('button', { name: 'Insert variable' })).toBeEnabled()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([
      { name: 'Launch', content: { channel: 'email', subject: 'Welcome aboard', doc: DOC } },
    ])
})

test('a template written as HTML is copied as source and saves its body only', async () => {
  const bodies: unknown[] = []
  serve([template('2', 'Plain')], bodies)
  const screen = await openEmailEditor()

  await screen.getByLabelText(/^Name/).fill('Launch')
  await screen.getByRole('combobox', { name: 'Start from template' }).click()
  await screen.getByRole('option', { name: 'Template 2' }).click()
  await expect.element(screen.getByLabelText('Body')).toHaveValue('<p>2</p>')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([
      { name: 'Launch', content: { channel: 'email', subject: 'Plain', body: '<p>2</p>' } },
    ])
})

test('a template without a subject keeps the subject already typed', async () => {
  serve([template('3', '')])
  const screen = await openEmailEditor()

  await screen.getByLabelText(/^Subject/).fill('My subject')
  await screen.getByRole('combobox', { name: 'Start from template' }).click()
  await screen.getByRole('option', { name: 'Template 3' }).click()

  await expect.element(screen.getByLabelText('Body')).toHaveValue('<p>3</p>')
  await expect.element(screen.getByLabelText(/^Subject/)).toHaveValue('My subject')
})

test('the template picker is hidden when the workspace has no templates', async () => {
  serve([])
  const screen = await openEmailEditor()

  await expect.element(screen.getByLabelText(/^Subject/)).toBeInTheDocument()
  expect(screen.getByText('Start from template').elements()).toHaveLength(0)
})

test('Cancel links back to the broadcasts list of the current workspace', async () => {
  serve([])
  const screen = await openEmailEditor()

  await expect
    .element(screen.getByRole('link', { name: 'Cancel' }))
    .toHaveAttribute('href', routeMount(broadcastsRoute, { workspaceId: 'TestWs01' }).initialPath)
})

// A full bucket of 5000 goes at once and 5000 more over the next 24 hours: the other 2000 take a
// second day.
test('an audience larger than the Send cap is warned about with the days it takes', async () => {
  serve([], [], { perDay: 5000, sentLast24h: 0 })
  worker.use(handleSiteSegmentsPreview({ body: { count: 12_000 } }))
  const screen = await openEmailEditor()

  await expect
    .element(screen.getByText(/12,000 contacts.*Send cap.*about 2 days/))
    .toBeInTheDocument()
})

// No preview handler is installed: a count asked without a Send cap fails the test as an unmocked
// request once it settles.
test('without a Send cap nothing is warned about and the audience is not counted', async () => {
  serve([])
  const screen = await openEmailEditor()

  await expect.element(screen.getByRole('combobox', { name: 'Segment' })).toBeInTheDocument()
  expect(screen.getByText(/Send cap/).elements()).toHaveLength(0)
})

// After 4000 sends in the last 24 hours, 1000 go at once and 5000 more within the day.
test('an audience that goes out within a day of the Send cap is not warned about', async () => {
  serve([], [], { perDay: 5000, sentLast24h: 4000 })
  worker.use(
    handleSiteSegmentsPreview(async ({ request }) => {
      const body = await request.json()
      return HttpResponse.json({ count: body.condition ? 20_000 : 5000 })
    }),
  )
  const screen = await openEmailEditor()
  const segment = screen.getByRole('combobox', { name: 'Segment' })

  await segment.click()
  await screen.getByRole('option', { name: 'VIPs' }).click()
  await expect.element(screen.getByText(/about 4 days/)).toBeInTheDocument()
  await segment.click()
  await screen.getByRole('option', { name: 'All contacts' }).click()

  await expect.element(screen.getByText(/Send cap/)).not.toBeInTheDocument()
})

test('the chosen Segment is counted by its condition', async () => {
  const previews: unknown[] = []
  serve([], [], { perDay: 5000, sentLast24h: 0 })
  worker.use(
    handleSiteSegmentsPreview(async ({ request }) => {
      const body = await request.json()
      previews.push(body)
      return HttpResponse.json({ count: body.condition ? 20_000 : 3000 })
    }),
  )
  const screen = await openEmailEditor()

  await expect.poll(() => previews).toEqual([{}])
  await screen.getByRole('combobox', { name: 'Segment' }).click()
  await screen.getByRole('option', { name: 'VIPs' }).click()

  await expect.element(screen.getByText(/20,000 contacts.*about 3 days/)).toBeInTheDocument()
  expect(previews).toEqual([{}, { condition: VIPS }])
})

test('the Send cap warning does not stop the Broadcast from being saved', async () => {
  const bodies: unknown[] = []
  serve([], bodies, { perDay: 5000, sentLast24h: 0 })
  worker.use(handleSiteSegmentsPreview({ body: { count: 12_000 } }))
  const screen = await openEmailEditor()

  await expect.element(screen.getByText(/about 2 days/)).toBeInTheDocument()
  await screen.getByLabelText(/^Name/).fill('Launch')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => bodies).toHaveLength(1)
})

const TELEGRAM_INTEGRATION = {
  ...EMAIL_INTEGRATION,
  id: '2',
  name: 'News bot',
  channel: 'telegram' as const,
  provider: 'telegram' as const,
  isDefault: false,
  config: { kind: 'telegram' as const, botId: 7, username: 'news_bot' },
}

// A Workspace with an email and a Telegram Integration is offered both Channels; picking Telegram
// shows the text editor with its counter, and the Broadcast is saved with a Telegram content.
test('a Telegram broadcast is written as a text and saved under its Channel', async () => {
  const bodies: unknown[] = []
  serve([], bodies)
  worker.use(handleSiteIntegrationsList({ body: [EMAIL_INTEGRATION, TELEGRAM_INTEGRATION] }))
  const { screen } = await renderWithRouter(<BroadcastCreatePage />, mount)

  await screen.getByRole('radio', { name: 'Telegram' }).click()
  await screen.getByRole('button', { name: 'Continue' }).click()
  await screen.getByLabelText(/^Name/).fill('News')
  await screen.getByRole('combobox', { name: 'Send through' }).click()
  await screen.getByRole('option', { name: 'News bot' }).click()
  await screen.getByLabelText('Message').fill('Hello <b>{{ first_name }}</b>')
  await expect.element(screen.getByText('29 / 4,096')).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([
      {
        name: 'News',
        integrationId: '2',
        content: {
          channel: 'telegram',
          text: 'Hello <b>{{ first_name }}</b>',
          buttons: [],
          linkPreview: true,
        },
      },
    ])
})

test('a text past the Telegram limit is flagged while it is written', async () => {
  serve([])
  worker.use(handleSiteIntegrationsList({ body: [TELEGRAM_INTEGRATION] }))
  const { screen } = await renderWithRouter(<BroadcastCreatePage />, mount)

  await screen.getByRole('button', { name: 'Continue' }).click()
  await screen.getByLabelText('Message').fill('a'.repeat(4097))

  await expect.element(screen.getByText('4,097 / 4,096')).toBeInTheDocument()
  await expect.element(screen.getByText(/cannot go out until it is shorter/)).toBeInTheDocument()
})

// The Telegram Broadcast has the same Message content as a step: an image (its caption has the
// smaller limit), link buttons, the link preview and an unsubscribe button.
test('a Telegram broadcast carries an image, buttons, no preview and an unsubscribe button', async () => {
  const bodies: unknown[] = []
  serve([], bodies)
  worker.use(handleSiteIntegrationsList({ body: [TELEGRAM_INTEGRATION] }))
  const { screen } = await renderWithRouter(<BroadcastCreatePage />, mount)

  await screen.getByRole('button', { name: 'Continue' }).click()
  await screen.getByLabelText(/^Name/).fill('News')
  await screen.getByLabelText('Message').fill('Hello')
  await screen.getByLabelText('Image URL').fill('https://example.com/a.png')
  await expect.element(screen.getByText('5 / 1,024')).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Add a button' }).click()
  await screen.getByRole('textbox', { name: 'Button 1 label' }).fill('Open')
  await screen.getByRole('textbox', { name: 'Button 1 URL' }).fill('https://example.com/b')
  await screen.getByText('Show a preview of links in the text').click({ force: true })
  await screen.getByText('Unsubscribe button', { exact: true }).click({ force: true })
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([
      {
        name: 'News',
        content: {
          channel: 'telegram',
          text: 'Hello',
          imageUrl: 'https://example.com/a.png',
          buttons: [{ label: 'Open', url: 'https://example.com/b' }],
          linkPreview: false,
          unsubscribeButton: 'Unsubscribe',
        },
      },
    ])
})

// Only the Contacts with a Binding on the bot are reached, so the Send cap is compared with the
// recipients the preview counts for the Channel and Integration, not with every matching Contact.
test('a Telegram broadcast is warned about by its recipients, not by every contact', async () => {
  const previews: unknown[] = []
  serve([], [], { perDay: 5000, sentLast24h: 0 })
  worker.use(
    handleSiteIntegrationsList({ body: [TELEGRAM_INTEGRATION] }),
    handleSiteSegmentsPreview(async ({ request }) => {
      const body = await request.json()
      previews.push(body)
      return HttpResponse.json({ count: 50_000, recipients: body.integrationId ? 12_000 : 100 })
    }),
  )
  const { screen } = await renderWithRouter(<BroadcastCreatePage />, mount)

  await screen.getByRole('button', { name: 'Continue' }).click()
  await expect.poll(() => previews).toEqual([{ channel: 'telegram' }])
  await expect.element(screen.getByText(/Send cap/)).not.toBeInTheDocument()
  await screen.getByRole('combobox', { name: 'Send through' }).click()
  await screen.getByRole('option', { name: 'News bot' }).click()

  await expect
    .element(screen.getByText(/12,000 contacts.*Send cap.*about 2 days/))
    .toBeInTheDocument()
  expect(previews).toEqual([{ channel: 'telegram' }, { channel: 'telegram', integrationId: '2' }])
})

const SMS_INTEGRATION = {
  ...EMAIL_INTEGRATION,
  id: '3',
  name: 'Acme Twilio',
  channel: 'sms' as const,
  provider: 'twilio' as const,
  isDefault: false,
  config: {
    kind: 'twilio' as const,
    accountSid: 'AC00000000000000000000000000000001',
    apiKeySid: null,
    from: '+15017250604',
    fromKind: 'number' as const,
    callbacksSet: true,
  },
}

// SMS is offered only once the Workspace has an SMS Integration (ADR 0080).
test('SMS is not offered without an SMS integration', async () => {
  serve([])
  worker.use(handleSiteIntegrationsList({ body: [EMAIL_INTEGRATION, TELEGRAM_INTEGRATION] }))
  const { screen } = await renderWithRouter(<BroadcastCreatePage />, mount)

  await expect.element(screen.getByRole('radio', { name: 'Telegram' })).toBeInTheDocument()
  expect(screen.getByRole('radio', { name: 'SMS' }).elements()).toHaveLength(0)
})

test('an SMS broadcast is written as a plain text and saved under its Channel', async () => {
  const bodies: unknown[] = []
  serve([], bodies)
  worker.use(handleSiteIntegrationsList({ body: [EMAIL_INTEGRATION, SMS_INTEGRATION] }))
  const { screen } = await renderWithRouter(<BroadcastCreatePage />, mount)

  await screen.getByRole('radio', { name: 'SMS' }).click()
  await screen.getByRole('button', { name: 'Continue' }).click()
  await screen.getByLabelText(/^Name/).fill('Flash sale')
  await screen.getByRole('combobox', { name: 'Send through' }).click()
  await screen.getByRole('option', { name: 'Acme Twilio' }).click()
  await screen.getByRole('textbox', { name: /^Message/ }).fill('Hi {{ first_name }}, 20% off')
  // A brace is a GSM-7 extension character, counted twice.
  await expect.element(screen.getByText('32 characters · GSM-7 · 1 segment')).toBeVisible()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([
      {
        name: 'Flash sale',
        integrationId: '3',
        content: { channel: 'sms', text: 'Hi {{ first_name }}, 20% off', linkTracking: true },
      },
    ])
})

test('an SMS broadcast counts its links as short links and may keep them as written', async () => {
  const bodies: unknown[] = []
  serve([], bodies)
  worker.use(handleSiteIntegrationsList({ body: [EMAIL_INTEGRATION, SMS_INTEGRATION] }))
  const { screen } = await renderWithRouter(<BroadcastCreatePage />, mount)
  const text = `Sale https://shop.example/${'p'.repeat(100)}`

  await screen.getByRole('radio', { name: 'SMS' }).click()
  await screen.getByRole('button', { name: 'Continue' }).click()
  await screen.getByLabelText(/^Name/).fill('Flash sale')
  await screen.getByRole('combobox', { name: 'Send through' }).click()
  await screen.getByRole('option', { name: 'Acme Twilio' }).click()
  await screen.getByRole('textbox', { name: /^Message/ }).fill(text)
  await expect
    .element(screen.getByText(`${5 + workspace.shortLinkLength} characters · GSM-7 · 1 segment`))
    .toBeVisible()
  await screen.getByText('Track links').click({ force: true })
  await expect.element(screen.getByText('126 characters · GSM-7 · 1 segment')).toBeVisible()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([
      {
        name: 'Flash sale',
        integrationId: '3',
        content: { channel: 'sms', text, linkTracking: false },
      },
    ])
})
