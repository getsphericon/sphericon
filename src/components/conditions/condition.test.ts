import type { RuleGroupType } from 'react-querybuilder'
import { expect, test } from 'vitest'

import {
  SiteConditionCombinator,
  SiteConditionDateOperator,
  SiteConditionEventOperator,
  SiteConditionNumberOperator,
  SiteConditionOperator,
} from '../../generated/site/types.gen.ts'
import type { SiteConditionEventLeaf, SiteConditionGroup } from '../../generated/site/types.gen.ts'
import {
  dateWithinOperator,
  emptyGroup,
  eventCount,
  eventPresets,
  toCondition,
  toDraft,
  toQuery,
} from './condition.ts'

const group: SiteConditionGroup = {
  kind: 'group',
  combinator: SiteConditionCombinator.OR,
  not: true,
  rules: [
    { kind: 'rule', field: 'email', operator: SiteConditionOperator.CONTAINS, value: 'acme' },
    { kind: 'rule', field: 'plan', operator: SiteConditionOperator.NULL },
    {
      kind: 'group',
      combinator: SiteConditionCombinator.AND,
      rules: [{ kind: 'rule', field: 'name', operator: SiteConditionOperator.EQ, value: 'Ada' }],
    },
  ],
}

test('a rule group survives the walk to the query builder and back', () => {
  const query = toQuery(group)
  const back = toDraft(query)

  expect(back).toEqual({
    ...group,
    rules: [
      group.rules[0],
      { kind: 'rule', field: 'plan', operator: SiteConditionOperator.NULL, value: '' },
      group.rules[2],
    ],
  })
})

test('the empty group becomes an empty query without a negation', () => {
  const query = toQuery(emptyGroup)

  expect(query).toEqual({ combinator: SiteConditionCombinator.AND, rules: [] })
})

test('the query builder tree is cleaned: an unknown operator is dropped, a bad combinator falls back to and', () => {
  const query: RuleGroupType = {
    combinator: 'xor',
    rules: [
      { field: 'email', operator: 'regex', value: '.*' },
      { field: 'email', operator: SiteConditionOperator.EQ, value: { a: 1 } },
    ],
  }

  const cleaned = toDraft(query)

  expect(cleaned).toEqual({
    kind: 'group',
    combinator: SiteConditionCombinator.AND,
    rules: [{ kind: 'rule', field: 'email', operator: SiteConditionOperator.EQ, value: '' }],
  })
})

test("a number typed into a number field is the rule leaf's text", () => {
  const typed = toDraft({
    combinator: SiteConditionCombinator.AND,
    rules: [{ field: 'custom:seats', operator: SiteConditionOperator.EQ, value: 12 }],
  })

  expect(typed.rules).toEqual([
    { kind: 'rule', field: 'custom:seats', operator: SiteConditionOperator.EQ, value: '12' },
  ])
})

const one = (leaf: SiteConditionEventLeaf): SiteConditionGroup => ({
  kind: 'group',
  combinator: SiteConditionCombinator.AND,
  rules: [leaf],
})

const ruleOf = (operator: string, value: unknown): RuleGroupType => ({
  combinator: SiteConditionCombinator.AND,
  rules: [{ field: 'event:purchase', operator, value }],
})

test('an event count survives the walk to the query builder and back', () => {
  const counted = one({
    kind: 'event',
    action: 'purchase',
    operator: SiteConditionEventOperator.AT_LEAST,
    times: 3,
    withinDays: 7,
  })

  const query = toQuery(counted)
  const back = toDraft(query)

  expect(query.rules).toEqual([
    { field: 'event:purchase', operator: 'atLeast', value: { times: 3, withinDays: 7 } },
  ])
  expect(back).toEqual(counted)
})

test('at least once and at most zero times show as the presets and come back as counts', () => {
  const performed = one({
    kind: 'event',
    action: 'purchase',
    operator: SiteConditionEventOperator.AT_LEAST,
    times: 1,
  })
  const notPerformed = one({
    kind: 'event',
    action: 'purchase',
    operator: SiteConditionEventOperator.AT_MOST,
    times: 0,
    withinDays: 30,
  })

  const performedQuery = toQuery(performed)
  const notPerformedQuery = toQuery(notPerformed)

  expect(performedQuery.rules).toEqual([
    { field: 'event:purchase', operator: eventPresets.performed, value: {} },
  ])
  expect(notPerformedQuery.rules).toEqual([
    { field: 'event:purchase', operator: eventPresets.notPerformed, value: { withinDays: 30 } },
  ])
  expect(toDraft(performedQuery)).toEqual(performed)
  expect(toDraft(notPerformedQuery)).toEqual(notPerformed)
})

test('a count operator picked without a count keeps it blank, and the draft is no Condition yet', () => {
  const atLeast = toDraft(ruleOf(SiteConditionEventOperator.AT_LEAST, { withinDays: 7 }))
  const atMost = toDraft(ruleOf(SiteConditionEventOperator.AT_MOST, ''))
  const typed = toDraft(ruleOf(SiteConditionEventOperator.AT_LEAST, { times: 1 }))

  expect(atLeast.rules).toEqual([
    { kind: 'event', action: 'purchase', operator: 'atLeast', withinDays: 7 },
  ])
  expect(atMost.rules).toEqual([{ kind: 'event', action: 'purchase', operator: 'atMost' }])
  expect(toCondition(atLeast)).toBeUndefined()
  expect(toCondition(atMost)).toBeUndefined()
  expect(toCondition(typed)).toEqual(
    one({ kind: 'event', action: 'purchase', operator: 'atLeast', times: 1 }),
  )
})

