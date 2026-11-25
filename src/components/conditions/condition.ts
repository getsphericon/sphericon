import { omit } from 'es-toolkit'
import { isRuleGroupType } from 'react-querybuilder'
import type { RuleGroupType, RuleType } from 'react-querybuilder'
import * as z from 'zod'

import {
  ConditionCombinator,
  ContactAttribute,
  EngagementTrackingChannel,
  EventCountOperator,
  HasOperator,
  MessageSender,
  MomentOperator,
  NumberOperator,
  PresenceOperator,
  TextOperator,
} from '../../generated/site/types.gen.ts'
import type {
  AttributeLeafInput,
  BirthdayLeafInput,
  CustomFieldLeafInput,
  EngagementTrackingLeafInput,
  EventFilter,
  EventFilterInput,
  EventFilterNode,
  EventLeaf,
  EventLeafInput,
  FilterPropertyLeafInput,
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
  SentByLeafInput,
  SmsLeafInput,
  TagLeafInput,
  TelegramLeafInput,
  TextTestInput,
  WithinLastTestInput,
} from '../../generated/site/types.gen.ts'
import {
  zAttributeTestInput,
  zBirthdayTestInput,
  zCreateRuleInput,
  zCreateSegmentInput,
  zCustomFieldTestInput,
  zEngagementTrackingTestInput,
  zEventFilterInput,
  zEventLeaf,
  zPropertyTestInput,
  zRuleConditionInput,
  zSegmentConditionInput,
} from '../../generated/site/zod.gen.ts'
import { dayOf, nextDay, startOfDay } from '../../utils/datetime.ts'

// The places a Condition is edited in (ADR 0181): a Segment's tests the Contact, a Rule's also the
// Event that fired it, and an Event matcher's filter only the properties of the one Event it is
// about (ADR 0133). Each place's generated schema decides what its Condition admits.
interface PlaceConditions {
  segment: SegmentConditionInput
  rule: RuleConditionInput
  eventFilter: EventFilterInput
}
export type ConditionPlace = keyof PlaceConditions

// A Condition the builder loads: a Segment's, a Rule's or an Event filter, as read or as sent.
export type AnyCondition =
  | SegmentCondition
  | RuleCondition
  | EventFilter
  | SegmentConditionInput
  | RuleConditionInput
  | EventFilterInput

// The empty group matches every contact, in either place.
export const emptyGroup = {
  kind: 'group',
  combinator: ConditionCombinator.AND,
  entries: [],
} satisfies SegmentCondition & RuleCondition

// The builder's fields, by the subject a leaf tests. A Contact attribute is its own name; the
// birthday, a Tag, Telegram, SMS and the triggering Event's property are one field each (a Tag's
// name and a property's key are typed in the value); a Custom field and an event action are one
// field per key, named by kind and key so a Custom field keyed like an attribute stays apart, and so
// is an Engagement tracking consent, one field per Channel.
export const subjectFields = {
  birthday: 'birthday',
  tag: 'tag',
  telegram: 'telegram',
  sms: 'sms',
  property: 'property',
} as const
const customFieldIdPrefix = 'customField:'
export const eventFieldPrefix = 'event:'
const trackingFieldPrefix = 'engagementTracking:'
const sentByFieldPrefix = 'sentBy:'

// A sent-by test is edited as a rule on the field "sentBy:<sender>", whose one operator names the
// Broadcast or Automation picked as its value.
export const sentByOperator = 'is'

// sentByFieldId is the field of the test that a sender of the kind sent the message.
export function sentByFieldId(sender: MessageSender): string {
  return `${sentByFieldPrefix}${sender}`
}

export function customFieldId(key: string): string {
  return `${customFieldIdPrefix}${key}`
}

// trackingFieldId is the field of the Engagement tracking consent on channel.
export function trackingFieldId(channel: EngagementTrackingChannel): string {
  return `${trackingFieldPrefix}${channel}`
}

// byName reads a name as a value of a generated enum, or as undefined.
function byName<T extends string>(values: Record<string, T>) {
  return new Map<string, T>(Object.values(values).map((value) => [value, value]))
}

