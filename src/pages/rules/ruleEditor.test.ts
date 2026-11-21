import { expect, expectTypeOf, test } from 'vitest'
import type { z } from 'zod'

import type { RuleConditionInput } from '../../generated/site/types.gen.ts'
import { ruleSchema } from './ruleEditor.ts'

// The generated create input reaches the Condition through a lazy schema typed any: what the
// editor sends must still be a Rule's typed tree (ADR 0181), checked by tsc.
test("a Rule's payload carries the Rule's typed Condition", () => {
  const payload = ruleSchema.create.parse({
    name: 'Pricing',
    triggerEvents: ['pricing_viewed'],
    condition: {
      kind: 'group',
      combinator: 'and',
      entries: [
        {
          kind: 'property',
          of: { kind: 'trigger' },
          key: 'plan',
          test: { kind: 'text', operator: 'eq', value: 'pro' },
        },
      ],
    },
    actions: [],
  })

  expectTypeOf<
    z.output<typeof ruleSchema.create>['condition']
  >().toEqualTypeOf<RuleConditionInput>()
  expect(payload.condition.entries).toHaveLength(1)
})
