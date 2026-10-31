import { HttpResponse } from 'msw'
import { expect, test, vi } from 'vitest'
import { userEvent } from 'vitest/browser'

import {
  handleSiteCustomFieldsList,
  handleSiteEventsActions,
  handleSiteSegmentsPreview,
} from '../../generated/site/msw.gen.ts'
import { SiteConditionOperator } from '../../generated/site/types.gen.ts'
import type { SiteConditionGroup } from '../../generated/site/types.gen.ts'
import { page, TIMESTAMPS } from '../../test/payloads.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { emptyGroup } from './condition.ts'
import type { ConditionDraft } from './condition.ts'
import { ConditionBuilder } from './ConditionBuilder.tsx'

function serve(previewBodies: unknown[] = []) {
  worker.use(
    handleSiteEventsActions({ body: { actions: ['purchase'] } }),
    handleSiteCustomFieldsList({
      body: page([
        {
          id: '1',
          key: 'plan',
          name: 'Plan',
          type: 'string',
          ruleOperators: [
            SiteConditionOperator.EQ,
            SiteConditionOperator.NEQ,
            SiteConditionOperator.CONTAINS,
            SiteConditionOperator.NOT_NULL,
          ],
          ...TIMESTAMPS,
        },
        {
          id: '2',
          key: 'age',
          name: 'Age',
          type: 'number',
          ruleOperators: [
            SiteConditionOperator.EQ,
            SiteConditionOperator.NEQ,
            SiteConditionOperator.NOT_NULL,
          ],
          ...TIMESTAMPS,
        },
      ]),
    }),
    handleSiteSegmentsPreview(async ({ request }) => {
      previewBodies.push(await request.json())
      return HttpResponse.json({ count: 3 })
    }),
  )
}

test('sends the current rule to the segment preview', async () => {
  const previews: unknown[] = []
  serve(previews)
  const value: SiteConditionGroup = {
    kind: 'group',
    combinator: 'and',
    rules: [{ kind: 'rule', field: 'email', operator: 'contains', value: '@x' }],
  }
  const { screen } = await renderWithRouter(
    <ConditionBuilder workspaceId="TestWs01" initialValue={value} onChange={vi.fn<() => void>()} />,
  )

  await screen.getByRole('button', { name: 'Preview segment' }).click()

  await expect.poll(() => previews.length).toBe(1)
  expect(previews).toEqual([{ condition: value }])
  await expect.element(screen.getByText('3 matching contacts')).toBeInTheDocument()
})

test('a rule edits an Event property and has no segment preview', async () => {
  serve()
  const value: SiteConditionGroup = {
    kind: 'group',
    combinator: 'and',
    rules: [{ kind: 'rule', field: 'trigger:url', operator: 'contains', value: 'pricing' }],
  }
  const { screen } = await renderWithRouter(
    <ConditionBuilder
      workspaceId="TestWs01"
      initialValue={value}
      onChange={vi.fn<() => void>()}
      triggerLeaves
    />,
  )

  await expect.element(screen.getByLabelText('Event property name')).toHaveValue('url')
  await expect
    .element(screen.getByRole('button', { name: 'Preview segment' }))
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
    <ConditionBuilder workspaceId="TestWs01" initialValue={value} onChange={vi.fn<() => void>()} />,
  )

  await expect.element(screen.getByRole('button', { name: 'Preview segment' })).toBeInTheDocument()
  await expect.element(screen.getByLabelText('Event property name')).not.toBeInTheDocument()
})

test('adding a rule emits the typed rule group', async () => {
  serve()
  const onChange = vi.fn<(draft: ConditionDraft) => void>()
  const { screen } = await renderWithRouter(
    <ConditionBuilder workspaceId="TestWs01" initialValue={emptyGroup} onChange={onChange} />,
  )

  await screen.getByRole('button', { name: /Rule/ }).first().click()

  await expect.poll(() => onChange.mock.calls.length).toBeGreaterThan(0)
  expect(onChange.mock.calls[0]?.[0]).toMatchObject({
    kind: 'group',
    combinator: 'and',
    rules: [{ kind: 'rule', field: 'subject_id', operator: 'eq' }],
  })
})

