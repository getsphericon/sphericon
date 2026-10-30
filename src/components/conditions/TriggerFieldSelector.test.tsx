import { expect, test, vi } from 'vitest'

import {
  handleSiteCustomFieldsList,
  handleSiteEventsActions,
} from '../../generated/site/msw.gen.ts'
import type { SiteConditionGroup } from '../../generated/site/types.gen.ts'
import { page } from '../../test/payloads.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import type { ConditionDraft } from './condition.ts'
import { ConditionBuilder } from './ConditionBuilder.tsx'

const withProperty = (field: string): SiteConditionGroup => ({
  kind: 'group',
  combinator: 'and',
  rules: [{ kind: 'rule', field, operator: 'eq', value: 'x' }],
})

function serve() {
  worker.use(
    handleSiteEventsActions({ body: { actions: ['purchase'] } }),
    handleSiteCustomFieldsList({ body: page([], 0) }),
  )
}

test('typing a key beside the Event property choice rewrites the field as trigger:<key>', async () => {
  serve()
  const onChange = vi.fn<(draft: ConditionDraft) => void>()
  const { screen } = await renderWithRouter(
    <ConditionBuilder
      workspaceId="TestWs01"
      initialValue={withProperty('trigger:url')}
      onChange={onChange}
      triggerLeaves
    />,
  )

  await screen.getByLabelText('Event property name').fill('path')

  await expect.poll(() => onChange.mock.calls.length).toBeGreaterThan(0)
  expect(onChange.mock.lastCall?.[0]).toMatchObject({
    rules: [{ kind: 'rule', field: 'trigger:path' }],
  })
})

test('a plain field has no key input', async () => {
  serve()
  const { screen } = await renderWithRouter(
    <ConditionBuilder
      workspaceId="TestWs01"
      initialValue={withProperty('email')}
      onChange={vi.fn<() => void>()}
      triggerLeaves
    />,
  )

  await expect.element(screen.getByRole('button', { name: /Rule/ }).first()).toBeInTheDocument()
  await expect.element(screen.getByLabelText('Event property name')).not.toBeInTheDocument()
})
