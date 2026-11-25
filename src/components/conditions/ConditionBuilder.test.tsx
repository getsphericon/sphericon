import { HttpResponse } from 'msw'
import { expect, test, vi } from 'vitest'
import { userEvent } from 'vitest/browser'

import {
  handleSiteAutomationsList,
  handleSiteBroadcastsList,
  handleSiteCustomFieldsList,
  handleSiteEventsActions,
  handleSiteEventsPropertyKeys,
  handleSiteSegmentsPreview,
} from '../../generated/site/msw.gen.ts'
import {
  MomentOperator,
  NumberOperator,
  PresenceOperator,
  TextOperator,
} from '../../generated/site/types.gen.ts'
import type { RuleCondition, SegmentCondition } from '../../generated/site/types.gen.ts'
import { page, TIMESTAMPS, testId } from '../../test/payloads.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { emptyGroup } from './condition.ts'
import type { ConditionDraft } from './condition.ts'
import { ConditionBuilder } from './ConditionBuilder.tsx'

// The Custom fields of each type, with what the server lists for the type.
function serve(previewBodies: unknown[] = []) {
  worker.use(
    handleSiteEventsActions({
      body: { actions: ['purchase'], eventsKeptDays: 400, messageActions: [], keptActions: [] },
    }),
    handleSiteCustomFieldsList({
      body: page([
        {
          id: testId(1),
          key: 'plan',
          name: 'Plan',
          type: 'string',
          conditionTests: {
            text: [TextOperator.EQ, TextOperator.NEQ, TextOperator.CONTAINS],
            number: [],
            moment: [],
            withinLast: false,
            presence: [PresenceOperator.SET],
          },
          revision: 1,
          ...TIMESTAMPS,
        },
        {
          id: testId(2),
          key: 'age',
          name: 'Age',
          type: 'number',
          conditionTests: {
            text: [TextOperator.EQ, TextOperator.NEQ],
            number: [NumberOperator.GT, NumberOperator.GTE, NumberOperator.LT, NumberOperator.LTE],
            moment: [],
            withinLast: false,
            presence: [PresenceOperator.SET],
          },
          revision: 1,
          ...TIMESTAMPS,
        },
        {
          id: testId(3),
          key: 'renews_at',
          name: 'Renews at',
          type: 'datetime',
          conditionTests: {
            text: [],
            number: [],
            moment: [MomentOperator.BEFORE, MomentOperator.AFTER],
            withinLast: true,
            presence: [PresenceOperator.SET],
          },
          revision: 1,
          ...TIMESTAMPS,
        },
      ]),
    }),
    handleSiteSegmentsPreview(async ({ request }) => {
      previewBodies.push(await request.json())
      return HttpResponse.json({ count: 3, unknownEngagement: 2 })
    }),
  )
}

const emailContains: SegmentCondition = {
  kind: 'group',
  combinator: 'and',
  entries: [
    {
      kind: 'attribute',
      attribute: 'email',
      test: { kind: 'text', operator: 'contains', value: '@x' },
    },
  ],
}

test('sends the current Condition to the segment preview', async () => {
  const previews: unknown[] = []
  serve(previews)
  const { screen } = await renderWithRouter(
    <ConditionBuilder
      workspaceId="TestWs01"
      place="segment"
      initialValue={emailContains}
      onChange={vi.fn<() => void>()}
    />,
  )

  await screen.getByRole('button', { name: 'Preview segment' }).click()

  await expect.poll(() => previews.length).toBe(1)
  expect(previews).toEqual([{ condition: emailContains }])
  await expect.element(screen.getByText('3 matching contacts')).toBeInTheDocument()
  await expect
    .element(screen.getByText('2 contacts left out: their opens and clicks are not tracked'))
    .toBeInTheDocument()
})

