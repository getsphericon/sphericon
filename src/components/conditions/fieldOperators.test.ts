import type { Operator } from 'react-querybuilder'
import { expect, test } from 'vitest'

import {
  SiteConditionDateOperator,
  SiteConditionNumberOperator,
  SiteConditionOperator,
  SiteCustomFieldType,
} from '../../generated/site/types.gen.ts'
import type { SiteCustomFieldResource } from '../../generated/site/types.gen.ts'
import { TIMESTAMPS } from '../../test/payloads.ts'
import { dateWithinOperator } from './condition.ts'
import { inputTypeOf, inputTypeOfField, operatorsOf, triggerOperatorsOf } from './fieldOperators.ts'

const names = (operators: Operator[] | null) => operators?.map((operator) => operator.name)

const field = (
  type: SiteCustomFieldType,
  ruleOperators: SiteConditionOperator[],
): SiteCustomFieldResource => ({ id: '1', key: 'k', name: 'K', type, ruleOperators, ...TIMESTAMPS })

const seats = field(SiteCustomFieldType.NUMBER, [
  SiteConditionOperator.EQ,
  SiteConditionOperator.NEQ,
  SiteConditionOperator.NOT_NULL,
])
const renewsAt = field(SiteCustomFieldType.DATETIME, [SiteConditionOperator.NOT_NULL])
const plan = field(SiteCustomFieldType.STRING, [
  SiteConditionOperator.EQ,
  SiteConditionOperator.CONTAINS,
])

test('a custom field offers the rule operators the server lists, then the leaves of its type', () => {
  const number = names(operatorsOf(seats))
  const datetime = names(operatorsOf(renewsAt))
  const text = names(operatorsOf(plan))

  expect(number).toEqual([
    SiteConditionOperator.EQ,
    SiteConditionOperator.NEQ,
    SiteConditionOperator.NOT_NULL,
    SiteConditionNumberOperator.GT,
    SiteConditionNumberOperator.GTE,
    SiteConditionNumberOperator.LT,
    SiteConditionNumberOperator.LTE,
  ])
  expect(datetime).toEqual([
    SiteConditionOperator.NOT_NULL,
    SiteConditionDateOperator.BEFORE,
    SiteConditionDateOperator.AFTER,
    dateWithinOperator,
  ])
  expect(text).toEqual([SiteConditionOperator.EQ, SiteConditionOperator.CONTAINS])
})

test('an Event property is tested as text or compared as a number, never as a moment', () => {
  const property = names(triggerOperatorsOf('trigger:score'))
  const attribute = triggerOperatorsOf('email')

  expect(property).toContain(SiteConditionOperator.CONTAINS)
  expect(property).toContain(SiteConditionNumberOperator.GTE)
  expect(property).not.toContain(dateWithinOperator)
  expect(attribute).toBeNull()
})

test('a comparison and a window take a number, before and after a day, a rule leaf its field’s', () => {
  const numberField = { inputType: inputTypeOfField(seats) }
  const textField = { inputType: inputTypeOfField(plan) }

  const comparison = inputTypeOf(SiteConditionNumberOperator.LTE, textField)
  const window = inputTypeOf(dateWithinOperator, textField)
  const moment = inputTypeOf(SiteConditionDateOperator.AFTER, textField)
  const isOnNumber = inputTypeOf(SiteConditionOperator.EQ, numberField)
  const isOnText = inputTypeOf(SiteConditionOperator.EQ, textField)

  expect(comparison).toBe('number')
  expect(window).toBe('number')
  expect(moment).toBe('date')
  expect(isOnNumber).toBe('number')
  expect(isOnText).toBe('text')
})
