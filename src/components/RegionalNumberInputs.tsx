import { createTheme, MantineThemeProvider, NumberInput } from '@mantine/core'
import { useMemo } from 'react'
import type { ReactNode } from 'react'

import { useFormat } from '../hooks/useFormat.ts'
import { numberSeparators } from '../utils/number.ts'

// RegionalNumberInputs makes the number inputs below it type a number the way the pages write it:
// in the User's Regional format (#1010), or the Instance's language while there is none, as
// useFormat does. Its decimal separator, the character between groups of digits and the grouping
// are read from Intl. The decimal separator is the only one a key types: in a locale whose group
// is a dot (de-DE), a typed dot taken for a decimal would make 1.000 a one. An input whose value
// is no quantity (a port) turns the grouping off with `thousandSeparator={false}`.
export function RegionalNumberInputs({ children }: { children: ReactNode }) {
  const { locale } = useFormat()
  const theme = useMemo(() => {
    const separators = numberSeparators(locale)
    return createTheme({
      components: {
        NumberInput: NumberInput.extend({
          defaultProps: {
            ...separators,
            allowedDecimalSeparators: [separators.decimalSeparator],
          },
        }),
      },
    })
  }, [locale])

  return <MantineThemeProvider theme={theme}>{children}</MantineThemeProvider>
}
