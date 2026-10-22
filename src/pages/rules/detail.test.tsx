import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteRuleFiringsList,
  handleSiteRulesActivate,
  handleSiteRulesGet,
} from '../../generated/site/msw.gen.ts'
import type { SiteRuleFiringResource, SiteRuleResource } from '../../generated/site/types.gen.ts'
import { Route as rulesDetailRoute } from '../../routes/workspaces/$workspaceId/rules/$ruleId/index.tsx'
import { page, TIMESTAMPS } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { RuleDetailPage } from './detail.tsx'

const DETAIL_ROUTE = routeMount(rulesDetailRoute, { workspaceId: 'TestWs01', ruleId: '7' })

const RULE: SiteRuleResource = {
  id: '7',
  name: 'Tag signups',
  status: 'draft',
  triggerEvents: ['signed_up'],
  actions: [{ type: 'apply_tag', tag: 'new' }],
  ...TIMESTAMPS,
}

const RUNNING: SiteRuleFiringResource = {
  id: '70',
  ruleId: '7',
  contactId: '33',
  action: 'signed_up',
  eventSourceId: 'evt-1',
  results: [
    { type: 'apply_tag', status: 'succeeded' },
    { type: 'remove_tag', status: 'skipped', detail: 'the contact has no such tag' },
  ],
  createdAt: '2026-01-02T00:00:00Z',
}

const FIRING: SiteRuleFiringResource = { ...RUNNING, completedAt: '2026-01-02T00:00:00Z' }

test('shows the rule and each Firing with its Event, Contact and every Action result', async () => {
  worker.use(
    handleSiteRulesGet({ body: RULE }),
    handleSiteRuleFiringsList({ body: page([FIRING], 1) }),
  )

  const { screen } = await renderWithRouter(<RuleDetailPage />, DETAIL_ROUTE)

  await expect.element(screen.getByRole('heading', { name: 'Tag signups' })).toBeInTheDocument()
  await expect.element(screen.getByText('signed_up')).toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: '33' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/contacts/33')
  await expect.element(screen.getByText('Succeeded')).toBeInTheDocument()
  await expect.element(screen.getByText('Skipped')).toBeInTheDocument()
  await expect.element(screen.getByText('the contact has no such tag')).toBeInTheDocument()
})

test('a Firing whose Actions are still running is marked in progress', async () => {
  worker.use(
    handleSiteRulesGet({ body: RULE }),
    handleSiteRuleFiringsList({
      body: page([{ ...RUNNING, results: [] }], 1),
    }),
  )

  const { screen } = await renderWithRouter(<RuleDetailPage />, DETAIL_ROUTE)

  await expect.element(screen.getByText('In progress')).toBeInTheDocument()
})

test('shows the empty state when the rule has not fired', async () => {
  worker.use(handleSiteRulesGet({ body: RULE }), handleSiteRuleFiringsList({ body: page([], 0) }))

  const { screen } = await renderWithRouter(<RuleDetailPage />, DETAIL_ROUTE)

  await expect.element(screen.getByText('This rule has not fired yet')).toBeInTheDocument()
})

test('activating the rule calls activate and shows it active', async () => {
  const activated: string[] = []
  worker.use(
    handleSiteRulesGet(() =>
      HttpResponse.json(activated.length > 0 ? { ...RULE, status: 'active' } : RULE),
    ),
    handleSiteRuleFiringsList({ body: page([], 0) }),
    handleSiteRulesActivate(({ params }) => {
      activated.push(params.id)
      return HttpResponse.json({ ...RULE, status: 'active' })
    }),
  )

  const { screen } = await renderWithRouter(<RuleDetailPage />, DETAIL_ROUTE)
  await screen.getByRole('button', { name: 'Activate' }).click()

  await expect.poll(() => activated).toEqual(['7'])
  await expect.element(screen.getByText('Rule activated')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Deactivate' })).toBeInTheDocument()
})

test('shows an error alert when the rule cannot be loaded', async () => {
  worker.use(handleSiteRulesGet(() => problem(404, { detail: 'rule not found' })))

  const { screen } = await renderWithRouter(<RuleDetailPage />, DETAIL_ROUTE)

  await expect.element(screen.getByText('Failed to load rules')).toBeVisible()
  await expect.element(screen.getByText('rule not found')).toBeVisible()
})
