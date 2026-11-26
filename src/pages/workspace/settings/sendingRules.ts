import * as z from 'zod'

import type { FrequencyCap, SendingWindow, Weekday } from '../../../generated/site/types.gen.ts'
import { SendingWindowChannel } from '../../../generated/site/types.gen.ts'
import {
  zCountryCode,
  zFrequencyCapChannel,
  zFrequencyCapMaxMessages,
  zFrequencyCapPeriodMinutes,
  zMinuteOfDay,
  zSendingRules,
  zWeekday,
} from '../../../generated/site/zod.gen.ts'
import { i18n } from '../../../i18n.ts'
import type { ResourceFormSchema, ResourceValues } from '../../../resources/resourceFormSchema.ts'
import { takesCountry } from '../../../utils/sendingRules.ts'

// The days of the week, Monday first, and the Channels a Frequency cap counts on, both in the
// contract's order.
export const WEEKDAYS = zWeekday.options
export const CAP_CHANNELS = zFrequencyCapChannel.options

const MINUTES_PER_DAY = zMinuteOfDay.maxValue ?? 0
const MINUTES_PER_HOUR = 60

// A minute of the day as a clock reads it ("08:00"); the end of the day reads "24:00".
export function clock(minute: number): string {
  const hours = Math.floor(minute / MINUTES_PER_HOUR)
  const minutes = minute % MINUTES_PER_HOUR
  return `${String(hours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}`
}

// The minute of the day a time input holds ("08:30"), or null for none.
function minuteOf(time: string): number | null {
  const match = /^(\d{2}):(\d{2})/.exec(time)
  if (!match) {
    return null
  }
  return Number(match[1]) * MINUTES_PER_HOUR + Number(match[2])
}

// A window's closing time read as a minute of the day: 00:00 is the end of the day, which a time
// input cannot hold as 24:00.
function closingMinuteOf(time: string): number | null {
  const minute = minuteOf(time)
  return minute === 0 ? MINUTES_PER_DAY : minute
}

// A period in minutes in the largest unit that divides it: days, hours or minutes.
export function periodIn(minutes: number): { unit: 'days' | 'hours' | 'minutes'; count: number } {
  if (minutes % MINUTES_PER_DAY === 0) {
    return { unit: 'days', count: minutes / MINUTES_PER_DAY }
  }
  if (minutes % MINUTES_PER_HOUR === 0) {
    return { unit: 'hours', count: minutes / MINUTES_PER_HOUR }
  }
  return { unit: 'minutes', count: minutes }
}

// The form's values: a blank country is the Channel's own rule; a cap with a blank maximum and
// period removes the cap there; a day of a window is open from opensAt to closesAt. The payload is
// what a save sends besides the revision, which the editor session adds.
export type SendingRulesValues = z.input<ReturnType<typeof sendingRulesSchema>>
export type SendingRulesPayload = z.output<ReturnType<typeof sendingRulesSchema>>
type CapValues = SendingRulesValues['frequencyCaps'][number]
type WindowValues = SendingRulesValues['sendingWindows'][number]
type DayValues = WindowValues['days'][Weekday]

// A day a new window or a closed day starts with when it opens: the default hours.
const DEFAULT_DAY: DayValues = { open: true, opensAt: '08:00', closesAt: '20:00' }

export function newCap(): CapValues {
  return { channel: 'sms', country: '', maxMessages: '', periodMinutes: '' }
}

// A week of day values, one per day of the contract's week: the record has every day or fails.
const week = z.record(zWeekday, z.custom<DayValues>())

// The values of each day of the week, from what dayValues makes of it.
function byDay(dayValues: (day: Weekday) => DayValues): Record<Weekday, DayValues> {
  return week.parse(Object.fromEntries(WEEKDAYS.map((day) => [day, dayValues(day)])))
}

export function newWindow(): WindowValues {
  return { country: '', days: byDay(() => ({ ...DEFAULT_DAY })) }
}

// The rules as the form edits them.
export function toValues(rules: ResourceValues<SendingRulesPayload>): SendingRulesValues {
  return {
    frequencyCaps: (rules.frequencyCaps ?? []).map((cap) => ({
      channel: cap.channel,
      country: cap.country ?? '',
      maxMessages: cap.maxMessages ?? '',
      periodMinutes: cap.periodMinutes ?? '',
    })),
    sendingWindows: (rules.sendingWindows ?? []).map((window) => ({
      country: window.country ?? '',
      days: byDay((day) => {
        const hours = window.hours.find((h) => h.day === day)
        return hours
          ? {
              open: true,
              opensAt: clock(hours.opensAt),
              closesAt: clock(hours.closesAt === MINUTES_PER_DAY ? 0 : hours.closesAt),
            }
          : { ...DEFAULT_DAY, open: false }
      }),
    })),
  }
}

// The messages of what the form refuses beyond the contract's own bounds, already translated.
interface SendingRulesMessages {
  countryOffPhone: string
  capIncomplete: string
  neverOpens: string
  closesAfterOpens: string
  timeRequired: string
  ruleRepeated: string
}

// A blank country is none; one typed in lower case is the same country.
const country = z
  .string()
  .trim()
  .toUpperCase()
  .pipe(z.union([z.literal(''), zCountryCode]))
  .transform((code) => (code === '' ? null : code))

