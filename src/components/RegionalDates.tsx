import { DatesProvider } from '@mantine/dates'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import type { ReactNode } from 'react'

import { useRegionalFormat } from '../hooks/useFormat.ts'
import { dayjsLocaleName, firstDayOfWeek, loadDayjsLocale } from '../utils/dayjsLocale.ts'

// RegionalDates makes the date inputs below it (the pickers) read and write dates in the User's
// Regional format (#1010): their month names, the order of the parts a typed date is read in and
// the day the week starts on. The locale data is loaded when the format is first used, and until
// it is there the pickers keep the locale they had (English at first), so an input never waits on
// it. With no Regional format the pickers keep their defaults, as they did before.
export function RegionalDates({ children }: { children: ReactNode }) {
  const regionalFormat = useRegionalFormat()
  const name = dayjsLocaleName(regionalFormat)
  const { data: loaded = 'en' } = useQuery({
    queryKey: ['dayjs-locale', name],
    queryFn: async () => {
      await loadDayjsLocale(name)
      return name
    },
    staleTime: Infinity,
    placeholderData: keepPreviousData,
  })

  const firstDay = firstDayOfWeek(regionalFormat)

  return (
    <DatesProvider
      settings={{ locale: loaded, ...(firstDay === undefined ? {} : { firstDayOfWeek: firstDay }) }}
    >
      {children}
    </DatesProvider>
  )
}
