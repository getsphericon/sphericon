import { resolveLocale } from '../i18n.ts'
import {
  formatDateTime as formatDateTimeIn,
  formatLongDate as formatLongDateIn,
} from '../utils/datetime.ts'
import { formatPercent as formatPercentIn } from '../utils/percent.ts'

// The Staff console has no signed-in User's Regional format to follow (#1010): it keeps the
// Instance's language and the browser's time zone, so its figures are the same for every
// Staff member of an Instance.

export function formatDateTime(value: Parameters<typeof formatDateTimeIn>[0]): string {
  return formatDateTimeIn(value, resolveLocale())
}

export function formatLongDate(value: Parameters<typeof formatLongDateIn>[0]): string {
  return formatLongDateIn(value, resolveLocale())
}

export function formatPercent(rate: number): string {
  return formatPercentIn(rate, resolveLocale())
}
