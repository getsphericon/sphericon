import type { TFunction } from 'i18next'
import type { Field, InputType, Operator } from 'react-querybuilder'

import { CustomFieldType, HasOperator } from '../../generated/site/types.gen.ts'
import type { SiteCustomFieldResource } from '../../generated/site/types.gen.ts'
import { leafTests, namedOperator, operatorName, testOperators, unnamed } from './condition.ts'
import type { NamedOperator, TestKind } from './condition.ts'

// The builder's operators, each the operator of a test the contract names (condition.ts reads them
// once from the generated enums and test unions), labelled from the en catalog.

// label is how the builder names the operator of a test.
function label(t: TFunction, named: NamedOperator): string {
  switch (named.kind) {
    case 'text':
      return t(($) => $.conditions.textOperators[named.operator])
    case 'number':
      return t(($) => $.conditions.numberOperators[named.operator])
    case 'moment':
      return t(($) => $.conditions.momentOperators[named.operator])
    case 'withinLast':
      return t(($) => $.conditions.withinLast)
    case 'presence':
      return t(($) => $.conditions.presenceOperators[named.operator])
    default:
      return unnamed(named)
  }
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

// operatorsOf is what the builder offers on a Custom field: per typed test, the operators the
// server lists for the field's type (conditionTests, the backend's one table of what fits).
export function operatorsOf(t: TFunction, field: SiteCustomFieldResource): Operator[] {
  const tests = field.conditionTests
  return leafTests.customField.flatMap((kind): Operator[] => {
    switch (kind) {
      case 'text':
        return tests.text.map((named) => operator(t, { kind, operator: named }, true))
      case 'number':
        return tests.number.map((named) => operator(t, { kind, operator: named }, true))
      case 'moment':
        return tests.moment.map((named) => operator(t, { kind, operator: named }, true))
      case 'withinLast':
        return tests.withinLast ? [operator(t, { kind }, true)] : []
      case 'presence':
        return tests.presence.map((named) => operator(t, { kind, operator: named }, true))
      default:
        return unnamed(kind)
    }
  })
}

// The value input a Custom field takes for its text test: a number field's is a number.
export function inputTypeOfField(field: SiteCustomFieldResource): InputType {
  return field.type === CustomFieldType.NUMBER ? 'number' : 'text'
}

// inputTypeOf is the value input an operator on the field takes: a number for a comparison and for
// the days of a window, a day for before and after, and the field's own for a text test.
export function inputTypeOf(name: string, field: Pick<Field, 'inputType'>): InputType | null {
  const kind = namedOperator(name)?.kind
  switch (kind) {
    case 'number':
    case 'withinLast':
      return 'number'
    case 'moment':
      return 'date'
    case 'text':
    case 'presence':
    case undefined:
      return field.inputType ?? null
    default:
      return unnamed(kind)
  }
}
