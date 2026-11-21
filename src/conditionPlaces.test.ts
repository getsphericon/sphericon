// What each place's generated Condition type and schema admit (ADR 0181).
import { expect, test } from 'vitest'
import { z } from 'zod'

import type { RuleConditionInput, SegmentConditionInput } from './generated/site/types.gen.ts'
import {
  zRuleConditionInput,
  zRuleConditionNode,
  zRuleConditionNodeInput,
  zSegmentConditionInput,
  zSegmentConditionNode,
  zSegmentConditionNodeInput,
} from './generated/site/zod.gen.ts'

const propertyTest = {
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

// The generator breaks the recursion of a tree at one schema with a lazy `any`; when it is the
// union's, the union loses its discriminator and a member reads as anything.
test("each place's entry schema is a discriminated union with no member typed any", () => {
  for (const node of [
    zSegmentConditionNode,
    zSegmentConditionNodeInput,
    zRuleConditionNode,
    zRuleConditionNodeInput,
  ]) {
    expect(node).toBeInstanceOf(z.ZodDiscriminatedUnion)
    expect(node.options.some((member) => member instanceof z.ZodAny)).toBe(false)
  }
})

test("a Segment's Condition takes neither a Rule's property test nor a text operator on a number", () => {
  const segment: SegmentConditionInput = {
    kind: 'group',
    combinator: 'and',
    entries: [
      { kind: 'customField', key: 'seats', test: { kind: 'number', operator: 'gte', value: 2 } },
      { kind: 'group', combinator: 'or', entries: [{ kind: 'tag', name: 'vip', operator: 'has' }] },
    ],
  }
  const rule: RuleConditionInput = { kind: 'group', combinator: 'and', entries: [propertyTest] }
  const misplaced: SegmentConditionInput = {
    kind: 'group',
    combinator: 'and',
    // @ts-expect-error a Segment admits no property test of the triggering Event
    entries: [propertyTest],
  }
  const nested: SegmentConditionInput = {
    kind: 'group',
    combinator: 'and',
    // @ts-expect-error not at any depth either
    entries: [{ kind: 'group', combinator: 'or', entries: [propertyTest] }],
  }
  const mistyped: SegmentConditionInput = {
    kind: 'group',
    combinator: 'and',
    // @ts-expect-error contains is a text operator, not a number test's
    entries: [textOperatorOnANumber],
  }

  expect(zSegmentConditionInput.safeParse(segment).success).toBe(true)
  expect(zRuleConditionInput.safeParse(rule).success).toBe(true)
  expect(zSegmentConditionInput.safeParse(misplaced).success).toBe(false)
  expect(zSegmentConditionInput.safeParse(nested).success).toBe(false)
  expect(zSegmentConditionInput.safeParse(mistyped).success).toBe(false)
})
