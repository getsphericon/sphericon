import { resolveLocale } from '../../i18n.ts'
import { formatPercent } from '../../utils/percent.ts'

// The change of a figure from the previous period, as a share of the size of the previous one
// (0.5 for half as many again): an amount that rose from a negative net revenue (refunds only) is
// a rise. Null when the previous period had none, where no share exists.
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

const UNITS = [
  { unit: 'days', seconds: 86_400 },
  { unit: 'hours', seconds: 3_600 },
  { unit: 'minutes', seconds: 60 },
  { unit: 'seconds', seconds: 1 },
] as const

// A duration in whole seconds as its largest unit and the one below it, in the locale's narrow
// units: "1d 3h", "5h 20m", "45s". A unit below those two is dropped, and Intl.DurationFormat leaves
// out a zero one; a duration of nothing is zero seconds, not an empty string.
export function formatDuration(totalSeconds: number): string {
  const seconds = Math.max(0, Math.round(totalSeconds))
  // Under a second there is no unit to lead with: it is shown as zero seconds.
  const found = UNITS.findIndex((u) => seconds >= u.seconds)
  const lead = found === -1 ? UNITS.length - 1 : found
  let rest = seconds
  const duration: Partial<Record<(typeof UNITS)[number]['unit'], number>> = {}
  for (const { unit, seconds: size } of UNITS.slice(lead, lead + 2)) {
    const count = Math.floor(rest / size)
    rest -= count * size
    duration[unit] = count
  }
  return new Intl.DurationFormat(resolveLocale(), {
    style: 'narrow',
    secondsDisplay: seconds === 0 ? 'always' : 'auto',
  }).format(duration)
}
