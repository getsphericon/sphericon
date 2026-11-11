import { useQuery } from '@tanstack/react-query'
import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'
import { userEvent } from 'vitest/browser'

import { siteRulesGetOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import {
  handleSiteAutomationsList,
  handleSiteCustomFieldsList,
  handleSiteEventsActions,
  handleSiteEventsList,
  handleSiteRulesDryRun,
  handleSiteRulesCreate,
  handleSiteRulesGet,
  handleSiteRulesRevert,
  handleSiteRulesUpdate,
} from '../../generated/site/msw.gen.ts'
import type {
  SiteAutomationResource,
  SiteConditionGroup,
  SiteRuleResource,
} from '../../generated/site/types.gen.ts'
import { zSiteUpdateRuleInput } from '../../generated/site/zod.gen.ts'
import { Route as rulesEditRoute } from '../../routes/workspaces/$workspaceId/rules/$ruleId/edit.tsx'
import { Route as rulesCreateRoute } from '../../routes/workspaces/$workspaceId/rules/new.tsx'
import { page, TIMESTAMPS } from '../../test/payloads.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { RuleCreatePage, RuleEditPage } from './resource.tsx'

const REFRESH_RULE = 'Refresh rule'

const CONDITION: SiteConditionGroup = {
  kind: 'group',
  combinator: 'and',
  rules: [{ kind: 'rule', field: 'email', operator: 'contains', value: '@x' }],
}

const STORED: SiteRuleResource = {
  id: '7',
  name: 'Tag signups',
  status: 'inactive',
  live: null,
  draft: {
    number: 1,
    triggerEvents: ['signed_up'],
    condition: CONDITION,
    actions: [{ type: 'apply_tag', tag: 'new' }],
  },
  ...TIMESTAMPS,
}

// Published as version 1, with a draft above it that applies another Tag.
const EDITED: SiteRuleResource = {
  ...STORED,
  status: 'active',
  live: { number: 1, triggerEvents: ['signed_up'], actions: [{ type: 'apply_tag', tag: 'live' }] },
  draft: {
    number: 2,
    triggerEvents: ['signed_up'],
    actions: [{ type: 'apply_tag', tag: 'drafted' }],
  },
}

const CREATE_ROUTE = routeMount(rulesCreateRoute, { workspaceId: 'TestWs01' })
const EDIT_ROUTE = routeMount(rulesEditRoute, { workspaceId: 'TestWs01', ruleId: '7' })

// Everything the rule form reads besides the rule itself: the known events (trigger picker and
// condition builder) and the custom fields (condition builder).
function serveCatalogue() {
  worker.use(
    handleSiteEventsActions({ body: { actions: ['signed_up', 'purchased'] } }),
    handleSiteCustomFieldsList({ body: page([], 0) }),
    handleSiteEventsList({ body: page([], 0) }),
  )
}

function RefetchRule() {
  const query = useQuery(siteRulesGetOptions({ path: { workspaceId: 'TestWs01', id: '7' } }))
  return (
    <>
      <button
        type="button"
        onClick={() => {
          void query.refetch()
        }}
      >
        {REFRESH_RULE}
      </button>
      <output>{query.data?.name}</output>
    </>
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
      return HttpResponse.json({
        ...STORED,
        name: 'Renamed',
        draft: { number: 1, triggerEvents: ['signed_up'], condition: CONDITION, actions: [] },
      })
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
  await expect.element(screen.getByRole('button', { name: 'Publish' })).toBeEnabled()
})

const automation = (id: string, name: string): SiteAutomationResource => ({
  id,
  name,
  status: 'active',
  live: { number: 1, triggerEvent: 'signed_up', exitEvents: [], goalEvent: null, steps: [] },
  draft: null,
  ...TIMESTAMPS,
})

test('an enroll action picks one of the automations and saves it', async () => {
  const bodies: unknown[] = []
  serveCatalogue()
  worker.use(
    handleSiteAutomationsList({
      body: page([automation('5', 'Welcome flow'), automation('6', 'Nurture flow')], 2),
    }),
    handleSiteRulesGet({
      body: {
        ...STORED,
        draft: {
          number: 1,
          triggerEvents: ['signed_up'],
          condition: CONDITION,
          actions: [{ type: 'enroll_automation', automationId: '5' }],
        },
      },
    }),
    handleSiteRulesUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(STORED)
    }),
  )

  const { screen } = await renderWithRouter(<RuleEditPage />, EDIT_ROUTE)

  await expect
    .element(screen.getByRole('combobox', { name: 'Automation' }))
    .toHaveValue('Welcome flow')
  await screen.getByRole('combobox', { name: 'Automation' }).click()
  await screen.getByRole('option', { name: 'Nurture flow' }).click()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([
      {
        name: 'Tag signups',
        triggerEvents: ['signed_up'],
        condition: CONDITION,
        actions: [{ type: 'enroll_automation', automationId: '6' }],
      },
    ])
})

