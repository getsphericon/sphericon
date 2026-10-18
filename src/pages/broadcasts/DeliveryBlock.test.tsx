import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteBroadcastsSchedule,
  handleSiteBroadcastsTestSend,
} from '../../generated/site/msw.gen.ts'
import type { SiteBroadcastResource } from '../../generated/site/types.gen.ts'
import { Route as broadcastsEditRoute } from '../../routes/workspaces/$slug/broadcasts/$broadcastId/edit.tsx'
import { Route as broadcastsReportRoute } from '../../routes/workspaces/$slug/broadcasts/$broadcastId/report.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { DeliveryBlock } from './DeliveryBlock.tsx'

const MOUNT = routeMount(broadcastsEditRoute, { slug: 'test', broadcastId: '7' })
const SEND_FAILED = 'Failed to send broadcast'

const draft: SiteBroadcastResource = {
  id: '7',
  name: 'Launch',
  subject: 'Hello',
  body: '<mjml></mjml>',
  bodyText: 'Hello',
  status: 'draft',
  stats: {
    recipientsTotal: 0,
    sentCount: 0,
    openedCount: 0,
    clickedCount: 0,
    unsubscribedCount: 0,
    failedCount: 0,
    deliveryRate: 0,
    failureRate: 0,
    openRate: 0,
    clickRate: 0,
    clickToOpenRate: 0,
    unsubscribeRate: 0,
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
    .toContainEqual([{ to: broadcastsReportRoute.to, params: { slug: 'test', broadcastId: '7' } }])
})

test('a test send posts the typed address and confirms without navigating', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteBroadcastsTestSend(async ({ request }) => {
      bodies.push(await request.json())
      return new HttpResponse(null, { status: 204 })
    }),
  )
  const { screen, navigate } = await renderWithRouter(<DeliveryBlock broadcast={draft} />, MOUNT)

  await screen.getByLabelText('Send a test to').fill('qa@example.com')
  await screen.getByRole('button', { name: 'Send test' }).click()

  await expect.element(screen.getByText('Test email sent')).toBeInTheDocument()
  expect(bodies).toEqual([{ email: 'qa@example.com' }])
  expect(navigate).not.toHaveBeenCalled()
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
