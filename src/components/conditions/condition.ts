import { isRuleGroupType } from 'react-querybuilder'
import type { RuleGroupType, RuleType } from 'react-querybuilder'
import * as z from 'zod'

import {
  SiteConditionCombinator,
  SiteConditionDateOperator,
  SiteConditionEventOperator,
  SiteConditionNumberOperator,
  SiteConditionOperator,
} from '../../generated/site/types.gen.ts'
import type {
  SiteConditionDateLeaf,
  SiteConditionDateWithinLeaf,
  SiteConditionEventLeaf,
  SiteConditionGroup,
  SiteConditionLeaf,
  SiteConditionNumberLeaf,
} from '../../generated/site/types.gen.ts'
import { zSiteConditionEventLeaf, zSiteConditionGroup } from '../../generated/site/zod.gen.ts'
import { dayOf, nextDay, startOfDay } from '../../utils/datetime.ts'
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

// A date-within leaf is edited as the builder operator "within" on a datetime field, its value the
// number of days; the API has no operator for it, only the leaf's kind.
export const dateWithinOperator = 'within'

// react-querybuilder edits a plain {combinator, rules} tree; the API's tree adds the `kind`
// discriminator and types operators and combinators. toQuery loads a Condition into the builder
// once; the builder keeps its own state from then on, and toDraft reads it back. timeZone is the
// User time zone the days of before and after are read in (ADR 0068), undefined for the browser's.
export function toQuery(group: SiteConditionGroup, timeZone?: string): RuleGroupType {
  return {
    combinator: group.combinator,
    ...(group.not === undefined ? {} : { not: group.not }),
    rules: group.rules.map((node) => {
      if (node.kind === 'group') {
        return toQuery(node, timeZone)
      }
      if (node.kind === 'event') {
        return toEventRule(node)
      }
      if (node.kind === 'number') {
        return { field: node.field, operator: node.operator, value: node.value }
      }
      if (node.kind === 'date') {
        return { field: node.field, operator: node.operator, value: dayOfSide(node, timeZone) }
      }
      if (node.kind === 'dateWithin') {
        return { field: node.field, operator: dateWithinOperator, value: node.withinDays }
      }
      return { field: node.field, operator: node.operator, value: node.value ?? '' }
    }),
  }
}

// The builder picks a day for before and after, and a day is whole on both sides: before D is
// before the start of D, after D is from the start of the next day, both in the User time zone. The
// API's before is strictly earlier than `at` and its after at or later, so the two split time there.
const zDay = z.iso.date()

function momentOfSide(operator: SiteConditionDateOperator, value: unknown, timeZone?: string) {
  const day = zDay.safeParse(value)
  if (!day.success) {
    return undefined
  }
  const from = operator === SiteConditionDateOperator.AFTER ? nextDay(day.data) : day.data
  return startOfDay(from, timeZone)
}

// The day a loaded leaf shows: the one `at` starts for before, the one it ends for after.
function dayOfSide(leaf: SiteConditionDateLeaf, timeZone?: string): string {
  const at = Date.parse(leaf.at)
  if (Number.isNaN(at)) {
    return ''
  }
  const lastOfDay = leaf.operator === SiteConditionDateOperator.AFTER ? at - 1 : at
  return dayOf(lastOfDay, timeZone)
}

// A Condition as the builder holds it: an event count, a number, a day or a window the user has not
// typed yet stays blank, and is never filled with a default. toCondition tells a complete draft
// from one still being edited.
type EventLeafDraft = Omit<SiteConditionEventLeaf, 'times'> &
  Partial<Pick<SiteConditionEventLeaf, 'times'>>
type NumberLeafDraft = Omit<SiteConditionNumberLeaf, 'value'> &
  Partial<Pick<SiteConditionNumberLeaf, 'value'>>
type DateLeafDraft = Omit<SiteConditionDateLeaf, 'at'> & Partial<Pick<SiteConditionDateLeaf, 'at'>>
type DateWithinLeafDraft = Omit<SiteConditionDateWithinLeaf, 'withinDays'> &
  Partial<Pick<SiteConditionDateWithinLeaf, 'withinDays'>>
