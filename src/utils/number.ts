// A number in the locale: its digit grouping and decimal separator.
export function formatNumber(value: number, locale: string): string {
  return new Intl.NumberFormat(locale).format(value)
}
