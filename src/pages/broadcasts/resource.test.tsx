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
  handleSiteBroadcastsSend,
  handleSiteBroadcastsUpdate,
  handleSiteSegmentsList,
  handleSiteTemplatesList,
} from '../../generated/site/msw.gen.ts'
import { Route as broadcastsEditRoute } from '../../routes/workspaces/$slug/broadcasts/$broadcastId/edit.tsx'
import { Route as broadcastsCreateRoute } from '../../routes/workspaces/$slug/broadcasts/new.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { BroadcastCreatePage, BroadcastEditPage } from './resource.tsx'

const CREATE_ROUTE = routeMount(broadcastsCreateRoute, { slug: 'test' })

const EDIT_ROUTE = routeMount(broadcastsEditRoute, { slug: 'test', broadcastId: '7' })

function ListProbe() {
  useQuery(siteBroadcastsListOptions({ path: { slug: 'test' } }))
  return null
}

function DetailProbe() {
  useQuery(siteBroadcastsGetOptions({ path: { slug: 'test', id: '7' } }))
  return null
}

function broadcast(status: string) {
  return {
    id: '7',
    name: 'Launch',
    subject: 'Hello',
    fromName: 'Ada',
    fromEmail: 'ada@example.com',
    body: '<p></p>',
    status,
  }
}

function empty() {
  return HttpResponse.json({ items: [], totalItems: 0 })
}

// Serves the broadcasts API: records write bodies and counts list/detail fetches.
function serve(status: string, detailGate?: Promise<void>) {
  const bodies: unknown[] = []
  const fetches = { list: 0, detail: 0 }
  worker.use(
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

  await screen.getByLabelText(/^Name/).fill('Launch')
  await screen.getByLabelText('Subject').fill('Hello')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => bodies).toEqual([{ name: 'Launch', subject: 'Hello' }])
  await expect.element(screen.getByText('Broadcast created')).toBeInTheDocument()
  await expect.poll(() => fetches.list).toBe(2)
  await expect.poll(() => navigate.mock.calls.length).toBe(1)
  expect(navigate.mock.calls[0]?.[0]).toMatchObject({
    params: { slug: 'test', broadcastId: '42' },
  })
})

test('editing a draft; a cleared sender name is sent as null and the audience is cleared too', async () => {
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
        subject: 'Hello',
        fromName: null,
        fromEmail: 'ada@example.com',
        segmentId: null,
        body: '<p></p>',
      },
    ])
  await expect.element(screen.getByText('Broadcast updated')).toBeInTheDocument()
  await expect.poll(() => fetches.list).toBe(2)
  await expect.poll(() => fetches.detail).toBeGreaterThan(1)
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
    handleSiteBroadcastsGet(() =>
      problem(404, { title: 'Not Found', detail: 'broadcast not found' }),
    ),
    handleSiteSegmentsList(empty),
    handleSiteTemplatesList(empty),
  )

  const { screen } = await renderWithRouter(<BroadcastEditPage />, EDIT_ROUTE)

  await expect.element(screen.getByText('Failed to load broadcasts')).toBeVisible()
  await expect.element(screen.getByText('broadcast not found')).toBeVisible()
})
