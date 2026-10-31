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
  holds: [],
  role: 'owner' as const,
  createdAt: '2026-01-01T00:00:00Z',
}

const VIPS = {
  kind: 'group' as const,
  combinator: 'and' as const,
  rules: [{ kind: 'rule' as const, field: 'email', operator: 'contains' as const, value: 'vip' }],
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
    handleSiteIntegrationsList({ body: [] }),
    handleSiteBroadcastsCreate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ id: '42', name: 'x' }, { status: 201 })
    }),
  )
}

test('picking a template copies its subject and document, and saves the document only', async () => {
  const bodies: unknown[] = []
  serve([template('1', 'Welcome aboard', DOC)], bodies)
  const { screen } = await renderWithRouter(<BroadcastCreatePage />, mount)

  await screen.getByLabelText(/^Name/).fill('Launch')
  await screen.getByRole('combobox', { name: 'Start from template' }).click()
  await screen.getByRole('option', { name: 'Template 1' }).click()
  await expect.element(screen.getByLabelText(/^Subject/)).toHaveValue('Welcome aboard')
  await expect.element(screen.getByRole('button', { name: 'Insert variable' })).toBeEnabled()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => bodies).toEqual([{ name: 'Launch', subject: 'Welcome aboard', doc: DOC }])
})

test('a template written as HTML is copied as source and saves its body only', async () => {
  const bodies: unknown[] = []
  serve([template('2', 'Plain')], bodies)
  const { screen } = await renderWithRouter(<BroadcastCreatePage />, mount)

  await screen.getByLabelText(/^Name/).fill('Launch')
  await screen.getByRole('combobox', { name: 'Start from template' }).click()
  await screen.getByRole('option', { name: 'Template 2' }).click()
  await expect.element(screen.getByLabelText('Body')).toHaveValue('<p>2</p>')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => bodies).toEqual([{ name: 'Launch', subject: 'Plain', body: '<p>2</p>' }])
})

test('a template without a subject keeps the subject already typed', async () => {
  serve([template('3', '')])
  const { screen } = await renderWithRouter(<BroadcastCreatePage />, mount)

  await screen.getByLabelText(/^Subject/).fill('My subject')
  await screen.getByRole('combobox', { name: 'Start from template' }).click()
  await screen.getByRole('option', { name: 'Template 3' }).click()

  await expect.element(screen.getByLabelText('Body')).toHaveValue('<p>3</p>')
  await expect.element(screen.getByLabelText(/^Subject/)).toHaveValue('My subject')
})

test('the template picker is hidden when the workspace has no templates', async () => {
  serve([])
  const { screen } = await renderWithRouter(<BroadcastCreatePage />, mount)

  await expect.element(screen.getByLabelText(/^Subject/)).toBeInTheDocument()
  expect(screen.getByText('Start from template').elements()).toHaveLength(0)
})

test('Cancel links back to the broadcasts list of the current workspace', async () => {
  serve([])
  const { screen } = await renderWithRouter(<BroadcastCreatePage />, mount)

  await expect
    .element(screen.getByRole('link', { name: 'Cancel' }))
    .toHaveAttribute('href', routeMount(broadcastsRoute, { workspaceId: 'TestWs01' }).initialPath)
})

// A full bucket of 5000 goes at once and 5000 more over the next 24 hours: the other 2000 take a
// second day.
test('an audience larger than the Send cap is warned about with the days it takes', async () => {
  serve([], [], { perDay: 5000, sentLast24h: 0 })
  worker.use(handleSiteSegmentsPreview({ body: { count: 12_000 } }))
  const { screen } = await renderWithRouter(<BroadcastCreatePage />, mount)

  await expect
    .element(screen.getByText(/12,000 contacts.*Send cap.*about 2 days/))
    .toBeInTheDocument()
})

// No preview handler is installed: a count asked without a Send cap fails the test as an unmocked
// request once it settles.
test('without a Send cap nothing is warned about and the audience is not counted', async () => {
  serve([])
  const { screen } = await renderWithRouter(<BroadcastCreatePage />, mount)

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
  const { screen } = await renderWithRouter(<BroadcastCreatePage />, mount)
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
  const { screen } = await renderWithRouter(<BroadcastCreatePage />, mount)

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
  const { screen } = await renderWithRouter(<BroadcastCreatePage />, mount)

  await expect.element(screen.getByText(/about 2 days/)).toBeInTheDocument()
  await screen.getByLabelText(/^Name/).fill('Launch')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => bodies).toHaveLength(1)
})
