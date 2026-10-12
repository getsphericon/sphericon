import { isRuleGroupType, type RuleGroupType } from 'react-querybuilder'
import * as z from 'zod'

import {
  SiteSegmentCombinator,
  type SiteSegmentRuleGroup,
  type SiteSegmentRuleNode,
  SiteSegmentOperator,
} from '../../generated/site/types.gen.ts'
import { zSiteSegmentRuleGroup } from '../../generated/site/zod.gen.ts'

// A Segment is always a rule: the empty group matches every contact.
export const emptyGroup: SiteSegmentRuleGroup = {
  kind: 'group',
  combinator: SiteSegmentCombinator.AND,
  rules: [],
}

// react-querybuilder edits a plain {combinator, rules} tree; the API's tree adds the `kind`
// discriminator and types operators and combinators. These two walk one into the other.
export function toQuery(group: SiteSegmentRuleGroup): RuleGroupType {
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

const operatorByName = new Map<string, SiteSegmentOperator>(
  Object.values(SiteSegmentOperator).map((operator) => [operator, operator]),
)
const combinatorByName = new Map<string, SiteSegmentCombinator>(
  Object.values(SiteSegmentCombinator).map((combinator) => [combinator, combinator]),
)

export function toGroup(query: RuleGroupType): SiteSegmentRuleGroup {
  return {
    kind: 'group',
    combinator: combinatorByName.get(query.combinator) ?? SiteSegmentCombinator.AND,
    ...(query.not === undefined ? {} : { not: query.not }),
    rules: query.rules.flatMap((node): SiteSegmentRuleNode[] => {
      if (typeof node === 'string') return []
      if (isRuleGroupType(node)) return [toGroup(node)]
      const operator = operatorByName.get(node.operator)
      if (!operator) return []
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

// The form holds a rule group as JSON text (form state is strings); this codec is the one
// place the text meets the typed group, validated by the generated schema.
const jsonText = z.codec(z.string(), z.json(), {
  decode: (text, ctx) => {
    try {
      return z.json().parse(JSON.parse(text))
    } catch {
      ctx.issues.push({ code: 'invalid_format', format: 'json', input: text })
      return z.NEVER
    }
  },
  encode: (value) => JSON.stringify(value),
})

export const definitionCodec: z.ZodType<SiteSegmentRuleGroup, string> =
  jsonText.pipe(zSiteSegmentRuleGroup)
