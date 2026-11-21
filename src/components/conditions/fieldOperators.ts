import type { Field, InputType, Operator } from 'react-querybuilder'

import {
  CustomFieldType,
  HasOperator,
  MomentOperator,
  NumberOperator,
  PresenceOperator,
  TextOperator,
} from '../../generated/site/types.gen.ts'
import type { SiteCustomFieldResource } from '../../generated/site/types.gen.ts'
import { withinLastOperator } from './condition.ts'

// How the builder names each operator of a typed test, and of a leaf that has or does not have.
const textLabels: Record<TextOperator, string> = {
  [TextOperator.EQ]: 'is',
  [TextOperator.NEQ]: 'is not',
  [TextOperator.CONTAINS]: 'contains',
  [TextOperator.BEGINS_WITH]: 'begins with',
}
const numberLabels: Record<NumberOperator, string> = {
  [NumberOperator.GT]: 'greater than',
  [NumberOperator.GTE]: 'greater than or equal to',
  [NumberOperator.LT]: 'less than',
  [NumberOperator.LTE]: 'less than or equal to',
}
const momentLabels: Record<MomentOperator, string> = {
  [MomentOperator.BEFORE]: 'is before',
  [MomentOperator.AFTER]: 'is after',
}
const presenceLabels: Record<PresenceOperator, string> = {
  [PresenceOperator.SET]: 'is set',
  [PresenceOperator.NOT_SET]: 'is not set',
}
const hasLabels: Record<HasOperator, string> = {
  [HasOperator.HAS]: 'has',
  [HasOperator.DOES_NOT_HAVE]: 'does not have',
}

const text = (operators: readonly TextOperator[]): Operator[] =>
  operators.map((operator) => ({ name: operator, label: textLabels[operator] }))
const number = (operators: readonly NumberOperator[]): Operator[] =>
  operators.map((operator) => ({ name: operator, label: numberLabels[operator] }))
const moment = (operators: readonly MomentOperator[]): Operator[] =>
  operators.map((operator) => ({ name: operator, label: momentLabels[operator] }))
// A presence test compares with nothing, so the builder shows no value input for it.
const presence = (operators: readonly PresenceOperator[]): Operator[] =>
  operators.map((operator) => ({ name: operator, label: presenceLabels[operator], arity: 'unary' }))
const withinLast: Operator = { name: withinLastOperator, label: 'within the last (days)' }

const allText = Object.values(TextOperator)
const allNumber = Object.values(NumberOperator)
const allMoment = Object.values(MomentOperator)
const allPresence = Object.values(PresenceOperator)

// A Contact attribute is text: it takes a text test or a presence test.
export const attributeOperators: Operator[] = [...text(allText), ...presence(allPresence)]

// The birthday is a date: a moment before or after a day, or a window in days back from now.
export const birthdayOperators: Operator[] = [...moment(allMoment), withinLast]

// A Tag is had or not, by its name, typed as the value.
export const tagOperators: Operator[] = Object.values(HasOperator).map((operator) => ({
  name: operator,
  label: hasLabels[operator],
}))

// Telegram and SMS: a Contact has a chat bound, or a phone that can be texted, or does not; no
// value to type.
export const haveOperators: Operator[] = tagOperators.map((operator) => ({
  ...operator,
  arity: 'unary',
}))

// A Rule's Event property has no declared type: it is tested as text, compared as a number or
// tested for presence. Its key is typed in its value, which every operator shows, presence too.
export const propertyOperators: Operator[] = [
  ...text(allText),
  ...number(allNumber),
  ...allPresence.map((operator) => ({ name: operator, label: presenceLabels[operator] })),
]

const presenceOperators = new Set<string>(allPresence)

// isPresence tells a presence test's operator, which compares with no value.
export function isPresence(operator: string): boolean {
  return presenceOperators.has(operator)
}

// operatorsOf is what the builder offers on a Custom field: per typed test, the operators the
// server lists for the field's type (conditionTests, the backend's one table of what fits).
export function operatorsOf(field: SiteCustomFieldResource): Operator[] {
  const tests = field.conditionTests
  return [
    ...text(tests.text),
    ...number(tests.number),
    ...moment(tests.moment),
    ...(tests.withinLast ? [withinLast] : []),
    ...presence(tests.presence),
  ]
}

// The value input a Custom field takes for its text test: a number field's is a number.
export function inputTypeOfField(field: SiteCustomFieldResource): InputType {
  return field.type === CustomFieldType.NUMBER ? 'number' : 'text'
}

const numberInput = new Set<string>([...allNumber, withinLastOperator])
const dateInput = new Set<string>(allMoment)

// inputTypeOf is the value input an operator on the field takes: a number for a comparison and for
// the days of a window, a day for before and after, and the field's own for a text test.
export function inputTypeOf(operator: string, field: Pick<Field, 'inputType'>): InputType | null {
  if (numberInput.has(operator)) {
    return 'number'
  }
  if (dateInput.has(operator)) {
    return 'date'
  }
  return field.inputType ?? null
}
