import { HttpResponse } from 'msw'
import { expect, test, vi } from 'vitest'

import {
  handleSiteCustomFieldsList,
  handleSiteEventsActions,
  handleSiteSegmentsPreview,
} from '../../generated/site/msw.gen.ts'
import type { SiteConditionGroup } from '../../generated/site/types.gen.ts'
import { page, TIMESTAMPS } from '../../test/payloads.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { emptyGroup } from './condition.ts'
import { ConditionBuilder } from './ConditionBuilder.tsx'

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
  const value: SiteConditionGroup = {
    kind: 'group',
    combinator: 'and',
    rules: [{ kind: 'rule', field: 'email', operator: 'contains', value: '@x' }],
  }
  const { screen } = await renderWithRouter(
    <ConditionBuilder workspaceId="TestWs01" value={value} onChange={vi.fn<() => void>()} />,
  )

  await screen.getByRole('button', { name: 'Preview audience' }).click()

  await expect.poll(() => previews.length).toBe(1)
  expect(previews).toEqual([{ condition: value }])
  await expect.element(screen.getByText('3 matching contacts')).toBeInTheDocument()
})

test('a rule edits an Event property and has no audience preview', async () => {
  serve()
  const value: SiteConditionGroup = {
    kind: 'group',
    combinator: 'and',
    rules: [{ kind: 'rule', field: 'trigger:url', operator: 'contains', value: 'pricing' }],
  }
  const { screen } = await renderWithRouter(
    <ConditionBuilder
      workspaceId="TestWs01"
      value={value}
      onChange={vi.fn<() => void>()}
      triggerLeaves
    />,
  )

  await expect.element(screen.getByLabelText('Event property name')).toHaveValue('url')
  await expect
    .element(screen.getByRole('button', { name: 'Preview audience' }))
    .not.toBeInTheDocument()
})

test('a segment offers no Event property', async () => {
  serve()
  const value: SiteConditionGroup = {
    kind: 'group',
    combinator: 'and',
    rules: [{ kind: 'rule', field: 'email', operator: 'contains', value: '@x' }],
  }
  const { screen } = await renderWithRouter(
    <ConditionBuilder workspaceId="TestWs01" value={value} onChange={vi.fn<() => void>()} />,
  )

  await expect.element(screen.getByRole('button', { name: 'Preview audience' })).toBeInTheDocument()
  await expect.element(screen.getByLabelText('Event property name')).not.toBeInTheDocument()
})

test('adding a rule emits the typed rule group', async () => {
  serve()
  const onChange = vi.fn<(group: SiteConditionGroup) => void>()
  const { screen } = await renderWithRouter(
    <ConditionBuilder workspaceId="TestWs01" value={emptyGroup} onChange={onChange} />,
  )

  await screen.getByRole('button', { name: /Rule/ }).first().click()

  await expect.poll(() => onChange.mock.calls.length).toBeGreaterThan(0)
  expect(onChange.mock.calls[0]?.[0]).toMatchObject({
    kind: 'group',
    combinator: 'and',
    rules: [{ kind: 'rule', field: 'subject_id', operator: 'eq' }],
  })
})
