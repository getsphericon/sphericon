// A number in the locale: its digit grouping and decimal separator.
export function formatNumber(value: number, locale: string): string {
  return new Intl.NumberFormat(locale).format(value)
}

// How a number input writes a number of the locale, in the terms of Mantine's NumberInput: the
// decimal separator, the character between groups of digits and how the digits are grouped
// (en-IN groups by two above the thousands: 12,34,567).
export interface NumberSeparators {
  decimalSeparator: string
  thousandSeparator: string
  thousandsGroupStyle: 'thousand' | 'lakh'
}

// numberSeparators reads the separators off a number Intl writes in the locale, so an input types
// a number the way the pages show it.
export function numberSeparators(locale: string): NumberSeparators {
  const parts = new Intl.NumberFormat(locale).formatToParts(1_234_567.5)
  const part = (type: Intl.NumberFormatPartTypes) =>
    parts.find((found) => found.type === type)?.value
  const integers = parts.filter((found) => found.type === 'integer')
  return {
    decimalSeparator: part('decimal') ?? '.',
    thousandSeparator: part('group') ?? '',
    // 1234567 groups as 1,234,567 by thousands and as 12,34,567 by lakh: the middle group tells.
    thousandsGroupStyle: integers[1]?.value.length === 2 ? 'lakh' : 'thousand',
  }
}
