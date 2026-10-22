import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'
import { userEvent } from 'vitest/browser'

import {
  handleSiteCustomFieldsList,
  handleSiteEventsActions,
  handleSiteRulesCreate,
  handleSiteRulesGet,
  handleSiteRulesUpdate,
} from '../../generated/site/msw.gen.ts'
import type { SiteConditionGroup, SiteRuleResource } from '../../generated/site/types.gen.ts'
import { Route as rulesEditRoute } from '../../routes/workspaces/$workspaceId/rules/$ruleId/edit.tsx'
import { Route as rulesCreateRoute } from '../../routes/workspaces/$workspaceId/rules/new.tsx'
import { page, TIMESTAMPS } from '../../test/payloads.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { RuleCreatePage, RuleEditPage } from './resource.tsx'

const CONDITION: SiteConditionGroup = {
  kind: 'group',
  combinator: 'and',
  rules: [{ kind: 'rule', field: 'email', operator: 'contains', value: '@x' }],
}

const STORED: SiteRuleResource = {
  id: '7',
  name: 'Tag signups',
  status: 'draft',
  triggerEvents: ['signed_up'],
  condition: CONDITION,
  actions: [{ type: 'apply_tag', tag: 'new' }],
  ...TIMESTAMPS,
}

const CREATE_ROUTE = routeMount(rulesCreateRoute, { workspaceId: 'TestWs01' })
const EDIT_ROUTE = routeMount(rulesEditRoute, { workspaceId: 'TestWs01', ruleId: '7' })

// Everything the rule form reads besides the rule itself: the known events (trigger picker and
// condition builder) and the custom fields (condition builder).
function serveCatalogue() {
  worker.use(
    handleSiteEventsActions({ body: { actions: ['signed_up', 'purchased'] } }),
    handleSiteCustomFieldsList({ body: page([], 0) }),
  )
}

test('creating a rule sends its name, trigger events, empty condition and actions, then opens the editor', async () => {
  const bodies: unknown[] = []
  serveCatalogue()
  worker.use(
    handleSiteRulesCreate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ ...STORED, id: '42' }, { status: 201 })
    }),
  )

  const { screen, navigate } = await renderWithRouter(<RuleCreatePage />, CREATE_ROUTE)

  await screen.getByLabelText('Name', { exact: false }).fill('Tag signups')
  await screen.getByRole('combobox', { name: 'Trigger events' }).fill('signed_up')
  await userEvent.keyboard('{Enter}')
  await screen.getByRole('button', { name: 'Add action' }).click()
  await screen.getByLabelText('Tag').fill('new')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([
      {
        name: 'Tag signups',
        triggerEvents: ['signed_up'],
        condition: { kind: 'group', combinator: 'and', rules: [] },
        actions: [{ type: 'apply_tag', tag: 'new' }],
      },
    ])
  await expect.element(screen.getByText('Rule created')).toBeInTheDocument()
  await expect.poll(() => navigate.mock.calls.length).toBe(1)
  expect(navigate.mock.calls[0]?.[0]).toMatchObject({
    params: { workspaceId: 'TestWs01', ruleId: '42' },
  })
})

test('a rule without trigger events is rejected before anything is sent', async () => {
  const bodies: unknown[] = []
  serveCatalogue()
  worker.use(
    handleSiteRulesCreate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(STORED, { status: 201 })
    }),
  )

  const { screen } = await renderWithRouter(<RuleCreatePage />, CREATE_ROUTE)

  await screen.getByLabelText('Name', { exact: false }).fill('No trigger')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.element(screen.getByText('This field is required')).toBeInTheDocument()
  expect(bodies).toEqual([])
})

test('editing shows the stored rule and saves a change, a removed action included', async () => {
  const bodies: unknown[] = []
  serveCatalogue()
  worker.use(
    handleSiteRulesGet({ body: STORED }),
    handleSiteRulesUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ ...STORED, name: 'Renamed', actions: [] })
    }),
  )

  const { screen } = await renderWithRouter(<RuleEditPage />, EDIT_ROUTE)

  await expect.element(screen.getByLabelText('Name', { exact: false })).toHaveValue('Tag signups')
  await expect.element(screen.getByLabelText('Tag')).toHaveValue('new')

  await screen.getByLabelText('Name', { exact: false }).fill('Renamed')
  await screen.getByRole('button', { name: 'Remove action' }).click()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([{ name: 'Renamed', triggerEvents: ['signed_up'], condition: CONDITION, actions: [] }])
  await expect.element(screen.getByText('Rule updated')).toBeInTheDocument()
})
