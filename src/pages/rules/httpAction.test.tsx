import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteCustomFieldsList,
  handleSiteEventsActions,
  handleSiteEventsList,
  handleSiteRulesGet,
  handleSiteRulesUpdate,
} from '../../generated/site/msw.gen.ts'
import type { SiteRuleResource } from '../../generated/site/types.gen.ts'
import { Route as rulesEditRoute } from '../../routes/workspaces/$workspaceId/rules/$ruleId/edit.tsx'
import { page, TIMESTAMPS } from '../../test/payloads.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { RuleEditPage } from './resource.tsx'

// The server masks an http Action: no URL, header names only, and whether a secret is stored.
const STORED: SiteRuleResource = {
  id: '7',
  name: 'Call Slack',
  status: 'inactive',
  live: null,
  draft: {
    number: 1,
    triggerEvents: ['signed_up'],
    actions: [
      {
        type: 'http',
        id: '01J00000000000000000000001',
        body: '{"text":"hi"}',
        headers: [{ name: 'Authorization' }],
        hasSecret: true,
      },
    ],
  },
  ...TIMESTAMPS,
}

const EDIT_ROUTE = routeMount(rulesEditRoute, { workspaceId: 'TestWs01', ruleId: '7' })

test('editing an http action keeps its id and sends no URL or header value it was not given', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteEventsActions({ body: { actions: ['signed_up'] } }),
    handleSiteCustomFieldsList({ body: page([], 0) }),
    handleSiteEventsList({ body: page([], 0) }),
    handleSiteRulesGet({ body: STORED }),
    handleSiteRulesUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(STORED)
    }),
  )

  const { screen } = await renderWithRouter(<RuleEditPage />, EDIT_ROUTE)

  await expect.element(screen.getByLabelText('Request body')).toHaveValue('{"text":"hi"}')
  await expect.element(screen.getByLabelText('URL', { exact: true })).toHaveValue('')
  await screen.getByLabelText('Request body').fill('{"text":"edited"}')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => bodies).toHaveLength(1)
  expect(bodies[0]).toMatchObject({
    actions: [
      {
        type: 'http',
        id: '01J00000000000000000000001',
        body: '{"text":"edited"}',
        headers: [{ name: 'Authorization' }],
      },
    ],
  })
  expect(JSON.stringify(bodies[0])).not.toMatch(/"(url|secret)":"[^"]/)
})
