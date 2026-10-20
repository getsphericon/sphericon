import { notifications } from '@mantine/notifications'
import { HttpResponse } from 'msw'
import { afterEach, expect, test } from 'vitest'

import {
  handleSiteAutomationsActivate,
  handleSiteAutomationsArchive,
  handleSiteAutomationsDeactivate,
  handleSiteAutomationsDelete,
  handleSiteAutomationsList,
  handleSiteAutomationsUnarchive,
} from '../../generated/site/msw.gen.ts'
import type { SiteAutomationResource } from '../../generated/site/types.gen.ts'
import { Route as automationsRoute } from '../../routes/workspaces/$workspaceId/automations/index.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { AutomationsListPage } from './list.tsx'

const LIST_ROUTE = routeMount(automationsRoute, { workspaceId: 'TestWs01' })
const NOW = '2026-01-01T00:00:00Z'

const WELCOME: SiteAutomationResource = {
  id: '1',
  name: 'Welcome flow',
  status: 'active',
  triggerEvent: 'contact.created',
  steps: [],
  createdAt: NOW,
  updatedAt: NOW,
}
const NURTURE: SiteAutomationResource = {
  ...WELCOME,
  id: '2',
  name: 'Nurture flow',
  status: 'draft',
  triggerEvent: 'email.opened',
}

// Toasts live in a global store and would cover the table's buttons in the next test.
afterEach(() => {
  notifications.clean()
})

test('lists automations with their status', async () => {
  worker.use(
    handleSiteAutomationsList(() =>
      HttpResponse.json({ items: [WELCOME, NURTURE], totalItems: 2 }),
    ),
  )

  const { screen } = await renderWithRouter(<AutomationsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Welcome flow')).toBeInTheDocument()
  await expect.element(screen.getByText('Nurture flow')).toBeInTheDocument()
  await expect.element(screen.getByText('Active', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Draft', { exact: true })).toBeInTheDocument()
})

test('shows the empty state when there are no automations', async () => {
  worker.use(handleSiteAutomationsList(() => HttpResponse.json({ items: [], totalItems: 0 })))

  const { screen } = await renderWithRouter(<AutomationsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Create your first automation.')).toBeInTheDocument()
})

test('shows an error alert when the list fails to load', async () => {
  worker.use(handleSiteAutomationsList(() => problem(500)))

  const { screen } = await renderWithRouter(<AutomationsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Failed to load automations').first()).toBeInTheDocument()
})

test('New automation and Edit link to the create and edit pages', async () => {
  worker.use(
    handleSiteAutomationsList(() => HttpResponse.json({ items: [WELCOME], totalItems: 1 })),
  )

  const { screen } = await renderWithRouter(<AutomationsListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('Welcome flow')).toBeInTheDocument()

  await expect
    .element(screen.getByRole('link', { name: 'New automation' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/automations/new')
  await expect
    .element(screen.getByRole('link', { name: 'Edit' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/automations/1/edit')
})

test('activating a draft automation calls the API and refreshes the list', async () => {
  const activated: string[] = []
  let listFetches = 0
  worker.use(
    handleSiteAutomationsList(() => {
      listFetches++
      return HttpResponse.json({ items: [NURTURE], totalItems: 1 })
    }),
    handleSiteAutomationsActivate(({ params }) => {
      activated.push(params.id)
      return HttpResponse.json({ ...NURTURE, status: 'active' })
    }),
  )

  const { screen } = await renderWithRouter(<AutomationsListPage />, LIST_ROUTE)
  await screen.getByRole('button', { name: 'Activate' }).click()

  await expect.poll(() => activated).toEqual(['2'])
  await expect.element(screen.getByText('Automation activated')).toBeInTheDocument()
  await expect.poll(() => listFetches).toBe(2)
})

test('deactivating an active automation calls the API', async () => {
  const deactivated: string[] = []
  worker.use(
    handleSiteAutomationsList(() => HttpResponse.json({ items: [WELCOME], totalItems: 1 })),
    handleSiteAutomationsDeactivate(({ params }) => {
      deactivated.push(params.id)
      return HttpResponse.json({ ...WELCOME, status: 'draft' })
    }),
  )

  const { screen } = await renderWithRouter(<AutomationsListPage />, LIST_ROUTE)
  await screen.getByRole('button', { name: 'Deactivate' }).click()

  await expect.poll(() => deactivated).toEqual(['1'])
  await expect.element(screen.getByText('Automation deactivated')).toBeInTheDocument()
})

test('deleting an automation asks for confirmation first', async () => {
  const deleted: string[] = []
  worker.use(
    handleSiteAutomationsList(() => HttpResponse.json({ items: [WELCOME], totalItems: 1 })),
    handleSiteAutomationsDelete(({ params }) => {
      deleted.push(params.id)
      return new HttpResponse(null, { status: 204 })
    }),
  )

  const { screen } = await renderWithRouter(<AutomationsListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('Welcome flow')).toBeInTheDocument()

  await screen.getByRole('button', { name: 'Delete' }).click()
  expect(deleted).toEqual([])
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete' }).click()

  await expect.poll(() => deleted).toEqual(['1'])
  await expect.element(screen.getByText('Automation deleted')).toBeInTheDocument()
})

test('the archived view lists archived automations and restores one', async () => {
  const flags: (string | null)[] = []
  const restored: string[] = []
  worker.use(
    handleSiteAutomationsList(({ request }) => {
      flags.push(new URL(request.url).searchParams.get('archived'))
      return HttpResponse.json({ items: [{ ...NURTURE, archivedAt: NOW }], totalItems: 1 })
    }),
    handleSiteAutomationsUnarchive(({ params }) => {
      restored.push(params.id)
      return HttpResponse.json(NURTURE)
    }),
  )

  const { screen } = await renderWithRouter(<AutomationsListPage />, LIST_ROUTE)
  await screen.getByRole('switch', { name: 'Show archived' }).click()
  await expect.poll(() => flags).toEqual(['false', 'true'])

  await screen.getByRole('button', { name: 'Restore' }).click()

  await expect.poll(() => restored).toEqual(['2'])
  await expect.element(screen.getByText('Automation restored')).toBeInTheDocument()
})

test('archiving an automation calls archive and refreshes the list', async () => {
  const archived: string[] = []
  let listFetches = 0
  worker.use(
    handleSiteAutomationsList(() => {
      listFetches++
      return HttpResponse.json({ items: [NURTURE], totalItems: 1 })
    }),
    handleSiteAutomationsArchive(({ params }) => {
      archived.push(params.id)
      return HttpResponse.json({ ...NURTURE, archivedAt: NOW })
    }),
  )

  const { screen } = await renderWithRouter(<AutomationsListPage />, LIST_ROUTE)
  await screen.getByRole('button', { name: 'Archive' }).click()

  await expect.poll(() => archived).toEqual(['2'])
  await expect.element(screen.getByText('Automation archived')).toBeInTheDocument()
  await expect.poll(() => listFetches).toBe(2)
})

test('an automation with active enrollments is not archived and their count is shown', async () => {
  worker.use(
    handleSiteAutomationsList(() => HttpResponse.json({ items: [WELCOME], totalItems: 1 })),
    handleSiteAutomationsArchive(() =>
      problem(409, { usedBy: [{ kind: 'enrollment', id: '1', name: 'Welcome flow', count: 3 }] }),
    ),
  )

  const { screen } = await renderWithRouter(<AutomationsListPage />, LIST_ROUTE)
  await screen.getByRole('button', { name: 'Archive' }).click()

  await expect.element(screen.getByText(/3 enrollments of "Welcome flow"/)).toBeInTheDocument()
})
