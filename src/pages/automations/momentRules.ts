import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { siteCustomFieldsListOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteCustomFieldResource } from '../../generated/site/types.gen.ts'
import {
  zCustomFieldType,
  zDateFieldInput,
  zDateOffsetDays,
  zDateRecurrence,
  zLocalTime,
  zWeekday,
} from '../../generated/site/zod.gen.ts'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'
import { boundsOf } from '../../utils/bounds.ts'
import { allSteps } from './definition.ts'
import type { DateDraft, Flow, OffsetSign, TimeOfDayDraft } from './definition.ts'

// The rules of the moments a date Trigger and a Wait until name (ADR 0126), checked in the editor on
// blur and on Save (docs/standards/ui.md) over the drafts the forms hold: what the contract refuses
// (the generated schemas' field, recurrence, offset and local time), and what the server checks
// against the Workspace's catalogue, said under the field before the server says it.

// The most days an offset counts before the date or after it, as the contract bounds it.
const offset = boundsOf(zDateOffsetDays)
export function offsetDaysBound(sign: OffsetSign): number {
  return sign < 0 ? -offset.min : offset.max
}

// The days of an offset as typed: a whole number, '' while blank (a missing value, which the zod
// error map words).
const days = z.union([z.literal(''), z.number()]).pipe(z.int().min(0))

// The field of a date as picked: null until one is (a missing value, as for days).
const field = z.union([z.null(), zDateFieldInput]).pipe(zDateFieldInput)

// The rules of a Wait until's time of day: its local time and its weekdays, as the contract takes
// them.
export const timeOfDaySchema = z.object({
  localTime: zLocalTime,
  weekdays: z.array(zWeekday),
}) satisfies z.ZodType<unknown, TimeOfDayDraft>

// What the date rules say in words of their own.
interface DateRuleTexts {
  birthdayYearly: string
  notDatetime: (key: string) => string
  daysBound: (max: number) => string
}

// The rules of a date: the contract's on each part, and three it states only in words. The birthday
// is always past, so it is only yearly (decision 5 of #1613); the offset holds within the
// contract's bound on its side of the date; and a Custom field must be one of the catalogue's datetime
// fields, while the catalogue is read (`datetimeKeys`; null skips the rule).
function dateSchema(texts: DateRuleTexts, datetimeKeys: ReadonlySet<string> | null) {
  return z
    .object({
      field: field.superRefine((picked, context) => {
        if (
          picked.kind === 'customField' &&
          datetimeKeys !== null &&
          !datetimeKeys.has(picked.key)
        ) {
          context.addIssue({ code: 'custom', message: texts.notDatetime(picked.key) })
        }
      }),
      recurrence: zDateRecurrence,
      days,
      sign: z.union([z.literal(-1), z.literal(1)]),
      localTime: zLocalTime,
    })
    .superRefine((draft, context) => {
      if (draft.field.kind === 'birthday' && draft.recurrence === zDateRecurrence.enum.once) {
        context.addIssue({ code: 'custom', path: ['recurrence'], message: texts.birthdayYearly })
      }
      const bound = offsetDaysBound(draft.sign)
      if (draft.days > bound) {
        context.addIssue({ code: 'custom', path: ['days'], message: texts.daysBound(bound) })
      }
    }) satisfies z.ZodType<unknown, DateDraft>
}

export type DateSchema = ReturnType<typeof dateSchema>

// Whether the flow names a date anywhere: its Trigger, or a Wait until on a date. Only then is
// the catalogue read.
export function namesDate(flow: Flow): boolean {
  return (
    flow.trigger.kind === 'date' ||
    allSteps(flow.path).some((step) => step.kind === 'wait_until' && step.draft.form === 'date')
  )
}

// The datetime Custom fields of the Workspace's catalogue (ADR 0006): what a date can name beside
// the birthday. Undefined until read; read only while `enabled`.
export function useDatetimeFields(enabled = true): SiteCustomFieldResource[] | undefined {
  const { workspaceId } = workspaceRoute.useParams()
  const catalogue = useQuery({
    ...siteCustomFieldsListOptions({ path: { workspaceId } }),
    enabled,
  })
  return useMemo(
    () => catalogue.data?.items.filter((f) => f.type === zCustomFieldType.enum.datetime),
    [catalogue.data],
  )
}

// useDateSchema is the rules of a date in the Workspace's words and against its catalogue, once
// read: the date form's, and the check of every date node before a Save.
export function useDateSchema(datetimeFields: SiteCustomFieldResource[] | undefined): DateSchema {
  const { t } = useTranslation()
  return useMemo(
    () =>
      dateSchema(
        {
          birthdayYearly: t(($) => $.automations.date.birthdayYearly),
          notDatetime: (key) => t(($) => $.automations.date.notDatetime, { key }),
          daysBound: (max) => t(($) => $.automations.date.daysBound, { max }),
        },
        datetimeFields === undefined ? null : new Set(datetimeFields.map((f) => f.key)),
      ),
    [t, datetimeFields],
  )
}
