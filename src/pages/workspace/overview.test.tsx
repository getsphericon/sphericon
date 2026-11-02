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
  SiteAnalyticsPeriod,
  SiteAnalyticsRevenue,
  SiteAnalyticsRevenuePeriod,
  SiteAnalyticsSending,
  SiteBroadcastResource,
} from '../../generated/site/types.gen.ts'
import { Route as overviewRoute } from '../../routes/workspaces/$workspaceId/index.tsx'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { formatDateTime } from '../../utils/datetime.ts'
import { OverviewPage } from './overview.tsx'

const finished = {
  steps: {
    integration: 'done',
    sendingDomain: 'done',
    contact: 'done',
    sentEmail: 'done',
  },
  completedAt: '2026-10-01T10:00:00Z',
} as const

const revenuePeriod: SiteAnalyticsRevenuePeriod = {
  from: '2026-10-02',
  to: '2026-10-31',
  total: 0,
  credited: 0,
  orders: 0,
  creditedOrders: 0,
  otherCurrencyOrders: [],
}

const revenue: SiteAnalyticsRevenue = {
  reportingCurrency: 'USD',
  firstOrderAt: '2026-10-01T10:00:00Z',
  current: revenuePeriod,
  previous: { ...revenuePeriod, from: '2026-09-02', to: '2026-10-01' },
  series: [],
}

// withRevenue is the revenue with the given figures of the current and the previous period.
const withRevenue = (
  current: Partial<SiteAnalyticsRevenuePeriod>,
  previous: Partial<SiteAnalyticsRevenuePeriod> = {},
): SiteAnalyticsRevenue => ({
  ...revenue,
  current: { ...revenue.current, ...current },
  previous: { ...revenue.previous, ...previous },
})

