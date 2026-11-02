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

// The change from the previous period as a signed percentage, or null when it has no share.
export function formatChange(current: number, previous: number): string | null {
  const change = changeFrom(current, previous)
  return change === null ? null : formatPercent(change, 'exceptZero')
}
