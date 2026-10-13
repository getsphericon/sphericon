import { resolveLocale } from '../i18n.ts'

// A 0–1 rate as a locale percentage, up to two decimals (0.15% stays 0.15%).
export function formatPercent(rate: number): string {
  return new Intl.NumberFormat(resolveLocale(), {
    style: 'percent',
    maximumFractionDigits: 2,
  }).format(rate)
}
