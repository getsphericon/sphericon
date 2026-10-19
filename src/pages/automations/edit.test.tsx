import { HttpResponse } from 'msw'
import { beforeEach, expect, test } from 'vitest'

import {
  handleSiteAutomationsGet,
  handleSiteAutomationsUpdate,
  handleSiteEnrollmentsList,
} from '../../generated/site/msw.gen.ts'
import type { SiteAutomationResource } from '../../generated/site/types.gen.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$slug/automations/$automationId/edit.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { AutomationEditPage } from './edit.tsx'

const EDIT_ROUTE = routeMount(automationsEditRoute, { slug: 'test', automationId: '5' })
const NOW = '2026-01-01T00:00:00Z'

// The page also asks for the failed Enrollments: none unless a test says otherwise.
beforeEach(() => {
  worker.use(
    handleSiteEnrollmentsList({
      body: { items: [], page: 1, pageSize: 25, totalItems: 0, totalPages: 0 },
    }),
  )
})

test('loads the automation into the builder', async () => {
  worker.use(
    handleSiteAutomationsGet({
      body: {
        id: '5',
        name: 'Welcome flow',
        status: 'draft',
        triggerEvent: 'email.clicked',
        steps: [{ type: 'wait', seconds: 60 }],
        createdAt: NOW,
        updatedAt: NOW,
      },
    }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)

  await expect.element(screen.getByText('Edit automation')).toBeInTheDocument()
  await expect.element(screen.getByLabelText('Name', { exact: false })).toHaveValue('Welcome flow')
  await expect
    .element(screen.getByRole('combobox', { name: 'Trigger event' }))
    .toHaveValue('Email clicked')
})

test('shows an error alert when the automation fails to load', async () => {
  worker.use(handleSiteAutomationsGet(() => problem(404, 'Gone')))

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)

  await expect.element(screen.getByText('Failed to load automations').first()).toBeInTheDocument()
})

const STORED: SiteAutomationResource = {
  id: '5',
  name: 'Welcome flow',
  status: 'draft',
  triggerEvent: 'email.clicked',
  steps: [
    { type: 'wait', seconds: 60 },
    { type: 'apply_tag', tag: 'vip' },
  ],
  createdAt: NOW,
  updatedAt: NOW,
}

test('saving from the builder sends the edited name, trigger and the canvas steps', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteAutomationsGet({ body: STORED }),
    handleSiteAutomationsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(STORED)
    }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await screen.getByLabelText('Name', { exact: false }).fill('  Renamed  ')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([
      {
        name: 'Renamed',
        triggerEvent: 'email.clicked',
        steps: [
          { type: 'wait', seconds: 60 },
          { type: 'apply_tag', tag: 'vip' },
        ],
      },
    ])
  await expect.element(screen.getByText('Automation updated')).toBeInTheDocument()
})

test('a failed save from the builder shows an error toast', async () => {
  worker.use(
    handleSiteAutomationsGet({ body: STORED }),
    handleSiteAutomationsUpdate(() => problem(500)),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.element(screen.getByText('Failed to save automation')).toBeInTheDocument()
})

test('shows no failed enrollments when none has a recorded reason', async () => {
  worker.use(
    handleSiteAutomationsGet({ body: STORED }),
    handleSiteEnrollmentsList({
      body: {
        items: [
          {
            id: '9',
            automationId: '5',
            contactId: '107',
            status: 'failed',
            currentStep: 0,
            createdAt: NOW,
            updatedAt: NOW,
          },
        ],
        page: 1,
        pageSize: 25,
        totalItems: 1,
        totalPages: 1,
      },
    }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)

  await expect.element(screen.getByText('Edit automation')).toBeInTheDocument()
  await expect.element(screen.getByText('Failed enrollments')).not.toBeInTheDocument()
})

test('shows no failed enrollments when the list cannot load', async () => {
  worker.use(
    handleSiteAutomationsGet({ body: STORED }),
    handleSiteEnrollmentsList(() => problem(500)),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)

  await expect.element(screen.getByText('Edit automation')).toBeInTheDocument()
  await expect.element(screen.getByText('Failed enrollments')).not.toBeInTheDocument()
})

test('lists the enrollments an HTTP step failed, with the reason and the response', async () => {
  worker.use(
    handleSiteAutomationsGet({ body: STORED }),
    handleSiteEnrollmentsList({
      body: {
        items: [
          {
            id: '9',
            automationId: '5',
            contactId: '107',
            status: 'failed',
            currentStep: 0,
            failure: { kind: 'http_status', status: 503, excerpt: 'upstream unavailable' },
            createdAt: NOW,
            updatedAt: NOW,
          },
        ],
        page: 1,
        pageSize: 25,
        totalItems: 1,
        totalPages: 1,
      },
    }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)

  await expect.element(screen.getByText('Failed enrollments')).toBeInTheDocument()
  await expect.element(screen.getByText('The receiver answered 503')).toBeInTheDocument()
  await expect.element(screen.getByText('upstream unavailable')).toBeInTheDocument()
})
