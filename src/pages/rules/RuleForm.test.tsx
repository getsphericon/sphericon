import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'
import { userEvent } from 'vitest/browser'

import {
  handleSiteCustomFieldsList,
  handleSiteEventsActions,
  handleSiteEventsList,
  handleSiteRulesDryRun,
  handleSiteRulesGet,
  handleSiteRulesUpdate,
} from '../../generated/site/msw.gen.ts'
import type { SiteEventResource, SiteRuleResource } from '../../generated/site/types.gen.ts'
import { Route as rulesEditRoute } from '../../routes/workspaces/$workspaceId/rules/$ruleId/edit.tsx'
import { page, TIMESTAMPS } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { RuleEditPage } from './resource.tsx'

const EDIT_ROUTE = routeMount(rulesEditRoute, { workspaceId: 'TestWs01', ruleId: '7' })

const STORED: SiteRuleResource = {
  id: '7',
  name: 'Tag signups',
  status: 'inactive',
  live: null,
  draft: { number: 1, triggerEvents: ['signed_up'], actions: [{ type: 'apply_tag', tag: 'new' }] },
  ...TIMESTAMPS,
}

const SIGNED_UP: SiteEventResource = {
  id: '55',
  subjectId: 's',
  action: 'signed_up',
  createdAt: '2026-01-02T00:00:00Z',
}

const HTTP_ID = '01J00000000000000000000001'

function serve(rule: SiteRuleResource, events: SiteEventResource[] = []) {
  worker.use(
    handleSiteEventsActions({ body: { actions: ['signed_up', 'purchased'] } }),
    handleSiteCustomFieldsList({ body: page([], 0) }),
    handleSiteEventsList({ body: page(events, events.length) }),
    handleSiteRulesGet({ body: rule }),
  )
}

test('a dry run that would not fire says why and lists no actions', async () => {
  serve(STORED, [SIGNED_UP])
  worker.use(handleSiteRulesDryRun({ body: { outcome: 'not_triggered', actions: [] } }))

  const { screen } = await renderWithRouter(<RuleEditPage />, EDIT_ROUTE)
  await screen.getByRole('combobox', { name: 'Event' }).click()
  await screen.getByRole('option', { name: /signed_up/ }).click()
  await screen.getByRole('button', { name: 'Try the rule' }).click()

  await expect
    .element(screen.getByText('Would not fire: the event is not one of the trigger events'))
    .toBeInTheDocument()
  await expect.element(screen.getByRole('listitem')).not.toBeInTheDocument()
})

test('a failed dry run shows the problem', async () => {
  serve(STORED, [SIGNED_UP])
  worker.use(handleSiteRulesDryRun(() => problem(404, { detail: 'event not found' })))

  const { screen } = await renderWithRouter(<RuleEditPage />, EDIT_ROUTE)
  await screen.getByRole('combobox', { name: 'Event' }).click()
  await screen.getByRole('option', { name: /signed_up/ }).click()
  await screen.getByRole('button', { name: 'Try the rule' }).click()

  await expect.element(screen.getByText('Failed to try the rule')).toBeInTheDocument()
  await expect.element(screen.getByText('event not found')).toBeInTheDocument()
})

test('removing the last trigger event is refused before anything is sent', async () => {
  const bodies: unknown[] = []
  serve(STORED)
  worker.use(
    handleSiteRulesUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(STORED)
    }),
  )

  const { screen } = await renderWithRouter(<RuleEditPage />, EDIT_ROUTE)
  await expect.element(screen.getByLabelText('Name', { exact: false })).toHaveValue('Tag signups')
  await screen.getByRole('combobox', { name: 'Trigger events' }).click()
  await userEvent.keyboard('{Backspace}')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.element(screen.getByText('This field is required')).toBeInTheDocument()
  expect(bodies).toEqual([])
})

test('switching an action to a URL call shows its fields and saves the headers kept', async () => {
  const bodies: unknown[] = []
  serve(STORED)
  worker.use(
    handleSiteRulesUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(STORED)
    }),
  )

  const { screen } = await renderWithRouter(<RuleEditPage />, EDIT_ROUTE)
  await expect.element(screen.getByLabelText('Tag')).toHaveValue('new')
  await screen.getByRole('combobox', { name: 'Action' }).click()
  await screen.getByRole('option', { name: 'Call a URL' }).click()

  await screen.getByLabelText('URL', { exact: true }).fill('https://hooks.example.com/a')
  await screen.getByRole('button', { name: 'Add header' }).click()
  await screen.getByLabelText('Header name').fill('X-One')
  await screen.getByLabelText('Header value').fill('1')
  await screen.getByRole('button', { name: 'Add header' }).click()
  await screen.getByLabelText('Header name').nth(1).fill('X-Two')
  await screen.getByRole('button', { name: 'Remove header' }).nth(1).click()
  await screen.getByLabelText('Request body').fill('{"a":1}')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => bodies).toHaveLength(1)
  expect(bodies[0]).toMatchObject({
    actions: [
      {
        type: 'http',
        url: 'https://hooks.example.com/a',
        headers: [{ name: 'X-One', value: '1' }],
        body: '{"a":1}',
      },
    ],
  })
})

test('the add header button disappears at the limit of five headers', async () => {
  serve({
    ...STORED,
    draft: {
      number: 1,
      triggerEvents: ['signed_up'],
      actions: [
        { type: 'http', id: HTTP_ID, headers: ['a', 'b', 'c', 'd', 'e'].map((name) => ({ name })) },
      ],
    },
  })

  const { screen } = await renderWithRouter(<RuleEditPage />, EDIT_ROUTE)

  await expect.element(screen.getByLabelText('Header name').first()).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Add header' })).not.toBeInTheDocument()
})
