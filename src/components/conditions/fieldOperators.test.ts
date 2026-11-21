import type { Operator } from 'react-querybuilder'
import { expect, test } from 'vitest'

import { CustomFieldType } from '../../generated/site/types.gen.ts'
import type {
  CustomFieldConditionTests,
  SiteCustomFieldResource,
} from '../../generated/site/types.gen.ts'
import { TIMESTAMPS, testId } from '../../test/payloads.ts'
import {
  attributeOperators,
  inputTypeOf,
  inputTypeOfField,
  operatorsOf,
  propertyOperators,
} from './fieldOperators.ts'

const names = (operators: Operator[]) => operators.map((operator) => operator.name)

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
  text: ['eq', 'neq'],
  number: ['gt', 'gte', 'lt', 'lte'],
  moment: [],
  withinLast: false,
  presence: ['set'],
})
const plan = field(CustomFieldType.STRING, {
  text: ['eq', 'neq', 'contains'],
  number: [],
  moment: [],
  withinLast: false,
  presence: ['set'],
})
const renewsAt = field(CustomFieldType.DATETIME, {
  text: [],
  number: [],
  moment: ['before', 'after'],
  withinLast: true,
  presence: ['set'],
})

test('a number Custom field offers text eq and neq, the number comparisons and presence', () => {
  const offered = operatorsOf(seats)

  expect(names(offered)).toEqual(['eq', 'neq', 'gt', 'gte', 'lt', 'lte', 'set'])
})

test('a string Custom field offers the text operators it takes and presence', () => {
  const offered = operatorsOf(plan)

  expect(names(offered)).toEqual(['eq', 'neq', 'contains', 'set'])
})

test('a datetime Custom field offers the moment operators, the window of days and presence', () => {
  const offered = operatorsOf(renewsAt)

  expect(names(offered)).toEqual(['before', 'after', 'withinLast', 'set'])
})

test('a presence test takes no value on a Custom field or an attribute', () => {
  const onField = operatorsOf(plan).find((operator) => operator.name === 'set')
  const onAttribute = attributeOperators.filter((operator) => operator.arity === 'unary')

  expect(onField?.arity).toBe('unary')
  expect(names(onAttribute)).toEqual(['set', 'notSet'])
})

test('an Event property is tested as text, compared as a number or tested for presence, never as a moment', () => {
  const property = names(propertyOperators)

  expect(property).toEqual([
    'eq',
    'neq',
    'contains',
    'beginsWith',
    'gt',
    'gte',
    'lt',
    'lte',
    'set',
    'notSet',
  ])
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
