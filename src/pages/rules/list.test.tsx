import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteRulesActivate,
  handleSiteRulesArchive,
  handleSiteRulesDeactivate,
  handleSiteRulesDelete,
  handleSiteRulesList,
  handleSiteRulesUnarchive,
} from '../../generated/site/msw.gen.ts'
import type { SiteRuleResource } from '../../generated/site/types.gen.ts'
import { Route as rulesRoute } from '../../routes/workspaces/$workspaceId/rules/index.tsx'
import { page, TIMESTAMPS } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { RulesListPage } from './list.tsx'

const LIST_ROUTE = routeMount(rulesRoute, { workspaceId: 'TestWs01' })
const ARCHIVED_ROUTE = { ...LIST_ROUTE, initialPath: `${LIST_ROUTE.initialPath}?archived=true` }

const WELCOME: SiteRuleResource = {
  id: '1',
  name: 'Tag signups',
  status: 'draft',
  triggerEvents: ['signed_up', 'trial_started'],
  actions: [{ type: 'apply_tag', tag: 'new' }],
  ...TIMESTAMPS,
}
const LIVE: SiteRuleResource = { ...WELCOME, id: '2', name: 'Untag churned', status: 'active' }

test('lists the workspace rules with their trigger events and status', async () => {
  worker.use(handleSiteRulesList({ body: page([WELCOME, LIVE], 2) }))

  const { screen } = await renderWithRouter(<RulesListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Tag signups')).toBeInTheDocument()
  await expect.element(screen.getByText('Untag churned')).toBeInTheDocument()
  await expect.element(screen.getByText('signed_up, trial_started').first()).toBeInTheDocument()
  await expect.element(screen.getByText('Draft')).toBeInTheDocument()
  await expect.element(screen.getByText('Active')).toBeInTheDocument()
})

test('shows the empty state when there are no rules', async () => {
  worker.use(handleSiteRulesList({ body: page([], 0) }))

  const { screen } = await renderWithRouter(<RulesListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Create your first rule.')).toBeInTheDocument()
})

test('shows an error alert when the list fails to load', async () => {
  worker.use(handleSiteRulesList(() => problem(500)))

  const { screen } = await renderWithRouter(<RulesListPage />, LIST_ROUTE)

  await expect.element(screen.getByText('Failed to load rules')).toBeInTheDocument()
})

test('New rule links to the create page', async () => {
  worker.use(handleSiteRulesList({ body: page([], 0) }))

  const { screen } = await renderWithRouter(<RulesListPage />, LIST_ROUTE)

  await expect
    .element(screen.getByRole('link', { name: 'New rule' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/rules/new')
})

test('the row menu links to the firings and the editor', async () => {
  worker.use(handleSiteRulesList({ body: page([WELCOME], 1) }))

  const { screen } = await renderWithRouter(<RulesListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('Tag signups')).toBeInTheDocument()

  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await expect
    .element(screen.getByRole('menuitem', { name: 'View' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/rules/1')
  await expect
    .element(screen.getByRole('menuitem', { name: 'Edit' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/rules/1/edit')
})

test('activating a draft rule calls activate and refreshes the list', async () => {
  const activated: string[] = []
  let listFetches = 0
  worker.use(
    handleSiteRulesList(() => {
      listFetches++
      return HttpResponse.json(
        page(activated.length > 0 ? [{ ...WELCOME, status: 'active' }] : [WELCOME]),
      )
    }),
    handleSiteRulesActivate(({ params }) => {
      activated.push(params.id)
      return HttpResponse.json({ ...WELCOME, status: 'active' })
    }),
  )

  const { screen } = await renderWithRouter(<RulesListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('Draft')).toBeInTheDocument()

  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await screen.getByRole('menuitem', { name: 'Activate' }).click()

  await expect.poll(() => activated).toEqual(['1'])
  await expect.element(screen.getByText('Rule activated')).toBeInTheDocument()
  await expect.poll(() => listFetches).toBe(2)
  await expect.element(screen.getByText('Active')).toBeInTheDocument()
})

test('deactivating an active rule calls deactivate', async () => {
  const deactivated: string[] = []
  worker.use(
    handleSiteRulesList({ body: page([LIVE], 1) }),
    handleSiteRulesDeactivate(({ params }) => {
      deactivated.push(params.id)
      return HttpResponse.json({ ...LIVE, status: 'draft' })
    }),
  )

  const { screen } = await renderWithRouter(<RulesListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('Untag churned')).toBeInTheDocument()

  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await screen.getByRole('menuitem', { name: 'Deactivate' }).click()

  await expect.poll(() => deactivated).toEqual(['2'])
  await expect.element(screen.getByText('Rule deactivated')).toBeInTheDocument()
})

test('a failed activation shows the problem', async () => {
  worker.use(
    handleSiteRulesList({ body: page([WELCOME], 1) }),
    handleSiteRulesActivate(() => problem(422, { detail: 'a Rule needs an Action' })),
  )

  const { screen } = await renderWithRouter(<RulesListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('Tag signups')).toBeInTheDocument()

  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await screen.getByRole('menuitem', { name: 'Activate' }).click()

  await expect.element(screen.getByText('Failed to change rule status')).toBeInTheDocument()
  await expect.element(screen.getByText('a Rule needs an Action')).toBeInTheDocument()
})

test('archiving a rule calls archive and refreshes the list', async () => {
  const archived: string[] = []
  let listFetches = 0
  worker.use(
    handleSiteRulesList(() => {
      listFetches++
      return HttpResponse.json(page([WELCOME], 1))
    }),
    handleSiteRulesArchive(({ params }) => {
      archived.push(params.id)
      return HttpResponse.json({ ...WELCOME, archivedAt: TIMESTAMPS.updatedAt })
    }),
  )

  const { screen } = await renderWithRouter(<RulesListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('Tag signups')).toBeInTheDocument()
  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await screen.getByRole('menuitem', { name: 'Archive' }).click()

  await expect.poll(() => archived).toEqual(['1'])
  await expect.element(screen.getByText('Rule archived')).toBeInTheDocument()
  await expect.poll(() => listFetches).toBe(2)
})

test('the archived control opens the archived view through the URL', async () => {
  worker.use(handleSiteRulesList(() => HttpResponse.json({ items: [], totalItems: 0 })))

  const { screen, navigate } = await renderWithRouter(<RulesListPage />, LIST_ROUTE)
  await screen.getByText('Archived', { exact: true }).click()

  expect(navigate).toHaveBeenCalledWith(
    expect.objectContaining({ search: { archived: true }, replace: true }),
  )
})

test('the archived view lists archived rules and restores one', async () => {
  const flags: (string | null)[] = []
  const restored: string[] = []
  worker.use(
    handleSiteRulesList(({ request }) => {
      flags.push(new URL(request.url).searchParams.get('archived'))
      return HttpResponse.json(page([{ ...WELCOME, archivedAt: TIMESTAMPS.updatedAt }], 1))
    }),
    handleSiteRulesUnarchive(({ params }) => {
      restored.push(params.id)
      return HttpResponse.json(WELCOME)
    }),
  )

  const { screen } = await renderWithRouter(<RulesListPage />, ARCHIVED_ROUTE)
  await expect.poll(() => flags).toEqual(['true'])

  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await expect.element(screen.getByRole('menuitem', { name: 'Edit' })).not.toBeInTheDocument()
  await screen.getByRole('menuitem', { name: 'Restore' }).click()

  await expect.poll(() => restored).toEqual(['1'])
  await expect.element(screen.getByText('Rule restored')).toBeInTheDocument()
})

test('deleting a rule asks for confirmation first', async () => {
  const deleted: string[] = []
  worker.use(
    handleSiteRulesList({ body: page([WELCOME], 1) }),
    handleSiteRulesDelete(({ params }) => {
      deleted.push(params.id)
      return new HttpResponse(null, { status: 204 })
    }),
  )

  const { screen } = await renderWithRouter(<RulesListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('Tag signups')).toBeInTheDocument()
  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await screen.getByRole('menuitem', { name: 'Delete' }).click()
  expect(deleted).toEqual([])
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete' }).click()

  await expect.poll(() => deleted).toEqual(['1'])
  await expect.element(screen.getByText('Rule deleted')).toBeInTheDocument()
})

test('a rule with firings is not deleted and their count is shown', async () => {
  worker.use(
    handleSiteRulesList({ body: page([WELCOME], 1) }),
    handleSiteRulesDelete(() =>
      problem(409, { usedBy: [{ kind: 'firing', id: '1', name: 'Tag signups', count: 4 }] }),
    ),
  )

  const { screen } = await renderWithRouter(<RulesListPage />, LIST_ROUTE)
  await expect.element(screen.getByText('Tag signups')).toBeInTheDocument()
  await screen.getByRole('button', { name: 'More actions' }).first().click()
  await screen.getByRole('menuitem', { name: 'Delete' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete' }).click()

  await expect.element(screen.getByText(/4 firings of "Tag signups"/)).toBeInTheDocument()
})
