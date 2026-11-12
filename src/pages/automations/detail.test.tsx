import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteAutomationReportsGet,
  handleSiteAutomationsGet,
  handleSiteAutomationsPublish,
  handleSiteAutomationsRevert,
  handleSiteEnrollmentsList,
  handleSiteIntegrationsGet,
} from '../../generated/site/msw.gen.ts'
import type {
  SiteAutomationResource,
  SiteAutomationResults,
} from '../../generated/site/types.gen.ts'
import { Route as automationsDetailRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/index.tsx'
import { emailIntegration, page, TIMESTAMPS, testId } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { AutomationDetailPage } from './detail.tsx'

const MOUNT = routeMount(automationsDetailRoute, {
  workspaceId: 'TestWs01',
  automationId: testId(1),
})

const NO_REVENUE = { reportingCurrency: 'USD', orders: 0, amount: 0 }

// A period in which nothing happened, nor in the one before.
const NO_RESULTS: SiteAutomationResults = {
  period: '30d',
  current: {
    entered: 0,
    completed: 0,
    goalReached: 0,
    exited: 0,
    failed: 0,
    from: '2026-02-16',
    to: '2026-03-17',
    averageDurationSeconds: null,
    revenue: NO_REVENUE,
  },
  previous: {
    entered: 0,
    completed: 0,
    goalReached: 0,
    exited: 0,
    failed: 0,
    from: '2026-01-17',
    to: '2026-02-15',
    averageDurationSeconds: null,
    revenue: NO_REVENUE,
  },
  series: [],
}

test('shows the automation and the contacts enrolled in it', async () => {
  worker.use(
    handleSiteAutomationsGet({
      body: {
        id: testId(1),
        name: 'Welcome series',
        status: 'active',
        live: {
          number: 1,
          triggerEvent: 'contact.created',
          exitEvents: [],
          goalEvent: null,
          steps: [],
        },
        draft: null,
        revision: 1,
        ...TIMESTAMPS,
      },
    }),
    handleSiteAutomationReportsGet({
      body: {
        opens: 0,
        clicks: 0,
        delivered: 0,
        bounced: 0,
        complained: 0,
        captured: 0,
        revenue: { reportingCurrency: 'USD', orders: 0, amount: 0 },
        links: [],
        steps: [],
        results: NO_RESULTS,
      },
    }),
    handleSiteEnrollmentsList({
      body: page(
        [
          {
            id: testId(5),
            automationId: testId(1),
            contactId: testId(42),
            status: 'completed',
            currentStep: 1,
            ...TIMESTAMPS,
          },
        ],
        1,
      ),
    }),
  )

  const { screen } = await renderWithRouter(<AutomationDetailPage />, MOUNT)

  await expect.element(screen.getByText('Welcome series')).toBeInTheDocument()
  await expect.element(screen.getByText(testId(42))).toBeInTheDocument()
  await expect.element(screen.getByRole('cell', { name: 'Completed' })).toBeInTheDocument()
})

test('shows the opens, the clicks and the clicked links of the automation', async () => {
  worker.use(
    handleSiteAutomationsGet({
      body: {
        id: testId(1),
        name: 'Welcome series',
        status: 'active',
        live: {
          number: 1,
          triggerEvent: 'contact.created',
          exitEvents: [],
          goalEvent: null,
          steps: [],
        },
        draft: null,
        revision: 1,
        ...TIMESTAMPS,
      },
    }),
    handleSiteAutomationReportsGet({
      body: {
        opens: 12,
        clicks: 9,
        delivered: 31,
        bounced: 4,
        complained: 5,
        captured: 8,
        revenue: { reportingCurrency: 'USD', orders: 0, amount: 0 },
        links: [
          { url: 'https://dest.test/a', clicks: 7 },
          { url: 'https://dest.test/b', clicks: 2 },
        ],
        steps: [],
        results: NO_RESULTS,
      },
    }),
    handleSiteEnrollmentsList({ body: page([], 0) }),
  )

  const { screen } = await renderWithRouter(<AutomationDetailPage />, MOUNT)

  await expect.element(screen.getByText('Opens', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('12', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Delivered', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('31', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Bounced', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('4', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Complained', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('5', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Captured, not delivered')).toBeInTheDocument()
  await expect.element(screen.getByText('8', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('https://dest.test/a')).toBeInTheDocument()
  await expect.element(screen.getByText('https://dest.test/b')).toBeInTheDocument()
})

test('marks an inactive automation as inactive', async () => {
  worker.use(
    handleSiteAutomationsGet({
      body: {
        id: testId(1),
        name: 'Unfinished flow',
        status: 'inactive',
        live: {
          number: 1,
          triggerEvent: 'contact.created',
          exitEvents: [],
          goalEvent: null,
          steps: [],
        },
        draft: null,
        revision: 1,
        ...TIMESTAMPS,
      },
    }),
    handleSiteAutomationReportsGet({
      body: {
        opens: 0,
        clicks: 0,
        delivered: 0,
        bounced: 0,
        complained: 0,
        captured: 0,
        revenue: { reportingCurrency: 'USD', orders: 0, amount: 0 },
        links: [],
        steps: [],
        results: NO_RESULTS,
      },
    }),
    handleSiteEnrollmentsList({ body: page([], 0) }),
  )

  const { screen } = await renderWithRouter(<AutomationDetailPage />, MOUNT)

  await expect.element(screen.getByText('Unfinished flow')).toBeInTheDocument()
  await expect.element(screen.getByText('Inactive', { exact: true })).toBeInTheDocument()
})

test('shows an error alert when the automation fails to load', async () => {
  worker.use(handleSiteAutomationsGet(() => problem(500)))

  const { screen } = await renderWithRouter(<AutomationDetailPage />, MOUNT)

  await expect.element(screen.getByText('Failed to load automations')).toBeInTheDocument()
})

test('shows the orders and the net revenue credited to the automation', async () => {
  worker.use(
    handleSiteAutomationsGet({
      body: {
        id: testId(1),
        name: 'Welcome series',
        status: 'active',
        live: {
          number: 1,
          triggerEvent: 'contact.created',
          exitEvents: [],
          goalEvent: null,
          steps: [],
        },
        draft: null,
        revision: 1,
        ...TIMESTAMPS,
      },
    }),
    handleSiteAutomationReportsGet({
      body: {
        opens: 0,
        clicks: 0,
        delivered: 0,
        bounced: 0,
        complained: 0,
        captured: 0,
        revenue: { reportingCurrency: 'USD', orders: 4, amount: 12000 },
        links: [],
        steps: [],
        results: NO_RESULTS,
      },
    }),
    handleSiteEnrollmentsList({ body: page([], 0) }),
  )

  const { screen } = await renderWithRouter(<AutomationDetailPage />, MOUNT)

  await expect.element(screen.getByText('Orders', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('4', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('$120.00')).toBeInTheDocument()
})

test('shows no orders or revenue on the automation until the first order sets the currency', async () => {
  worker.use(
    handleSiteAutomationsGet({
      body: {
        id: testId(1),
        name: 'Welcome series',
        status: 'active',
        live: {
          number: 1,
          triggerEvent: 'contact.created',
          exitEvents: [],
          goalEvent: null,
          steps: [],
        },
        draft: null,
        revision: 1,
        ...TIMESTAMPS,
      },
    }),
    handleSiteAutomationReportsGet({
      body: {
        opens: 0,
        clicks: 0,
        delivered: 0,
        bounced: 0,
        complained: 0,
        captured: 0,
        revenue: { reportingCurrency: null, orders: 0, amount: 0 },
        links: [],
        steps: [],
        results: NO_RESULTS,
      },
    }),
    handleSiteEnrollmentsList({ body: page([], 0) }),
  )

  const { screen } = await renderWithRouter(<AutomationDetailPage />, MOUNT)

  await expect.element(screen.getByText('Opens', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Orders', { exact: true })).not.toBeInTheDocument()
  await expect.element(screen.getByText('Revenue', { exact: true })).not.toBeInTheDocument()
})

// An active Automation with a draft above its live version (ADR 0065).
const PENDING: SiteAutomationResource = {
  id: testId(1),
  name: 'Welcome series',
  status: 'active',
  live: { number: 1, triggerEvent: 'signup', exitEvents: [], goalEvent: null, steps: [] },
  draft: {
    number: 2,
    triggerEvent: 'signup',
    exitEvents: [],
    goalEvent: null,
    steps: [{ type: 'wait', seconds: 60 }],
  },
  revision: 1,
  ...TIMESTAMPS,
}

const PUBLISHED: SiteAutomationResource = { ...PENDING, live: PENDING.draft, draft: null }

const NO_FIGURES = {
  opens: 0,
  clicks: 0,
  delivered: 0,
  bounced: 0,
  complained: 0,
  captured: 0,
  revenue: { reportingCurrency: 'USD', orders: 0, amount: 0 },
  links: [],
  steps: [],
  results: NO_RESULTS,
}

test('shows what the live version does, its exit and goal events included, not the draft above it', async () => {
  worker.use(
    handleSiteAutomationsGet({
      body: {
        ...PENDING,
        live: {
          number: 1,
          triggerEvent: 'signup',
          exitEvents: ['account_closed'],
          goalEvent: 'course_started',
          steps: [
            { type: 'email', subject: 'Welcome aboard' },
            { type: 'email', subject: 'Second look', integrationId: testId(21) },
            { type: 'apply_tag', tag: 'onboarded' },
          ],
        },
        draft: {
          number: 2,
          triggerEvent: 'course_completed',
          exitEvents: [],
          goalEvent: null,
          steps: [{ type: 'http' }],
        },
      },
    }),
    handleSiteAutomationReportsGet({ body: NO_FIGURES }),
    handleSiteEnrollmentsList({ body: page([], 0) }),
    handleSiteIntegrationsGet({ body: emailIntegration(testId(21), 'Mailgun relay') }),
  )

  const { screen } = await renderWithRouter(<AutomationDetailPage />, MOUNT)

  await expect.element(screen.getByText('Live version 1')).toBeInTheDocument()
  await expect.element(screen.getByText('signup')).toBeInTheDocument()
  await expect.element(screen.getByText('account_closed')).toBeInTheDocument()
  await expect.element(screen.getByText('course_started')).toBeInTheDocument()
  await expect.element(screen.getByText('Send email: Welcome aboard')).toBeInTheDocument()
  await expect
    .element(screen.getByText('Send email: Second look · Mailgun relay'))
    .toBeInTheDocument()
  await expect.element(screen.getByText('Apply tag: onboarded')).toBeInTheDocument()
  await expect.element(screen.getByText('course_completed')).not.toBeInTheDocument()
  await expect.element(screen.getByText('Call a URL')).not.toBeInTheDocument()
})

test('a send step through a deleted Integration names it as deleted', async () => {
  worker.use(
    handleSiteAutomationsGet({
      body: {
        ...PENDING,
        live: {
          number: 1,
          triggerEvent: 'signup',
          exitEvents: [],
          goalEvent: null,
          steps: [{ type: 'email', subject: 'Second look', integrationId: testId(21) }],
        },
      },
    }),
    handleSiteAutomationReportsGet({ body: NO_FIGURES }),
    handleSiteEnrollmentsList({ body: page([], 0) }),
    handleSiteIntegrationsGet({
      body: emailIntegration(testId(21), 'Mailgun relay', { discardedAt: '2026-02-01T00:00:00Z' }),
    }),
  )

  const { screen } = await renderWithRouter(<AutomationDetailPage />, MOUNT)

  await expect
    .element(screen.getByText('Send email: Second look · Mailgun relay (deleted)'))
    .toBeInTheDocument()
})

test('a draft above the live version is marked pending, and Publish makes it live', async () => {
  const published: string[] = []
  worker.use(
    handleSiteAutomationsGet(() => HttpResponse.json(published.length > 0 ? PUBLISHED : PENDING)),
    handleSiteAutomationReportsGet({ body: NO_FIGURES }),
    handleSiteEnrollmentsList({ body: page([], 0) }),
    handleSiteAutomationsPublish(({ params }) => {
      published.push(params.id)
      return HttpResponse.json(PUBLISHED)
    }),
  )

  const { screen } = await renderWithRouter(<AutomationDetailPage />, MOUNT)
  await expect.element(screen.getByText('Unpublished changes')).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Publish' }).click()

  await expect.poll(() => published).toEqual([testId(1)])
  await expect.element(screen.getByText('Automation published')).toBeInTheDocument()
  await expect.element(screen.getByText('Unpublished changes')).not.toBeInTheDocument()
})

test('Revert drops the draft', async () => {
  const reverted: string[] = []
  const live: SiteAutomationResource = { ...PENDING, draft: null }
  worker.use(
    handleSiteAutomationsGet(() => HttpResponse.json(reverted.length > 0 ? live : PENDING)),
    handleSiteAutomationReportsGet({ body: NO_FIGURES }),
    handleSiteEnrollmentsList({ body: page([], 0) }),
    handleSiteAutomationsRevert(({ params }) => {
      reverted.push(params.id)
      return HttpResponse.json(live)
    }),
  )

  const { screen } = await renderWithRouter(<AutomationDetailPage />, MOUNT)
  await screen.getByRole('button', { name: 'Revert' }).click()

  await expect.poll(() => reverted).toEqual([testId(1)])
  await expect.element(screen.getByText('Unpublished changes reverted')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Revert' })).not.toBeInTheDocument()
})

test('a never-published automation offers Publish but not Revert', async () => {
  worker.use(
    handleSiteAutomationsGet({ body: { ...PENDING, status: 'inactive', live: null } }),
    handleSiteAutomationReportsGet({ body: NO_FIGURES }),
    handleSiteEnrollmentsList({ body: page([], 0) }),
  )

  const { screen } = await renderWithRouter(<AutomationDetailPage />, MOUNT)

  await expect.element(screen.getByRole('button', { name: 'Publish' })).toBeEnabled()
  await expect.element(screen.getByRole('button', { name: 'Revert' })).not.toBeInTheDocument()
  await expect.element(screen.getByText('Unpublished changes')).not.toBeInTheDocument()
})

test('a refused publish shows the reason', async () => {
  worker.use(
    handleSiteAutomationsGet({ body: PENDING }),
    handleSiteAutomationReportsGet({ body: NO_FIGURES }),
    handleSiteEnrollmentsList({ body: page([], 0) }),
    handleSiteAutomationsPublish(() => problem(422, { detail: 'integration not found' })),
  )

  const { screen } = await renderWithRouter(<AutomationDetailPage />, MOUNT)
  await screen.getByRole('button', { name: 'Publish' }).click()

  await expect.element(screen.getByText('Failed to change automation status')).toBeInTheDocument()
  await expect.element(screen.getByText('integration not found')).toBeInTheDocument()
})

// The 30 days to 17 March against the 30 days before: what the report answers when the page names
// no period, the contract's default.
const MONTH: SiteAutomationResults = {
  period: '30d',
  current: {
    entered: 4,
    completed: 1,
    goalReached: 1,
    exited: 1,
    failed: 1,
    from: '2026-02-16',
    to: '2026-03-17',
    averageDurationSeconds: 85_050,
    revenue: { reportingCurrency: 'USD', orders: 1, amount: 3500 },
  },
  previous: {
    entered: 3,
    completed: 1,
    goalReached: 1,
    exited: 1,
    failed: 0,
    from: '2026-01-17',
    to: '2026-02-15',
    averageDurationSeconds: 57_600,
    revenue: { reportingCurrency: 'USD', orders: 1, amount: 2500 },
  },
  series: [],
}

// The 7 days to 17 March against the 7 days before.
const WEEK: SiteAutomationResults = {
  period: '7d',
  current: { ...MONTH.current, entered: 9, from: '2026-03-11', to: '2026-03-17' },
  previous: { ...MONTH.previous, entered: 6, from: '2026-03-04', to: '2026-03-10' },
  series: [],
}

// The results the report answers for the period a request names (none: the default).
const RESULTS_BY_PERIOD = new Map<string | null, SiteAutomationResults>([
  [null, MONTH],
  ['7d', WEEK],
])

test('shows what the enrollments did in the period against the period before', async () => {
  worker.use(
    handleSiteAutomationsGet({ body: PENDING }),
    handleSiteAutomationReportsGet({ body: { ...NO_FIGURES, results: MONTH } }),
    handleSiteEnrollmentsList({ body: page([], 0) }),
  )

  const { screen } = await renderWithRouter(<AutomationDetailPage />, MOUNT)

  await expect.element(screen.getByText('Entered', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Previous 30 days: 3 (+33.33%)')).toBeInTheDocument()
  await expect.element(screen.getByText('Previous 30 days: 0', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('23h 37m')).toBeInTheDocument()
  await expect.element(screen.getByText('Previous 30 days: 16h (+47.66%)')).toBeInTheDocument()
  await expect.element(screen.getByText('$35.00')).toBeInTheDocument()
  await expect.element(screen.getByText('Previous 30 days: $25.00 (+40%)')).toBeInTheDocument()
})

test('the period picked shows the results of that period', async () => {
  worker.use(
    handleSiteAutomationsGet({ body: PENDING }),
    handleSiteAutomationReportsGet(({ request }) => {
      const results = RESULTS_BY_PERIOD.get(new URL(request.url).searchParams.get('period'))
      return HttpResponse.json({ ...NO_FIGURES, results: results ?? NO_RESULTS })
    }),
    handleSiteEnrollmentsList({ body: page([], 0) }),
  )
  const { screen } = await renderWithRouter(<AutomationDetailPage />, MOUNT)
  await expect.element(screen.getByText('Previous 30 days: 3 (+33.33%)')).toBeInTheDocument()

  await screen.getByText('7 days', { exact: true }).click()

  await expect.element(screen.getByText('Previous 7 days: 6 (+50%)')).toBeInTheDocument()
  await expect.element(screen.getByText(/from 03\/11\/2026 to 03\/17\/2026/)).toBeInTheDocument()
  await expect.element(screen.getByText('Previous 30 days: 3 (+33.33%)')).not.toBeInTheDocument()
})

test('shows what each send step sent and skipped, by its position', async () => {
  worker.use(
    handleSiteAutomationsGet({ body: { ...PENDING, draft: null } }),
    handleSiteAutomationReportsGet({
      body: {
        ...NO_FIGURES,
        steps: [
          { step: 0, sent: 41, skipped: 3 },
          { step: 2, sent: 17, skipped: 29 },
        ],
      },
    }),
    handleSiteEnrollmentsList({ body: page([], 0) }),
  )

  const { screen } = await renderWithRouter(<AutomationDetailPage />, MOUNT)

  await expect.element(screen.getByText('Send steps')).toBeInTheDocument()
  await expect.element(screen.getByRole('row', { name: '1 41 3' })).toBeInTheDocument()
  await expect.element(screen.getByRole('row', { name: '3 17 29' })).toBeInTheDocument()
})

test('an archived automation is read-only: it offers no edit and no publish', async () => {
  worker.use(
    handleSiteAutomationsGet({
      body: {
        id: testId(1),
        name: 'Old referral nudge',
        status: 'inactive',
        live: null,
        draft: {
          number: 1,
          triggerEvent: 'contact.created',
          exitEvents: [],
          goalEvent: null,
          steps: [],
        },
        archivedAt: TIMESTAMPS.updatedAt,
        revision: 1,
        ...TIMESTAMPS,
      },
    }),
    handleSiteAutomationReportsGet({
      body: {
        opens: 0,
        clicks: 0,
        delivered: 0,
        bounced: 0,
        complained: 0,
        captured: 0,
        revenue: { reportingCurrency: 'USD', orders: 0, amount: 0 },
        links: [],
        steps: [],
        results: NO_RESULTS,
      },
    }),
    handleSiteEnrollmentsList({ body: page([], 0) }),
  )

  const { screen } = await renderWithRouter(<AutomationDetailPage />, MOUNT)

  await expect.element(screen.getByText('Old referral nudge')).toBeInTheDocument()
  await expect.element(screen.getByRole('link', { name: 'Edit' })).not.toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Publish' })).not.toBeInTheDocument()
})
