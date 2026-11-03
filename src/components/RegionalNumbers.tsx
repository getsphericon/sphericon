import { useEffect } from 'react'

import { useRegionalFormat } from '../hooks/useFormat.ts'
import { setRegionalFormat } from '../i18n.ts'

// RegionalNumbers makes a number a string prints (`{{count, number}}`) follow the User's Regional
// format (#1022), as RegionalDates does for the date inputs: it reads the cached profile and hands
// the format to the i18n instance, which re-renders the strings on screen. With no Regional
// format, and on a public page or the Staff console where there is no profile, the numbers stay
// in the Instance's language. It renders nothing; mount it once under the QueryClientProvider.
export function RegionalNumbers() {
  const regionalFormat = useRegionalFormat()

  useEffect(() => {
    setRegionalFormat(regionalFormat)
  }, [regionalFormat])

  // The instance outlives this component, so the format does not stay behind when it goes.
  useEffect(
    () => () => {
      setRegionalFormat(undefined)
    },
    [],
  )

  return null
}
