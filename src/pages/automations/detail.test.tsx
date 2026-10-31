import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteAutomationReportsGet,
  handleSiteAutomationsGet,
  handleSiteAutomationsPublish,
  handleSiteAutomationsRevert,
  handleSiteEnrollmentsList,
} from '../../generated/site/msw.gen.ts'
import type { SiteAutomationResource } from '../../generated/site/types.gen.ts'
import { Route as automationsDetailRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/index.tsx'
import { page, TIMESTAMPS } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { AutomationDetailPage } from './detail.tsx'

const MOUNT = routeMount(automationsDetailRoute, { workspaceId: 'TestWs01', automationId: '1' })

test('shows the automation and the contacts enrolled in it', async () => {
  worker.use(
    handleSiteAutomationsGet({
      body: {
        id: '1',
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
      },
    }),
    handleSiteEnrollmentsList({
      body: page(
        [
          {
            id: '5',
            automationId: '1',
            contactId: '42',
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
  await expect.element(screen.getByText('42')).toBeInTheDocument()
  await expect.element(screen.getByText('Completed')).toBeInTheDocument()
})

test('shows the opens, the clicks and the clicked links of the automation', async () => {
  worker.use(
    handleSiteAutomationsGet({
      body: {
        id: '1',
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
        id: '1',
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
        id: '1',
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
      },
    }),
    handleSiteEnrollmentsList({ body: page([], 0) }),
  )

  const { screen } = await renderWithRouter(<AutomationDetailPage />, MOUNT)

  await expect.element(screen.getByText('Orders', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('4', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('$120.00')).toBeInTheDocument()
})

test('shows no orders or revenue on the automation until the currency is chosen', async () => {
  worker.use(
    handleSiteAutomationsGet({
      body: {
        id: '1',
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
  id: '1',
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
          integrationId: null,
          steps: [
            { type: 'email', subject: 'Welcome aboard' },
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
  )

  const { screen } = await renderWithRouter(<AutomationDetailPage />, MOUNT)

  await expect.element(screen.getByText('Live version 1')).toBeInTheDocument()
  await expect.element(screen.getByText('signup')).toBeInTheDocument()
  await expect.element(screen.getByText('account_closed')).toBeInTheDocument()
  await expect.element(screen.getByText('course_started')).toBeInTheDocument()
  await expect.element(screen.getByText('Workspace default')).toBeInTheDocument()
  await expect.element(screen.getByText('Send email: Welcome aboard')).toBeInTheDocument()
  await expect.element(screen.getByText('Apply tag: onboarded')).toBeInTheDocument()
  await expect.element(screen.getByText('course_completed')).not.toBeInTheDocument()
  await expect.element(screen.getByText('Call a URL')).not.toBeInTheDocument()
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

  await expect.poll(() => published).toEqual(['1'])
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

  await expect.poll(() => reverted).toEqual(['1'])
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
