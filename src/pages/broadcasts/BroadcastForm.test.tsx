import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteBroadcastsCreate,
  handleSiteSegmentsList,
  handleSiteSendingDomainsList,
  handleSiteTemplatesList,
} from '../../generated/site/msw.gen.ts'
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

function serve(templates: ReturnType<typeof template>[], bodies: unknown[] = []) {
  worker.use(
    handleSiteSegmentsList(() =>
      HttpResponse.json({ items: [{ id: '9', name: 'VIPs' }], totalItems: 1 }),
    ),
    handleSiteTemplatesList(() =>
      HttpResponse.json({ items: templates, totalItems: templates.length }),
    ),
    handleSiteSendingDomainsList(() => HttpResponse.json({ items: [], totalItems: 0 })),
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
