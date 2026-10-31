import dayjs from 'dayjs'
import localizedFormat from 'dayjs/plugin/localizedFormat.js'
import relativeTime from 'dayjs/plugin/relativeTime.js'
import timezone from 'dayjs/plugin/timezone.js'
import utc from 'dayjs/plugin/utc.js'

import { resolveLocale } from '../i18n.ts'

// Every date the app shows goes through here. Dates follow the instance locale
// (SPHERICON_APP_LOCALE), not the browser's: dayjs is set to the resolved app locale once.
//
// timeZone is the IANA zone to show the instant in; undefined is the browser's. A signed-in page
// formats through useDateFormat, which passes the User time zone (ADR 0068).
dayjs.extend(localizedFormat)
dayjs.extend(relativeTime)
dayjs.extend(utc)
dayjs.extend(timezone)
dayjs.locale(resolveLocale())

type DateInput = string | number | Date

function inZone(value: DateInput, timeZone: string | undefined) {
  return timeZone === undefined ? dayjs(value) : dayjs(value).tz(timeZone)
}

export function formatDateTime(value: DateInput, timeZone?: string): string {
  return inZone(value, timeZone).format('L LTS')
}

export function formatDate(value: DateInput, timeZone?: string): string {
  return inZone(value, timeZone).format('L')
}

// The instant a calendar day ("2026-03-01") begins in the zone, as an ISO timestamp.
export function startOfDay(day: string, timeZone?: string): string {
  const start = timeZone === undefined ? dayjs(day) : dayjs.tz(day, timeZone)
  return start.toISOString()
}

// The calendar day after day.
export function nextDay(day: string): string {
  return dayjs.utc(day).add(1, 'day').format('YYYY-MM-DD')
}

// The calendar day ("2026-03-01") the instant falls on in the zone.
export function dayOf(value: DateInput, timeZone?: string): string {
  return inZone(value, timeZone).format('YYYY-MM-DD')
}

// A date spelled out ("October 30, 2026"), for a sentence rather than a table cell.
export function formatLongDate(value: DateInput): string {
  return dayjs(value).format('LL')
}

// How long ago (or how far ahead) the moment is, as "5 minutes ago".
export function formatRelative(value: DateInput): string {
  return dayjs(value).fromNow()
}

// The browser's own zone: what the app shows times in while the User has chosen none.
export function browserTimeZone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone
}

// Every zone a Workspace or a User may choose, UTC first.
export const TIME_ZONES = [
  'UTC',
  ...Intl.supportedValuesOf('timeZone').filter((zone) => zone !== 'UTC'),
]
