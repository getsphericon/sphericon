import { Badge } from '@mantine/core'
import type { MantineSize } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import type { SiteBroadcastStatus } from '../../generated/site/types.gen.ts'

const STATUS_COLORS: Record<SiteBroadcastStatus, string> = {
  draft: 'gray',
  scheduled: 'blue',
  sending: 'yellow',
  sent: 'teal',
  failed: 'red',
}

// The status of a Broadcast, as the list and the report show it.
export function BroadcastStatusBadge({
  status,
  size = 'md',
}: {
  status: SiteBroadcastStatus
  size?: MantineSize
}) {
  const { t } = useTranslation()
  return (
    <Badge color={STATUS_COLORS[status]} variant="light" size={size}>
      {t(($) => $.broadcasts.status[status])}
    </Badge>
  )
}
