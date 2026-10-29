import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteBroadcastLinksList,
  handleSiteBroadcastsGet,
} from '../../generated/site/msw.gen.ts'
import { Route as broadcastsReportRoute } from '../../routes/workspaces/$workspaceId/broadcasts/$broadcastId/report.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { BroadcastReportPage } from './report.tsx'

const REPORT_ROUTE = routeMount(broadcastsReportRoute, {
  workspaceId: 'TestWs01',
  broadcastId: '5',
})

const STATS = {
  recipientsTotal: 200,
  sentCount: 190,
  openedCount: 80,
  clickedCount: 20,
  unsubscribedCount: 3,
  failedCount: 10,
  deliveredCount: 171,
  bouncedCount: 13,
  complainedCount: 6,
  capturedCount: 9,
  deliveryRate: 0.95,
  openRate: 0.5,
  clickRate: 0.125,
  clickToOpenRate: 0.25,
  unsubscribeRate: 0.015,
  failureRate: 0.05,
  revenue: { reportingCurrency: 'USD', orders: 11, amount: 4550 },
}

const BROADCAST = {
  id: '5',
  name: 'Big news',
  subject: 'Read this',
  status: 'sent',
  stats: STATS,
}

test('renders the broadcast header, counters and rates', async () => {
  worker.use(
    handleSiteBroadcastsGet(() => HttpResponse.json(BROADCAST)),
    handleSiteBroadcastLinksList(() => HttpResponse.json({ items: [] })),
  )

  const { screen } = await renderWithRouter(<BroadcastReportPage />, REPORT_ROUTE)

  await expect.element(screen.getByText('Big news')).toBeInTheDocument()
  await expect.element(screen.getByText('Read this')).toBeInTheDocument()
  await expect.element(screen.getByText('Sent', { exact: true }).first()).toBeInTheDocument()
  await expect.element(screen.getByText('200')).toBeInTheDocument()
  await expect.element(screen.getByText('Delivered', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('171')).toBeInTheDocument()
  await expect.element(screen.getByText('Bounced', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('13')).toBeInTheDocument()
  await expect.element(screen.getByText('Complained', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('6', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Captured, not delivered')).toBeInTheDocument()
  await expect.element(screen.getByText('9', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('95%')).toBeInTheDocument()
  await expect.element(screen.getByText('12.5%')).toBeInTheDocument()
  await expect.element(screen.getByText('Sending is on hold')).not.toBeInTheDocument()
})

test('lists the clicked links with their counts', async () => {
  worker.use(
    handleSiteBroadcastsGet(() => HttpResponse.json(BROADCAST)),
    handleSiteBroadcastLinksList(() =>
      HttpResponse.json({
        items: [
          { url: 'https://dest.test/a', clicks: 7 },
          { url: 'https://dest.test/b', clicks: 2 },
        ],
      }),
    ),
  )

  const { screen } = await renderWithRouter(<BroadcastReportPage />, REPORT_ROUTE)

  await expect.element(screen.getByText('Clicked links')).toBeInTheDocument()
  await expect.element(screen.getByText('https://dest.test/a')).toBeInTheDocument()
  await expect.element(screen.getByText('7', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('https://dest.test/b')).toBeInTheDocument()
})

test('shows the hold alert when the broadcast is on hold', async () => {
  worker.use(
    handleSiteBroadcastsGet(() =>
      HttpResponse.json({ ...BROADCAST, status: 'sending', holdReason: 'no_integration' }),
    ),
    handleSiteBroadcastLinksList(() => HttpResponse.json({ items: [] })),
  )

  const { screen } = await renderWithRouter(<BroadcastReportPage />, REPORT_ROUTE)

  await expect.element(screen.getByText('Sending is on hold')).toBeInTheDocument()
  await expect.element(screen.getByText(/No email provider is configured/)).toBeInTheDocument()
})

test('shows an error alert when the broadcast fails to load', async () => {
  worker.use(handleSiteBroadcastsGet(() => problem(500)))

  const { screen } = await renderWithRouter(<BroadcastReportPage />, REPORT_ROUTE)

  await expect.element(screen.getByText('Failed to load broadcasts')).toBeInTheDocument()
})

test('shows how far a sending broadcast has got', async () => {
  worker.use(
    handleSiteBroadcastsGet(() =>
      HttpResponse.json({
        ...BROADCAST,
        status: 'sending',
        progress: { processedCount: 120, remainingCount: 80 },
      }),
    ),
    handleSiteBroadcastLinksList(() => HttpResponse.json({ items: [] })),
  )

  const { screen } = await renderWithRouter(<BroadcastReportPage />, REPORT_ROUTE)

  await expect.element(screen.getByText('Sent 120 of 200')).toBeInTheDocument()
})

test('shows the orders and the net revenue credited to the broadcast', async () => {
  worker.use(
    handleSiteBroadcastsGet(() => HttpResponse.json(BROADCAST)),
    handleSiteBroadcastLinksList(() => HttpResponse.json({ items: [] })),
  )

  const { screen } = await renderWithRouter(<BroadcastReportPage />, REPORT_ROUTE)

  await expect.element(screen.getByText('Orders', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('11', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Revenue', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('$45.50')).toBeInTheDocument()
})

test('shows no orders or revenue until the reporting currency is chosen', async () => {
  worker.use(
    handleSiteBroadcastsGet(() =>
      HttpResponse.json({
        ...BROADCAST,
        stats: { ...STATS, revenue: { reportingCurrency: null, orders: 0, amount: 0 } },
      }),
    ),
    handleSiteBroadcastLinksList(() => HttpResponse.json({ items: [] })),
  )

  const { screen } = await renderWithRouter(<BroadcastReportPage />, REPORT_ROUTE)

  await expect.element(screen.getByText('Big news')).toBeInTheDocument()
  await expect.element(screen.getByText('Orders', { exact: true })).not.toBeInTheDocument()
  await expect.element(screen.getByText('Revenue', { exact: true })).not.toBeInTheDocument()
})
