import { HttpResponse } from 'msw'
import { beforeEach, expect, test } from 'vitest'

import {
  handleSiteBroadcastsSchedule,
  handleSiteBroadcastsTestSend,
  handleSiteWorkspacesList,
  handleSiteSegmentsGet,
  handleSiteSegmentsPreview,
  handleSiteBroadcastsSend,
} from '../../generated/site/msw.gen.ts'
import type {
  SiteBroadcastResource,
  SiteWorkspaceResource,
} from '../../generated/site/types.gen.ts'
import { Route as broadcastsEditRoute } from '../../routes/workspaces/$workspaceId/broadcasts/$broadcastId/edit.tsx'
import { Route as broadcastsReportRoute } from '../../routes/workspaces/$workspaceId/broadcasts/$broadcastId/report.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { DeliveryBlock } from './DeliveryBlock.tsx'

// The Workspace the page belongs to, with no Send cap unless a test gives it one.
const WORKSPACE: SiteWorkspaceResource = {
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
  sendCap: null,
  role: 'owner',
  createdAt: '2026-01-01T00:00:00Z',
}

beforeEach(() => {
  worker.use(handleSiteWorkspacesList({ body: [WORKSPACE] }))
})

const MOUNT = routeMount(broadcastsEditRoute, { workspaceId: 'TestWs01', broadcastId: '7' })
const SEND_FAILED = 'Failed to send broadcast'

const draft: SiteBroadcastResource = {
  id: '7',
  name: 'Launch',
  subject: 'Hello',
  body: '<p></p>',
  status: 'draft',
  stats: {
    recipientsTotal: 0,
    sentCount: 0,
    openedCount: 0,
    clickedCount: 0,
    unsubscribedCount: 0,
    failedCount: 0,
    deliveredCount: 0,
    bouncedCount: 0,
    complainedCount: 0,
    capturedCount: 0,
    deliveryRate: 0,
    failureRate: 0,
    openRate: 0,
    clickRate: 0,
    clickToOpenRate: 0,
    unsubscribeRate: 0,
    revenue: { reportingCurrency: 'USD', orders: 0, amount: 0 },
  },
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

test('scheduling a draft posts the chosen time as an ISO timestamp and opens the report', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteBroadcastsSchedule(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(draft)
    }),
  )
  const { screen, navigate } = await renderWithRouter(<DeliveryBlock broadcast={draft} />, MOUNT)

  await screen.getByLabelText('Schedule for').fill('2030-05-06T07:08')
  await screen.getByRole('button', { name: 'Schedule', exact: true }).click()

  await expect.element(screen.getByText('Broadcast scheduled')).toBeInTheDocument()
  expect(bodies).toEqual([{ scheduledAt: new Date('2030-05-06T07:08').toISOString() }])
  await expect
    .poll(() => navigate.mock.calls)
    .toContainEqual([
      { to: broadcastsReportRoute.to, params: { workspaceId: 'TestWs01', broadcastId: '7' } },
    ])
})

test('a test send posts the typed address and confirms without navigating', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteBroadcastsTestSend(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ from: 'noreply@sphericon.test', captured: false })
    }),
  )
  const { screen, navigate } = await renderWithRouter(<DeliveryBlock broadcast={draft} />, MOUNT)

  await screen.getByLabelText('Send a test to').fill('qa@example.com')
  await screen.getByRole('button', { name: 'Send test' }).click()

  await expect.element(screen.getByText('Test email sent')).toBeInTheDocument()
  await expect.element(screen.getByText(/Sent from noreply@sphericon\.test/)).toBeInTheDocument()
  expect(bodies).toEqual([{ email: 'qa@example.com' }])
  expect(navigate).not.toHaveBeenCalled()
})

