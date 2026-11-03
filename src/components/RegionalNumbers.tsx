import { useEffect, useLayoutEffect } from 'react'

import { useRegionalFormat } from '../hooks/useFormat.ts'
import { setRegionalFormat } from '../i18n.ts'

// RegionalNumbers makes a number a string prints (`{{count, number}}`) follow the User's Regional
// format (#1022), as RegionalDates does for the date inputs: it reads the cached profile and hands
// the format to the i18n instance, which re-renders the strings on screen. With no Regional
// format the numbers stay in the Instance's language. `instance` pins them there whatever the
// profile says: the Staff console has no User preference to follow (ADR 0096). It renders
// nothing; mount it once, under the QueryClientProvider.
export function RegionalNumbers({ instance = false }: { instance?: boolean }) {
  const regionalFormat = useRegionalFormat()
  const tag = instance ? undefined : regionalFormat

  // Before the browser paints, so a string never shows its numbers in the old format first.
  useLayoutEffect(() => {
    setRegionalFormat(tag)
  }, [tag])

  // The instance outlives this component, so the format does not stay behind when it goes.
  useEffect(
    () => () => {
      setRegionalFormat(undefined)
    },
    [],
  )

  return null
}
