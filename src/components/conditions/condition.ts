import { isRuleGroupType } from 'react-querybuilder'
import type { RuleGroupType, RuleType } from 'react-querybuilder'
import * as z from 'zod'

import {
  ConditionCombinator,
  ContactAttribute,
  EventCountOperator,
  HasOperator,
  MomentOperator,
  NumberOperator,
  PresenceOperator,
  TextOperator,
} from '../../generated/site/types.gen.ts'
import type {
  AttributeLeafInput,
  BirthdayLeafInput,
  CustomFieldLeafInput,
  EventLeaf,
  EventLeafInput,
  MomentTest,
  MomentTestInput,
  NumberTestInput,
  PresenceTestInput,
  RuleCondition,
  RuleConditionInput,
  RuleConditionNode,
  RulePropertyLeafInput,
  SegmentCondition,
  SegmentConditionInput,
  SmsLeafInput,
  TagLeafInput,
  TelegramLeafInput,
  TextTestInput,
  WithinLastTestInput,
} from '../../generated/site/types.gen.ts'
import {
  zEventLeaf,
  zRuleConditionInput,
  zSegmentConditionInput,
} from '../../generated/site/zod.gen.ts'
import { dayOf, nextDay, startOfDay } from '../../utils/datetime.ts'

// The places a Condition is edited in (ADR 0181): a Segment's tests the Contact, a Rule's also the
// Event that fired it. Each place's generated schema decides what its Condition admits.
interface PlaceConditions {
  segment: SegmentConditionInput
  rule: RuleConditionInput
}
export type ConditionPlace = keyof PlaceConditions

// A Condition the builder loads: a Segment's or a Rule's, as read or as sent.
type AnyCondition = SegmentCondition | RuleCondition | SegmentConditionInput | RuleConditionInput

// The empty group matches every contact, in either place.
export const emptyGroup = {
  kind: 'group',
  combinator: ConditionCombinator.AND,
  entries: [],
} satisfies SegmentCondition & RuleCondition

// The builder's fields, by the subject a leaf tests. A Contact attribute is its own name; the
// birthday, a Tag, Telegram, SMS and the triggering Event's property are one field each (a Tag's
// name and a property's key are typed in the value); a Custom field and an event action are one
// field per key, named by kind and key so a Custom field keyed like an attribute stays apart.
export const subjectFields = {
  birthday: 'birthday',
  tag: 'tag',
  telegram: 'telegram',
  sms: 'sms',
  property: 'property',
} as const
const customFieldIdPrefix = 'customField:'
export const eventFieldPrefix = 'event:'

export function customFieldId(key: string): string {
  return `${customFieldIdPrefix}${key}`
}

const attributeByName = new Map<string, ContactAttribute>(
  Object.values(ContactAttribute).map((attribute) => [attribute, attribute]),
)

// An event leaf is edited as a builder rule on the field "event:<action>" whose value holds the
// count and the window. "Performed" and "not performed" are the builder's presets over the leaf:
// at least once and at most zero times; the API knows only atLeast and atMost.
export const eventPresets = { performed: 'performed', notPerformed: 'notPerformed' } as const

const presetBounds = new Map<string, Pick<EventLeaf, 'operator' | 'times'>>([
  [eventPresets.performed, { operator: EventCountOperator.AT_LEAST, times: 1 }],
  [eventPresets.notPerformed, { operator: EventCountOperator.AT_MOST, times: 0 }],
])

// The builder rule's value of an event field: the count and the window, each blank until it is
// typed. A value that does not fit is read as neither.
const zEventCount = zEventLeaf.pick({ times: true, withinDays: true }).partial()
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

const eventOperatorByName = new Map<string, EventCountOperator>(
  Object.values(EventCountOperator).map((operator) => [operator, operator]),
)

// isCountOperator tells the builder's atLeast and atMost, which take a count, from the presets.
export function isCountOperator(operator: string): boolean {
  return eventOperatorByName.has(operator)
}