test("a Segment's builder offers no test of the triggering Event", async () => {
  serve()
  const { screen } = await renderWithRouter(
    <ConditionBuilder
      workspaceId="TestWs01"
      place="segment"
      initialValue={emailContains}
      onChange={vi.fn<() => void>()}
    />,
  )

  await screen.getByTestId('fields').click()

  await expect.element(screen.getByRole('option', { name: 'Plan', exact: true })).toBeVisible()
  await expect
    .element(screen.getByRole('option', { name: 'Event property' }))
    .not.toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Preview segment' })).toBeInTheDocument()
})

// An event field reads as its action: a System event by its words, a tracked action by its name.
test('an event field names a System event by its words and a tracked action as it was tracked', async () => {
  serve()
  worker.use(
    handleSiteEventsActions({
      body: {
        actions: ['purchase', 'email.opened'],
        eventsKeptDays: 400,
        messageActions: [],
        keptActions: [],
      },
    }),
  )
  const { screen } = await renderWithRouter(
    <ConditionBuilder
      workspaceId="TestWs01"
      place="segment"
      initialValue={emailContains}
      onChange={vi.fn<() => void>()}
    />,
  )

  await screen.getByTestId('fields').click()

  await expect.element(screen.getByRole('option', { name: 'Opened an email' })).toBeVisible()
  await expect.element(screen.getByRole('option', { name: 'Did "purchase"' })).toBeVisible()
})

test("a Rule's builder offers the test of the triggering Event, with its key, and no segment preview", async () => {
  serve()
  const onChange = vi.fn<(draft: ConditionDraft) => void>()
  const { screen } = await renderWithRouter(
    <ConditionBuilder
      workspaceId="TestWs01"
      place="rule"
      initialValue={emptyGroup}
      onChange={onChange}
    />,
  )

  await screen.getByRole('button', { name: /Rule/ }).first().click()
  await screen.getByTestId('fields').click()
  await screen.getByRole('option', { name: 'Event property' }).click()
  await screen.getByLabelText('Event property name').fill('url')
  await screen.getByTestId('value-editor').fill('/pricing')

  await expect
    .poll(() => onChange.mock.lastCall?.[0])
    .toEqual({
      kind: 'group',
      combinator: 'and',
      entries: [
        {
          kind: 'property',
          of: { kind: 'trigger' },
          key: 'url',
          test: { kind: 'text', operator: 'eq', value: '/pricing' },
        },
      ],
    })
  await expect
    .element(screen.getByRole('button', { name: 'Preview segment' }))
    .not.toBeInTheDocument()
})

test("a property's presence test keeps its key and asks for no value", async () => {
  serve()
  const onChange = vi.fn<(draft: ConditionDraft) => void>()
  const value: RuleCondition = {
    kind: 'group',
    combinator: 'and',
    entries: [
      {
        kind: 'property',
        of: { kind: 'trigger' },
        key: 'coupon',
        test: { kind: 'text', operator: 'eq', value: 'x' },
      },
    ],
  }
  const { screen } = await renderWithRouter(
    <ConditionBuilder
      workspaceId="TestWs01"
      place="rule"
      initialValue={value}
      onChange={onChange}
    />,
  )

  await screen.getByTestId('operators').click()
  await screen.getByRole('option', { name: 'is set' }).click()

  await expect.element(screen.getByLabelText('Event property name')).toHaveValue('coupon')
  await expect.element(screen.getByTestId('value-editor')).not.toBeInTheDocument()
  await expect
    .poll(() => onChange.mock.lastCall?.[0].entries)
    .toEqual([
      {
        kind: 'property',
        of: { kind: 'trigger' },
        key: 'coupon',
        test: { kind: 'presence', operator: 'set' },
      },
    ])
})

test('adding a rule emits the typed Condition', async () => {
  serve()
  const onChange = vi.fn<(draft: ConditionDraft) => void>()
  const { screen } = await renderWithRouter(
    <ConditionBuilder
      workspaceId="TestWs01"
      place="segment"
      initialValue={emptyGroup}
      onChange={onChange}
    />,
  )

  await screen.getByRole('button', { name: /Rule/ }).first().click()

  await expect.poll(() => onChange.mock.calls.length).toBeGreaterThan(0)
  expect(onChange.mock.calls[0]?.[0]).toEqual({
    kind: 'group',
    combinator: 'and',
    entries: [
      {
        kind: 'attribute',
        attribute: 'email',
        test: { kind: 'text', operator: 'eq', value: '' },
      },
    ],
  })
})