type NodeDraft =
  | SiteConditionLeaf
  | EventLeafDraft
  | NumberLeafDraft
  | DateLeafDraft
  | DateWithinLeafDraft
  | ConditionDraft
export type ConditionDraft = Omit<SiteConditionGroup, 'rules'> & { rules: NodeDraft[] }

const operatorByName = new Map<string, SiteConditionOperator>(
  Object.values(SiteConditionOperator).map((operator) => [operator, operator]),
)
const numberOperatorByName = new Map<string, SiteConditionNumberOperator>(
  Object.values(SiteConditionNumberOperator).map((operator) => [operator, operator]),
)
const dateOperatorByName = new Map<string, SiteConditionDateOperator>(
  Object.values(SiteConditionDateOperator).map((operator) => [operator, operator]),
)
const combinatorByName = new Map<string, SiteConditionCombinator>(
  Object.values(SiteConditionCombinator).map((combinator) => [combinator, combinator]),
)

// numberOf reads a builder value as a number: a number input gives one, a value kept from a text
// operator may be its text. Blank, or anything else, is no number.
function numberOf(value: unknown): number | undefined {
  if (typeof value === 'number') {
    return Number.isFinite(value) ? value : undefined
  }
  if (typeof value !== 'string' || value.trim() === '') {
    return undefined
  }
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : undefined
}

// textOf reads a builder value as a rule leaf's text: a number input on a number field gives a
// number, which the leaf holds as its text.
function textOf(value: unknown): string {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return String(value)
  }
  return typeof value === 'string' ? value : ''
}

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

function toTypedLeaf(rule: RuleType, timeZone?: string): NodeDraft | undefined {
  const compared = numberOperatorByName.get(rule.operator)
  if (compared !== undefined) {
    const value = numberOf(rule.value)
    return {
      kind: 'number',
      field: rule.field,
      operator: compared,
      ...(value === undefined ? {} : { value }),
    }
  }
  const side = dateOperatorByName.get(rule.operator)
  if (side !== undefined) {
    const at = momentOfSide(side, rule.value, timeZone)
    return { kind: 'date', field: rule.field, operator: side, ...(at === undefined ? {} : { at }) }
  }
  if (rule.operator === dateWithinOperator) {
    const withinDays = numberOf(rule.value)
    return {
      kind: 'dateWithin',
      field: rule.field,
      ...(withinDays === undefined ? {} : { withinDays }),
    }
  }
  return undefined
}

function toLeaf(rule: RuleType, timeZone?: string): NodeDraft[] {
  if (rule.field.startsWith(eventFieldPrefix)) {
    return toEventLeaf(rule.field.slice(eventFieldPrefix.length), rule)
  }
  const typed = toTypedLeaf(rule, timeZone)
  if (typed !== undefined) {
    return [typed]
  }
  const operator = operatorByName.get(rule.operator)
  if (!operator) {
    return []
  }
  return [{ kind: 'rule', field: rule.field, operator, value: textOf(rule.value) }]
}

export function toDraft(query: RuleGroupType, timeZone?: string): ConditionDraft {
  return {
    kind: 'group',
    combinator: combinatorByName.get(query.combinator) ?? SiteConditionCombinator.AND,
    ...(query.not === undefined ? {} : { not: query.not }),
    rules: query.rules.flatMap((node): NodeDraft[] => {
      if (typeof node === 'string') {
        return []
      }
      if (isRuleGroupType(node)) {
        return [toDraft(node, timeZone)]
      }
      return toLeaf(node, timeZone)
    }),
  }
}

// toCondition is the draft as the API's Condition, or undefined while a count, a number, a day or
// a window is blank: the generated schema decides.
export function toCondition(draft: ConditionDraft): SiteConditionGroup | undefined {
  const parsed = zSiteConditionGroup.safeParse(draft)
  return parsed.success ? parsed.data : undefined
}

// The form holds a rule group as JSON text (jsonText); validated by the generated schema, so a
// draft with a blank count, number, day or window is refused on submit.
export const conditionCodec: z.ZodType<SiteConditionGroup, string> =
  jsonText.pipe(zSiteConditionGroup)

// draftText is the JSON text a form holds for the builder's draft, complete or not.
export function draftText(draft: ConditionDraft): string {
  return z.encode(jsonText, draft)
}
