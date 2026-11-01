import { HttpResponse } from 'msw'
import { beforeEach, expect, test } from 'vitest'

import {
  handleSiteAnalyticsRevenue,
  handleSiteAutomationsList,
  handleSiteBroadcastsList,
  handleSiteSendingDomainsList,
  handleSiteSetupChecklistGet,
} from '../../generated/site/msw.gen.ts'
import { SiteAnalyticsRange } from '../../generated/site/types.gen.ts'
import type { SiteAnalyticsRevenue } from '../../generated/site/types.gen.ts'
import { Route as overviewRoute } from '../../routes/workspaces/$workspaceId/index.tsx'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { OverviewPage } from './overview.tsx'

const finished = {
  steps: {
    integration: 'done',
    sendingDomain: 'done',
    contact: 'done',
    sentEmail: 'done',
  },
  complete: true,
} as const

const revenue: SiteAnalyticsRevenue = {
  reportingCurrency: 'USD',
  firstOrderAt: '2026-10-01T10:00:00Z',
  total: 0,
  credited: 0,
  orders: 0,
  creditedOrders: 0,
  otherCurrencyOrders: [],
  series: [],
}

// The test router does not run the route's search validation, so the period is spelled out.
const base = routeMount(overviewRoute, { workspaceId: 'TestWs01' })
const mount = { ...base, initialPath: `${base.initialPath}?range=30d` }

beforeEach(() => {
  worker.use(
    handleSiteBroadcastsList(() => HttpResponse.json({ items: [], totalItems: 0 })),
    handleSiteAutomationsList(() => HttpResponse.json({ items: [], totalItems: 0 })),
    handleSiteSetupChecklistGet({ body: finished }),
    handleSiteAnalyticsRevenue({ body: revenue }),
  )
})

test('an unfinished checklist is shown with its steps', async () => {
  worker.use(
    handleSiteSendingDomainsList({
      body: { items: [], page: 1, pageSize: 20, totalItems: 0, totalPages: 1 },
    }),
    handleSiteSetupChecklistGet({
      body: {
        steps: {
          integration: 'done',
          sendingDomain: 'pending',
          contact: 'todo',
          sentEmail: 'todo',
        },
        complete: false,
      },
    }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('Set up Sphericon')).toBeInTheDocument()
  await expect.element(screen.getByText('Waiting for DNS')).toBeInTheDocument()
})

test('a finished checklist is not shown', async () => {
  worker.use(handleSiteSetupChecklistGet({ body: finished }))

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('Set up Sphericon')).not.toBeInTheDocument()
})

test('a finished checklist leaves the shortcuts and what was created last', async () => {
  worker.use(
    handleSiteSetupChecklistGet({ body: finished }),
    handleSiteBroadcastsList(() =>
      HttpResponse.json({
        items: [
          {
            id: '1',
            name: 'Spring launch',
            status: 'sent',
            createdAt: '2026-10-27T08:09:00Z',
            stats: { recipientsTotal: 100, sentCount: 100, openedCount: 38 },
          },
        ],
        totalItems: 1,
      }),
    ),
    handleSiteAutomationsList(() =>
      HttpResponse.json({
        items: [
          {
            id: '2',
            name: 'Welcome series',
            status: 'active',
            createdAt: '2026-10-28T08:09:00Z',
          },
        ],
        totalItems: 1,
      }),
    ),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('Create a new…')).toBeInTheDocument()
  await expect.element(screen.getByText('Spring launch')).toBeInTheDocument()
  await expect.element(screen.getByText('Welcome series')).toBeInTheDocument()
  await expect.element(screen.getByText('Automation', { exact: true })).toBeInTheDocument()
})

test('the contact step offers every way to add one and no single page', async () => {
  worker.use(
    handleSiteSetupChecklistGet({
      body: {
        steps: {
          integration: 'done',
          sendingDomain: 'done',
          contact: 'todo',
          sentEmail: 'todo',
        },
        complete: false,
      },
    }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('Add your first Contact')).toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Install the tracker' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/settings/tracking')
  await expect
    .element(screen.getByRole('link', { name: 'Use the API' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/settings/api-keys')
  await expect
    .element(screen.getByRole('link', { name: 'Add one by hand' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/contacts')
})

test('a done contact step offers no action', async () => {
  worker.use(
    handleSiteSetupChecklistGet({
      body: {
        steps: {
          integration: 'todo',
          sendingDomain: 'todo',
          contact: 'done',
          sentEmail: 'todo',
        },
        complete: false,
      },
    }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('Add your first Contact')).toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Install the tracker' }))
    .not.toBeInTheDocument()
})

test('revenue shows the total, the credited share and the orders in the Reporting currency', async () => {
  worker.use(
    handleSiteAnalyticsRevenue({
      body: {
        ...revenue,
        total: 123456,
        credited: 30864,
        orders: 17,
        creditedOrders: 5,
      },
    }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('$1,234.56')).toBeInTheDocument()
  await expect.element(screen.getByText('25%')).toBeInTheDocument()
  await expect.element(screen.getByText('17', { exact: true })).toBeInTheDocument()
  await expect
    .element(screen.getByText(/orders sent to Sphericon, not of your shop/))
    .toBeInTheDocument()
})

test('a currency without minor units is formatted by its own exponent', async () => {
  worker.use(
    handleSiteAnalyticsRevenue({
      body: { ...revenue, reportingCurrency: 'JPY', total: 5000, credited: 0, orders: 2 },
    }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('¥5,000')).toBeInTheDocument()
})

test('the period of the URL is the one sent to the server', async () => {
  const ranges: (string | null)[] = []
  worker.use(
    handleSiteAnalyticsRevenue(({ request }) => {
      ranges.push(new URL(request.url).searchParams.get('range'))
      return HttpResponse.json(revenue)
    }),
  )

  await renderWithRouter(<OverviewPage />, { ...base, initialPath: `${base.initialPath}?range=7d` })

  await expect.poll(() => ranges).toEqual([SiteAnalyticsRange['7D']])
})

test('picking a period puts it in the URL', async () => {
  const { screen, navigate } = await renderWithRouter(<OverviewPage />, mount)

  await screen.getByText('Last 90 days').click()

  await expect
    .poll(() => navigate.mock.lastCall?.[0])
    .toMatchObject({ search: { range: SiteAnalyticsRange['90D'] } })
})

test('a Workspace that never sent an Order has no revenue block, whatever its currency', async () => {
  worker.use(handleSiteAnalyticsRevenue({ body: { ...revenue, firstOrderAt: null } }))

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('Create a new…')).toBeInTheDocument()
  await expect.element(screen.getByText('Revenue', { exact: true })).not.toBeInTheDocument()
  await expect.element(screen.getByText('Total revenue')).not.toBeInTheDocument()
})

test('orders in another currency are named with the way to change the currency', async () => {
  worker.use(
    handleSiteAnalyticsRevenue({
      body: { ...revenue, otherCurrencyOrders: [{ currency: 'EUR', orders: 3 }] },
    }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect
    .element(screen.getByText(/3 orders in EUR are not counted: revenue is summed in USD/))
    .toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Change the reporting currency' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/settings')
})

test('revenue is shown beside an unfinished checklist too', async () => {
  worker.use(
    handleSiteSetupChecklistGet({
      body: {
        steps: {
          integration: 'done',
          sendingDomain: 'done',
          contact: 'todo',
          sentEmail: 'todo',
        },
        complete: false,
      },
    }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('Set up Sphericon')).toBeInTheDocument()
  await expect.element(screen.getByText('Revenue', { exact: true })).toBeInTheDocument()
})