const counted: SegmentCondition = {
  kind: 'group',
  combinator: 'and',
  entries: [{ kind: 'event', action: 'purchase', operator: 'atLeast', times: 2, withinDays: 7 }],
}

test('an event rule edits its count and its window, and previews what it shows', async () => {
  const previews: unknown[] = []
  serve(previews)
  const { screen } = await renderWithRouter(
    <ConditionBuilder
      workspaceId="TestWs01"
      place="segment"
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
          entries: [
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
      place="segment"
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
  const value: SegmentCondition = {
    kind: 'group',
    combinator: 'and',
    entries: [{ kind: 'event', action: 'purchase', operator: 'atLeast', times: 1 }],
  }
  const { screen } = await renderWithRouter(
    <ConditionBuilder
      workspaceId="TestWs01"
      place="segment"
      initialValue={value}
      onChange={vi.fn<() => void>()}
    />,
  )

  await expect.element(screen.getByTestId('operators')).toHaveValue('performed')
  await expect.element(screen.getByLabelText('In the last (days)')).toHaveValue('')
  await expect.element(screen.getByLabelText('Times')).not.toBeInTheDocument()
})

const customFieldTests: SegmentCondition = {
  kind: 'group',
  combinator: 'and',
  entries: [
    { kind: 'customField', key: 'age', test: { kind: 'number', operator: 'gte', value: 30 } },
    { kind: 'customField', key: 'plan', test: { kind: 'text', operator: 'eq', value: 'pro' } },
    { kind: 'customField', key: 'renews_at', test: { kind: 'presence', operator: 'set' } },
  ],
}

async function operatorOptions(row: number) {
  const { screen } = await renderWithRouter(
    <ConditionBuilder
      workspaceId="TestWs01"
      place="segment"
      initialValue={customFieldTests}
      onChange={vi.fn<() => void>()}
    />,
  )
  await screen.getByTestId('operators').nth(row).click()
  const options = screen.getByRole('option')
  await expect.element(options.first()).toBeVisible()
  return options.elements().map((option) => option.textContent)
}

test('a number Custom field shows a comparison with its number', async () => {
  serve()
  const { screen } = await renderWithRouter(
    <ConditionBuilder
      workspaceId="TestWs01"
      place="segment"
      initialValue={customFieldTests}
      onChange={vi.fn<() => void>()}
    />,
  )

  await expect
    .element(screen.getByTestId('operators').first())
    .toHaveValue('greater than or equal to')
  await expect.element(screen.getByTestId('value-editor').first()).toHaveValue('30')
})

test('a number Custom field offers text is and is not, the number comparisons and presence', async () => {
  serve()

  const offered = await operatorOptions(0)

  expect(offered).toEqual([
    'is',
    'is not',
    'greater than',
    'greater than or equal to',
    'less than',
    'less than or equal to',
    'is set',
  ])
})

test('a string Custom field offers the text operators it takes and presence', async () => {
  serve()

  const offered = await operatorOptions(1)

  expect(offered).toEqual(['is', 'is not', 'contains', 'is set'])
})

test('a datetime Custom field offers the moment operators, the window and presence', async () => {
  serve()

  const offered = await operatorOptions(2)

  expect(offered).toEqual(['is before', 'is after', 'within the last (days)', 'is set'])
})

test('the Telegram field offers has and does not have, and round-trips with no value', async () => {
  serve()
  const onChange = vi.fn<(draft: ConditionDraft) => void>()
  const value: SegmentCondition = {
    kind: 'group',
    combinator: 'and',
    entries: [{ kind: 'telegram', operator: 'has' }],
  }
  const { screen } = await renderWithRouter(
    <ConditionBuilder
      workspaceId="TestWs01"
      place="segment"
      initialValue={value}
      onChange={onChange}
    />,
  )

  await expect.element(screen.getByTestId('fields')).toHaveValue('Telegram')
  await expect.element(screen.getByTestId('operators')).toHaveValue('has')
  await expect.element(screen.getByTestId('value-editor')).not.toBeInTheDocument()

  await screen.getByTestId('operators').click()
  await expect.element(screen.getByRole('option', { name: 'has', exact: true })).toBeVisible()
  await screen.getByRole('option', { name: 'does not have' }).click()

  await expect
    .poll(() => onChange.mock.lastCall?.[0].entries)
    .toEqual([{ kind: 'telegram', operator: 'doesNotHave' }])
})

test('a rule picks the SMS field: has, no value, and does not have round-trips', async () => {
  serve()
  const onChange = vi.fn<(draft: ConditionDraft) => void>()
  const { screen } = await renderWithRouter(
    <ConditionBuilder
      workspaceId="TestWs01"
      place="segment"
      initialValue={emptyGroup}
      onChange={onChange}
    />,
  )

  await screen.getByRole('button', { name: /Rule/ }).first().click()
  await screen.getByTestId('fields').click()
  await screen.getByRole('option', { name: 'SMS', exact: true }).click()

  await expect.element(screen.getByTestId('fields')).toHaveValue('SMS')
  await expect.element(screen.getByTestId('operators')).toHaveValue('has')
  await expect.element(screen.getByTestId('value-editor')).not.toBeInTheDocument()
  await expect
    .poll(() => onChange.mock.lastCall?.[0].entries)
    .toEqual([{ kind: 'sms', operator: 'has' }])

  await screen.getByTestId('operators').click()
  await screen.getByRole('option', { name: 'does not have' }).click()

  await expect
    .poll(() => onChange.mock.lastCall?.[0].entries)
    .toEqual([{ kind: 'sms', operator: 'doesNotHave' }])
})

test('a rule picks a tracking consent field: is a state picked from the list, or is not set', async () => {
  serve()
  const onChange = vi.fn<(draft: ConditionDraft) => void>()
  const { screen } = await renderWithRouter(
    <ConditionBuilder
      workspaceId="TestWs01"
      place="segment"
      initialValue={emptyGroup}
      onChange={onChange}
    />,
  )

  await screen.getByRole('button', { name: /Rule/ }).first().click()
  await screen.getByTestId('fields').click()
  await screen.getByRole('option', { name: 'Email tracking consent' }).click()
  await screen.getByTestId('value-editor').click()
  await screen.getByRole('option', { name: 'denied' }).click()

  await expect
    .poll(() => onChange.mock.lastCall?.[0].entries)
    .toEqual([
      {
        kind: 'engagementTracking',
        channel: 'email',
        test: { kind: 'text', operator: 'eq', value: 'denied' },
      },
    ])

  await screen.getByTestId('operators').click()
  await expect.element(screen.getByRole('option', { name: 'contains' })).not.toBeInTheDocument()
  await screen.getByRole('option', { name: 'is not set' }).click()

  await expect.element(screen.getByTestId('value-editor')).not.toBeInTheDocument()
  await expect
    .poll(() => onChange.mock.lastCall?.[0].entries)
    .toEqual([
      {
        kind: 'engagementTracking',
        channel: 'email',
        test: { kind: 'presence', operator: 'notSet' },
      },
    ])
})

test('a loaded Tag shows its name and is edited as the Tag it tests', async () => {
  serve()
  const onChange = vi.fn<(draft: ConditionDraft) => void>()
  const value: SegmentCondition = {
    kind: 'group',
    combinator: 'and',
    entries: [{ kind: 'tag', name: 'vip', operator: 'has' }],
  }
  const { screen } = await renderWithRouter(
    <ConditionBuilder
      workspaceId="TestWs01"
      place="segment"
      initialValue={value}
      onChange={onChange}
    />,
  )

  await expect.element(screen.getByTestId('fields')).toHaveValue('Tag')
  await expect.element(screen.getByTestId('value-editor')).toHaveValue('vip')
  await screen.getByTestId('value-editor').fill('churned')

  await expect
    .poll(() => onChange.mock.lastCall?.[0].entries)
    .toEqual([{ kind: 'tag', name: 'churned', operator: 'has' }])
})

// An event test of a click with a filter that names an archived Broadcast and tests the link, over
// all time: it counts only the Events kept (ADR 0153).
const clickedLastMonth: SegmentCondition = {
  kind: 'group',
  combinator: 'and',
  entries: [
    {
      kind: 'event',
      action: 'email.clicked',
      operator: 'atLeast',
      times: 2,
      filter: {
        kind: 'group',
        combinator: 'and',
        entries: [
          { kind: 'sentBy', sender: 'broadcast', id: testId(9) },
          {
            kind: 'property',
            key: 'url',
            test: { kind: 'text', operator: 'contains', value: '/pricing' },
          },
        ],
      },
    },
  ],
}

// What the filter of a click offers: the keys of recent clicks, the kept Broadcasts and, asked
// for, the archived ones.
function serveClicks(previewBodies: unknown[]) {
  worker.use(
    handleSiteEventsActions({
      body: {
        actions: ['email.clicked'],
        eventsKeptDays: 400,
        messageActions: ['email.clicked'],
        keptActions: [],
      },
    }),
    handleSiteCustomFieldsList({ body: page([]) }),
    handleSiteEventsPropertyKeys({ body: { keys: ['url'] } }),
    handleSiteBroadcastsList(({ request }) => {
      const archived = new URL(request.url).searchParams.get('archived') === 'true'
      const items = archived
        ? [{ id: testId(9), name: 'Last month', archivedAt: TIMESTAMPS.createdAt }]
        : [{ id: testId(8), name: 'Newsletter', archivedAt: null }]
      return HttpResponse.json({ items, page: 1, pageSize: 100, totalItems: 1, totalPages: 1 })
    }),
    handleSiteAutomationsList(() =>
      HttpResponse.json({ items: [], page: 1, pageSize: 100, totalItems: 0, totalPages: 0 }),
    ),
    handleSiteSegmentsPreview(async ({ request }) => {
      previewBodies.push(await request.json())
      return HttpResponse.json({ count: 1, unknownEngagement: 0, eventsKeptDays: 400 })
    }),
  )
}

test("an event test's filter opens in its own builder, offering the senders of the message", async () => {
  const previews: unknown[] = []
  serveClicks(previews)
  const { screen } = await renderWithRouter(
    <ConditionBuilder
      workspaceId="TestWs01"
      place="segment"
      initialValue={clickedLastMonth}
      onChange={vi.fn<() => void>()}
    />,
  )

  await expect
    .element(screen.getByText('Events of the last 400 days are kept').first())
    .toBeInTheDocument()
  await screen.getByRole('button', { name: 'Filter (2 tests)' }).click()

  const archived = screen.getByLabelText('Offer archived broadcasts and automations')
  await expect.element(archived).toBeChecked()
  const sender = screen.getByRole('dialog').getByTestId('value-editor').first()
  await expect.element(sender).toHaveValue('Last month (archived)')
  await screen.getByRole('button', { name: 'Done' }).click()
  await screen.getByRole('button', { name: 'Preview segment' }).click()

  await expect.poll(() => previews).toEqual([{ condition: clickedLastMonth }])
  await expect.element(screen.getByText('1 matching contact')).toBeInTheDocument()
})

test("a Branch path's event test opens its filter's builder as a Segment's does", async () => {
  serveClicks([])
  const { screen } = await renderWithRouter(
    <ConditionBuilder
      workspaceId="TestWs01"
      place="branch"
      initialValue={clickedLastMonth}
      onChange={vi.fn<() => void>()}
    />,
  )

  await screen.getByRole('button', { name: 'Filter (2 tests)' }).click()

  const sender = screen.getByRole('dialog').getByTestId('value-editor').first()
  await expect.element(sender).toHaveValue('Last month (archived)')
})