const counted: SiteConditionGroup = {
  kind: 'group',
  combinator: 'and',
  rules: [{ kind: 'event', action: 'purchase', operator: 'atLeast', times: 2, withinDays: 7 }],
}

test('an event rule edits its count and its window, and previews what it shows', async () => {
  const previews: unknown[] = []
  serve(previews)
  const { screen } = await renderWithRouter(
    <ConditionBuilder
      workspaceId="TestWs01"
      initialValue={counted}
      onChange={vi.fn<() => void>()}
    />,
  )

  await screen.getByLabelText('Times').fill('3')
  await screen.getByLabelText('In the last (days)').fill('14')
  await screen.getByRole('button', { name: 'Preview segment' }).click()

  await expect.element(screen.getByLabelText('Times')).toHaveValue('3')
  await expect.element(screen.getByLabelText('In the last (days)')).toHaveValue('14')
  await expect
    .poll(() => previews)
    .toEqual([
      {
        condition: {
          ...counted,
          rules: [
            { kind: 'event', action: 'purchase', operator: 'atLeast', times: 3, withinDays: 14 },
          ],
        },
      },
    ])
})

// The operator the user picked stays picked while the count is typed: "at least 1" on the way to
// 15 is not turned into the "performed" preset, and an emptied count stays empty and required.
test('a count is typed digit by digit and can be emptied', async () => {
  serve()
  const { screen } = await renderWithRouter(
    <ConditionBuilder
      workspaceId="TestWs01"
      initialValue={counted}
      onChange={vi.fn<() => void>()}
    />,
  )
  const times = screen.getByLabelText('Times')

  await userEvent.clear(times)
  await userEvent.type(times, '15')

  await expect.element(times).toHaveValue('15')
  await expect.element(screen.getByTestId('operators')).toHaveValue('at least (times)')

  await userEvent.clear(times)

  await expect.element(times).toHaveValue('')
  await expect.element(screen.getByText('This field is required')).toBeInTheDocument()
  await expect.element(screen.getByTestId('operators')).toHaveValue('at least (times)')
  await expect.element(screen.getByRole('button', { name: 'Preview segment' })).toBeDisabled()
})

test('a loaded count of at least once shows as the performed preset, with only its window', async () => {
  serve()
  const value: SiteConditionGroup = {
    kind: 'group',
    combinator: 'and',
    rules: [{ kind: 'event', action: 'purchase', operator: 'atLeast', times: 1 }],
  }
  const { screen } = await renderWithRouter(
    <ConditionBuilder workspaceId="TestWs01" initialValue={value} onChange={vi.fn<() => void>()} />,
  )

  await expect.element(screen.getByTestId('operators')).toHaveValue('performed')
  await expect.element(screen.getByLabelText('In the last (days)')).toHaveValue('')
  await expect.element(screen.getByLabelText('Times')).not.toBeInTheDocument()
})

// A number field offers the comparisons beside the operators the server lists for it, and takes a
// number: the loaded comparison shows its operator and its number.
test('a number custom field shows a comparison with its number', async () => {
  serve()
  const value: SiteConditionGroup = {
    kind: 'group',
    combinator: 'and',
    rules: [{ kind: 'number', field: 'custom:age', operator: 'gte', value: 30 }],
  }
  const { screen } = await renderWithRouter(
    <ConditionBuilder workspaceId="TestWs01" initialValue={value} onChange={vi.fn<() => void>()} />,
  )

  await expect.element(screen.getByTestId('operators')).toHaveValue('greater than or equal to')
  await expect.element(screen.getByTestId('value-editor')).toHaveValue('30')
})
