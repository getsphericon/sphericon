import { Badge } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import type { SiteSendingDomainResource } from '../../generated/site/types.gen.ts'

// SendingDomainStatus is where a Sending domain stands: verified, verification lost since it was
// verified, or still pending.
export function SendingDomainStatus({ record }: { record: SiteSendingDomainResource }) {
  const { t } = useTranslation()
  if (record.verified) {
    return (
      <Badge color="teal" variant="light">
        {t(($) => $.settings.sendingDomains.verified)}
      </Badge>
    )
  }
  if (record.verificationLostAt) {
    return (
      <Badge color="red" variant="light">
        {t(($) => $.settings.sendingDomains.lost)}
      </Badge>
    )
  }
  return (
    <Badge color="gray" variant="light">
      {t(($) => $.settings.sendingDomains.pending)}
    </Badge>
  )
}
