import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteRuleFiringsList,
  handleSiteRulesPublish,
  handleSiteRulesGet,
  handleSiteRulesRevert,
  handleSiteRulesUnarchive,
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
  status: 'inactive',
  live: null,
  draft: { number: 1, triggerEvents: ['signed_up'], actions: [{ type: 'apply_tag', tag: 'new' }] },
  ...TIMESTAMPS,
}

// Published as version 1, with a draft above it that applies another Tag.
const EDITED: SiteRuleResource = {
  ...RULE,
  status: 'active',
  live: { number: 1, triggerEvents: ['signed_up'], actions: [{ type: 'apply_tag', tag: 'new' }] },
  draft: {
    number: 2,
    triggerEvents: ['signed_up'],
    actions: [{ type: 'apply_tag', tag: 'newer' }],
  },
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
  await expect.element(screen.getByRole('cell', { name: 'signed_up' })).toBeInTheDocument()
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

test('publishing the rule calls publish and shows it active', async () => {
  const published: string[] = []
  worker.use(
    handleSiteRulesGet(() =>
      HttpResponse.json(published.length > 0 ? { ...RULE, status: 'active' } : RULE),
    ),
    handleSiteRuleFiringsList({ body: page([], 0) }),
    handleSiteRulesPublish(({ params }) => {
      published.push(params.id)
      return HttpResponse.json({ ...RULE, status: 'active' })
    }),
  )

  const { screen } = await renderWithRouter(<RuleDetailPage />, DETAIL_ROUTE)
  await screen.getByRole('button', { name: 'Publish' }).click()

  await expect.poll(() => published).toEqual(['7'])
  await expect.element(screen.getByText('Rule published')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Deactivate' })).toBeInTheDocument()
})

test('shows an error alert when the rule cannot be loaded', async () => {
  worker.use(handleSiteRulesGet(() => problem(404, { detail: 'rule not found' })))

  const { screen } = await renderWithRouter(<RuleDetailPage />, DETAIL_ROUTE)

  await expect.element(screen.getByText('Failed to load rules')).toBeVisible()
  await expect.element(screen.getByText('rule not found')).toBeVisible()
})

test('an archived rule offers Restore instead of Publish and Edit', async () => {
  const restored: string[] = []
  worker.use(
    handleSiteRulesGet({ body: { ...RULE, archivedAt: TIMESTAMPS.updatedAt } }),
    handleSiteRuleFiringsList({ body: page([], 0) }),
    handleSiteRulesUnarchive(({ params }) => {
      restored.push(params.id)
      return HttpResponse.json(RULE)
    }),
  )

  const { screen } = await renderWithRouter(<RuleDetailPage />, DETAIL_ROUTE)
  await expect.element(screen.getByRole('button', { name: 'Publish' })).not.toBeInTheDocument()
  await screen.getByRole('button', { name: 'Restore' }).click()

  await expect.poll(() => restored).toEqual(['7'])
  await expect.element(screen.getByText('Rule restored')).toBeInTheDocument()
})

test('a rule never published shows no pending changes and offers no Revert', async () => {
  worker.use(handleSiteRulesGet({ body: RULE }), handleSiteRuleFiringsList({ body: page([], 0) }))

  const { screen } = await renderWithRouter(<RuleDetailPage />, DETAIL_ROUTE)

  await expect.element(screen.getByRole('button', { name: 'Publish' })).toBeEnabled()
  await expect.element(screen.getByText('Unpublished changes')).not.toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Revert' })).not.toBeInTheDocument()
})

test('a live rule with a draft shows the live version and the pending changes, and publishes them', async () => {
  const published: string[] = []
  const afterPublish = { ...EDITED, live: EDITED.draft, draft: null }
  worker.use(
    handleSiteRulesGet(() => HttpResponse.json(published.length > 0 ? afterPublish : EDITED)),
    handleSiteRuleFiringsList({ body: page([], 0) }),
    handleSiteRulesPublish(({ params }) => {
      published.push(params.id)
      return HttpResponse.json(afterPublish)
    }),
  )

  const { screen } = await renderWithRouter(<RuleDetailPage />, DETAIL_ROUTE)
  await expect.element(screen.getByText('Live version 1')).toBeInTheDocument()
  await expect.element(screen.getByText('Apply tag: new')).toBeInTheDocument()
  await expect.element(screen.getByText('Unpublished changes')).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Publish' }).click()

  await expect.poll(() => published).toEqual(['7'])
  await expect.element(screen.getByText('Live version 2')).toBeInTheDocument()
  await expect.element(screen.getByText('Unpublished changes')).not.toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Publish' })).toBeDisabled()
})

test('reverting drops the draft', async () => {
  const reverted: string[] = []
  const afterRevert = { ...EDITED, draft: null }
  worker.use(
    handleSiteRulesGet(() => HttpResponse.json(reverted.length > 0 ? afterRevert : EDITED)),
    handleSiteRuleFiringsList({ body: page([], 0) }),
    handleSiteRulesRevert(({ params }) => {
      reverted.push(params.id)
      return HttpResponse.json(afterRevert)
    }),
  )

  const { screen } = await renderWithRouter(<RuleDetailPage />, DETAIL_ROUTE)
  await screen.getByRole('button', { name: 'Revert' }).click()

  await expect.poll(() => reverted).toEqual(['7'])
  await expect.element(screen.getByText('Unpublished changes dropped')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Revert' })).not.toBeInTheDocument()
})

test('a refused revert shows the reason', async () => {
  worker.use(
    handleSiteRulesGet({ body: EDITED }),
    handleSiteRuleFiringsList({ body: page([], 0) }),
    handleSiteRulesRevert(() => problem(422, { detail: 'no unpublished changes to revert' })),
  )

  const { screen } = await renderWithRouter(<RuleDetailPage />, DETAIL_ROUTE)
  await screen.getByRole('button', { name: 'Revert' }).click()

  await expect.element(screen.getByText('no unpublished changes to revert')).toBeInTheDocument()
})
