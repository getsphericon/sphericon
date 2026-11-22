import type { Operator } from 'react-querybuilder'
import { expect, test } from 'vitest'

import {
  CustomFieldType,
  MomentOperator,
  NumberOperator,
  PresenceOperator,
  TextOperator,
} from '../../generated/site/types.gen.ts'
import type {
  CustomFieldConditionTests,
  SiteCustomFieldResource,
} from '../../generated/site/types.gen.ts'
import { i18n } from '../../i18n.ts'
import { TIMESTAMPS, testId } from '../../test/payloads.ts'
import { withinLastOperator } from './condition.ts'
import {
  attributeOperators,
  inputTypeOf,
  inputTypeOfField,
  operatorsOf,
  propertyOperators,
} from './fieldOperators.ts'

const names = (operators: Operator[]) => operators.map((operator) => operator.name)
const t = i18n.t

const field = (
  type: CustomFieldType,
  conditionTests: CustomFieldConditionTests,
): SiteCustomFieldResource => ({
  id: testId(1),
  key: 'k',
  name: 'K',
  type,
  conditionTests,
  revision: 1,
  ...TIMESTAMPS,
})

// What the server lists for each type (the Condition module's one table of what fits).
const seats = field(CustomFieldType.NUMBER, {
  text: [TextOperator.EQ, TextOperator.NEQ],
  number: [NumberOperator.GT, NumberOperator.GTE, NumberOperator.LT, NumberOperator.LTE],
  moment: [],
  withinLast: false,
  presence: [PresenceOperator.SET],
})
const plan = field(CustomFieldType.STRING, {
  text: [TextOperator.EQ, TextOperator.NEQ, TextOperator.CONTAINS],
  number: [],
  moment: [],
  withinLast: false,
  presence: [PresenceOperator.SET],
})
const renewsAt = field(CustomFieldType.DATETIME, {
  text: [],
  number: [],
  moment: [MomentOperator.BEFORE, MomentOperator.AFTER],
  withinLast: true,
  presence: [PresenceOperator.SET],
})

test('a number Custom field offers text eq and neq, the number comparisons and presence', () => {
  const offered = operatorsOf(t, seats)

  expect(names(offered)).toEqual([
    TextOperator.EQ,
    TextOperator.NEQ,
    NumberOperator.GT,
    NumberOperator.GTE,
    NumberOperator.LT,
    NumberOperator.LTE,
    PresenceOperator.SET,
  ])
})

test('a string Custom field offers the text operators it takes and presence', () => {
  const offered = operatorsOf(t, plan)

  expect(names(offered)).toEqual([
    TextOperator.EQ,
    TextOperator.NEQ,
    TextOperator.CONTAINS,
    PresenceOperator.SET,
  ])
})

test('a datetime Custom field offers the moment operators, the window of days and presence', () => {
  const offered = operatorsOf(t, renewsAt)

  expect(names(offered)).toEqual([
    MomentOperator.BEFORE,
    MomentOperator.AFTER,
    withinLastOperator,
    PresenceOperator.SET,
  ])
})

test('a presence test takes no value on a Custom field or an attribute', () => {
  const onField = operatorsOf(t, plan).find((operator) => operator.name === 'set')
  const onAttribute = attributeOperators(t).filter((operator) => operator.arity === 'unary')

  expect(onField?.arity).toBe('unary')
  expect(names(onAttribute)).toEqual([PresenceOperator.SET, PresenceOperator.NOT_SET])
})

test('an Event property is tested as text, compared as a number or tested for presence, never as a moment', () => {
  const property = names(propertyOperators(t))

  expect(property).toEqual([
    TextOperator.EQ,
    TextOperator.NEQ,
    TextOperator.CONTAINS,
    TextOperator.BEGINS_WITH,
    NumberOperator.GT,
    NumberOperator.GTE,
    NumberOperator.LT,
    NumberOperator.LTE,
    PresenceOperator.SET,
    PresenceOperator.NOT_SET,
  ])
})

test('an operator is named from the catalog by its test', () => {
  const onAttribute = attributeOperators(t).map((operator) => operator.label)

  expect(onAttribute).toEqual(['is', 'is not', 'contains', 'begins with', 'is set', 'is not set'])
})

test('a comparison and a window take a number, before and after a day, a text test its field’s', () => {
  const numberField = { inputType: inputTypeOfField(seats) }
  const textField = { inputType: inputTypeOfField(plan) }

  const comparison = inputTypeOf('lte', textField)
  const window = inputTypeOf('withinLast', textField)
  const moment = inputTypeOf('after', textField)
  const isOnNumber = inputTypeOf('eq', numberField)
  const isOnText = inputTypeOf('eq', textField)

  expect(comparison).toBe('number')
  expect(window).toBe('number')
  expect(moment).toBe('date')
  expect(isOnNumber).toBe('number')
  expect(isOnText).toBe('text')
})
