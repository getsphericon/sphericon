import type { RuleGroupType } from 'react-querybuilder'
import { expect, test } from 'vitest'

import {
  SiteConditionCombinator,
  SiteConditionEventOperator,
  SiteConditionOperator,
} from '../../generated/site/types.gen.ts'
import type { SiteConditionEventLeaf, SiteConditionGroup } from '../../generated/site/types.gen.ts'
import { emptyGroup, eventCount, eventPresets, toCondition, toDraft, toQuery } from './condition.ts'

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
      { field: 'email', operator: SiteConditionOperator.EQ, value: 42 },
    ],
  }

  const cleaned = toDraft(query)

  expect(cleaned).toEqual({
    kind: 'group',
    combinator: SiteConditionCombinator.AND,
    rules: [{ kind: 'rule', field: 'email', operator: SiteConditionOperator.EQ, value: '' }],
  })
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
