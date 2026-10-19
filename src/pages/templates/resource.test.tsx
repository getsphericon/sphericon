import { useQuery } from '@tanstack/react-query'
import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  siteTemplatesGetOptions,
  siteTemplatesListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import {
  handleSiteBrandKitGet,
  handleSiteTemplatesCreate,
  handleSiteTemplatesGet,
  handleSiteTemplatesList,
  handleSiteTemplatesUpdate,
} from '../../generated/site/msw.gen.ts'
import { Route as templatesEditRoute } from '../../routes/workspaces/$slug/templates/$templateId/edit.tsx'
import { Route as templatesCreateRoute } from '../../routes/workspaces/$slug/templates/new.tsx'
import { UNSAVED_BRAND_KIT } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { TemplateCreatePage, TemplateEditPage } from './resource.tsx'

const CREATE_ROUTE = routeMount(templatesCreateRoute, { slug: 'test' })

const EDIT_ROUTE = routeMount(templatesEditRoute, { slug: 'test', templateId: '7' })

function ListProbe() {
  useQuery(siteTemplatesListOptions({ path: { slug: 'test' } }))
  return null
}

function DetailProbe() {
  useQuery(siteTemplatesGetOptions({ path: { slug: 'test', id: '7' } }))
  return null
}

test('creating a template sends the filled keys, refreshes the list and opens the edit page', async () => {
  const bodies: unknown[] = []
  let listFetches = 0
  worker.use(
    handleSiteBrandKitGet({ body: UNSAVED_BRAND_KIT }),
    handleSiteTemplatesCreate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ id: '42', name: 'Welcome' }, { status: 201 })
    }),
    handleSiteTemplatesList(() => {
      listFetches++
      return HttpResponse.json({ items: [], totalItems: 0 })
    }),
  )

  const { screen, navigate } = await renderWithRouter(
    <>
      <ListProbe />
      <TemplateCreatePage />
    </>,
    CREATE_ROUTE,
  )
  await expect.poll(() => listFetches).toBe(1)

  await screen.getByLabelText('Name', { exact: false }).fill('Welcome')
  await screen.getByLabelText('Subject').fill('Hi')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => bodies).toEqual([{ name: 'Welcome', subject: 'Hi' }])
  await expect.element(screen.getByText('Template created')).toBeInTheDocument()
  await expect.poll(() => listFetches).toBe(2)
  await expect.poll(() => navigate.mock.calls.length).toBe(1)
  expect(navigate.mock.calls[0]?.[0]).toMatchObject({
    params: { slug: 'test', templateId: '42' },
  })
})

test('a blank required name is rejected with a readable message before anything is sent', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteBrandKitGet({ body: UNSAVED_BRAND_KIT }),
    handleSiteTemplatesCreate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ id: '1' }, { status: 201 })
    }),
  )

  const { screen } = await renderWithRouter(<TemplateCreatePage />, CREATE_ROUTE)
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.element(screen.getByText('This field is required')).toBeVisible()
  expect(bodies).toEqual([])
})

test('editing shows a loader, then existing values; cleared fields are sent as empty strings', async () => {
  const bodies: unknown[] = []
  let detailFetches = 0
  let listFetches = 0
  const detailGate = Promise.withResolvers<undefined>()
  worker.use(
    handleSiteTemplatesUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ id: '7', name: 'Welcome' })
    }),
    handleSiteTemplatesList(() => {
      listFetches++
      return HttpResponse.json({ items: [], totalItems: 0 })
    }),
    handleSiteTemplatesGet(async () => {
      detailFetches++
      if (detailFetches === 1) {
        await detailGate.promise
      }
      return HttpResponse.json({ id: '7', name: 'Welcome', subject: 'Hi', body: '<p>x</p>' })
    }),
  )

  const { screen } = await renderWithRouter(
    <>
      <ListProbe />
      <DetailProbe />
      <TemplateEditPage />
    </>,
    EDIT_ROUTE,
  )

  await expect.element(screen.getByLabelText('Name', { exact: false })).not.toBeInTheDocument()
  detailGate.resolve(undefined)

  await expect.element(screen.getByLabelText('Name', { exact: false })).toHaveValue('Welcome')
  await expect.element(screen.getByLabelText('Subject')).toHaveValue('Hi')

  await screen.getByLabelText('Subject').fill('')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => bodies).toEqual([{ name: 'Welcome', subject: '', body: '<p>x</p>' }])
  await expect.element(screen.getByText('Template updated')).toBeInTheDocument()
  await expect.poll(() => listFetches).toBe(2)
  await expect.poll(() => detailFetches).toBeGreaterThan(1)
})

test('editing shows an error alert when the template cannot be loaded', async () => {
  worker.use(
    handleSiteTemplatesGet(() =>
      problem(404, { title: 'Not Found', detail: 'template not found' }),
    ),
  )

  const { screen } = await renderWithRouter(<TemplateEditPage />, EDIT_ROUTE)

  await expect.element(screen.getByText('Failed to load templates')).toBeVisible()
  await expect.element(screen.getByText('template not found')).toBeVisible()
})
