import { isRuleGroupType } from 'react-querybuilder'
import type { RuleGroupType } from 'react-querybuilder'
import * as z from 'zod'

import { SiteConditionCombinator, SiteConditionOperator } from '../../generated/site/types.gen.ts'
import type { SiteConditionGroup, SiteConditionNode } from '../../generated/site/types.gen.ts'
import { zSiteConditionGroup } from '../../generated/site/zod.gen.ts'
import { jsonText } from '../../utils/jsonText.ts'

// A Segment is always a rule: the empty group matches every contact.
export const emptyGroup: SiteConditionGroup = {
  kind: 'group',
  combinator: SiteConditionCombinator.AND,
  rules: [],
}

// react-querybuilder edits a plain {combinator, rules} tree; the API's tree adds the `kind`
// discriminator and types operators and combinators. These two walk one into the other.
export function toQuery(group: SiteConditionGroup): RuleGroupType {
  return {
    combinator: group.combinator,
    ...(group.not === undefined ? {} : { not: group.not }),
    rules: group.rules.map((node) =>
      node.kind === 'group'
        ? toQuery(node)
        : { field: node.field, operator: node.operator, value: node.value ?? '' },
    ),
  }
}

const operatorByName = new Map<string, SiteConditionOperator>(
  Object.values(SiteConditionOperator).map((operator) => [operator, operator]),
)
const combinatorByName = new Map<string, SiteConditionCombinator>(
  Object.values(SiteConditionCombinator).map((combinator) => [combinator, combinator]),
)

export function toGroup(query: RuleGroupType): SiteConditionGroup {
  return {
    kind: 'group',
    combinator: combinatorByName.get(query.combinator) ?? SiteConditionCombinator.AND,
    ...(query.not === undefined ? {} : { not: query.not }),
    rules: query.rules.flatMap((node): SiteConditionNode[] => {
      if (typeof node === 'string') {
        return []
      }
      if (isRuleGroupType(node)) {
        return [toGroup(node)]
      }
      const operator = operatorByName.get(node.operator)
      if (!operator) {
        return []
      }
      return [
        {
          kind: 'rule',
          field: node.field,
          operator,
          value: typeof node.value === 'string' ? node.value : '',
        },
      ]
    }),
  }
}

// The form holds a rule group as JSON text (jsonText); validated by the generated schema.
export const conditionCodec: z.ZodType<SiteConditionGroup, string> =
  jsonText.pipe(zSiteConditionGroup)
