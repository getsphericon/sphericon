import dayjs from 'dayjs'
import customParseFormat from 'dayjs/plugin/customParseFormat.js'
import localizedFormat from 'dayjs/plugin/localizedFormat.js'
import timezone from 'dayjs/plugin/timezone.js'
import utc from 'dayjs/plugin/utc.js'

// Every date the app shows goes through here, written in the locale it is given: a signed-in page
// formats through useFormat, which passes the User's Regional format (#1010) or, while there is
// none, the Instance's language; a public page and the Staff console pass the Instance's. The
// locale is the BCP 47 tag of the Regional format; it decides the order of the parts, the
// separators and the 12 or 24 hour clock, and it never touches the language of the UI.
//
// timeZone is the IANA zone to show the instant in; undefined is the browser's (ADR 0068).
// dayjs only does the calendar arithmetic and the zone conversion; the localized format plugin is
// what the date inputs' `L` and `LL` formats read, and the custom parse format one makes them read
// a typed date in that format rather than by the browser's guess.
dayjs.extend(customParseFormat)
dayjs.extend(localizedFormat)
dayjs.extend(utc)
dayjs.extend(timezone)

type DateInput = string | number | Date

function inZone(value: DateInput, timeZone: string | undefined) {
  return timeZone === undefined ? dayjs(value) : dayjs(value).tz(timeZone)
}

// The instant a value names, read the way dayjs reads it: a bare calendar day ("1990-05-01", a
// birthday) is that day's start in the browser's zone, not in UTC, so it never shows as the day
// before west of Greenwich.
function instantOf(value: DateInput): Date {
  return dayjs(value).toDate()
}

// A bare calendar day ("2026-12-31"): a day of the calendar, not an instant, so no time zone moves it.
function isCalendarDay(value: DateInput): value is string {
  return typeof value === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(value)
}

const dateFormats = new Map<string, Intl.DateTimeFormat>()

// An Intl.DateTimeFormat for the locale, zone and options, built once.
function dateFormat(
  locale: string,
  timeZone: string | undefined,
  options: Intl.DateTimeFormatOptions,
): Intl.DateTimeFormat {
  const key = JSON.stringify([locale, timeZone, options])
  let format = dateFormats.get(key)
  if (format === undefined) {
    format = new Intl.DateTimeFormat(locale, { ...options, timeZone })
    dateFormats.set(key, format)
  }
  return format
}

const SHORT_DATE: Intl.DateTimeFormatOptions = {
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
}
// The locale's own medium time: "09:00:00" in de-DE and en-GB, "9:00:00 AM" in en-US.
const TIME: Intl.DateTimeFormatOptions = { timeStyle: 'medium' }

// A calendar day ("2026-12-31", a chart's day or a birthday) written in the locale, the same day
// whatever zone the User or the browser is in.
export function formatDay(day: string, locale: string): string {
  return dateFormat(locale, 'UTC', SHORT_DATE).format(new Date(`${day}T12:00:00Z`))
}

export function formatDate(value: DateInput, locale: string, timeZone?: string): string {
  if (isCalendarDay(value)) {
    return formatDay(value, locale)
  }
  return dateFormat(locale, timeZone, SHORT_DATE).format(instantOf(value))
}

export function formatDateTime(value: DateInput, locale: string, timeZone?: string): string {
  const instant = instantOf(value)
  const date = dateFormat(locale, timeZone, SHORT_DATE).format(instant)
  const time = dateFormat(locale, timeZone, TIME).format(instant)
  return `${date} ${time}`
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

// The calendar day count days before day.
export function daysBefore(day: string, count: number): string {
  return dayjs.utc(day).subtract(count, 'day').format('YYYY-MM-DD')
}

// The calendar day ("2026-03-01") the instant falls on in the zone.
export function dayOf(value: DateInput, timeZone?: string): string {
  return inZone(value, timeZone).format('YYYY-MM-DD')
}

// A date spelled out ("October 30, 2026"), for a sentence rather than a table cell.
export function formatLongDate(value: DateInput, locale: string): string {
  if (isCalendarDay(value)) {
    return dateFormat(locale, 'UTC', { dateStyle: 'long' }).format(new Date(`${value}T12:00:00Z`))
  }
  return dateFormat(locale, undefined, { dateStyle: 'long' }).format(instantOf(value))
}

const RELATIVE_UNITS = [
  ['year', 31_536_000],
  ['month', 2_592_000],
  ['day', 86_400],
  ['hour', 3_600],
  ['minute', 60],
  ['second', 1],
] as const

// How long ago (or how far ahead) the moment is, as "5 minutes ago": the largest unit it spans.
export function formatRelative(value: DateInput, locale: string): string {
  const seconds = (instantOf(value).getTime() - Date.now()) / 1000
  const [unit, size] = RELATIVE_UNITS.find(([, span]) => Math.abs(seconds) >= span) ?? ['second', 1]
  return new Intl.RelativeTimeFormat(locale, { numeric: 'auto' }).format(
    Math.round(seconds / size),
    unit,
  )
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
