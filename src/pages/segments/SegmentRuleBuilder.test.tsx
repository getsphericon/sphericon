import { HttpResponse } from 'msw'
import { expect, test, vi } from 'vitest'

import {
  handleSiteCustomFieldsList,
  handleSiteEventsActions,
  handleSiteSegmentsPreview,
} from '../../generated/site/msw.gen.ts'
import type { SiteSegmentRuleGroup } from '../../generated/site/types.gen.ts'
import { page, TIMESTAMPS } from '../../test/payloads.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { emptyGroup } from './rules.ts'
import { SegmentRuleBuilder } from './SegmentRuleBuilder.tsx'

function serve(previewBodies: unknown[] = []) {
  worker.use(
    handleSiteEventsActions({ body: { actions: ['purchase'] } }),
    handleSiteCustomFieldsList({
      body: page([
        { id: '1', key: 'plan', name: 'Plan', type: 'string', ...TIMESTAMPS },
        { id: '2', key: 'age', name: 'Age', type: 'number', ...TIMESTAMPS },
      ]),
    }),
    handleSiteSegmentsPreview(async ({ request }) => {
      previewBodies.push(await request.json())
      return HttpResponse.json({ count: 3 })
    }),
  )
}

test('sends the current rule to the audience preview', async () => {
  const previews: unknown[] = []
  serve(previews)
  const value: SiteSegmentRuleGroup = {
    kind: 'group',
    combinator: 'and',
    rules: [{ kind: 'rule', field: 'email', operator: 'contains', value: '@x' }],
  }
  const { screen } = await renderWithRouter(
    <SegmentRuleBuilder workspaceId="TestWs01" value={value} onChange={() => {}} />,
  )

  await screen.getByRole('button', { name: 'Preview audience' }).click()

  await expect.poll(() => previews.length).toBe(1)
  expect(previews).toEqual([{ definition: value }])
  await expect.element(screen.getByText('3 matching contacts')).toBeInTheDocument()
})

test('adding a rule emits the typed rule group', async () => {
  serve()
  const onChange = vi.fn<(group: SiteSegmentRuleGroup) => void>()
  const { screen } = await renderWithRouter(
    <SegmentRuleBuilder workspaceId="TestWs01" value={emptyGroup} onChange={onChange} />,
  )

  await screen.getByRole('button', { name: /Rule/ }).first().click()

  await expect.poll(() => onChange.mock.calls.length).toBeGreaterThan(0)
  expect(onChange.mock.calls[0]?.[0]).toMatchObject({
    kind: 'group',
    combinator: 'and',
    rules: [{ kind: 'rule', field: 'subject_id', operator: 'eq' }],
  })
})
