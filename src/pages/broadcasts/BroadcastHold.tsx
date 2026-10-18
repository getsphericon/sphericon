import { Alert, Badge } from '@mantine/core'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'

// Why a broadcast can be held (Outbound send, ADR 0015). The API sends the reason as
// a plain string, so an unknown one falls back to the generic explanation.
const HOLD_REASONS = ['no_integration', 'unverified_domain', 'workspace_suspended'] as const

function holdExplanation(t: TFunction, reason: string): string {
  const known = HOLD_REASONS.find((r) => r === reason)
  return known
    ? t(($) => $.broadcasts.hold.reasons[known])
    : t(($) => $.broadcasts.hold.description)
}

/** Explains a held broadcast: reversible, nothing lost, and what lifts the hold. */
export function BroadcastHoldAlert({ reason }: { reason: string }) {
  const { t } = useTranslation()
  return (
    <Alert color="yellow" title={t(($) => $.broadcasts.hold.title)}>
      {holdExplanation(t, reason)} {t(($) => $.broadcasts.hold.description)}
    </Alert>
  )
}

/** Compact "on hold" marker for list rows. */
export function BroadcastHoldBadge() {
  const { t } = useTranslation()
  return (
    <Badge color="orange" variant="light">
      {t(($) => $.broadcasts.hold.badge)}
    </Badge>
  )
}