test('trying the rule on a recorded event shows whether it would fire and which actions would run', async () => {
  const bodies: unknown[] = []
  serveCatalogue()
  worker.use(
    handleSiteRulesGet({ body: STORED }),
    handleSiteEventsList({
      body: page(
        [
          {
            id: '55',
            subjectId: 's',
            action: 'signed_up',
            occurredAt: '2026-01-02T00:00:00Z',
            createdAt: '2026-01-02T00:00:00Z',
          },
        ],
        1,
      ),
    }),
    handleSiteRulesDryRun(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({
        outcome: 'would_fire',
        actions: [{ type: 'apply_tag', summary: 'Apply tag "new"' }],
      })
    }),
  )

  const { screen } = await renderWithRouter(<RuleEditPage />, EDIT_ROUTE)
  await screen.getByRole('combobox', { name: 'Event' }).click()
  await screen.getByRole('option', { name: /signed_up/ }).click()
  await screen.getByRole('button', { name: 'Try the rule' }).click()

  await expect.poll(() => bodies).toEqual([{ eventId: '55' }])
  await expect.element(screen.getByText('Would fire. These actions would run:')).toBeInTheDocument()
  await expect.element(screen.getByText('Apply tag "new"')).toBeInTheDocument()
})

test('the editor edits the draft of a live rule, and Publish waits for unsaved changes', async () => {
  serveCatalogue()
  worker.use(handleSiteRulesGet({ body: EDITED }))

  const { screen } = await renderWithRouter(<RuleEditPage />, EDIT_ROUTE)

  await expect.element(screen.getByLabelText('Tag')).toHaveValue('drafted')
  await expect.element(screen.getByText('Unpublished changes')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Publish' })).toBeEnabled()
  await screen.getByLabelText('Tag').fill('edited')
  await expect.element(screen.getByRole('button', { name: 'Publish' })).toBeDisabled()
})

test('after a revert the editor shows the live version', async () => {
  const reverted: string[] = []
  const afterRevert = { ...EDITED, draft: null }
  serveCatalogue()
  worker.use(
    handleSiteRulesGet(() => HttpResponse.json(reverted.length > 0 ? afterRevert : EDITED)),
    handleSiteRulesRevert(({ params }) => {
      reverted.push(params.id)
      return HttpResponse.json(afterRevert)
    }),
  )

  const { screen } = await renderWithRouter(<RuleEditPage />, EDIT_ROUTE)
  await expect.element(screen.getByLabelText('Tag')).toHaveValue('drafted')
  await screen.getByRole('button', { name: 'Revert' }).click()

  await expect.poll(() => reverted).toEqual(['7'])
  await expect.element(screen.getByLabelText('Tag')).toHaveValue('live')
})

test('a background refetch preserves unfinished structured edits and their dirty state', async () => {
  let latest = STORED
  serveCatalogue()
  worker.use(handleSiteRulesGet(() => HttpResponse.json(latest)))

  const { screen } = await renderWithRouter(
    <>
      <RefetchRule />
      <RuleEditPage />
    </>,
    EDIT_ROUTE,
  )
  await expect.element(screen.getByLabelText('Tag')).toHaveValue('new')
  await screen.getByLabelText('Tag').fill('unfinished')
  latest = { ...STORED, name: 'Refetched name' }
  await screen.getByRole('button', { name: REFRESH_RULE }).click()

  await expect.element(screen.getByText('Refetched name')).toBeInTheDocument()
  await expect.element(screen.getByLabelText('Name', { exact: false })).toHaveValue('Tag signups')
  await expect.element(screen.getByLabelText('Tag')).toHaveValue('unfinished')
  await expect.element(screen.getByRole('button', { name: 'Publish' })).toBeDisabled()
})

// A server whose first write waits at the gate until released; it then holds what it was sent as
// the draft above the live version, which a refetch reads.
function gatedRuleServer(initial: SiteRuleResource) {
  const received = Promise.withResolvers<undefined>()
  const release = Promise.withResolvers<undefined>()
  let server = initial
  worker.use(
    handleSiteRulesGet(() => HttpResponse.json(server)),
    handleSiteRulesUpdate(async ({ request }) => {
      const { name, ...definition } = zSiteUpdateRuleInput.parse(await request.json())
      received.resolve(undefined)
      await release.promise
      server = {
        ...server,
        name: name ?? server.name,
        draft: {
          number: server.draft?.number ?? (server.live?.number ?? 0) + 1,
          triggerEvents: definition.triggerEvents ?? [],
          actions: definition.actions ?? [],
        },
      }
      return HttpResponse.json(server)
    }),
  )
  return { received: received.promise, release: () => release.resolve(undefined) }
}

test.each([
  ['a live rule without a draft', { ...EDITED, draft: null }, 'live'],
  ['a rule with a draft', EDITED, 'drafted'],
])(
  'on %s, an edit made while Save is pending stays, unsaved, after the acknowledgement',
  async (_case, rule, tag) => {
    serveCatalogue()
    const gate = gatedRuleServer(rule)

    const { screen } = await renderWithRouter(<RuleEditPage />, EDIT_ROUTE)
    await expect.element(screen.getByLabelText('Tag')).toHaveValue(tag)
    await screen.getByLabelText('Tag').fill('saved')
    await screen.getByRole('button', { name: 'Save' }).click()
    await gate.received
    await screen.getByLabelText('Name', { exact: false }).fill('Renamed meanwhile')
    gate.release()

    await expect.element(screen.getByText('Rule updated')).toBeInTheDocument()
    await expect.element(screen.getByText('Unpublished changes')).toBeInTheDocument()
    await expect
      .element(screen.getByLabelText('Name', { exact: false }))
      .toHaveValue('Renamed meanwhile')
    await expect.element(screen.getByLabelText('Tag')).toHaveValue('saved')
    await expect.element(screen.getByRole('button', { name: 'Publish' })).toBeDisabled()
  },
)

test('Publish and Revert wait while Save is pending', async () => {
  serveCatalogue()
  const gate = gatedRuleServer(EDITED)

  const { screen } = await renderWithRouter(<RuleEditPage />, EDIT_ROUTE)
  await expect.element(screen.getByRole('button', { name: 'Publish' })).toBeEnabled()
  await screen.getByRole('button', { name: 'Save' }).click()
  await gate.received

  await expect.element(screen.getByRole('button', { name: 'Publish' })).toBeDisabled()
  await expect.element(screen.getByRole('button', { name: 'Revert' })).toBeDisabled()
  gate.release()
  await expect.element(screen.getByText('Rule updated')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Publish' })).toBeEnabled()
})

test('Revert over unsaved edits asks first, and keeps them when the person stays', async () => {
  const reverted: string[] = []
  serveCatalogue()
  worker.use(
    handleSiteRulesGet({ body: EDITED }),
    handleSiteRulesRevert(({ params }) => {
      reverted.push(params.id)
      return HttpResponse.json({ ...EDITED, draft: null })
    }),
  )

  const { screen } = await renderWithRouter(<RuleEditPage />, EDIT_ROUTE)
  await screen.getByLabelText('Tag').fill('edited')
  await screen.getByRole('button', { name: 'Revert' }).click()
  const dialog = screen.getByRole('dialog', { name: 'Revert without saving?' })
  await expect.element(dialog.getByText('Your changes are not saved.')).toBeInTheDocument()
  await dialog.getByRole('button', { name: 'Keep editing' }).click()

  await expect.element(screen.getByLabelText('Tag')).toHaveValue('edited')
  expect(reverted).toEqual([])

  await screen.getByRole('button', { name: 'Revert' }).click()
  await screen
    .getByRole('dialog', { name: 'Revert without saving?' })
    .getByRole('button', { name: 'Revert' })
    .click()
  await expect.poll(() => reverted).toEqual(['7'])
  await expect.element(screen.getByLabelText('Tag')).toHaveValue('live')
})