// The test router does not run the route's search validation, so the period is spelled out.
const base = routeMount(overviewRoute, { workspaceId: 'TestWs01' })
const noSends: SiteAnalyticsSending = {
  broadcast: 0,
  automation: 0,
  transactional: 0,
  delivered: 0,
  bounced: 0,
  complained: 0,
}
const period: SiteAnalyticsPeriod = {
  from: '2026-10-02',
  to: '2026-10-31',
  sending: noSends,
  email: {
    sentCount: 0,
    openedCount: 0,
    clickedCount: 0,
    openRate: 0,
    clickRate: 0,
    clickToOpenRate: 0,
  },
  newContacts: 0,
}
const overview: SiteAnalyticsOverview = {
  contacts: { total: 0, active: 0, unsubscribed: 0 },
  automations: { total: 0, active: 0, runsActive: 0, runsCompleted: 0 },
  current: period,
  previous: { ...period, from: '2026-09-02', to: '2026-10-01' },
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

// about is an item's object, for an item that stands for one failure.
const about = (id: string, name: string) => ({ subject: { id, name }, count: 1 })

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

test('every kind of item is a sentence, with a link where there is somewhere to go', async () => {
  worker.use(
    handleSiteAttentionGet({
      body: {
        items: [
          { kind: 'integrationProviderStopped', severity: 'blocked', ...about('1', 'Amazon SES') },
          { kind: 'broadcastHeld', severity: 'blocked', ...about('2', 'Unexplained') },
          {
            kind: 'deliverabilityWarning',
            severity: 'degraded',
            count: 1,
            abuse: { domain: 'mail.acme.test', metric: 'complaint', rate: 0.0015 },
          },
          { kind: 'providerLimitSpent', severity: 'degraded', ...about('3', 'Postmark') },
          { kind: 'integrationProviderProbation', severity: 'degraded', ...about('4', 'Probed') },
          { kind: 'integrationProviderSandbox', severity: 'degraded', ...about('5', 'Sandboxed') },
          { kind: 'broadcastFailed', severity: 'notice', ...about('6', 'Autumn sale') },
          { kind: 'webhookDisabled', severity: 'notice', ...about('7', 'https://hooks.test/x') },
          { kind: 'sendingDomainNotBulkReady', severity: 'notice', ...about('8', 'bare.acme.com') },
        ],
      },
    }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect
    .element(screen.getByText(/stopped the account behind Amazon SES/))
    .toBeInTheDocument()
  await expect.element(screen.getByText(/Unexplained is on hold/)).toBeInTheDocument()
  await expect
    .element(screen.getByText(/complaint rate on mail.acme.test is 0.15%/))
    .toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Review suppressions' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/suppressions')
  await expect.element(screen.getByText(/Postmark has sent its daily limit/)).toBeInTheDocument()
  await expect.element(screen.getByText(/behind Probed under review/)).toBeInTheDocument()
  await expect.element(screen.getByText(/Sandboxed is in the provider/)).toBeInTheDocument()
  await expect.element(screen.getByText('Autumn sale failed to send.')).toBeInTheDocument()
  await expect
    .element(
      screen.getByText(
        'The webhook https://hooks.test/x disabled itself after failing deliveries.',
      ),
    )
    .toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Open webhooks' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/settings/webhooks')
  await expect
    .element(screen.getByText(/bare.acme.com has no published DMARC record/))
    .toBeInTheDocument()
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

test('a broadcast the sending limit will slow says when it is expected to finish', async () => {
  const finish = '2026-11-02T21:00:00Z'
  worker.use(
    handleSiteAttentionGet({
      body: {
        items: [
          {
            kind: 'scheduledBroadcastOverSendCap',
            severity: 'degraded',
            subject: { id: '14', name: 'Black Friday' },
            count: 1500,
            estimatedCompletionAt: finish,
          },
        ],
      },
    }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect
    .element(screen.getByText(/will slow Black Friday \(about 1,500 contacts\)/))
    .toBeInTheDocument()
  await expect
    .element(screen.getByText(new RegExp(`expected to finish by ${formatDateTime(finish)}`)))
    .toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Open the broadcast' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/broadcasts/14/report')
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
                broadcast({ id: '3', name: 'Flash sale', status: 'sending' }),
                broadcast({
                  id: '2',
                  name: 'Autumn sale',
                  status: 'scheduled',
                  scheduledAt: '2026-11-03T09:00:00Z',
                }),
              ],
        totalItems: 2,
      })
    }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('Autumn sale')).toBeInTheDocument()
  await expect.element(screen.getByText(/^Scheduled for/)).toBeInTheDocument()
  await expect.element(screen.getByText('Sending: 100 of 100')).toBeInTheDocument()
  await expect.element(screen.getByText('Spring launch')).toBeInTheDocument()
  await expect.element(screen.getByText(/38% opened/)).toBeInTheDocument()
  expect(asked.toSorted()).toEqual(['sending+scheduled by scheduledSoonest', 'sent by sentLatest'])
})

