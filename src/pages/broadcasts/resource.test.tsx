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
import { Route as broadcastsEditRoute } from '../../routes/workspaces/$workspaceId/broadcasts/$broadcastId/edit.tsx'
import { Route as broadcastsCreateRoute } from '../../routes/workspaces/$workspaceId/broadcasts/new.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
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
    params: { workspaceId: 'TestWs01', broadcastId: '42' },
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

const DOC = {
  type: 'doc',
  content: [{ type: 'paragraph', content: [{ type: 'text', text: 'Hello there' }] }],
}

test('saving a broadcast written in the editor sends its document and no body', async () => {
  const { bodies } = serve('draft')
  worker.use(
    handleSiteBroadcastsGet(() =>
      HttpResponse.json({ ...broadcast('draft'), body: '<p>Hello there</p>', doc: DOC }),
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
        subject: 'Hello',
        fromName: 'Ada',
        fromEmail: 'ada@example.com',
        segmentId: null,
        doc: DOC,
      },
    ])
})

test('a broadcast with no document opens as editable source', async () => {
  serve('draft')
  worker.use(
    handleSiteBroadcastsGet(() =>
      HttpResponse.json({ ...broadcast('draft'), body: '<table><tr><td>Hi</td></tr></table>' }),
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
    handleSiteBroadcastsGet(() => HttpResponse.json({ ...broadcast('draft'), doc: DOC })),
    handleSiteBroadcastsUpdate(() => problem(422, { detail: 'heading level 9 is not allowed' })),
  )

  const { screen } = await renderWithRouter(<BroadcastEditPage />, EDIT_ROUTE)
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.element(screen.getByText('heading level 9 is not allowed')).toBeInTheDocument()
})
