import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import type { MessageStep } from '../../components/conditions/ConditionBuilder.tsx'
import {
  zMessageEngagement,
  zWaitForEventName,
  zWaitForEventStepInput,
} from '../../generated/site/zod.gen.ts'
import { AWAITED_FORMS, awaitedMatchersOf, waitForEventConditionOf } from './definition.ts'
import type { WaitForEventDraft } from './definition.ts'

// The rules of a Wait for event (ADR 0124, ADR 0136, ADR 0180), checked in the editor on blur and
// on Save (docs/standards/ui.md) over the draft its panel holds: what the contract refuses, said
// under the field and on the node before the server says it. Only the form it awaits is checked:
// the other form's draft is kept, never saved. Whether another Step of the Automation holds the
// name, or an exit or the goal ends the Enrollment on an awaited action, only the server knows: its
// refusal lands on the node.

// What the rules say in words of their own. A part left blank is a missing value, which the zod
// error map words.
interface WaitForEventTexts {
  nameFormat: string
  timeoutPositive: string
  matchersRequired: string
  matcherFilterIncomplete: string
  conditionIncomplete: string
  messageStepNotBefore: string
  openNotTracked: string
}

// The send Steps whose message the Wait for event may await: those that can come before it, and
// whether each one's message is opened (an email's) or only clicked.
type AwaitableSteps = readonly Pick<MessageStep, 'id' | 'opens'>[]

function waitForEventSchema(texts: WaitForEventTexts, messageSteps: AwaitableSteps) {
  return z
    .object({
      name: z.string().superRefine((name, context) => {
        if (name === '') {
          context.addIssue({ code: 'custom', input: '' })
        } else if (!zWaitForEventName.safeParse(name).success) {
          context.addIssue({ code: 'custom', message: texts.nameFormat })
        }
      }),
      // In whole seconds, as a wait's delay, within the contract's bound.
      timeoutSeconds: z.union([z.literal(''), z.number()]).superRefine((seconds, context) => {
        if (seconds === '') {
          context.addIssue({ code: 'custom', input: '' })
        } else if (!zWaitForEventStepInput.shape.timeoutSeconds.safeParse(seconds).success) {
          context.addIssue({ code: 'custom', message: texts.timeoutPositive })
        }
      }),
      form: z.enum(AWAITED_FORMS),
      matchers: z.array(z.object({ action: z.string(), filterText: z.string() })),
      conditionText: z.string(),
      messageStep: z.string(),
      engagement: zMessageEngagement,
    })
    .superRefine((draft, context) => {
      const issue = (path: keyof WaitForEventDraft, message?: string) =>
        context.addIssue(
          message === undefined
            ? { code: 'custom', path: [path], input: '' }
            : { code: 'custom', path: [path], message },
        )
      if (draft.form === 'message') {
        // A send Step that can come before the wait; an open only of a message whose opens are
        // tracked, an email's.
        const picked = messageSteps.find((step) => step.id === draft.messageStep)
        if (draft.messageStep === '') {
          issue('messageStep')
        } else if (picked === undefined) {
          issue('messageStep', texts.messageStepNotBefore)
        } else if (draft.engagement === 'opened' && !picked.opens) {
          issue('engagement', texts.openNotTracked)
        }
        return
      }
      // At least one row names an action (a blank row is none), and no filter is still being typed.
      const read = awaitedMatchersOf(draft.matchers)
      if (read.length === 0) {
        issue('matchers', texts.matchersRequired)
      } else if (read.some((matcher) => matcher.incomplete)) {
        issue('matchers', texts.matcherFilterIncomplete)
      }
      // None, or a complete one.
      if (waitForEventConditionOf(draft.conditionText) === null) {
        issue('conditionText', texts.conditionIncomplete)
      }
    }) satisfies z.ZodType<unknown, WaitForEventDraft>
}

export type WaitForEventSchema = ReturnType<typeof waitForEventSchema>

// useWaitForEventSchema is the rules of a Wait for event in the Workspace's words, given the send
// Steps that can come before it: its panel's, and the check of every Wait for event node before a
// Save.
export function useWaitForEventSchema(): (messageSteps: AwaitableSteps) => WaitForEventSchema {
  const { t } = useTranslation()
  return useCallback(
    (messageSteps) =>
      waitForEventSchema(
        {
          nameFormat: t(($) => $.automations.waitForEvent.nameFormat),
          timeoutPositive: t(($) => $.automations.waitForEvent.timeoutPositive),
          matchersRequired: t(($) => $.automations.waitForEvent.matchersRequired),
          matcherFilterIncomplete: t(($) => $.automations.waitForEvent.matcherFilterIncomplete),
          conditionIncomplete: t(($) => $.automations.waitForEvent.conditionIncomplete),
          messageStepNotBefore: t(($) => $.automations.waitForEvent.messageStepNotBefore),
          openNotTracked: t(($) => $.automations.waitForEvent.openNotTracked),
        },
        messageSteps,
      ),
    [t],
  )
}