test('a new Workspace sees what to do and no wall of zeros', async () => {
  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText(/Nothing has gone out yet/)).toBeInTheDocument()
  await expect.element(screen.getByText('Nothing is sending or scheduled.')).not.toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'New broadcast' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/broadcasts/new')
  await expect
    .element(screen.getByRole('link', { name: 'New automation' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/automations')
  await expect.element(screen.getByText('Last 30 days')).not.toBeInTheDocument()
  await expect.element(screen.getByText('Sending', { exact: true })).not.toBeInTheDocument()
  await expect.element(screen.getByText('Audience', { exact: true })).not.toBeInTheDocument()
})

test('the period shows what was sent through every surface and the audience', async () => {
  worker.use(
    handleSiteAnalyticsOverview({
      body: {
        ...overview,
        contacts: { total: 120, active: 110, unsubscribed: 10 },
        current: {
          ...period,
          sending: {
            broadcast: 100,
            automation: 30,
            transactional: 5,
            delivered: 130,
            bounced: 4,
            complained: 1,
          },
          newContacts: 7,
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
      },
    }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('Add your first Contact')).toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Install the tracker' }))
    .not.toBeInTheDocument()
})

test('each period figure shows the previous period and the change since', async () => {
  worker.use(
    handleSiteAnalyticsOverview({
      body: {
        ...overview,
        contacts: { total: 120, active: 110, unsubscribed: 10 },
        current: {
          ...period,
          sending: { ...noSends, broadcast: 10, delivered: 9, bounced: 3, complained: 1 },
          email: { ...period.email, sentCount: 10, openRate: 0.5, clickRate: 0.2 },
          newContacts: 6,
        },
        previous: {
          ...overview.previous,
          sending: { ...noSends, broadcast: 5, delivered: 4, bounced: 2, complained: 2 },
          email: { ...period.email, sentCount: 8, openRate: 0.25, clickRate: 0.1 },
          newContacts: 4,
        },
      },
    }),
    handleSiteAnalyticsRevenue({ body: { ...revenue, firstOrderAt: null } }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('Previous 30 days: 5 (+100%)')).toBeInTheDocument()
  await expect.element(screen.getByText('Previous 30 days: 4 (+125%)')).toBeInTheDocument()
  await expect.element(screen.getByText('Previous 30 days: 2 (+50%)')).toBeInTheDocument()
  await expect.element(screen.getByText('Previous 30 days: 2 (-50%)')).toBeInTheDocument()
  await expect.element(screen.getByText('Previous 30 days: 4 (+50%)')).toBeInTheDocument()
  await expect.element(screen.getByText('Previous 30 days: 25% (+100%)')).toBeInTheDocument()
  await expect.element(screen.getByText('Previous 30 days: 10% (+100%)')).toBeInTheDocument()
  // Eight cards, seven of them period figures: the snapshot of the subscribed has no previous period.
  expect(screen.getByText(/^Previous 30 days/).elements()).toHaveLength(7)
})

test('a figure the previous period had nothing of shows no change', async () => {
  worker.use(
    handleSiteAnalyticsOverview({
      body: {
        ...overview,
        contacts: { total: 120, active: 110, unsubscribed: 10 },
        current: {
          ...period,
          sending: { ...noSends, broadcast: 10 },
          email: { ...period.email, sentCount: 10, openRate: 0.5 },
        },
      },
    }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('10', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Previous 30 days: 0%').first()).toBeInTheDocument()
  await expect.element(screen.getByText(/Previous 30 days: .*\(/)).not.toBeInTheDocument()
})

test('revenue compares the total, the credited share and the orders with the previous period', async () => {
  worker.use(
    handleSiteAnalyticsRevenue({
      body: withRevenue(
        { total: 123456, credited: 30864, orders: 17, creditedOrders: 5 },
        { total: 61728, credited: 30864, orders: 0, creditedOrders: 0 },
      ),
    }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('Previous 30 days: $617.28 (+100%)')).toBeInTheDocument()
  await expect.element(screen.getByText('Previous 30 days: 50% (-50%)')).toBeInTheDocument()
  await expect.element(screen.getByText('Previous 30 days: 0', { exact: true })).toBeInTheDocument()
})

test('revenue shows the total, the credited share and the orders in the Reporting currency', async () => {
  worker.use(
    handleSiteAnalyticsRevenue({
      body: withRevenue({ total: 123456, credited: 30864, orders: 17, creditedOrders: 5 }),
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
      body: { ...withRevenue({ total: 5000, orders: 2 }), reportingCurrency: 'JPY' },
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
  worker.use(
    handleSiteAnalyticsOverview({
      body: { ...overview, contacts: { total: 3, active: 3, unsubscribed: 0 } },
    }),
  )
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
      body: withRevenue({ otherCurrencyOrders: [{ currency: 'EUR', orders: 3 }] }),
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
      },
    }),
  )

  const { screen } = await renderWithRouter(<OverviewPage />, mount)

  await expect.element(screen.getByText('Set up Sphericon')).toBeInTheDocument()
  await expect.element(screen.getByText('Revenue', { exact: true })).toBeInTheDocument()
})
