// A 0–1 rate as a percentage in the locale, up to two decimals (0.15% stays 0.15%). A change from
// an earlier figure passes `exceptZero`, which signs it: +50%, -20%, 0%.
export function formatPercent(
  rate: number,
  locale: string,
  signDisplay: Intl.NumberFormatOptions['signDisplay'] = 'auto',
): string {
  return new Intl.NumberFormat(locale, {
    style: 'percent',
    maximumFractionDigits: 2,
    signDisplay,
  }).format(rate)
}
