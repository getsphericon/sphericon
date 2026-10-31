import type { Field, InputType, Operator } from 'react-querybuilder'

import {
  SiteConditionDateOperator,
  SiteConditionNumberOperator,
  SiteConditionOperator,
  SiteCustomFieldType,
} from '../../generated/site/types.gen.ts'
import type { SiteCustomFieldResource } from '../../generated/site/types.gen.ts'
import { dateWithinOperator } from './condition.ts'
import { triggerFieldPrefix } from './triggerFieldPrefix.ts'

// How the builder names each operator of a rule leaf.
const ruleOperatorLabels: Record<SiteConditionOperator, string> = {
  [SiteConditionOperator.EQ]: 'is',
  [SiteConditionOperator.NEQ]: 'is not',
  [SiteConditionOperator.CONTAINS]: 'contains',
  [SiteConditionOperator.BEGINS_WITH]: 'begins with',
  [SiteConditionOperator.NULL]: 'is empty',
  [SiteConditionOperator.NOT_NULL]: 'is set',
  [SiteConditionOperator.HAS]: 'has',
  [SiteConditionOperator.DOES_NOT_HAVE]: 'does not have',
}

function labelled(operator: SiteConditionOperator): Operator {
  return { name: operator, label: ruleOperatorLabels[operator] }
}

// A contact attribute is text.
export const textOperators: Operator[] = [
  SiteConditionOperator.EQ,
  SiteConditionOperator.NEQ,
  SiteConditionOperator.CONTAINS,
  SiteConditionOperator.BEGINS_WITH,
  SiteConditionOperator.NULL,
  SiteConditionOperator.NOT_NULL,
].map((operator) => labelled(operator))

// A number leaf compares a number custom field or an Event property with a number.
const numberOperators: Operator[] = [
  { name: SiteConditionNumberOperator.GT, label: 'greater than' },
  { name: SiteConditionNumberOperator.GTE, label: 'greater than or equal to' },
  { name: SiteConditionNumberOperator.LT, label: 'less than' },
  { name: SiteConditionNumberOperator.LTE, label: 'less than or equal to' },
]

// The date leaves test a datetime custom field against a day, or a window in days back from now.
const dateOperators: Operator[] = [
  { name: SiteConditionDateOperator.BEFORE, label: 'before' },
  { name: SiteConditionDateOperator.AFTER, label: 'after' },
  { name: dateWithinOperator, label: 'within the last (days)' },
]

// The leaves a custom field of a type takes besides the rule leaf (the contract's number and date
// leaves).
const typedOperators: Partial<Record<SiteCustomFieldType, Operator[]>> = {
  [SiteCustomFieldType.NUMBER]: numberOperators,
  [SiteCustomFieldType.DATETIME]: dateOperators,
}

// operatorsOf is what the builder offers on a custom field: the rule leaf's operators the server
// lists for it (ruleOperators, the backend's one table of what fits a type), then the comparisons
// of a number field or the moments of a datetime field.
export function operatorsOf(field: SiteCustomFieldResource): Operator[] {
  return [
    ...field.ruleOperators.map((operator) => labelled(operator)),
    ...(typedOperators[field.type] ?? []),
  ]
}

// The value input a custom field takes for its rule leaf: a number field's is a number.
export function inputTypeOfField(field: SiteCustomFieldResource): InputType {
  return field.type === SiteCustomFieldType.NUMBER ? 'number' : 'text'
}

// A Rule's Event property ("trigger:<key>") has no declared type: it is tested as text, or
// compared as a number.
const triggerOperators: Operator[] = [...textOperators, ...numberOperators]

// triggerOperatorsOf is the operators of a field typed as an Event property, or null to keep the
// field's own.
export function triggerOperatorsOf(field: string): Operator[] | null {
  return field.startsWith(triggerFieldPrefix) ? triggerOperators : null
}

const numberInput = new Set<string>([
  ...numberOperators.map((operator) => operator.name),
  dateWithinOperator,
])
const dateInput = new Set<string>([
  SiteConditionDateOperator.BEFORE,
  SiteConditionDateOperator.AFTER,
])

// inputTypeOf is the value input an operator on the field takes: a number for a comparison and for
// the days of within, a day for before and after, and the field's own for a rule leaf.
export function inputTypeOf(operator: string, field: Pick<Field, 'inputType'>): InputType | null {
  if (numberInput.has(operator)) {
    return 'number'
  }
  if (dateInput.has(operator)) {
    return 'date'
  }
  return field.inputType ?? null
}
