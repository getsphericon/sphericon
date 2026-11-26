import { useTranslation } from 'react-i18next'
import type * as z from 'zod'

import { allSteps, isFork } from './definition.ts'
import type { DateDraft, Flow, FlowStep, TimeOfDayDraft } from './definition.ts'
import { TRIGGER_NODE_ID } from './flowLayout.ts'
import { forkError } from './forkRules.ts'
import type { ForkError } from './forkRules.ts'
import { namesDate, timeOfDaySchema, useDateSchema, useDatetimeFields } from './momentRules.ts'

// What is wrong with a node of the canvas: the Trigger's date, a fork's paths or a Wait until's
// moment. A Save checks every node and sends nothing while one is wrong; the node shows its error
// once it was checked (on blur or on Save), and its panel shows each under its field.
export interface NodeError {
  id: string
  message: string
}

// useNodeErrors is the check of every node of a flow, the Trigger first, then the Steps in the
// canvas's order. The Workspace's catalogue, which a date's Custom field must be a datetime field
// of, is read while `flow` names a date.
export function useNodeErrors(flow: Flow): (flow: Flow) => NodeError[] {
  const { t } = useTranslation()
  const dateSchema = useDateSchema(useDatetimeFields(namesDate(flow)))
  const labels: Record<keyof DateDraft | keyof TimeOfDayDraft, string> = {
    field: t(($) => $.automations.date.fieldLabel),
    recurrence: t(($) => $.automations.date.recurrenceLabel),
    days: t(($) => $.automations.date.daysLabel),
    sign: t(($) => $.automations.date.directionLabel),
    localTime: t(($) => $.automations.localTimeLabel),
    weekdays: t(($) => $.automations.weekdaysLabel),
  }

  // The first thing wrong with a moment's draft: a part left blank by its label, else the rule's
  // own words.
  const momentError = <T extends DateDraft | TimeOfDayDraft>(
    schema: z.ZodType<unknown, T>,
    draft: T,
  ): string | null => {
    const [issue] = schema.safeParse(draft).error?.issues ?? []
    if (issue === undefined) {
      return null
    }
    const [part] = issue.path
    if (typeof part === 'string' && Object.hasOwn(labels, part)) {
      const value: unknown = Reflect.get(draft, part)
      if (value === '' || value === null) {
        return t(($) => $.automations.fieldRequired, { field: Reflect.get(labels, part) })
      }
    }
    return issue.message
  }

  const forkErrorText = (error: ForkError): string => {
    switch (error.kind) {
      case 'condition':
        return t(($) => $.automations.forkConditionError, { number: error.index + 1 })
      case 'weight':
        return t(($) => $.automations.forkWeightError, { number: error.index + 1 })
      case 'sum':
      default:
        return t(($) => $.automations.weightSumError, { sum: error.sum })
    }
  }

  const stepError = (step: FlowStep): string | null => {
    if (isFork(step)) {
      const error = forkError(step)
      return error === null ? null : forkErrorText(error)
    }
    if (step.kind !== 'wait_until') {
      return null
    }
    return step.draft.form === 'date'
      ? momentError(dateSchema, step.draft.date)
      : momentError(timeOfDaySchema, step.draft.timeOfDay)
  }

  return (current) => {
    const triggerError =
      current.trigger.kind === 'date' ? momentError(dateSchema, current.trigger.date) : null
    return [
      ...(triggerError === null ? [] : [{ id: TRIGGER_NODE_ID, message: triggerError }]),
      ...allSteps(current.path).flatMap((step) => {
        const message = stepError(step)
        return message === null ? [] : [{ id: step.id, message }]
      }),
    ]
  }
}
