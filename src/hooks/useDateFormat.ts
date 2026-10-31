import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'

import { siteUserGetMeOptions } from '../generated/site/@tanstack/react-query.gen.ts'
import { formatDate, formatDateTime } from '../utils/datetime.ts'

type DateInput = Parameters<typeof formatDateTime>[0]

// useDateFormat formats instants in the User time zone (ADR 0068). It reads the profile from the
// cache and never fetches it: a signed-in page's guard has loaded it, and where there is none (a
// public page, the Staff console) or the User chose no zone, the browser's zone holds. A saved
// zone re-renders every caller through the cache.
export function useDateFormat() {
  const timeZone = useQuery({
    ...siteUserGetMeOptions(),
    enabled: false,
    select: (me) => me.timezone ?? undefined,
  }).data

  return useMemo(
    () => ({
      formatDateTime: (value: DateInput) => formatDateTime(value, timeZone),
      formatDate: (value: DateInput) => formatDate(value, timeZone),
    }),
    [timeZone],
  )
}
