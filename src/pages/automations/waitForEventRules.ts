import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { zWaitForEventName } from '../../generated/site/zod.gen.ts'
import { awaitedMatchersOf, waitForEventConditionOf } from './definition.ts'
import type { WaitForEventDraft } from './definition.ts'

// The rules of a Wait for event on an Event (ADR 0124, ADR 0136, ADR 0180), checked in the editor
// on blur and on Save (docs/standards/ui.md) over the draft its panel holds: what the contract
// refuses, said under the field and on the node before the server says it. Whether another Step of
// the Automation holds the name, or an exit or the goal ends the Enrollment on an awaited action,
// only the server knows: its refusal lands on the node.

// What the rules say in words of their own. A part left blank is a missing value, which the zod
// error map words.
interface WaitForEventTexts {
  nameFormat: string
  timeoutPositive: string
  matchersRequired: string
  matcherFilterIncomplete: string
  conditionIncomplete: string
}

function waitForEventSchema(texts: WaitForEventTexts) {
  return z.object({
    name: z.string().superRefine((name, context) => {
      if (name === '') {
        context.addIssue({ code: 'custom', input: '' })
      } else if (!zWaitForEventName.safeParse(name).success) {
        context.addIssue({ code: 'custom', message: texts.nameFormat })
      }
    }),
    // In seconds, as a wait's delay, and more than zero.
    timeoutSeconds: z
      .union([z.literal(''), z.number()])
      .pipe(z.int().min(1, { error: texts.timeoutPositive })),
    // At least one row names an action (a blank row is none), and no filter is still being typed.
    matchers: z
      .array(z.object({ action: z.string(), filterText: z.string() }))
      .superRefine((drafts, context) => {
        const read = awaitedMatchersOf(drafts)
        if (read.length === 0) {
          context.addIssue({ code: 'custom', message: texts.matchersRequired })
        } else if (read.some((matcher) => matcher.incomplete)) {
          context.addIssue({ code: 'custom', message: texts.matcherFilterIncomplete })
        }
      }),
    // None, or a complete one.
    conditionText: z.string().superRefine((text, context) => {
      if (waitForEventConditionOf(text) === null) {
        context.addIssue({ code: 'custom', message: texts.conditionIncomplete })
      }
    }),
  }) satisfies z.ZodType<unknown, WaitForEventDraft>
}

export type WaitForEventSchema = ReturnType<typeof waitForEventSchema>

// useWaitForEventSchema is the rules of a Wait for event in the Workspace's words: its panel's,
// and the check of every Wait for event node before a Save.
export function useWaitForEventSchema(): WaitForEventSchema {
  const { t } = useTranslation()
  return useMemo(
    () =>
      waitForEventSchema({
        nameFormat: t(($) => $.automations.waitForEvent.nameFormat),
        timeoutPositive: t(($) => $.automations.waitForEvent.timeoutPositive),
        matchersRequired: t(($) => $.automations.waitForEvent.matchersRequired),
        matcherFilterIncomplete: t(($) => $.automations.waitForEvent.matcherFilterIncomplete),
        conditionIncomplete: t(($) => $.automations.waitForEvent.conditionIncomplete),
      }),
    [t],
  )
}
