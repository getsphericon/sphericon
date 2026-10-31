import { expect, test } from 'vitest'

import { handleSiteCustomFieldsList } from '../../generated/site/msw.gen.ts'
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

test('shows an error alert when the list fails to load', async () => {
  worker.use(handleSiteCustomFieldsList(() => problem(500)))

  const { screen } = await renderWithRouter(<CustomFieldsListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Failed to load custom fields')).toBeInTheDocument()
})
