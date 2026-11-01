import type { TFunction } from 'i18next'

// Why a broadcast can be held (Outbound send, ADR 0015). The API sends the reason as
// a plain string, so an unknown one falls back to the generic explanation.
const HOLD_REASONS = [
  'no_integration',
  'unverified_domain',
  'workspace_suspended',
  'provider_stopped',
] as const

// holdExplanation is what lifts a hold of the given reason, in words.
export function holdExplanation(t: TFunction, reason: string): string {
  const known = HOLD_REASONS.find((r) => r === reason)
  return known
    ? t(($) => $.broadcasts.hold.reasons[known])
    : t(($) => $.broadcasts.hold.description)
}