function presetOf(leaf: EventLeaf): string | undefined {
  const preset = [...presetBounds].find(
    ([, bound]) => bound.operator === leaf.operator && bound.times === leaf.times,
  )
  return preset?.[0]
}

// A loaded leaf that is a preset's count shows as the preset, whose value is the window alone.
function toEventRule(leaf: EventLeaf): RuleType {
  const preset = presetOf(leaf)
  return {
    field: `${eventFieldPrefix}${leaf.action}`,
    operator: preset ?? leaf.operator,
    value: eventCountOf(preset === undefined ? leaf.times : undefined, leaf.withinDays),
  }
}

// The value of a property rule: the property's key, typed in an input of its own, and the value
// its test compares with. A value that does not fit is read as a blank key.
const zPropertyValue = z.object({ key: z.string(), value: z.unknown().optional() })
interface PropertyValue {
  key: string
  value?: unknown
}

export function propertyValue(value: unknown): PropertyValue {
  const parsed = zPropertyValue.safeParse(value)
  return parsed.success ? parsed.data : { key: '' }
}

export function propertyValueOf(key: string, value: unknown): PropertyValue {
  return value === undefined ? { key } : { key, value }
}

// The withinLast test is edited as the builder operator "withinLast", its value the number of
// days; the API has no operator for it, only the test's kind. Every other operator's name is the
// API's, and the names of the typed tests are disjoint, so the operator alone tells the test.
export const withinLastOperator = 'withinLast'

type AnyTest =
  | TextTestInput
  | NumberTestInput
  | MomentTest
  | MomentTestInput
  | WithinLastTestInput
  | PresenceTestInput

// The builder picks a day for before and after, and a day is whole on both sides: before D is
// before the start of D, after D is from the start of the next day, both in the User time zone. The
// API's before is strictly earlier than `at` and its after at or later, so the two split time there.
const zDay = z.iso.date()

function momentOfSide(operator: MomentOperator, value: unknown, timeZone?: string) {
  const day = zDay.safeParse(value)
  if (!day.success) {
    return undefined
  }
  const from = operator === MomentOperator.AFTER ? nextDay(day.data) : day.data
  return startOfDay(from, timeZone)
}

// The day a loaded moment test shows: the one `at` starts for before, the one it ends for after.
function dayOfSide(test: MomentTestInput, timeZone?: string): string {
  const at = Date.parse(test.at)
  if (Number.isNaN(at)) {
    return ''
  }
  const lastOfDay = test.operator === MomentOperator.AFTER ? at - 1 : at
  return dayOf(lastOfDay, timeZone)
}

// A leaf or test of a kind the switch over the kinds does not name: tsc refuses the call once a
// kind is added to the contract and not here.
function unnamed(kind: never): never {
  throw new Error(`a Condition entry of no known kind: ${JSON.stringify(kind)}`)
}

// The builder rule's operator and value of a test.
function testRule(test: AnyTest, timeZone?: string): Pick<RuleType, 'operator' | 'value'> {
  switch (test.kind) {
    case 'text':
      return { operator: test.operator, value: test.value }
    case 'number':
      return { operator: test.operator, value: test.value }
    case 'moment':
      return { operator: test.operator, value: dayOfSide(test, timeZone) }
    case 'withinLast':
      return { operator: withinLastOperator, value: test.days }
    case 'presence':
      return { operator: test.operator, value: '' }
    default:
      return unnamed(test)
  }
}

function toRule(node: Exclude<RuleConditionNode, { kind: 'group' }>, timeZone?: string): RuleType {
  switch (node.kind) {
    case 'attribute':
      return { field: node.attribute, ...testRule(node.test, timeZone) }
    case 'birthday':
      return { field: subjectFields.birthday, ...testRule(node.test, timeZone) }
    case 'tag':
      return { field: subjectFields.tag, operator: node.operator, value: node.name }
    case 'telegram':
      return { field: subjectFields.telegram, operator: node.operator, value: '' }
    case 'sms':
      return { field: subjectFields.sms, operator: node.operator, value: '' }
    case 'customField':
      return { field: customFieldId(node.key), ...testRule(node.test, timeZone) }
    case 'event':
      return toEventRule(node)
    case 'property': {
      const { operator, value } = testRule(node.test, timeZone)
      return { field: subjectFields.property, operator, value: propertyValueOf(node.key, value) }
    }
    default:
      return unnamed(node)
  }
}

