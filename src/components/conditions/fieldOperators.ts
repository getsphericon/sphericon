import type { TFunction } from 'i18next'
import type { Field, InputType, Operator } from 'react-querybuilder'

import { CustomFieldType, HasOperator } from '../../generated/site/types.gen.ts'
import type { SiteCustomFieldResource } from '../../generated/site/types.gen.ts'
import { leafTests, namedOperator, operatorName, testOperators } from './condition.ts'
import type { NamedOperator, TestKind } from './condition.ts'

// The builder's operators, each the operator of a test the contract names (condition.ts reads them
// once from the generated enums and test unions), labelled from the en catalog.

// label is how the builder names the operator of a test. The switch narrows the operator by its
// test's kind, one case per kind, and returns once after it: tsc refuses the read of the label
// while a kind is missing.
function label(t: TFunction, named: NamedOperator): string {
  let text: string
  switch (named.kind) {
    case 'text':
      text = t(($) => $.conditions.textOperators[named.operator])
      break
    case 'number':
      text = t(($) => $.conditions.numberOperators[named.operator])
      break
    case 'moment':
      text = t(($) => $.conditions.momentOperators[named.operator])
      break
    case 'withinLast':
      text = t(($) => $.conditions.withinLast)
      break
    case 'presence':
      text = t(($) => $.conditions.presenceOperators[named.operator])
      break
  }
  return text
}

// operator is the builder's operator of a test's. A presence test compares with nothing, so the
// builder shows no value input for it, unless the leaf types something else in its value (a
// property's key).
function operator(t: TFunction, named: NamedOperator, unaryPresence: boolean): Operator {
  const base = { name: operatorName(named), label: label(t, named) }
  return named.kind === 'presence' && unaryPresence ? { ...base, arity: 'unary' } : base
}

// operatorsOfTests is every operator of the tests of the kinds given, in their order.
function operatorsOfTests(t: TFunction, kinds: readonly TestKind[], unaryPresence = true) {
  return kinds.flatMap((kind) =>
    testOperators[kind].map((named: NamedOperator) => operator(t, named, unaryPresence)),
  )
}

// A Contact attribute is text: it takes a text test or a presence test.
export function attributeOperators(t: TFunction): Operator[] {
  return operatorsOfTests(t, leafTests.attribute)
}

// The birthday is a date: a moment before or after a day, or a window in days back from now.
export function birthdayOperators(t: TFunction): Operator[] {
  return operatorsOfTests(t, leafTests.birthday)
}

// A Rule's Event property has no declared type: it is tested as text, compared as a number or
// tested for presence. Its key is typed in its value, which every operator shows, presence too.
export function propertyOperators(t: TFunction): Operator[] {
  return operatorsOfTests(t, leafTests.property, false)
}

// A Tag is had or not, by its name, typed as the value.
export function tagOperators(t: TFunction): Operator[] {
  return Object.values(HasOperator).map((has) => ({
    name: has,
    label: t(($) => $.conditions.hasOperators[has]),
  }))
}

// Telegram and SMS: a Contact has a chat bound, or a phone that can be texted, or does not; no
// value to type.
export function haveOperators(t: TFunction): Operator[] {
  return tagOperators(t).map((has) => ({ ...has, arity: 'unary' }))
}

// isPresence tells a presence test's operator, which compares with no value.
export function isPresence(name: string): boolean {
  return namedOperator(name)?.kind === 'presence'
}

// The operators of each typed test that a Custom field's type admits, as the server lists them
// (conditionTests, the backend's one table of what fits).
const fieldTestOperators: Record<
  TestKind,
  (t: TFunction, tests: SiteCustomFieldResource['conditionTests']) => Operator[]
> = {
  text: (t, tests) =>
    tests.text.map((named) => operator(t, { kind: 'text', operator: named }, true)),
  number: (t, tests) =>
    tests.number.map((named) => operator(t, { kind: 'number', operator: named }, true)),
  moment: (t, tests) =>
    tests.moment.map((named) => operator(t, { kind: 'moment', operator: named }, true)),
  withinLast: (t, tests) => (tests.withinLast ? [operator(t, { kind: 'withinLast' }, true)] : []),
  presence: (t, tests) =>
    tests.presence.map((named) => operator(t, { kind: 'presence', operator: named }, true)),
}

// operatorsOf is what the builder offers on a Custom field: per typed test, the operators the
// server lists for the field's type.
export function operatorsOf(t: TFunction, field: SiteCustomFieldResource): Operator[] {
  return leafTests.customField.flatMap((kind) => fieldTestOperators[kind](t, field.conditionTests))
}

// The value input a Custom field takes for its text test: a number field's is a number.
export function inputTypeOfField(field: SiteCustomFieldResource): InputType {
  return field.type === CustomFieldType.NUMBER ? 'number' : 'text'
}

type InputOf = (field: Pick<Field, 'inputType'>) => InputType | null

// The field's own value input: a number field's is a number.
const ownInput: InputOf = (field) => field.inputType ?? null

// The value input each test's operators take: a number for a comparison and for the days of a
// window, a day for before and after, and the field's own for a text test.
const testInputs: Record<TestKind, InputOf> = {
  text: ownInput,
  number: () => 'number',
  moment: () => 'date',
  withinLast: () => 'number',
  presence: ownInput,
}

// inputTypeOf is the value input an operator on the field takes; one that names no test (an event
// count's, a Tag's) takes the field's own.
export function inputTypeOf(name: string, field: Pick<Field, 'inputType'>): InputType | null {
  const kind = namedOperator(name)?.kind
  return kind === undefined ? ownInput(field) : testInputs[kind](field)
}
