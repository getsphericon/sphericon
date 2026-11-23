import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteAutomationReportsGet,
  handleSiteAutomationsGet,
  handleSiteAutomationsPublish,
  handleSiteAutomationsRevert,
  handleSiteEnrollmentsList,
  handleSiteIntegrationsGet,
  handleSiteSkippedEntriesList,
  handleSiteWorkspacesList,
} from '../../generated/site/msw.gen.ts'
import type { SiteAutomationResource, AutomationResults } from '../../generated/site/types.gen.ts'
import { Route as automationsDetailRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/index.tsx'
import {
  emailIntegration,
  page,
  stepId,
  testId,
  TIMESTAMPS,
  workspace,
} from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { formatDateTime } from '../../utils/datetime.ts'
import { AutomationDetailPage } from './detail.tsx'

const MOUNT = routeMount(automationsDetailRoute, {
  workspaceId: 'TestWs01',
  automationId: testId(1),
})

// When the wait an Enrollment stands on is due.
const WAIT_DUE = '2026-03-20T09:30:00Z'

const NO_REVENUE = { reportingCurrency: 'USD', orders: 0, amount: 0 }

// A period in which nothing happened, nor in the one before.
const NO_RESULTS: AutomationResults = {
  period: '30d',
  current: {
    entered: 0,
    completed: 0,
    goalReached: 0,
    exited: 0,
    failed: 0,
    from: '2026-02-16',
    to: '2026-03-17',
    contacts: 0,
    skipped: { cycle: 0, activeEnrollment: 0, once: 0 },
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
    contacts: 0,
    skipped: { cycle: 0, activeEnrollment: 0, once: 0 },
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
          trigger: { kind: 'event' as const, action: 'contact.created' },
          exitEvents: [],
          goalEvent: null,
          steps: { id: stepId(1), steps: [] },
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
        versionSteps: [],
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
            ...TIMESTAMPS,
          },
          {
            id: testId(6),
            automationId: testId(1),
            contactId: testId(43),
            status: 'active',
            currentStepId: stepId(2),
            resumeAt: WAIT_DUE,
            ...TIMESTAMPS,
          },
        ],
        2,
      ),
    }),
  )

  const { screen } = await renderWithRouter(<AutomationDetailPage />, MOUNT)

  await expect.element(screen.getByText('Welcome series')).toBeInTheDocument()
  await expect.element(screen.getByText(testId(42))).toBeInTheDocument()
  await expect.element(screen.getByRole('cell', { name: 'Completed' })).toBeInTheDocument()
  // An Enrollment on a wait shows when the wait is due.
  await expect.element(screen.getByRole('cell', { name: 'Waiting until' })).toBeInTheDocument()
  await expect.element(screen.getByText(formatDateTime(WAIT_DUE, 'en'))).toBeInTheDocument()
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
          trigger: { kind: 'event' as const, action: 'contact.created' },
          exitEvents: [],
          goalEvent: null,
          steps: { id: stepId(1), steps: [] },
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
        versionSteps: [],
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
          trigger: { kind: 'event' as const, action: 'contact.created' },
          exitEvents: [],
          goalEvent: null,
          steps: { id: stepId(1), steps: [] },
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
        versionSteps: [],
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
          trigger: { kind: 'event' as const, action: 'contact.created' },
          exitEvents: [],
          goalEvent: null,
          steps: { id: stepId(1), steps: [] },
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
        versionSteps: [],
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
          trigger: { kind: 'event' as const, action: 'contact.created' },
          exitEvents: [],
          goalEvent: null,
          steps: { id: stepId(1), steps: [] },
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
        versionSteps: [],
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
  live: {
    number: 1,
    trigger: { kind: 'event' as const, action: 'signup' },
    exitEvents: [],
    goalEvent: null,
    steps: { id: stepId(1), steps: [] },
  },
  draft: {
    number: 2,
    trigger: { kind: 'event' as const, action: 'signup' },
    exitEvents: [],
    goalEvent: null,
    steps: { id: stepId(1), steps: [{ id: stepId(11), type: 'wait', seconds: 60 }] },
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
  versionSteps: [],
  results: NO_RESULTS,
}

