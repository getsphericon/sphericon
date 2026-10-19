import { resolveLocale } from '../i18n.ts'

// Dates follow the instance locale (SPHERICON_APP_LOCALE), not the browser's. Native toLocaleString() defaults to navigator.language, so we pass
// the resolved app locale explicitly. Kept dependency-free (no dayjs/date-fns):
// the app has no date-picker components, only display formatting.

type DateInput = string | number | Date

export function formatDateTime(value: DateInput): string {
  return new Date(value).toLocaleString(resolveLocale())
}

export function formatDate(value: DateInput): string {
  return new Date(value).toLocaleDateString(resolveLocale())
}
