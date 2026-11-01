import { HttpResponse } from 'msw'
import { beforeEach, expect, test } from 'vitest'

import {
  handleSiteAnalyticsOverview,
  handleSiteAnalyticsRevenue,
  handleSiteAttentionGet,
  handleSiteBroadcastsList,
  handleSiteSendingDomainsList,
  handleSiteSetupChecklistGet,
} from '../../generated/site/msw.gen.ts'
import { SiteAnalyticsRange } from '../../generated/site/types.gen.ts'
import type {
  SiteAnalyticsOverview,
  SiteAnalyticsRevenue,
  SiteBroadcastResource,
} from '../../generated/site/types.gen.ts'
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
const overview: SiteAnalyticsOverview = {
  contacts: { total: 0, active: 0, unsubscribed: 0, newInRange: 0 },
  sending: {
    broadcast: 0,
    automation: 0,
    transactional: 0,
    delivered: 0,
    bounced: 0,
    complained: 0,
  },
  email: {
    sentCount: 0,
    openedCount: 0,
    clickedCount: 0,
    openRate: 0,
    clickRate: 0,
    clickToOpenRate: 0,
  },
  automations: { total: 0, active: 0, runsActive: 0, runsCompleted: 0 },
  timeseries: [],
}

const broadcast = (b: Pick<SiteBroadcastResource, 'status'> & Partial<SiteBroadcastResource>) => ({
  id: '1',
  name: 'Spring launch',
  subject: 'Spring is here',
  body: '',
  createdAt: '2026-10-27T08:09:00Z',
  stats: {
    recipientsTotal: 100,
    sentCount: 100,
    openedCount: 38,
    clickedCount: 0,
    unsubscribedCount: 0,
    failedCount: 0,
    deliveredCount: 100,
    bouncedCount: 0,
    complainedCount: 0,
    capturedCount: 0,
    deliveryRate: 1,
    openRate: 0.38,
    clickRate: 0,
    clickToOpenRate: 0,
    unsubscribeRate: 0,
    failureRate: 0,
    revenue: { reportingCurrency: null, orders: 0, amount: 0 },
  },
  ...b,
})

const mount = { ...base, initialPath: `${base.initialPath}?range=30d` }

beforeEach(() => {
  worker.use(
    handleSiteBroadcastsList(() => HttpResponse.json({ items: [], totalItems: 0 })),
    handleSiteAnalyticsOverview({ body: overview }),
    handleSiteSetupChecklistGet({ body: finished }),
    handleSiteAnalyticsRevenue({ body: revenue }),
    handleSiteAttentionGet({ body: { items: [] } }),
  )
})

test('nothing that needs attention shows no block', async () => {
  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('Sends', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Needs attention')).not.toBeInTheDocument()
})

test('each item says what is wrong and links to where it is fixed', async () => {
  worker.use(
    handleSiteAttentionGet({
      body: {
        items: [
          {
            kind: 'sendingDomainVerificationLost',
            severity: 'blocked',
            subject: { id: '6', name: 'lost.acme.com' },
            count: 1,
          },
          {
            kind: 'enrollmentsFailed',
            severity: 'notice',
            subject: { id: '100', name: 'Welcome series' },
            count: 2,
          },
          { kind: 'sendCapSpent', severity: 'degraded', count: 1 },
        ],
      },
    }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('Needs attention')).toBeInTheDocument()
  await expect
    .element(screen.getByText(/The DKIM record of lost.acme.com is gone/))
    .toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Open email settings' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/settings/emails')
  await expect
    .element(screen.getByText('2 runs of Welcome series failed this week.'))
    .toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Open the automation' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/automations/100')
  await expect.element(screen.getByText(/sent its daily sending limit/)).toBeInTheDocument()
})

test('a held broadcast says what lifts the hold', async () => {
  worker.use(
    handleSiteAttentionGet({
      body: {
        items: [
          {
            kind: 'broadcastHeld',
            severity: 'blocked',
            subject: { id: '12', name: 'Spring launch' },
            count: 1,
            holdReason: 'unverified_domain',
          },
        ],
      },
    }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect
    .element(screen.getByText(/Spring launch is on hold. The sender domain is not verified/))
    .toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Open the broadcast' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/broadcasts/12/report')
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

test('what is going out comes soonest first and what went out last with its open rate', async () => {
  const asked: string[] = []
  worker.use(
    handleSiteBroadcastsList(({ request }) => {
      const query = new URL(request.url).searchParams
      asked.push(`${query.getAll('status').join('+')} by ${query.get('order')}`)
      return HttpResponse.json({
        items:
          query.get('order') === 'sentLatest'
            ? [broadcast({ status: 'sent', sentAt: '2026-10-27T08:09:00Z' })]
            : [
                broadcast({
                  id: '2',
                  name: 'Autumn sale',
                  status: 'scheduled',
                  scheduledAt: '2026-11-03T09:00:00Z',
                }),
              ],
        totalItems: 1,
      })
    }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('Autumn sale')).toBeInTheDocument()
  await expect.element(screen.getByText(/^Scheduled for/)).toBeInTheDocument()
  await expect.element(screen.getByText('Spring launch')).toBeInTheDocument()
  await expect.element(screen.getByText(/38% opened/)).toBeInTheDocument()
  expect(asked.toSorted()).toEqual(['sending+scheduled by scheduledSoonest', 'sent by sentLatest'])
})

test('a new Workspace sees what to do and no wall of zeros', async () => {
  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('Nothing is sending or scheduled.')).toBeInTheDocument()
  await expect.element(screen.getByText('Nothing has been sent yet.')).toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'New broadcast' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/broadcasts/new')
  await expect.element(screen.getByText('Sending', { exact: true })).not.toBeInTheDocument()
  await expect.element(screen.getByText('Audience', { exact: true })).not.toBeInTheDocument()
})

test('the period shows what was sent through every surface and the audience', async () => {
  worker.use(
    handleSiteAnalyticsOverview({
      body: {
        ...overview,
        contacts: { total: 120, active: 110, unsubscribed: 10, newInRange: 7 },
        sending: {
          broadcast: 100,
          automation: 30,
          transactional: 5,
          delivered: 130,
          bounced: 4,
          complained: 1,
        },
      },
    }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('135', { exact: true })).toBeInTheDocument()
  await expect
    .element(screen.getByText('Broadcasts 100 · Automations 30 · Transactional 5'))
    .toBeInTheDocument()
  await expect.element(screen.getByText('110', { exact: true })).toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Email metrics' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/analytics/email-metrics?range=30d')
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

  await expect.element(screen.getByText('Sends', { exact: true })).toBeInTheDocument()
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