const attributeByName = byName(ContactAttribute)
const trackingChannelByName = byName(EngagementTrackingChannel)

// An event leaf is edited as a builder rule on the field "event:<action>" whose value holds the
// count and the window. "Performed" and "not performed" are the builder's presets over the leaf:
// at least once and at most zero times; the API knows only atLeast and atMost.
export const eventPresets = { performed: 'performed', notPerformed: 'notPerformed' } as const

const presetBounds = new Map<string, Pick<EventLeaf, 'operator' | 'times'>>([
  [eventPresets.performed, { operator: EventCountOperator.AT_LEAST, times: 1 }],
  [eventPresets.notPerformed, { operator: EventCountOperator.AT_MOST, times: 0 }],
])

// The builder rule's value of an event field: the count and the window, each blank until it is
// typed, and the filter of the Events it counts (ADR 0153) as its own builder leaves it, complete or
// still being typed. A value that does not fit is read as none of them.
const zFilterDraft = z.custom<ConditionDraft>(
  (value) => typeof value === 'object' && value !== null && 'entries' in value,
)
const zEventCount = zEventLeaf
  .pick({ times: true, withinDays: true })
  .partial()
  .extend({ filter: zFilterDraft.optional() })
type EventCount = z.infer<typeof zEventCount>

export function eventCount(value: unknown): EventCount {
  const parsed = zEventCount.safeParse(value)
  return parsed.success ? parsed.data : {}
}

// eventCountOf is the value of an event rule, leaving out what is blank and a filter that tests
// nothing.
export function eventCountOf(
  times?: number,
  withinDays?: number,
  filter?: ConditionDraft,
): EventCount {
  return {
    ...(times === undefined ? {} : { times }),
    ...(withinDays === undefined ? {} : { withinDays }),
    ...(filter === undefined || testsNothing(filter) ? {} : { filter }),
  }
}

// reachesPastKept tells an event test that counts Events older than the analytical Events the
// Instance keeps (ADR 0019, ADR 0153): one with no window, or a longer one, of an action whose
// Events expire, so it counts only the Events kept. keptDays 0 keeps every Event.
export function reachesPastKept(count: {
  action: string
  withinDays: number | undefined
  keptDays: number
  keptActions: readonly string[]
}): boolean {
  if (count.keptDays <= 0 || count.keptActions.includes(count.action)) {
    return false
  }
  return count.withinDays === undefined || count.withinDays > count.keptDays
}

// testsNothing tells a filter that holds no test, which is no filter: an empty group that is not
// negated holds for every Event.
function testsNothing(filter: AnyCondition | ConditionDraft): boolean {
  return filter.entries.length === 0 && filter.not !== true
}

const eventOperatorByName = byName(EventCountOperator)

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

