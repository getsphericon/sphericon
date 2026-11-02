import { HttpResponse } from 'msw'
import { beforeEach, expect, test } from 'vitest'

import {
  handleSiteCustomFieldsList,
  handleSiteCustomFieldsUpdate,
  handleSiteWorkspacesList,
} from '../../generated/site/msw.gen.ts'
import type { SiteWorkspaceResource } from '../../generated/site/types.gen.ts'
import { Route as customFieldsRoute } from '../../routes/workspaces/$workspaceId/custom-fields/index.tsx'
import { page, TIMESTAMPS } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { CustomFieldsListPage } from './list.tsx'

const LIST_ROUTE = routeMount(customFieldsRoute, { workspaceId: 'TestWs01' })

const PLAN = {
  id: '1',
  key: 'plan',
  name: 'Plan',
  type: 'string' as const,
  ruleOperators: [],
  ...TIMESTAMPS,
}
const SEATS = {
  id: '2',
  key: 'seats',
  name: 'Seats',
  type: 'number' as const,
  ruleOperators: [],
  ...TIMESTAMPS,
}

test('lists the custom fields with their key and type', async () => {
  worker.use(handleSiteCustomFieldsList({ body: page([PLAN, SEATS], 2) }))

  const { screen } = await renderWithRouter(<CustomFieldsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Plan', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('plan', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Text', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Seats', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Number', { exact: true })).toBeInTheDocument()
})

test('shows the empty state when there are no custom fields', async () => {
  worker.use(handleSiteCustomFieldsList({ body: page([], 0) }))

  const { screen } = await renderWithRouter(<CustomFieldsListPage />, LIST_ROUTE)

  await expect
    .element(
      screen.getByText(
        'Custom fields appear here once a contact is identified with a new attribute.',
      ),
    )
    .toBeInTheDocument()
})

const workspace = (permissions: SiteWorkspaceResource['permissions']) =>
  handleSiteWorkspacesList({
    body: [
      {
        id: 'TestWs01',
        name: 'Acme',
        collectKey: 'omck_test_key',
        postalAddress: '',
        timezone: 'UTC',
        accountLinkAvailable: false,
        permissions,
        mcpUrl: 'https://sphericon.test',
        trackerUrl: 'https://t.sphericon.test',
        holds: [],
        role: 'admin',
        createdAt: '2026-01-01T00:00:00Z',
      },
    ],
  })

// Every test but the one that grants it starts as a member who may not manage custom fields.
beforeEach(() => {
  worker.use(workspace([]))
})

test('changes a field name and type when the member may manage custom fields', async () => {
  const bodies: unknown[] = []
  worker.use(
    workspace(['custom_fields:manage']),
    handleSiteCustomFieldsList({ body: page([PLAN], 1) }),
    handleSiteCustomFieldsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ ...PLAN, name: 'Renewal', type: 'datetime' })
    }),
  )
  const { screen } = await renderWithRouter(<CustomFieldsListPage />, LIST_ROUTE)

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Edit' }).click()
  const dialog = screen.getByRole('dialog')
  await dialog.getByLabelText(/^Name/).fill('Renewal')
  await dialog.getByRole('combobox', { name: 'Type' }).click()
  await screen.getByRole('option', { name: 'Date and time' }).click()
  await dialog.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => bodies).toEqual([{ name: 'Renewal', type: 'datetime' }])
})

test('offers no edit without the permission to manage custom fields', async () => {
  worker.use(handleSiteCustomFieldsList({ body: page([PLAN], 1) }))

  const { screen } = await renderWithRouter(<CustomFieldsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Plan', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'More actions' })).not.toBeInTheDocument()
})

test('shows an error alert when the list fails to load', async () => {
  worker.use(handleSiteCustomFieldsList(() => problem(500)))

  const { screen } = await renderWithRouter(<CustomFieldsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Failed to load custom fields')).toBeInTheDocument()
})
