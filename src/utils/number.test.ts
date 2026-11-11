import { expect, test } from 'vitest'

import { formatNumber, numberSeparators } from './number.ts'
import { REGIONAL_FORMATS } from './regionalFormats.ts'

test('a locale writes its own decimal and group separators', () => {
  expect(numberSeparators('en-US')).toEqual({
    decimalSeparator: '.',
    thousandSeparator: ',',
    thousandsGroupStyle: 'thousand',
  })
  expect(numberSeparators('de-DE')).toEqual({
    decimalSeparator: ',',
    thousandSeparator: '.',
    thousandsGroupStyle: 'thousand',
  })
})

// The browser's own locale data decides the separators (Chromium writes de-CH with an ASCII
// apostrophe, Node with a typographic one), so an input is checked against the number the pages
// show, whatever the data says.
test('an input writes a number as the pages show it, in every Regional format', () => {
  for (const locale of REGIONAL_FORMATS) {
    const {
      decimalSeparator: d,
      thousandSeparator: t,
      thousandsGroupStyle,
    } = numberSeparators(locale)
    const typed = thousandsGroupStyle === 'lakh' ? `12${t}34${t}567${d}5` : `1${t}234${t}567${d}5`
    expect({ locale, typed }).toEqual({ locale, typed: formatNumber(1_234_567.5, locale) })
  }
})
