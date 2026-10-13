import { Badge } from '@mantine/core'
import { useTranslation } from 'react-i18next'

// BillingBadge marks a billing hold (ADR 0009) and the Audit entries about one, so a
// payment problem reads the same in the console, the hold list and the customer's log.
export function BillingBadge() {
  const { t } = useTranslation()
  return (
    <Badge color="orange" variant="light">
      {t(($) => $.billingBadge)}
    </Badge>
  )
}
