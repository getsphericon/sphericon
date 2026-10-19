import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteBroadcastsCreate,
  handleSiteSegmentsList,
  handleSiteTemplatesList,
} from '../../generated/site/msw.gen.ts'
import { Route as broadcastsRoute } from '../../routes/workspaces/$slug/broadcasts/index.tsx'
import { Route as broadcastsCreateRoute } from '../../routes/workspaces/$slug/broadcasts/new.tsx'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { BroadcastCreatePage } from './resource.tsx'

const mount = routeMount(broadcastsCreateRoute, { slug: 'test' })

const template = (id: string, subject: string) => ({
  id,
  name: `Template ${id}`,
  subject,
  body: `<p>${id}</p>`,
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
    handleSiteBroadcastsCreate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ id: '42', name: 'x' }, { status: 201 })
    }),
  )
}

test('picking a template copies its subject and body into the form', async () => {
  serve([template('1', 'Welcome aboard')])
  const { screen } = await renderWithRouter(<BroadcastCreatePage />, mount)

  await screen.getByRole('combobox', { name: 'Start from template' }).click()
  await screen.getByRole('option', { name: 'Template 1' }).click()

  await expect.element(screen.getByLabelText(/^Subject/)).toHaveValue('Welcome aboard')
  await expect.element(screen.getByLabelText(/^Email body/)).toHaveValue('<p>1</p>')
})

test('a template without a subject keeps the subject already typed', async () => {
  serve([template('2', '')])
  const { screen } = await renderWithRouter(<BroadcastCreatePage />, mount)

  await screen.getByLabelText(/^Subject/).fill('My subject')
  await screen.getByRole('combobox', { name: 'Start from template' }).click()
  await screen.getByRole('option', { name: 'Template 2' }).click()

  await expect.element(screen.getByLabelText(/^Email body/)).toHaveValue('<p>2</p>')
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
    .toHaveAttribute('href', routeMount(broadcastsRoute, { slug: 'test' }).initialPath)
})