// The form's schema: what it checks before a save and the body the save sends.
export function sendingRulesSchema(messages: SendingRulesMessages) {
  const cap = z
    .object({
      channel: zFrequencyCapChannel,
      country,
      maxMessages: z.union([z.literal(''), zFrequencyCapMaxMessages]),
      periodMinutes: z.union([z.literal(''), zFrequencyCapPeriodMinutes]),
    })
    .superRefine((value, ctx) => {
      if (value.country !== null && !takesCountry(value.channel)) {
        ctx.addIssue({ code: 'custom', path: ['country'], message: messages.countryOffPhone })
      }
      if ((value.maxMessages === '') !== (value.periodMinutes === '')) {
        const path = value.maxMessages === '' ? 'maxMessages' : 'periodMinutes'
        ctx.addIssue({ code: 'custom', path: [path], message: messages.capIncomplete })
      }
    })
    .transform((value): FrequencyCap => ({
      channel: value.channel,
      country: value.country,
      maxMessages: value.maxMessages === '' ? null : value.maxMessages,
      periodMinutes: value.periodMinutes === '' ? null : value.periodMinutes,
    }))
  const day = z.object({ open: z.boolean(), opensAt: z.string(), closesAt: z.string() })
  const window = z
    .object({ country, days: z.record(zWeekday, day) })
    .superRefine((value, ctx) => {
      const open = WEEKDAYS.filter((d) => value.days[d].open)
      if (open.length === 0) {
        ctx.addIssue({ code: 'custom', path: ['country'], message: messages.neverOpens })
      }
      for (const d of open) {
        const opens = minuteOf(value.days[d].opensAt)
        const closes = closingMinuteOf(value.days[d].closesAt)
        if (opens === null) {
          ctx.addIssue({
            code: 'custom',
            path: ['days', d, 'opensAt'],
            message: messages.timeRequired,
          })
        }
        if (closes === null) {
          ctx.addIssue({
            code: 'custom',
            path: ['days', d, 'closesAt'],
            message: messages.timeRequired,
          })
        }
        if (opens !== null && closes !== null && closes <= opens) {
          ctx.addIssue({
            code: 'custom',
            path: ['days', d, 'closesAt'],
            message: messages.closesAfterOpens,
          })
        }
      }
    })
    .transform((value): SendingWindow => ({
      channel: SendingWindowChannel.SMS,
      country: value.country,
      hours: WEEKDAYS.filter((d) => value.days[d].open).map((d) => ({
        day: d,
        opensAt: minuteOf(value.days[d].opensAt) ?? 0,
        closesAt: closingMinuteOf(value.days[d].closesAt) ?? 0,
      })),
    }))
  // A Workspace has one cap and one window per Channel and country: a second is refused under
  // the field that names its key, its country, or its Channel for the Channel's own.
  return zSendingRules
    .extend({ frequencyCaps: z.array(cap), sendingWindows: z.array(window) })
    .superRefine((value, ctx) => {
      const repeated = (
        list: 'frequencyCaps' | 'sendingWindows',
        keys: { channel: string; country: string | null }[],
      ) => {
        const seen = new Set<string>()
        for (const [i, key] of keys.entries()) {
          const id = `${key.channel}:${key.country ?? ''}`
          if (seen.has(id)) {
            const field = key.country === null && list === 'frequencyCaps' ? 'channel' : 'country'
            ctx.addIssue({ code: 'custom', path: [list, i, field], message: messages.ruleRepeated })
          }
          seen.add(id)
        }
      }
      repeated('frequencyCaps', value.frequencyCaps)
      repeated('sendingWindows', value.sendingWindows)
    })
}

// The messages of the form's schema, translated when an issue is raised.
const translated: SendingRulesMessages = {
  get countryOffPhone() {
    return i18n.t(($) => $.settings.sendingRules.countryOffPhone)
  },
  get capIncomplete() {
    return i18n.t(($) => $.settings.sendingRules.capIncomplete)
  },
  get neverOpens() {
    return i18n.t(($) => $.settings.sendingRules.neverOpens)
  },
  get closesAfterOpens() {
    return i18n.t(($) => $.settings.sendingRules.closesAfterOpens)
  },
  get timeRequired() {
    return i18n.t(($) => $.settings.sendingRules.timeRequired)
  },
  get ruleRepeated() {
    return i18n.t(($) => $.settings.sendingRules.ruleRepeated)
  },
}

const schema = sendingRulesSchema(translated)

// The rules' form in an editor session (defineModalEditor): the form's values to and from the save.
export const sendingRulesFormSchema: ResourceFormSchema<SendingRulesPayload, SendingRulesValues> = {
  blank: { frequencyCaps: [], sendingWindows: [] },
  create: schema,
  update: schema,
  toValues,
}

// The field of the form that shows a refusal the server reports at path (its problem's `errors`),
// for the values the refused save was made of. The server names a window's day by its place among
// the hours sent (sendingWindows.<i>.hours.<j>), which are the open days in the week's order; the
// form names it by its day (sendingWindows.<i>.days.<day>). A window's hours as a whole, and its
// Channel, which the form does not show, are its country's field, where the form puts its own
// refusals of a window.
export function formPathOf(path: string, values: SendingRulesValues): string {
  const day = /^sendingWindows\.(\d+)\.hours\.(\d+)(?:\.(\w+))?$/.exec(path)
  if (day) {
    const [, i, j, field] = day
    const window = values.sendingWindows[Number(i)]
    const open = window ? WEEKDAYS.filter((d) => window.days[d].open) : []
    const weekday = open[Number(j)]
    if (weekday === undefined) {
      return path
    }
    // A time is its input's; the day itself (listed twice) is its checkbox.
    const input = field === 'opensAt' || field === 'closesAt' ? field : 'open'
    return `sendingWindows.${i}.days.${weekday}.${input}`
  }
  const whole = /^sendingWindows\.(\d+)\.(?:hours|channel)$/.exec(path)
  if (whole) {
    return `sendingWindows.${whole[1]}.country`
  }
  return path
}
