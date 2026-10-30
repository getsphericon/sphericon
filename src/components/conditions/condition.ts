import { isRuleGroupType } from 'react-querybuilder'
import type { RuleGroupType, RuleType } from 'react-querybuilder'
import * as z from 'zod'

import {
  SiteConditionCombinator,
  SiteConditionEventOperator,
  SiteConditionOperator,
} from '../../generated/site/types.gen.ts'
import type {
  SiteConditionEventLeaf,
  SiteConditionGroup,
  SiteConditionLeaf,
} from '../../generated/site/types.gen.ts'
import { zSiteConditionEventLeaf, zSiteConditionGroup } from '../../generated/site/zod.gen.ts'
import { jsonText } from '../../utils/jsonText.ts'

// A Segment is always a rule: the empty group matches every contact.
export const emptyGroup: SiteConditionGroup = {
  kind: 'group',
  combinator: SiteConditionCombinator.AND,
  rules: [],
}

// An event leaf is edited as a builder rule on the field "event:<action>" whose value holds the
// count and the window. "Performed" and "not performed" are the builder's presets over the leaf:
// at least once and at most zero times; the API knows only atLeast and atMost.
export const eventFieldPrefix = 'event:'
export const eventPresets = { performed: 'performed', notPerformed: 'notPerformed' } as const

const presetBounds = new Map<string, Pick<SiteConditionEventLeaf, 'operator' | 'times'>>([
  [eventPresets.performed, { operator: SiteConditionEventOperator.AT_LEAST, times: 1 }],
  [eventPresets.notPerformed, { operator: SiteConditionEventOperator.AT_MOST, times: 0 }],
])

// The builder rule's value of an event field: the count and the window, each blank until it is
// typed. A value that does not fit is read as neither.
const zEventCount = zSiteConditionEventLeaf.pick({ times: true, withinDays: true }).partial()
type EventCount = z.infer<typeof zEventCount>

export function eventCount(value: unknown): EventCount {
  const parsed = zEventCount.safeParse(value)
  return parsed.success ? parsed.data : {}
}

// eventCountOf is the value of an event rule, leaving out what is blank.
export function eventCountOf(times?: number, withinDays?: number): EventCount {
  return {
    ...(times === undefined ? {} : { times }),
    ...(withinDays === undefined ? {} : { withinDays }),
  }
}

const eventOperatorByName = new Map<string, SiteConditionEventOperator>(
  Object.values(SiteConditionEventOperator).map((operator) => [operator, operator]),
)

// isCountOperator tells the builder's atLeast and atMost, which take a count, from the presets.
export function isCountOperator(operator: string): boolean {
  return eventOperatorByName.has(operator)
}

function presetOf(leaf: SiteConditionEventLeaf): string | undefined {
  const preset = [...presetBounds].find(
    ([, bound]) => bound.operator === leaf.operator && bound.times === leaf.times,
  )
  return preset?.[0]
}

// A loaded leaf that is a preset's count shows as the preset, whose value is the window alone.
function toEventRule(leaf: SiteConditionEventLeaf): RuleType {
  const preset = presetOf(leaf)
  return {
    field: `${eventFieldPrefix}${leaf.action}`,
    operator: preset ?? leaf.operator,
    value: eventCountOf(preset === undefined ? leaf.times : undefined, leaf.withinDays),
  }
}

// react-querybuilder edits a plain {combinator, rules} tree; the API's tree adds the `kind`
// discriminator and types operators and combinators. toQuery loads a Condition into the builder
// once; the builder keeps its own state from then on, and toDraft reads it back.
export function toQuery(group: SiteConditionGroup): RuleGroupType {
  return {
    combinator: group.combinator,
    ...(group.not === undefined ? {} : { not: group.not }),
    rules: group.rules.map((node) => {
      if (node.kind === 'group') {
        return toQuery(node)
      }
      if (node.kind === 'event') {
        return toEventRule(node)
      }
      return { field: node.field, operator: node.operator, value: node.value ?? '' }
    }),
  }
}

// A Condition as the builder holds it: an event count the user has not typed yet stays blank, and
// is never filled with a default. toCondition tells a complete draft from one still being edited.
type EventLeafDraft = Omit<SiteConditionEventLeaf, 'times'> &
  Partial<Pick<SiteConditionEventLeaf, 'times'>>
type NodeDraft = SiteConditionLeaf | EventLeafDraft | ConditionDraft
export type ConditionDraft = Omit<SiteConditionGroup, 'rules'> & { rules: NodeDraft[] }

const operatorByName = new Map<string, SiteConditionOperator>(
  Object.values(SiteConditionOperator).map((operator) => [operator, operator]),
)
const combinatorByName = new Map<string, SiteConditionCombinator>(
  Object.values(SiteConditionCombinator).map((combinator) => [combinator, combinator]),
)

function boundOf(rule: RuleType, count: EventCount) {
  const operator = eventOperatorByName.get(rule.operator)
  if (operator === undefined) {
    return presetBounds.get(rule.operator)
  }
  return { operator, ...(count.times === undefined ? {} : { times: count.times }) }
}

function toEventLeaf(action: string, rule: RuleType): EventLeafDraft[] {
  const count = eventCount(rule.value)
  const bound = boundOf(rule, count)
  if (!bound) {
    return []
  }
  const span = count.withinDays === undefined ? {} : { withinDays: count.withinDays }
  return [{ kind: 'event', action, ...bound, ...span }]
}

function toLeaf(rule: RuleType): NodeDraft[] {
  if (rule.field.startsWith(eventFieldPrefix)) {
    return toEventLeaf(rule.field.slice(eventFieldPrefix.length), rule)
  }
  const operator = operatorByName.get(rule.operator)
  if (!operator) {
    return []
  }
  return [
    {
      kind: 'rule',
      field: rule.field,
      operator,
      value: typeof rule.value === 'string' ? rule.value : '',
    },
  ]
}

export function toDraft(query: RuleGroupType): ConditionDraft {
  return {
    kind: 'group',
    combinator: combinatorByName.get(query.combinator) ?? SiteConditionCombinator.AND,
    ...(query.not === undefined ? {} : { not: query.not }),
    rules: query.rules.flatMap((node): NodeDraft[] => {
      if (typeof node === 'string') {
        return []
      }
      if (isRuleGroupType(node)) {
        return [toDraft(node)]
      }
      return toLeaf(node)
    }),
  }
}

// toCondition is the draft as the API's Condition, or undefined while a count is blank: the
// generated schema decides.
export function toCondition(draft: ConditionDraft): SiteConditionGroup | undefined {
  const parsed = zSiteConditionGroup.safeParse(draft)
  return parsed.success ? parsed.data : undefined
}

// The form holds a rule group as JSON text (jsonText); validated by the generated schema, so a
// draft with a blank count is refused on submit.
export const conditionCodec: z.ZodType<SiteConditionGroup, string> =
  jsonText.pipe(zSiteConditionGroup)

// draftText is the JSON text a form holds for the builder's draft, complete or not.
export function draftText(draft: ConditionDraft): string {
  return z.encode(jsonText, draft)
}
