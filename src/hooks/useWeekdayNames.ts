import { useTranslation } from 'react-i18next'

import type { Weekday } from '../generated/site/types.gen.ts'

// useWeekdayNames names each day of the week, keyed by the contract's Weekday: the one set of
// labels every screen that shows a weekday reads.
export function useWeekdayNames(): Record<Weekday, string> {
  const { t } = useTranslation()
  return {
    monday: t(($) => $.weekday.monday),
    tuesday: t(($) => $.weekday.tuesday),
    wednesday: t(($) => $.weekday.wednesday),
    thursday: t(($) => $.weekday.thursday),
    friday: t(($) => $.weekday.friday),
    saturday: t(($) => $.weekday.saturday),
    sunday: t(($) => $.weekday.sunday),
  }
}