test('a test send captured by a Test inbox says it was not sent to the address named', async () => {
  worker.use(
    handleSiteBroadcastsTestSend(() =>
      HttpResponse.json({ from: 'hello@acme.test', captured: true }),
    ),
  )
  const { screen } = await renderWithRouter(<DeliveryBlock broadcast={draft} />, MOUNT)

  await screen.getByLabelText('Send a test to').fill('qa@example.com')
  await screen.getByRole('button', { name: 'Send test' }).click()

  await expect
    .element(screen.getByText('Captured in your Test inbox, not sent to the address you named.'))
    .toBeInTheDocument()
  await expect.element(screen.getByText(/Sent from/)).not.toBeInTheDocument()
})

test('a rejected test send shows the error toast', async () => {
  worker.use(
    handleSiteBroadcastsTestSend(() => problem(422, { title: 'Bad', detail: 'No integration' })),
  )
  const { screen } = await renderWithRouter(<DeliveryBlock broadcast={draft} />, MOUNT)

  await screen.getByLabelText('Send a test to').fill('qa@example.com')
  await screen.getByRole('button', { name: 'Send test' }).click()

  await expect.element(screen.getByText(SEND_FAILED)).toBeInTheDocument()
})

const capped = (perDay: number) => ({ ...WORKSPACE, sendCap: { perDay, sentLast24h: 0 } })

test('warns before sending an audience larger than the Send cap, in days, and still sends', async () => {
  const previewed: unknown[] = []
  const condition = { kind: 'group' as const, combinator: 'and' as const, rules: [] }
  worker.use(
    handleSiteWorkspacesList({ body: [capped(1000)] }),
    handleSiteSegmentsGet({
      body: {
        id: '3',
        name: 'Active',
        condition,
        createdAt: '2026-01-01T00:00:00Z',
        updatedAt: '2026-01-01T00:00:00Z',
      },
    }),
    handleSiteSegmentsPreview(async ({ request }) => {
      previewed.push(await request.json())
      return HttpResponse.json({ count: 2500 })
    }),
    handleSiteBroadcastsSend({ body: { ...draft, status: 'sending' } }),
  )
  const { screen } = await renderWithRouter(
    <DeliveryBlock broadcast={{ ...draft, segmentId: '3' }} />,
    MOUNT,
  )

  await expect
    .element(
      screen.getByText(/audience of 2,500 is larger than the Send cap of 1,000 per 24 hours/),
    )
    .toBeInTheDocument()
  await expect.element(screen.getByText(/over about 3 days/)).toBeInTheDocument()
  expect(previewed).toEqual([{ condition }])
  await expect.element(screen.getByRole('button', { name: 'Send now' })).toBeEnabled()
})

test('previews the whole list for a broadcast without a segment', async () => {
  const previewed: unknown[] = []
  worker.use(
    handleSiteWorkspacesList({ body: [capped(100)] }),
    handleSiteSegmentsPreview(async ({ request }) => {
      previewed.push(await request.json())
      return HttpResponse.json({ count: 150 })
    }),
  )
  const { screen } = await renderWithRouter(<DeliveryBlock broadcast={draft} />, MOUNT)

  await expect.element(screen.getByText(/over about 2 days/)).toBeInTheDocument()
  expect(previewed).toEqual([{}])
})

test('says nothing when the audience fits the Send cap', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [capped(1000)] }),
    handleSiteSegmentsPreview({ body: { count: 1000 } }),
  )
  const { screen } = await renderWithRouter(<DeliveryBlock broadcast={draft} />, MOUNT)

  await expect.element(screen.getByRole('button', { name: 'Send now' })).toBeInTheDocument()
  await expect.element(screen.getByText(/larger than the Send cap/)).not.toBeInTheDocument()
})

test('a Workspace without a Send cap previews nothing', async () => {
  const previewed: unknown[] = []
  worker.use(
    handleSiteSegmentsPreview(async ({ request }) => {
      previewed.push(await request.json())
      return HttpResponse.json({ count: 10 })
    }),
  )
  const { screen } = await renderWithRouter(<DeliveryBlock broadcast={draft} />, MOUNT)

  await expect.element(screen.getByRole('button', { name: 'Send now' })).toBeInTheDocument()
  expect(previewed).toEqual([])
})
