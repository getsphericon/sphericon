import { resolveLocale } from '../../i18n.ts'

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
