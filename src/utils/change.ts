import { resolveLocale } from '../i18n.ts'
import { formatPercent } from './percent.ts'

// The change of a figure from the previous period, as a share of the size of the previous one
// (0.5 for half as many again): an amount that rose from a negative net revenue (refunds only) is
// a rise. Null when the previous period had none, where no share exists. Shared by the Automation
// report and the overview.
export function changeFrom(current: number, previous: number): number | null {
  if (previous === 0) {
    return null
  }
  return (current - previous) / Math.abs(previous)
}

// The change of a 0–1 rate from the previous period in percentage points, signed, the unit left to
// the caller: 20% to 30% is +10. A share of the earlier rate (+50%) would read as fifty points next
// to a figure that is itself a percentage, and would have none from a rate of zero.
export function formatPointChange(current: number, previous: number): string {
  return new Intl.NumberFormat(resolveLocale(), {
    maximumFractionDigits: 1,
    signDisplay: 'exceptZero',
  }).format((current - previous) * 100)
}

// The change from the previous period as a signed percentage, or null when it has no share.
export function formatChange(current: number, previous: number): string | null {
  const change = changeFrom(current, previous)
  return change === null ? null : formatPercent(change, 'exceptZero')
}
