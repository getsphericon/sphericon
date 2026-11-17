import type { Operator } from 'react-querybuilder'
import { expect, test } from 'vitest'

import {
  ConditionDateOperator,
  ConditionNumberOperator,
  ConditionOperator,
  CustomFieldType,
} from '../../generated/site/types.gen.ts'
import type { SiteCustomFieldResource } from '../../generated/site/types.gen.ts'
import { TIMESTAMPS, testId } from '../../test/payloads.ts'
import { dateWithinOperator } from './condition.ts'
import { inputTypeOf, inputTypeOfField, operatorsOf, triggerOperatorsOf } from './fieldOperators.ts'

const names = (operators: Operator[] | null) => operators?.map((operator) => operator.name)

const field = (
  type: CustomFieldType,
  ruleOperators: ConditionOperator[],
): SiteCustomFieldResource => ({
  id: testId(1),
  key: 'k',
  name: 'K',
  type,
  ruleOperators,
  revision: 1,
  ...TIMESTAMPS,
})

const seats = field(CustomFieldType.NUMBER, [
  ConditionOperator.EQ,
  ConditionOperator.NEQ,
  ConditionOperator.NOT_NULL,
])
const renewsAt = field(CustomFieldType.DATETIME, [ConditionOperator.NOT_NULL])
const plan = field(CustomFieldType.STRING, [ConditionOperator.EQ, ConditionOperator.CONTAINS])

test('a custom field offers the rule operators the server lists, then the leaves of its type', () => {
  const number = names(operatorsOf(seats))
  const datetime = names(operatorsOf(renewsAt))
  const text = names(operatorsOf(plan))

  expect(number).toEqual([
    ConditionOperator.EQ,
    ConditionOperator.NEQ,
    ConditionOperator.NOT_NULL,
    ConditionNumberOperator.GT,
    ConditionNumberOperator.GTE,
    ConditionNumberOperator.LT,
    ConditionNumberOperator.LTE,
  ])
  expect(datetime).toEqual([
    ConditionOperator.NOT_NULL,
    ConditionDateOperator.BEFORE,
    ConditionDateOperator.AFTER,
    dateWithinOperator,
  ])
  expect(text).toEqual([ConditionOperator.EQ, ConditionOperator.CONTAINS])
})

test('an Event property is tested as text or compared as a number, never as a moment', () => {
  const property = names(triggerOperatorsOf('trigger:score'))
  const attribute = triggerOperatorsOf('email')

  expect(property).toContain(ConditionOperator.CONTAINS)
  expect(property).toContain(ConditionNumberOperator.GTE)
  expect(property).not.toContain(dateWithinOperator)
  expect(attribute).toBeNull()
})

test('a comparison and a window take a number, before and after a day, a rule leaf its field’s', () => {
  const numberField = { inputType: inputTypeOfField(seats) }
  const textField = { inputType: inputTypeOfField(plan) }

  const comparison = inputTypeOf(ConditionNumberOperator.LTE, textField)
  const window = inputTypeOf(dateWithinOperator, textField)
  const moment = inputTypeOf(ConditionDateOperator.AFTER, textField)
  const isOnNumber = inputTypeOf(ConditionOperator.EQ, numberField)
  const isOnText = inputTypeOf(ConditionOperator.EQ, textField)

  expect(comparison).toBe('number')
  expect(window).toBe('number')
  expect(moment).toBe('date')
  expect(isOnNumber).toBe('number')
  expect(isOnText).toBe('text')
})
