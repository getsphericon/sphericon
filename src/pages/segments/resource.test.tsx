import { useQuery } from '@tanstack/react-query'
import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  siteSegmentsGetOptions,
  siteSegmentsListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import {
  handleSiteCustomFieldsList,
  handleSiteEventsActions,
  handleSiteSegmentsCreate,
  handleSiteSegmentsGet,
  handleSiteSegmentsList,
  handleSiteSegmentsUpdate,
} from '../../generated/site/msw.gen.ts'
import type { SiteSegmentRuleGroup } from '../../generated/site/types.gen.ts'
import { Route as segmentsEditRoute } from '../../routes/workspaces/$slug/segments/$segmentId/edit.tsx'
import { Route as segmentsCreateRoute } from '../../routes/workspaces/$slug/segments/new.tsx'
import { page } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { SegmentCreatePage, SegmentEditPage } from './resource.tsx'

const RULE: SiteSegmentRuleGroup = {
  kind: 'group',
  combinator: 'and',
  rules: [{ kind: 'rule', field: 'email', operator: 'contains', value: '@x' }],
}

const CREATE_ROUTE = routeMount(segmentsCreateRoute, { slug: 'test' })

const EDIT_ROUTE = routeMount(segmentsEditRoute, { slug: 'test', segmentId: '7' })

// Stands in for the segments list screen: an active list query, so invalidation after a save
// is observable as a refetch.
function ListProbe() {
  useQuery(siteSegmentsListOptions({ path: { slug: 'test' } }))
  return null
}

function DetailProbe() {
  useQuery(siteSegmentsGetOptions({ path: { slug: 'test', id: '7' } }))
  return null
}

// Everything the segment form reads besides the segment itself (rule builder catalogues).
function serveCatalogue() {
  worker.use(
    handleSiteEventsActions({ body: { actions: [] } }),
    handleSiteCustomFieldsList({ body: page([], 0) }),
  )
}

test('creating a segment sends name and a rule, refreshes the list and opens the edit page; there is no type selector', async () => {
  const bodies: unknown[] = []
  let listFetches = 0
  serveCatalogue()
  worker.use(
    handleSiteSegmentsCreate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ id: '42', name: 'Everyone' }, { status: 201 })
    }),
    handleSiteSegmentsList(() => {
      listFetches++
      return HttpResponse.json({ items: [], totalItems: 0 })
    }),
  )

  const { screen, navigate } = await renderWithRouter(
    <>
      <ListProbe />
      <SegmentCreatePage />
    </>,
    CREATE_ROUTE,
  )
  await expect.poll(() => listFetches).toBe(1)
  expect(screen.getByLabelText('Type').elements()).toEqual([])

  await screen.getByLabelText('Name', { exact: false }).fill('Everyone')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => bodies.length).toBe(1)
  // A new segment starts as the empty rule group, which matches every contact.
  expect(bodies).toEqual([
    { name: 'Everyone', definition: { kind: 'group', combinator: 'and', rules: [] } },
  ])
  await expect.element(screen.getByText('Segment created')).toBeInTheDocument()
  await expect.poll(() => listFetches).toBe(2)
  await expect.poll(() => navigate.mock.calls.length).toBe(1)
  expect(navigate.mock.calls[0]?.[0]).toMatchObject({
    params: { slug: 'test', segmentId: '42' },
  })
})

test('a blank name is rejected before anything is sent', async () => {
  const bodies: unknown[] = []
  serveCatalogue()
  worker.use(
    handleSiteSegmentsCreate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ id: '1' }, { status: 201 })
    }),
  )

  const { screen } = await renderWithRouter(<SegmentCreatePage />, CREATE_ROUTE)

  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.element(screen.getByLabelText('Name', { exact: false })).toBeInvalid()
  expect(bodies).toEqual([])
})

test('editing shows the stored rule untouched, saves a renamed segment and refreshes list and detail', async () => {
  const bodies: unknown[] = []
  let detailFetches = 0
  let listFetches = 0
  serveCatalogue()
  worker.use(
    handleSiteSegmentsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ id: '7', name: 'Renamed', definition: RULE })
    }),
    handleSiteSegmentsList(() => {
      listFetches++
      return HttpResponse.json({ items: [], totalItems: 0 })
    }),
    handleSiteSegmentsGet(() => {
      detailFetches++
      return HttpResponse.json({ id: '7', name: 'Mail users', definition: RULE })
    }),
  )

  const { screen } = await renderWithRouter(
    <>
      <ListProbe />
      <DetailProbe />
      <SegmentEditPage />
    </>,
    EDIT_ROUTE,
  )

  await expect.element(screen.getByLabelText('Name', { exact: false })).toHaveValue('Mail users')
  expect(screen.getByLabelText('Type').elements()).toEqual([])

  await screen.getByLabelText('Name', { exact: false }).fill('Renamed')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => bodies).toEqual([{ name: 'Renamed', definition: RULE }])
  await expect.element(screen.getByText('Segment updated')).toBeInTheDocument()
  await expect.poll(() => listFetches).toBe(2)
  await expect.poll(() => detailFetches).toBeGreaterThan(1)
})

test('editing shows an error alert when the segment cannot be loaded', async () => {
  serveCatalogue()
  worker.use(
    handleSiteSegmentsGet(() => problem(404, { title: 'Not Found', detail: 'segment not found' })),
  )

  const { screen } = await renderWithRouter(<SegmentEditPage />, EDIT_ROUTE)

  await expect.element(screen.getByText('Failed to load segments')).toBeVisible()
  await expect.element(screen.getByText('segment not found')).toBeVisible()
})

test('editing shows no form while the segment loads, then the loaded values', async () => {
  const gate = Promise.withResolvers<undefined>()
  serveCatalogue()
  worker.use(
    handleSiteSegmentsGet(async () => {
      await gate.promise
      return HttpResponse.json({ id: '7', name: 'Mail users', definition: RULE })
    }),
  )

  const { screen } = await renderWithRouter(<SegmentEditPage />, EDIT_ROUTE)

  await expect.element(screen.getByLabelText('Name', { exact: false })).not.toBeInTheDocument()
  gate.resolve(undefined)

  await expect.element(screen.getByLabelText('Name', { exact: false })).toHaveValue('Mail users')
})
