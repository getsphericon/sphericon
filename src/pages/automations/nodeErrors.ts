import { useTranslation } from 'react-i18next'
import type * as z from 'zod'

import { allSteps, isFork } from './definition.ts'
import type {
  DateDraft,
  Flow,
  FlowPath,
  FlowStep,
  TimeOfDayDraft,
  WaitForEventDraft,
} from './definition.ts'
import { TRIGGER_NODE_ID } from './flowLayout.ts'
import { forkError } from './forkRules.ts'
import type { ForkError } from './forkRules.ts'
import { messageStepsOf } from './messageSteps.ts'
import { namesDate, timeOfDaySchema, useDateSchema, useDatetimeFields } from './momentRules.ts'
import { useStepInfo } from './stepKinds.ts'
import { useWaitForEventSchema } from './waitForEventRules.ts'

// What is wrong with a node of the canvas: the Trigger's date, a fork's paths, a Wait until's
// moment or what a Wait for event awaits, a send Step it names among them, which must come before
// it. A Save checks every node and sends nothing while one is wrong; the node shows its error once
// it was checked (on blur or on Save), and its panel shows each under its field.
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
  const waitForEventSchema = useWaitForEventSchema()
  const infoOf = useStepInfo()
  const labels: Record<keyof DateDraft | keyof TimeOfDayDraft, string> = {
    field: t(($) => $.automations.date.fieldLabel),
    recurrence: t(($) => $.automations.date.recurrenceLabel),
    days: t(($) => $.automations.date.daysLabel),
    sign: t(($) => $.automations.date.directionLabel),
    localTime: t(($) => $.automations.localTimeLabel),
    weekdays: t(($) => $.automations.weekdaysLabel),
  }

  const waitForEventLabels: Record<'name' | 'timeoutSeconds' | 'messageStep', string> = {
    name: t(($) => $.automations.waitForEvent.nameLabel),
    timeoutSeconds: t(($) => $.automations.waitForEvent.timeoutLabel),
    messageStep: t(($) => $.automations.waitForEvent.messageStepLabel),
  }

  // The first thing wrong with a draft: a part left blank by its label, else the rule's own words.
  const draftError = <T extends DateDraft | TimeOfDayDraft | WaitForEventDraft>(
    schema: z.ZodType<unknown, T>,
    draft: T,
    named: Record<string, string>,
  ): string | null => {
    const [issue] = schema.safeParse(draft).error?.issues ?? []
    if (issue === undefined) {
      return null
    }
    const [part] = issue.path
    if (typeof part === 'string' && Object.hasOwn(named, part)) {
      const value: unknown = Reflect.get(draft, part)
      if (value === '' || value === null) {
        return t(($) => $.automations.fieldRequired, { field: Reflect.get(named, part) })
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

  const momentError = <T extends DateDraft | TimeOfDayDraft>(
    schema: z.ZodType<unknown, T>,
    draft: T,
  ): string | null => draftError(schema, draft, labels)

  const stepError = (root: FlowPath, step: FlowStep): string | null => {
    if (step.kind === 'wait_for_event') {
      const schema = waitForEventSchema(messageStepsOf(infoOf, root, step.id))
      return draftError(schema, step.draft, waitForEventLabels)
    }
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
        const message = stepError(current.path, step)
        return message === null ? [] : [{ id: step.id, message }]
      }),
    ]
  }
}