// react-querybuilder edits a plain {combinator, rules} tree of field, operator and value; the API's
// tree is typed by leaf and by test. toQuery loads a Condition into the builder once; the builder
// keeps its own state from then on, and toDraft reads it back. timeZone is the User time zone the
// days of before and after are read in (ADR 0068), undefined for the browser's.
export function toQuery(condition: AnyCondition, timeZone?: string): RuleGroupType {
  const entries: readonly RuleConditionNode[] = condition.entries
  return {
    combinator: condition.combinator,
    ...(condition.not === undefined ? {} : { not: condition.not }),
    rules: entries.map((node) =>
      node.kind === 'group' ? toQuery(node, timeZone) : toRule(node, timeZone),
    ),
  }
}

// A Condition as the builder holds it: an event count, a number, a day or a window the user has not
// typed yet stays blank, and is never filled with a default. toCondition tells a complete draft
// from one still being edited.
type Blank<T, K extends keyof T> = Omit<T, K> & Partial<Pick<T, K>>
type TestDraft =
  | TextTestInput
  | PresenceTestInput
  | Blank<NumberTestInput, 'value'>
  | Blank<MomentTestInput, 'at'>
  | Blank<WithinLastTestInput, 'days'>
type Tested<T extends { test: unknown }> = Omit<T, 'test'> & { test: TestDraft }
type NodeDraft =
  | Tested<AttributeLeafInput>
  | Tested<BirthdayLeafInput>
  | TagLeafInput
  | TelegramLeafInput
  | SmsLeafInput
  | Tested<CustomFieldLeafInput>
  | Blank<EventLeafInput, 'times'>
  | Blank<Tested<RulePropertyLeafInput>, 'key'>
  | ConditionDraft
export type ConditionDraft = Omit<RuleConditionInput, 'entries'> & { entries: NodeDraft[] }

function byName<T extends string>(values: Record<string, T>) {
  return new Map<string, T>(Object.values(values).map((value) => [value, value]))
}
const textOperatorByName = byName(TextOperator)
const numberOperatorByName = byName(NumberOperator)
const momentOperatorByName = byName(MomentOperator)
const presenceOperatorByName = byName(PresenceOperator)
const hasOperatorByName = byName(HasOperator)
const combinatorByName = byName(ConditionCombinator)

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

// textOf reads a builder value as a text test's value: a number input on a number field gives a
// number, which the test holds as its text.
function textOf(value: unknown): string {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return String(value)
  }
  return typeof value === 'string' ? value : ''
}

// toTest reads the test a builder operator names, with the value it compares with, or undefined
// for an operator that names no test.
function toTest(operator: string, value: unknown, timeZone?: string): TestDraft | undefined {
  const text = textOperatorByName.get(operator)
  if (text !== undefined) {
    return { kind: 'text', operator: text, value: textOf(value) }
  }
  const compared = numberOperatorByName.get(operator)
  if (compared !== undefined) {
    const number = numberOf(value)
    return {
      kind: 'number',
      operator: compared,
      ...(number === undefined ? {} : { value: number }),
    }
  }
  const side = momentOperatorByName.get(operator)
  if (side !== undefined) {
    const at = momentOfSide(side, value, timeZone)
    return { kind: 'moment', operator: side, ...(at === undefined ? {} : { at }) }
  }
  if (operator === withinLastOperator) {
    const days = numberOf(value)
    return { kind: 'withinLast', ...(days === undefined ? {} : { days }) }
  }
  const presence = presenceOperatorByName.get(operator)
  return presence === undefined ? undefined : { kind: 'presence', operator: presence }
}

