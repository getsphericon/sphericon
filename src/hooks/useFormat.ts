import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'

import { siteUserGetMeOptions } from '../generated/site/@tanstack/react-query.gen.ts'
import { resolveLocale } from '../i18n.ts'
import { formatChange, formatPointChange } from '../utils/change.ts'
import {
  formatDate,
  formatDateTime,
  formatDay,
  formatLongDate,
  formatRelative,
} from '../utils/datetime.ts'
import { formatDuration } from '../utils/duration.ts'
import { formatMoney } from '../utils/money.ts'
import { formatNumber } from '../utils/number.ts'
import { formatPercent } from '../utils/percent.ts'

type DateInput = Parameters<typeof formatDateTime>[0]

// useTimeZone is the User time zone (ADR 0068), or undefined for the browser's. It reads the
// profile from the cache and never fetches it: a signed-in page's guard has loaded it, and where
// there is none (a public page, the Staff console) or the User chose no zone, the browser's zone
// holds. A saved zone re-renders every caller through the cache.
export function useTimeZone(): string | undefined {
  return useQuery({
    ...siteUserGetMeOptions(),
    enabled: false,
    select: (me) => me.timezone ?? undefined,
  }).data
}

// useRegionalFormat is the User's Regional format (#1010): the locale, with its region, their
// dates, times and numbers are written in, or undefined while they chose none. Read from the
// cache like the time zone, so a public page and the Staff console never have one.
export function useRegionalFormat(): string | undefined {
  return useQuery({
    ...siteUserGetMeOptions(),
    enabled: false,
    select: (me) => me.regionalFormat ?? undefined,
  }).data
}

// useLocale is the locale a signed-in page writes dates, times and numbers in: the User's
// Regional format, or the Instance's language until they choose one. The UI's own language is
// the Instance's either way.
function useLocale(): string {
  return useRegionalFormat() ?? resolveLocale()
}

// useFormat formats everything a signed-in page shows in the User's Regional format and time
// zone: the one path for a date, a time, a number, a percentage, a money amount and a duration.
export function useFormat() {
  const timeZone = useTimeZone()
  const locale = useLocale()

  return useMemo(
    () => ({
      locale,
      formatDateTime: (value: DateInput) => formatDateTime(value, locale, timeZone),
      formatDate: (value: DateInput) => formatDate(value, locale, timeZone),
      formatLongDate: (value: DateInput) => formatLongDate(value, locale),
      formatRelative: (value: DateInput) => formatRelative(value, locale),
      formatDay: (day: string) => formatDay(day, locale),
      formatNumber: (value: number) => formatNumber(value, locale),
      formatPercent: (rate: number) => formatPercent(rate, locale),
      // A rate over nothing is undefined (null): shown as a dash, never as 0%.
      formatRate: (rate: number | null) => (rate === null ? '–' : formatPercent(rate, locale)),
      formatMoney: (minorUnits: number, currency: string) =>
        formatMoney(minorUnits, currency, locale),
      formatDuration: (totalSeconds: number) => formatDuration(totalSeconds, locale),
      formatChange: (current: number, previous: number) => formatChange(current, previous, locale),
      formatPointChange: (current: number, previous: number) =>
        formatPointChange(current, previous, locale),
    }),
    [timeZone, locale],
  )
}
