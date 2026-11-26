import * as z from 'zod'

import type {
  FrequencyCap,
  FrequencyCapChannel,
  SendingWindow,
  SiteSendingRulesResource,
  Weekday,
} from '../../../generated/site/types.gen.ts'
import {
  zCountryCode,
  zFrequencyCapChannel,
  zFrequencyCapMaxMessages,
  zFrequencyCapPeriodMinutes,
  zMinuteOfDay,
  zSendingRules,
  zWeekday,
} from '../../../generated/site/zod.gen.ts'

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

// Whether a cap or a window may be set for a country: only on a phone Channel, by the number's
// country.
export function takesCountry(channel: FrequencyCapChannel): boolean {
  return channel === 'sms'
}

// The form's values: a blank country is the Channel's own rule; a cap with a blank maximum and
// period removes the cap there; a day of a window is open from opensAt to closesAt.
type SendingRulesValues = z.input<ReturnType<typeof sendingRulesSchema>>
type CapValues = SendingRulesValues['frequencyCaps'][number]
type WindowValues = SendingRulesValues['sendingWindows'][number]
type DayValues = WindowValues['days'][Weekday]

// A day a new window or a closed day starts with when it opens: the default hours.
const DEFAULT_DAY: DayValues = { open: true, opensAt: '08:00', closesAt: '20:00' }

export function newCap(): CapValues {
  return { channel: 'sms', country: '', maxMessages: '', periodMinutes: '' }
}

// The values of each day of the week, from what dayValues makes of it.
function byDay(dayValues: (day: Weekday) => DayValues): Record<Weekday, DayValues> {
  return {
    monday: dayValues('monday'),
    tuesday: dayValues('tuesday'),
    wednesday: dayValues('wednesday'),
    thursday: dayValues('thursday'),
    friday: dayValues('friday'),
    saturday: dayValues('saturday'),
    sunday: dayValues('sunday'),
  }
}

export function newWindow(): WindowValues {
  return { country: '', days: byDay(() => ({ ...DEFAULT_DAY })) }
}

// The rules as the form edits them.
export function toValues(rules: SiteSendingRulesResource): SendingRulesValues {
  return {
    frequencyCaps: rules.frequencyCaps.map((cap) => ({
      channel: cap.channel,
      country: cap.country ?? '',
      maxMessages: cap.maxMessages ?? '',
      periodMinutes: cap.periodMinutes ?? '',
    })),
    sendingWindows: rules.sendingWindows.map((window) => ({
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
      channel: 'sms',
      country: value.country,
      hours: WEEKDAYS.filter((d) => value.days[d].open).map((d) => ({
        day: d,
        opensAt: minuteOf(value.days[d].opensAt) ?? 0,
        closesAt: closingMinuteOf(value.days[d].closesAt) ?? 0,
      })),
    }))
  return zSendingRules.extend({ frequencyCaps: z.array(cap), sendingWindows: z.array(window) })
}