// The tests a leaf takes, besides what a Custom field's type takes, which is checked at save.
const attributeTests = new Set<TestDraft['kind']>(['text', 'presence'])
const birthdayTests = new Set<TestDraft['kind']>(['moment', 'withinLast'])
const propertyTests = new Set<TestDraft['kind']>(['text', 'number', 'presence'])

function boundOf(rule: RuleType, count: EventCount) {
  const operator = eventOperatorByName.get(rule.operator)
  if (operator === undefined) {
    return presetBounds.get(rule.operator)
  }
  return { operator, ...(count.times === undefined ? {} : { times: count.times }) }
}

function toEventLeaf(action: string, rule: RuleType): NodeDraft[] {
  const count = eventCount(rule.value)
  const bound = boundOf(rule, count)
  if (!bound) {
    return []
  }
  const span = count.withinDays === undefined ? {} : { withinDays: count.withinDays }
  return [{ kind: 'event', action, ...bound, ...span }]
}

function toHaveLeaf(rule: RuleType): NodeDraft[] {
  const operator = hasOperatorByName.get(rule.operator)
  if (operator === undefined) {
    return []
  }
  switch (rule.field) {
    case subjectFields.tag:
      return [{ kind: 'tag', name: textOf(rule.value), operator }]
    case subjectFields.telegram:
      return [{ kind: 'telegram', operator }]
    default:
      return [{ kind: 'sms', operator }]
  }
}

function toPropertyLeaf(rule: RuleType, timeZone?: string): NodeDraft[] {
  const { key, value } = propertyValue(rule.value)
  const test = toTest(rule.operator, value, timeZone)
  if (test === undefined || !propertyTests.has(test.kind)) {
    return []
  }
  return [{ kind: 'property', of: { kind: 'trigger' }, ...(key === '' ? {} : { key }), test }]
}

// toLeaf reads a builder rule as the leaf its field and operator name, or as nothing when they
// name none.
function toLeaf(rule: RuleType, timeZone?: string): NodeDraft[] {
  const { field } = rule
  if (field.startsWith(eventFieldPrefix)) {
    return toEventLeaf(field.slice(eventFieldPrefix.length), rule)
  }
  if (
    field === subjectFields.tag ||
    field === subjectFields.telegram ||
    field === subjectFields.sms
  ) {
    return toHaveLeaf(rule)
  }
  if (field === subjectFields.property) {
    return toPropertyLeaf(rule, timeZone)
  }
  const test = toTest(rule.operator, rule.value, timeZone)
  if (test === undefined) {
    return []
  }
  if (field.startsWith(customFieldIdPrefix)) {
    return [{ kind: 'customField', key: field.slice(customFieldIdPrefix.length), test }]
  }
  if (field === subjectFields.birthday) {
    return birthdayTests.has(test.kind) ? [{ kind: 'birthday', test }] : []
  }
  const attribute = attributeByName.get(field)
  if (attribute === undefined || !attributeTests.has(test.kind)) {
    return []
  }
  return [{ kind: 'attribute', attribute, test }]
}

export function toDraft(query: RuleGroupType, timeZone?: string): ConditionDraft {
  return {
    kind: 'group',
    combinator: combinatorByName.get(query.combinator) ?? ConditionCombinator.AND,
    ...(query.not === undefined ? {} : { not: query.not }),
    entries: query.rules.flatMap((node): NodeDraft[] => {
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

// Each place's Condition schema, typed: the generated one reaches a group's entries through a lazy
// schema typed any, so a form's schema takes its Condition from here.
export const conditionSchemas: { [P in ConditionPlace]: z.ZodType<PlaceConditions[P]> } = {
  segment: zSegmentConditionInput,
  rule: zRuleConditionInput,
}

// toCondition is the draft as the place's Condition, or undefined while a count, a number, a day,
// a window or a property's key is blank, or while it holds what the place does not admit: the
// generated schema decides.
export function toCondition<P extends ConditionPlace>(
  draft: ConditionDraft,
  place: P,
): PlaceConditions[P] | undefined {
  const parsed = conditionSchemas[place].safeParse(draft)
  return parsed.success ? parsed.data : undefined
}