test('an event rule with a text operator is dropped, and a value out of range is blank', () => {
  const textOperator = toDraft(ruleOf(SiteConditionOperator.EQ, { times: 2 }))
  const negative = toDraft(ruleOf(SiteConditionEventOperator.AT_MOST, { times: -1 }))

  expect(textOperator.rules).toEqual([])
  expect(negative.rules).toEqual([{ kind: 'event', action: 'purchase', operator: 'atMost' }])
  expect(eventCount({ times: 2, withinDays: 0 })).toEqual({})
})

const compared = (operator: string, value: unknown): RuleGroupType => ({
  combinator: SiteConditionCombinator.AND,
  rules: [{ field: 'custom:seats', operator, value }],
})

test('a number comparison survives the walk to the query builder and back', () => {
  const atLeastTen: SiteConditionGroup = {
    kind: 'group',
    combinator: SiteConditionCombinator.AND,
    rules: [
      {
        kind: 'number',
        field: 'custom:seats',
        operator: SiteConditionNumberOperator.GTE,
        value: 10,
      },
    ],
  }

  const query = toQuery(atLeastTen)

  expect(query.rules).toEqual([{ field: 'custom:seats', operator: 'gte', value: 10 }])
  expect(toDraft(query)).toEqual(atLeastTen)
})

test('a number typed as text counts, and a blank one keeps the draft from being a Condition', () => {
  const typed = toDraft(compared(SiteConditionNumberOperator.LT, '2.5'))
  const blank = toDraft(compared(SiteConditionNumberOperator.LT, ''))

  expect(typed.rules).toEqual([
    { kind: 'number', field: 'custom:seats', operator: 'lt', value: 2.5 },
  ])
  expect(blank.rules).toEqual([{ kind: 'number', field: 'custom:seats', operator: 'lt' }])
  expect(toCondition(blank)).toBeUndefined()
})

// Paris is an hour ahead of UTC in March: a day there starts at 23:00 UTC the evening before.
const paris = 'Europe/Paris'

const dated = (operator: SiteConditionDateOperator, at: string): SiteConditionGroup => ({
  kind: 'group',
  combinator: SiteConditionCombinator.AND,
  rules: [{ kind: 'date', field: 'custom:renews_at', operator, at }],
})

const picked = (operator: string, day: string): RuleGroupType => ({
  combinator: SiteConditionCombinator.AND,
  rules: [{ field: 'custom:renews_at', operator, value: day }],
})

test('before a day is before its start and after a day is from the next day, in the User zone', () => {
  const before = toDraft(picked(SiteConditionDateOperator.BEFORE, '2026-03-01'), paris)
  const after = toDraft(picked(SiteConditionDateOperator.AFTER, '2026-03-01'), paris)

  expect(toCondition(before)).toEqual(
    dated(SiteConditionDateOperator.BEFORE, '2026-02-28T23:00:00.000Z'),
  )
  expect(toCondition(after)).toEqual(
    dated(SiteConditionDateOperator.AFTER, '2026-03-01T23:00:00.000Z'),
  )
})

test('a loaded before or after shows the day it bounds, in the User zone', () => {
  const before = toQuery(dated(SiteConditionDateOperator.BEFORE, '2026-02-28T23:00:00Z'), paris)
  const after = toQuery(dated(SiteConditionDateOperator.AFTER, '2026-03-01T23:00:00Z'), paris)
  const afterInUtc = toQuery(dated(SiteConditionDateOperator.AFTER, '2026-03-01T23:00:00Z'), 'UTC')

  expect(before.rules).toEqual([
    { field: 'custom:renews_at', operator: 'before', value: '2026-03-01' },
  ])
  expect(after.rules).toEqual([
    { field: 'custom:renews_at', operator: 'after', value: '2026-03-01' },
  ])
  expect(afterInUtc.rules).toEqual([
    { field: 'custom:renews_at', operator: 'after', value: '2026-03-01' },
  ])
})

test('within takes a number of days, and a date without a day picked is blank', () => {
  const within = toDraft(compared(dateWithinOperator, 7))
  const noDay = toDraft(compared(SiteConditionDateOperator.BEFORE, ''))

  expect(within.rules).toEqual([{ kind: 'dateWithin', field: 'custom:seats', withinDays: 7 }])
  expect(
    toQuery({
      ...emptyGroup,
      rules: [{ kind: 'dateWithin', field: 'custom:seats', withinDays: 7 }],
    }).rules,
  ).toEqual([{ field: 'custom:seats', operator: dateWithinOperator, value: 7 }])
  expect(noDay.rules).toEqual([{ kind: 'date', field: 'custom:seats', operator: 'before' }])
  expect(toCondition(noDay)).toBeUndefined()
})
