import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteBrandKitGet,
  handleSiteTemplatesCreate,
  handleSiteTemplatesGet,
  handleSiteTemplatesUpdate,
} from '../../generated/site/msw.gen.ts'
import { Route as templatesEditRoute } from '../../routes/workspaces/$slug/templates/$templateId/edit.tsx'
import { Route as templatesRoute } from '../../routes/workspaces/$slug/templates/index.tsx'
import { Route as templatesCreateRoute } from '../../routes/workspaces/$slug/templates/new.tsx'
import { hex, TIMESTAMPS, UNSAVED_BRAND_KIT } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { TemplateCreatePage, TemplateEditPage } from './resource.tsx'

const EDIT_ROUTE = routeMount(templatesEditRoute, { slug: 'test', templateId: '7' })

const DOC = {
  type: 'doc',
  content: [{ type: 'paragraph', content: [{ type: 'text', text: 'Hello there' }] }],
}

const WITH_DOC = {
  id: '7',
  name: 'Welcome',
  subject: 'Hi',
  body: '<p>Hello there</p>',
  doc: DOC,
  editorVersion: '1.7.14',
  ...TIMESTAMPS,
}

const AS_HTML = {
  id: '7',
  name: 'Imported',
  subject: 'Hi',
  body: '<table><tr><td>Hello</td></tr></table>',
  doc: null,
  editorVersion: null,
  ...TIMESTAMPS,
}

function captureUpdates(saved: object) {
  const bodies: unknown[] = []
  worker.use(
    handleSiteTemplatesUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(saved)
    }),
  )
  return bodies
}

test('Cancel links back to the templates list of the current workspace', async () => {
  const { screen } = await renderWithRouter(
    <TemplateCreatePage />,
    routeMount(templatesCreateRoute, { slug: 'test' }),
  )

  await expect
    .element(screen.getByRole('link', { name: 'Cancel' }))
    .toHaveAttribute('href', routeMount(templatesRoute, { slug: 'test' }).initialPath)
})

test('saving a message written in the editor sends its document and no body', async () => {
  worker.use(handleSiteTemplatesGet({ body: WITH_DOC }))
  const bodies = captureUpdates(WITH_DOC)

  const { screen } = await renderWithRouter(<TemplateEditPage />, EDIT_ROUTE)
  await expect.element(screen.getByRole('button', { name: 'Insert variable' })).toBeEnabled()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => bodies).toEqual([{ name: 'Welcome', subject: 'Hi', doc: DOC }])
})

test('the variable picker inserts a merge tag into the document', async () => {
  worker.use(handleSiteTemplatesGet({ body: WITH_DOC }))
  const bodies = captureUpdates(WITH_DOC)

  const { screen } = await renderWithRouter(<TemplateEditPage />, EDIT_ROUTE)
  await expect.element(screen.getByRole('button', { name: 'Insert variable' })).toBeEnabled()
  await screen.getByRole('button', { name: 'Insert variable' }).click()
  await screen.getByRole('menuitem', { name: 'first_name' }).click()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => JSON.stringify(bodies)).toContain('{{ first_name }}')
})

test('a message with no document opens as editable source and saves a body only', async () => {
  worker.use(handleSiteTemplatesGet({ body: AS_HTML }))
  const bodies = captureUpdates(AS_HTML)

  const { screen } = await renderWithRouter(<TemplateEditPage />, EDIT_ROUTE)
  const source = screen.getByLabelText('Body')
  await expect.element(source).toHaveValue(AS_HTML.body)
  await source.fill('<p>Rewritten</p>')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([{ name: 'Imported', subject: 'Hi', body: '<p>Rewritten</p>' }])
})

test('a refused save shows the reason the server gave', async () => {
  worker.use(
    handleSiteTemplatesGet({ body: WITH_DOC }),
    handleSiteTemplatesUpdate(() => problem(422, { detail: 'heading level 9 is not allowed' })),
  )

  const { screen } = await renderWithRouter(<TemplateEditPage />, EDIT_ROUTE)
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.element(screen.getByText('heading level 9 is not allowed')).toBeInTheDocument()
})

test('a new message is authored in the Brand kit and its palette is written into the document', async () => {
  worker.use(
    handleSiteBrandKitGet({
      body: {
        ...UNSAVED_BRAND_KIT,
        saved: true,
        accentColor: hex('ff5500'),
        updatedAt: TIMESTAMPS.updatedAt,
      },
    }),
  )
  const bodies: unknown[] = []
  worker.use(
    handleSiteTemplatesCreate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(WITH_DOC, { status: 201 })
    }),
  )

  const { screen } = await renderWithRouter(
    <TemplateCreatePage />,
    routeMount(templatesCreateRoute, { slug: 'test' }),
  )
  await screen.getByLabelText('Name', { exact: false }).fill('Branded')
  await expect.element(screen.getByRole('button', { name: 'Insert variable' })).toBeEnabled()
  await screen.getByRole('button', { name: 'Insert variable' }).click()
  await screen.getByRole('menuitem', { name: 'first_name' }).click()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => JSON.stringify(bodies)).toContain('globalContent')
  expect(JSON.stringify(bodies)).toContain(hex('ff5500'))
})

test('a message already written is opened with the look it has, not the Brand kit', async () => {
  worker.use(handleSiteTemplatesGet({ body: WITH_DOC }))
  const bodies = captureUpdates(WITH_DOC)

  const { screen } = await renderWithRouter(<TemplateEditPage />, EDIT_ROUTE)
  await expect.element(screen.getByRole('button', { name: 'Insert variable' })).toBeEnabled()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => bodies).toEqual([{ name: 'Welcome', subject: 'Hi', doc: DOC }])
})
