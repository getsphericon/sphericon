import type { RuleGroupType } from 'react-querybuilder'
import { expect, test } from 'vitest'

import { SiteConditionCombinator, SiteConditionOperator } from '../../generated/site/types.gen.ts'
import type { SiteConditionGroup } from '../../generated/site/types.gen.ts'
import { emptyGroup, toGroup, toQuery } from './condition.ts'

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
  const back = toGroup(query)

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

  const cleaned = toGroup(query)

  expect(cleaned).toEqual({
    kind: 'group',
    combinator: SiteConditionCombinator.AND,
    rules: [{ kind: 'rule', field: 'email', operator: SiteConditionOperator.EQ, value: '' }],
  })
})
