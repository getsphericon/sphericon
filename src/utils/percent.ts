import { resolveLocale } from '../i18n.ts'

// A 0–1 rate as a locale percentage, up to two decimals (0.15% stays 0.15%). A change from an
// earlier figure passes `exceptZero`, which signs it: +50%, -20%, 0%.
export function formatPercent(
  rate: number,
  signDisplay: Intl.NumberFormatOptions['signDisplay'] = 'auto',
): string {
  return new Intl.NumberFormat(resolveLocale(), {
    style: 'percent',
    maximumFractionDigits: 2,
    signDisplay,
  }).format(rate)
}
