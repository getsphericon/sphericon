import { Alert, Badge } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import type { BroadcastHoldReason } from '../../generated/site/types.gen.ts'
import { holdExplanation } from '../../utils/broadcastHold.ts'

/** Explains a held broadcast: reversible, nothing lost, and what lifts the hold. */
export function BroadcastHoldAlert({ reason }: { reason: BroadcastHoldReason }) {
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
