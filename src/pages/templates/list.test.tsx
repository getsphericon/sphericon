import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSiteTemplatesDelete, handleSiteTemplatesList } from '../../generated/site/msw.gen.ts'
import { Route as templatesRoute } from '../../routes/workspaces/$slug/templates/index.tsx'
import { page, TIMESTAMPS } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { TemplatesListPage } from './list.tsx'

const SLUG = { slug: 'test' }
const LIST_ROUTE = routeMount(templatesRoute, SLUG)

const WELCOME = { id: '1', name: 'Welcome', subject: 'Hello there', body: '', ...TIMESTAMPS }
const RECEIPT = { id: '2', name: 'Receipt', subject: 'Your receipt', body: '', ...TIMESTAMPS }

test('lists the workspace templates', async () => {
  worker.use(handleSiteTemplatesList({ body: page([WELCOME, RECEIPT], 2) }))

  const { screen } = await renderWithRouter(<TemplatesListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Welcome')).toBeInTheDocument()
  await expect.element(screen.getByText('Hello there')).toBeInTheDocument()
  await expect.element(screen.getByText('Receipt', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Your receipt')).toBeInTheDocument()
})

test('shows the empty state when there are no templates', async () => {
  worker.use(handleSiteTemplatesList({ body: page([], 0) }))

  const { screen } = await renderWithRouter(<TemplatesListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Create your first template.')).toBeInTheDocument()
})

test('shows an error alert when the list fails to load', async () => {
  worker.use(handleSiteTemplatesList(() => problem(500)))

  const { screen } = await renderWithRouter(<TemplatesListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Failed to load templates')).toBeInTheDocument()
})

test('New template links to the create page', async () => {
  worker.use(handleSiteTemplatesList({ body: page([], 0) }))

  const { screen } = await renderWithRouter(<TemplatesListPage />, LIST_ROUTE)

  await expect
    .element(screen.getByRole('link', { name: 'New template' }))
    .toHaveAttribute('href', '/workspaces/test/templates/new')
})

test('Edit links to the template edit page', async () => {
  worker.use(handleSiteTemplatesList({ body: page([WELCOME], 1) }))

  const { screen } = await renderWithRouter(<TemplatesListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('Welcome')).toBeInTheDocument()

  await expect
    .element(screen.getByRole('link', { name: 'Edit' }))
    .toHaveAttribute('href', '/workspaces/test/templates/1/edit')
})

test('deleting a template asks for confirmation, deletes it and refreshes the list', async () => {
  const deleted: string[] = []
  let listFetches = 0
  worker.use(
    handleSiteTemplatesList(() => {
      listFetches++
      return HttpResponse.json({
        items: deleted.length ? [RECEIPT] : [WELCOME, RECEIPT],
        totalItems: 2,
      })
    }),
    handleSiteTemplatesDelete(({ params }) => {
      deleted.push(params.id)
      return new HttpResponse(null, { status: 204 })
    }),
  )

  const { screen } = await renderWithRouter(<TemplatesListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('Welcome')).toBeInTheDocument()

  await screen.getByRole('button', { name: 'Delete' }).first().click()
  expect(deleted).toEqual([])
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete' }).click()

  await expect.poll(() => deleted).toEqual(['1'])
  await expect.element(screen.getByText('Template deleted')).toBeInTheDocument()
  await expect.poll(() => listFetches).toBe(2)
  await expect.element(screen.getByText('Welcome')).not.toBeInTheDocument()
})
