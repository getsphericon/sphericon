import { resolveLocale } from '../i18n.ts'

// Dates follow the instance locale (SPHERICON_APP_LOCALE), not the browser's. Native toLocaleString() defaults to navigator.language, so we pass
// the resolved app locale explicitly. Kept dependency-free (no dayjs/date-fns):
// the app has no date-picker components, only display formatting.
//
// timeZone is the IANA zone to show the instant in; undefined is the browser's. A signed-in page
// formats through useDateFormat, which passes the User time zone (ADR 0068).

type DateInput = string | number | Date

export function formatDateTime(value: DateInput, timeZone?: string): string {
  return new Date(value).toLocaleString(resolveLocale(), { timeZone })
}

export function formatDate(value: DateInput, timeZone?: string): string {
  return new Date(value).toLocaleDateString(resolveLocale(), { timeZone })
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
