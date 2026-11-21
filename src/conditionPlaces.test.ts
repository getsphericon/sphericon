// SPIKE (#1674): what each place's generated Condition type and schema admit.
import { expect, test } from 'vitest'

import type { RuleConditionInput, SegmentConditionInput } from './generated/site/types.gen.ts'
import { zRuleConditionInput, zSegmentConditionInput } from './generated/site/zod.gen.ts'

const ruleOnlyLeaf = {
  kind: 'property',
  of: { kind: 'trigger' },
  key: 'plan',
  test: { kind: 'text', operator: 'eq', value: 'pro' },
} as const

const textOperatorOnANumber = {
  kind: 'customField',
  key: 'seats',
  test: { kind: 'number', operator: 'contains', value: 2 },
} as const

test("a Segment's Condition type takes neither a Rule-only leaf nor a text operator on a number", () => {
  const segment: SegmentConditionInput = {
    kind: 'group',
    combinator: 'and',
    entries: [
      { kind: 'customField', key: 'seats', test: { kind: 'number', operator: 'gte', value: 2 } },
      { kind: 'group', combinator: 'or', entries: [{ kind: 'tag', name: 'vip', operator: 'has' }] },
    ],
  }
  const rule: RuleConditionInput = { kind: 'group', combinator: 'and', entries: [ruleOnlyLeaf] }
  // @ts-expect-error a Segment admits no property test of the triggering Event
  const misplaced: SegmentConditionInput = {
    kind: 'group',
    combinator: 'and',
    entries: [ruleOnlyLeaf],
  }
  const nested: SegmentConditionInput = {
    kind: 'group',
    combinator: 'and',
    // @ts-expect-error not at any depth either
    entries: [{ kind: 'group', combinator: 'or', entries: [ruleOnlyLeaf] }],
  }
  // @ts-expect-error contains is a text operator, not a number test's
  const mistyped: SegmentConditionInput = {
    kind: 'group',
    combinator: 'and',
    entries: [textOperatorOnANumber],
  }

  expect(zSegmentConditionInput.safeParse(segment).success).toBe(true)
  expect(zRuleConditionInput.safeParse(rule).success).toBe(true)
  expect(zSegmentConditionInput.safeParse(misplaced).success).toBe(false)
  expect(zSegmentConditionInput.safeParse(nested).success).toBe(false)
  expect(zSegmentConditionInput.safeParse(mistyped).success).toBe(false)
})
