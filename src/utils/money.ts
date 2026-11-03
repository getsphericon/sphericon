// An integer amount in the minor unit of an ISO 4217 currency as a currency string in the locale.
// The currency's own number of decimal places comes from Intl (JPY has none, KWD three), so no
// table of exponents is kept here.
export function formatMoney(minorUnits: number, currency: string, locale: string): string {
  const format = new Intl.NumberFormat(locale, { style: 'currency', currency })
  const exponent = format.resolvedOptions().maximumFractionDigits ?? 2
  return format.format(minorUnits / 10 ** exponent)
}