// A loaded leaf that is a preset's count shows as the preset, whose value is the window and the
// filter alone.
function toEventRule(leaf: EventLeaf): RuleType {
  const preset = presetOf(leaf)
  return {
    field: `${eventFieldPrefix}${leaf.action}`,
    operator: preset ?? leaf.operator,
    value: eventCountOf(
      preset === undefined ? leaf.times : undefined,
      leaf.withinDays,
      leaf.filter,
    ),
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

// A builder operator as the test it names: every operator of the contract's typed tests by its
// name, read once from the generated enums, and withinLast.
export type NamedOperator =
  | { kind: 'text'; operator: TextOperator }
  | { kind: 'number'; operator: NumberOperator }
  | { kind: 'moment'; operator: MomentOperator }
  | { kind: 'withinLast' }
  | { kind: 'presence'; operator: PresenceOperator }
export type TestKind = NamedOperator['kind']

// Every test's operators, each named by its builder operator, in the contract's order of each enum.
export const testOperators: { [K in TestKind]: Extract<NamedOperator, { kind: K }>[] } = {
  text: Object.values(TextOperator).map((operator) => ({ kind: 'text', operator })),
  number: Object.values(NumberOperator).map((operator) => ({ kind: 'number', operator })),
  moment: Object.values(MomentOperator).map((operator) => ({ kind: 'moment', operator })),
  withinLast: [{ kind: 'withinLast' }],
  presence: Object.values(PresenceOperator).map((operator) => ({ kind: 'presence', operator })),
}

// The builder operator's name of a test's operator.
export function operatorName(named: NamedOperator): string {
  return named.kind === 'withinLast' ? withinLastOperator : named.operator
}

const operatorsByName = new Map<string, NamedOperator>(
  Object.values(testOperators).flatMap((operators: NamedOperator[]) =>
    operators.map((named): [string, NamedOperator] => [operatorName(named), named]),
  ),
)

// namedOperator is the test a builder operator names, or undefined for one that names none (an
// event count's, a Tag's).
export function namedOperator(operator: string): NamedOperator | undefined {
  return operatorsByName.get(operator)
}

// The kinds of test each leaf takes, in the contract's order: its generated test union's. A Custom
// field's type narrows its own further, which the server lists per field and checks at save.
function kindsOf(union: { options: readonly { shape: { kind: { value: TestKind } } }[] }) {
  return union.options.map((option) => option.shape.kind.value)
}
export const leafTests = {
  attribute: kindsOf(zAttributeTestInput),
  birthday: kindsOf(zBirthdayTestInput),
  customField: kindsOf(zCustomFieldTestInput),
  property: kindsOf(zPropertyTestInput),
  engagementTracking: kindsOf(zEngagementTrackingTestInput),
}

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

// The switches below narrow a union of the contract by its kind, one case per member, and return
// once after the switch: tsc refuses the read of the result while a case is missing, so a kind
// added to the contract and not here fails to compile, and no arm is left that nothing reaches.

// The builder rule's operator and value of a test.
function testRule(test: AnyTest, timeZone?: string): Pick<RuleType, 'operator' | 'value'> {
  let rule: Pick<RuleType, 'operator' | 'value'>
  switch (test.kind) {
    case 'text':
      rule = { operator: test.operator, value: test.value }
      break
    case 'number':
      rule = { operator: test.operator, value: test.value }
      break
    case 'moment':
      rule = { operator: test.operator, value: dayOfSide(test, timeZone) }
      break
    case 'withinLast':
      rule = { operator: withinLastOperator, value: test.days }
      break
    case 'presence':
      rule = { operator: test.operator, value: '' }
      break
  }
  return rule
}

function toRule(
  node: Exclude<RuleConditionNode | EventFilterNode, { kind: 'group' }>,
  timeZone?: string,
): RuleType {
  let rule: RuleType
  switch (node.kind) {
    case 'attribute':
      rule = { field: node.attribute, ...testRule(node.test, timeZone) }
      break
    case 'birthday':
      rule = { field: subjectFields.birthday, ...testRule(node.test, timeZone) }
      break
    case 'tag':
      rule = { field: subjectFields.tag, operator: node.operator, value: node.name }
      break
    case 'telegram':
      rule = { field: subjectFields.telegram, operator: node.operator, value: '' }
      break
    case 'sms':
      rule = { field: subjectFields.sms, operator: node.operator, value: '' }
      break
    case 'engagementTracking':
      rule = { field: trackingFieldId(node.channel), ...testRule(node.test, timeZone) }
      break
    case 'customField':
      rule = { field: customFieldId(node.key), ...testRule(node.test, timeZone) }
      break
    case 'event':
      rule = toEventRule(node)
      break
    case 'property': {
      const { operator, value } = testRule(node.test, timeZone)
      rule = { field: subjectFields.property, operator, value: propertyValueOf(node.key, value) }
      break
    }
    case 'sentBy':
      rule = { field: sentByFieldId(node.sender), operator: sentByOperator, value: node.id }
      break
  }
  return rule
}

// react-querybuilder edits a plain {combinator, rules} tree of field, operator and value; the API's
// tree is typed by leaf and by test. toQuery loads a Condition into the builder once; the builder
// keeps its own state from then on, and toDraft reads it back. timeZone is the User time zone the
// days of before and after are read in (ADR 0068), undefined for the browser's.
export function toQuery(condition: AnyCondition, timeZone?: string): RuleGroupType {
  const entries: readonly (RuleConditionNode | EventFilterNode)[] = condition.entries
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
  | Tested<EngagementTrackingLeafInput>
  | Tested<CustomFieldLeafInput>
  | (Omit<Blank<EventLeafInput, 'times'>, 'filter'> & { filter?: ConditionDraft })
  | Blank<Tested<RulePropertyLeafInput>, 'key'>
  | Blank<Tested<FilterPropertyLeafInput>, 'key'>
  | SentByLeafInput
  | ConditionDraft
export type ConditionDraft = Omit<RuleConditionInput, 'entries'> & { entries: NodeDraft[] }

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
  const named = namedOperator(operator)
  if (named === undefined) {
    return undefined
  }
  let test: TestDraft
  switch (named.kind) {
    case 'text':
      test = { kind: 'text', operator: named.operator, value: textOf(value) }
      break
    case 'number': {
      const number = numberOf(value)
      test = {
        kind: 'number',
        operator: named.operator,
        ...(number === undefined ? {} : { value: number }),
      }
      break
    }
    case 'moment': {
      const at = momentOfSide(named.operator, value, timeZone)
      test = { kind: 'moment', operator: named.operator, ...(at === undefined ? {} : { at }) }
      break
    }
    case 'withinLast': {
      const days = numberOf(value)
      test = { kind: 'withinLast', ...(days === undefined ? {} : { days }) }
      break
    }
    case 'presence':
      test = { kind: 'presence', operator: named.operator }
      break
  }
  return test
}

// takes reads the test a builder operator names as a test the leaf takes, or undefined.
function takes(kinds: readonly TestKind[], test: TestDraft | undefined): TestDraft | undefined {
  return test !== undefined && kinds.includes(test.kind) ? test : undefined
}

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
  const filtered = count.filter === undefined ? {} : { filter: count.filter }
  return [{ kind: 'event', action, ...bound, ...span, ...filtered }]
}

const senderByName = byName(MessageSender)

// toSentByLeaf reads a builder rule as the test that the sender its field names, picked as the
// rule's value, sent the message; nothing until one is picked.
function toSentByLeaf(rule: RuleType): NodeDraft[] {
  const sender = senderByName.get(rule.field.slice(sentByFieldPrefix.length))
  const id = textOf(rule.value)
  return sender === undefined || id === '' ? [] : [{ kind: 'sentBy', sender, id }]
}

// toHaveLeaf reads a builder rule as a leaf the Contact has or not, or as nothing when its
// operator is neither.
function toHaveLeaf(rule: RuleType, leaf: (operator: HasOperator) => NodeDraft): NodeDraft[] {
  const operator = hasOperatorByName.get(rule.operator)
  return operator === undefined ? [] : [leaf(operator)]
}

function toPropertyLeaf(rule: RuleType, timeZone?: string): NodeDraft[] {
  const { key, value } = propertyValue(rule.value)
  const test = takes(leafTests.property, toTest(rule.operator, value, timeZone))
  if (test === undefined) {
    return []
  }
  return [{ kind: 'property', of: { kind: 'trigger' }, ...(key === '' ? {} : { key }), test }]
}

type LeafKind = Exclude<NodeDraft['kind'], 'group'>

// How a builder rule reads as a leaf of each kind, or as nothing when its operator names no test
// the leaf takes. tsc checks that every kind of leaf is here.
const leaves: Record<LeafKind, (rule: RuleType, timeZone?: string) => NodeDraft[]> = {
  event: (rule) => toEventLeaf(rule.field.slice(eventFieldPrefix.length), rule),
  tag: (rule) =>
    toHaveLeaf(rule, (operator) => ({ kind: 'tag', name: textOf(rule.value), operator })),
  telegram: (rule) => toHaveLeaf(rule, (operator) => ({ kind: 'telegram', operator })),
  sms: (rule) => toHaveLeaf(rule, (operator) => ({ kind: 'sms', operator })),
  property: toPropertyLeaf,
  sentBy: toSentByLeaf,
  engagementTracking: (rule, timeZone) => {
    const channel = trackingChannelByName.get(rule.field.slice(trackingFieldPrefix.length))
    const test = takes(leafTests.engagementTracking, toTest(rule.operator, rule.value, timeZone))
    return channel === undefined || test === undefined
      ? []
      : [{ kind: 'engagementTracking', channel, test }]
  },
  customField: (rule, timeZone) => {
    const test = takes(leafTests.customField, toTest(rule.operator, rule.value, timeZone))
    const key = rule.field.slice(customFieldIdPrefix.length)
    return test === undefined ? [] : [{ kind: 'customField', key, test }]
  },
  birthday: (rule, timeZone) => {
    const test = takes(leafTests.birthday, toTest(rule.operator, rule.value, timeZone))
    return test === undefined ? [] : [{ kind: 'birthday', test }]
  },
  attribute: (rule, timeZone) => {
    const attribute = attributeByName.get(rule.field)
    const test = takes(leafTests.attribute, toTest(rule.operator, rule.value, timeZone))
    return attribute === undefined || test === undefined
      ? []
      : [{ kind: 'attribute', attribute, test }]
  },
}

const subjectKinds = new Map<string, LeafKind>(
  Object.values(subjectFields).map((field) => [field, field]),
)

// leafKindOf is the kind of leaf a builder field edits, or undefined for a field that names none.
function leafKindOf(field: string): LeafKind | undefined {
  if (field.startsWith(eventFieldPrefix)) {
    return 'event'
  }
  if (field.startsWith(customFieldIdPrefix)) {
    return 'customField'
  }
  if (field.startsWith(trackingFieldPrefix)) {
    return 'engagementTracking'
  }
  if (field.startsWith(sentByFieldPrefix)) {
    return 'sentBy'
  }
  return attributeByName.has(field) ? 'attribute' : subjectKinds.get(field)
}

// toLeaf reads a builder rule as the leaf its field and operator name, or as nothing when they
// name none.
function toLeaf(rule: RuleType, timeZone?: string): NodeDraft[] {
  const kind = leafKindOf(rule.field)
  return kind === undefined ? [] : leaves[kind](rule, timeZone)
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

// Each place's Condition schema, typed.
const conditionSchemas: { [P in ConditionPlace]: z.ZodType<PlaceConditions[P]> } = {
  segment: zSegmentConditionInput,
  rule: zRuleConditionInput,
  eventFilter: zEventFilterInput,
}

// The create input of a Segment and of a Rule, with the place's typed Condition schema: the
// generated input schema reaches its Condition through a lazy schema typed any (the zod generator
// types a reference to a schema it has not emitted yet so, and has no setting that changes it at
// @hey-api/openapi-ts next-20260930, the newest), so a form takes its schema from here. This is the
// one place that works around it.
export const placeInputSchemas = {
  segment: zCreateSegmentInput.extend({ condition: conditionSchemas.segment }),
  rule: zCreateRuleInput.extend({ condition: conditionSchemas.rule }),
}

// toCondition is the draft as the place's Condition, or undefined while a count, a number, a day,
// a window or a property's key is blank, or while it holds what the place does not admit: the
// generated schema decides. An Event filter's property tests name no subject (subjectless).
export function toCondition<P extends ConditionPlace>(
  draft: ConditionDraft,
  place: P,
): PlaceConditions[P] | undefined {
  const parsed = conditionSchemas[place].safeParse(
    place === 'eventFilter' ? subjectless(draft) : draft,
  )
  return parsed.success ? parsed.data : undefined
}

// subjectless is the draft with its property tests naming no subject, as an Event filter holds
// them: the builder reads a property test as the triggering Event's, and a filter tests the one
// Event it is about.
export function subjectless(draft: ConditionDraft): ConditionDraft {
  return {
    ...draft,
    entries: draft.entries.map((node): NodeDraft => {
      if (node.kind === 'group') {
        return subjectless(node)
      }
      if (node.kind === 'property' && 'of' in node) {
        return omit(node, ['of'])
      }
      return node
    }),
  }
}
