import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

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
const ARCHIVED_ROUTE = { ...LIST_ROUTE, initialPath: `${LIST_ROUTE.initialPath}?archived=true` }
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

test('New automation opens the create modal and Edit links to the edit page', async () => {
  worker.use(
    handleSiteAutomationsList(() => HttpResponse.json({ items: [WELCOME], totalItems: 1 })),
  )

  const { screen } = await renderWithRouter(<AutomationsListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('Welcome flow')).toBeInTheDocument()

  await screen.getByRole('button', { name: 'New automation' }).click()
  await expect.element(screen.getByRole('dialog')).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Cancel' }).click()
  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await expect
    .element(screen.getByRole('menuitem', { name: 'Edit' }))
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
  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await screen.getByRole('menuitem', { name: 'Activate' }).click()

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
  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await screen.getByRole('menuitem', { name: 'Deactivate' }).click()

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

  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await screen.getByRole('menuitem', { name: 'Delete' }).click()
  expect(deleted).toEqual([])
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete' }).click()

  await expect.poll(() => deleted).toEqual(['1'])
  await expect.element(screen.getByText('Automation deleted')).toBeInTheDocument()
})

test('an automation that sent mail is not deleted and the count of its messages is shown', async () => {
  worker.use(
    handleSiteAutomationsList(() => HttpResponse.json({ items: [WELCOME], totalItems: 1 })),
    handleSiteAutomationsDelete(() =>
      problem(409, { usedBy: [{ kind: 'message', id: '1', name: 'Welcome flow', count: 5 }] }),
    ),
  )

  const { screen } = await renderWithRouter(<AutomationsListPage />, LIST_ROUTE)
  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await screen.getByRole('menuitem', { name: 'Delete' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete' }).click()

  await expect.element(screen.getByText(/5 messages sent by "Welcome flow"/)).toBeInTheDocument()
})

test('the archived control opens the archived view through the URL', async () => {
  worker.use(handleSiteAutomationsList(() => HttpResponse.json({ items: [], totalItems: 0 })))

  const { screen, navigate } = await renderWithRouter(<AutomationsListPage />, LIST_ROUTE)
  await screen.getByText('Archived', { exact: true }).click()

  expect(navigate).toHaveBeenCalledWith(
    expect.objectContaining({ search: { archived: true }, replace: true }),
  )
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

  const { screen } = await renderWithRouter(<AutomationsListPage />, ARCHIVED_ROUTE)
  await expect.poll(() => flags).toEqual(['true'])

  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await screen.getByRole('menuitem', { name: 'Restore' }).click()

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
  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await screen.getByRole('menuitem', { name: 'Archive' }).click()

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
  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await screen.getByRole('menuitem', { name: 'Archive' }).click()

  await expect.element(screen.getByText(/3 enrollments of "Welcome flow"/)).toBeInTheDocument()
})