test('shows what the live version does, its exit and goal events included, not the draft above it', async () => {
  worker.use(
    handleSiteAutomationsGet({
      body: {
        ...PENDING,
        live: {
          number: 1,
          trigger: { kind: 'event' as const, action: 'signup' },
          exitEvents: [{ action: 'account_closed' }],
          goalEvent: { action: 'course_started' },
          steps: {
            id: stepId(1),
            steps: [
              { id: stepId(11), type: 'email', subject: 'Welcome aboard', body: '' },
              {
                id: stepId(12),
                type: 'email',
                subject: 'Second look',
                body: '',
                integrationId: testId(21),
              },
              { id: stepId(13), type: 'apply_tag', tag: 'onboarded' },
              { id: stepId(14), type: 'end' },
            ],
          },
        },
        draft: {
          number: 2,
          trigger: { kind: 'event' as const, action: 'course_completed' },
          exitEvents: [],
          goalEvent: null,
          steps: {
            id: stepId(1),
            steps: [{ id: stepId(11), type: 'http', body: '{}', headers: [], hasSecret: false }],
          },
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
  await expect.element(screen.getByText('End', { exact: true })).toBeInTheDocument()
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
          trigger: { kind: 'event' as const, action: 'signup' },
          exitEvents: [],
          goalEvent: null,
          steps: {
            id: stepId(1),
            steps: [
              {
                id: stepId(11),
                type: 'email',
                subject: 'Second look',
                body: '',
                integrationId: testId(21),
              },
            ],
          },
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
const MONTH: AutomationResults = {
  period: '30d',
  current: {
    entered: 4,
    completed: 1,
    goalReached: 1,
    exited: 1,
    failed: 1,
    from: '2026-02-16',
    to: '2026-03-17',
    contacts: 4,
    skipped: { cycle: 6, activeEnrollment: 2, once: 7 },
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
    contacts: 2,
    skipped: { cycle: 3, activeEnrollment: 1, once: 5 },
    averageDurationSeconds: 57_600,
    revenue: { reportingCurrency: 'USD', orders: 1, amount: 2500 },
  },
  series: [],
}

// The 7 days to 17 March against the 7 days before.
const WEEK: AutomationResults = {
  period: '7d',
  current: { ...MONTH.current, entered: 9, from: '2026-03-11', to: '2026-03-17' },
  previous: { ...MONTH.previous, entered: 6, from: '2026-03-04', to: '2026-03-10' },
  series: [],
}

// The results the report answers for the period a request names (none: the default).
const RESULTS_BY_PERIOD = new Map<string | null, AutomationResults>([
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
  await expect.element(screen.getByText('Contacts', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Previous 30 days: 2 (+100%)')).toBeInTheDocument()
  await expect.element(screen.getByText('Skipped: a cycle')).toBeInTheDocument()
  await expect.element(screen.getByText('Previous 30 days: 3 (+100%)')).toBeInTheDocument()
  await expect.element(screen.getByText('Skipped: an enrollment was active')).toBeInTheDocument()
  await expect.element(screen.getByText('Skipped: enrolled before')).toBeInTheDocument()
  await expect.element(screen.getByText('Previous 30 days: 5 (+40%)')).toBeInTheDocument()
})

test('lists the skipped entries, each with its reason, and filters them by contact', async () => {
  worker.use(
    handleSiteAutomationsGet({ body: PENDING }),
    handleSiteAutomationReportsGet({ body: { ...NO_FIGURES, results: NO_RESULTS } }),
    handleSiteEnrollmentsList({ body: page([], 0) }),
    handleSiteSkippedEntriesList(({ request }) => {
      const contactId = new URL(request.url).searchParams.get('contactId')
      const all = [
        {
          id: testId(1),
          automationId: testId(1),
          contactId: testId(7),
          reason: 'once' as const,
          eventSourceId: 'evt-7',
          occurredAt: '2026-03-16T10:00:00Z',
          createdAt: '2026-03-16T10:00:01Z',
        },
        {
          id: testId(2),
          automationId: testId(1),
          contactId: testId(8),
          reason: 'active_enrollment' as const,
          eventSourceId: 'evt-8',
          occurredAt: '2026-03-15T10:00:00Z',
          createdAt: '2026-03-15T10:00:01Z',
        },
      ]
      const items = all.filter((e) => contactId === null || e.contactId === contactId)
      return HttpResponse.json(page(items, items.length))
    }),
  )
  const { screen } = await renderWithRouter(<AutomationDetailPage />, MOUNT)

  await screen.getByRole('button', { name: 'Skipped entries' }).click()

  await expect.element(screen.getByText('An enrollment was active')).toBeInTheDocument()
  await expect.element(screen.getByText('Enrolled before')).toBeInTheDocument()
  await screen.getByLabelText('Contact ID').fill(testId(8))
  await expect.element(screen.getByText('Enrolled before')).not.toBeInTheDocument()
  await expect.element(screen.getByText('An enrollment was active')).toBeInTheDocument()
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

test('shows what each send step sent and skipped, by its id across versions, named by the live version', async () => {
  const live = {
    number: 2,
    trigger: { kind: 'event' as const, action: 'signup' },
    exitEvents: [],
    goalEvent: null,
    steps: {
      id: stepId(1),
      steps: [
        { id: stepId(11), type: 'email' as const, subject: 'Welcome aboard', body: '' },
        { id: stepId(12), type: 'wait' as const, seconds: 60 },
      ],
    },
  }
  worker.use(
    handleSiteAutomationsGet({ body: { ...PENDING, live, draft: null } }),
    handleSiteAutomationReportsGet({
      body: {
        ...NO_FIGURES,
        steps: [
          { stepId: stepId(11), sent: 41, skipped: 3 },
          { stepId: stepId(13), sent: 17, skipped: 29 },
        ],
        versionSteps: [
          { version: 1, stepId: stepId(13), sent: 17, skipped: 29 },
          { version: 1, stepId: stepId(11), sent: 20, skipped: 3 },
          { version: 2, stepId: stepId(11), sent: 21, skipped: 0 },
        ],
      },
    }),
    handleSiteEnrollmentsList({ body: page([], 0) }),
    // The live version sends email: the page reads the Workspace's postal address (ADR 0156).
    handleSiteWorkspacesList({ body: [workspace()] }),
  )

  const { screen } = await renderWithRouter(<AutomationDetailPage />, MOUNT)

  await expect.element(screen.getByText('Send steps')).toBeInTheDocument()
  await expect
    .element(screen.getByRole('row', { name: '1. Send email: Welcome aboard 41 3' }))
    .toBeInTheDocument()
  await expect
    .element(screen.getByRole('row', { name: 'Not in the live version 17 29' }))
    .toBeInTheDocument()
  await expect
    .element(screen.getByRole('row', { name: '2 1. Send email: Welcome aboard 21 0' }))
    .toBeInTheDocument()
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
          trigger: { kind: 'event' as const, action: 'contact.created' },
          exitEvents: [],
          goalEvent: null,
          steps: { id: stepId(1), steps: [] },
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
        versionSteps: [],
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
