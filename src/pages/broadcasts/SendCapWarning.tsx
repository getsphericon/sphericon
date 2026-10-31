import { Alert } from '@mantine/core'
import { IconClockHour4 } from '@tabler/icons-react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import {
  siteSegmentsGetOptions,
  siteWorkspacesListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { siteSegmentsPreview } from '../../generated/site/sdk.gen.ts'
import type { SiteBroadcastResource } from '../../generated/site/types.gen.ts'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'

// Before a draft is sent or scheduled: when its audience (the segment preview's count, the whole
// list without a segment) is larger than the Workspace's Send cap lets out in a day, it will go
// out over several days (ADR 0067). The warning never blocks sending; without a Send cap, or
// with an audience that fits, nothing is shown.
export function SendCapWarning({ broadcast }: { broadcast: SiteBroadcastResource }) {
  const { t, i18n } = useTranslation()
  const { workspaceId } = workspaceRoute.useParams()
  const workspaces = useQuery(siteWorkspacesListOptions())
  const sendCap = workspaces.data?.find((w) => w.id === workspaceId)?.sendCap ?? null
  const segmentId = broadcast.segmentId ?? null
  const segment = useQuery({
    ...siteSegmentsGetOptions({ path: { workspaceId, id: segmentId ?? '' } }),
    enabled: sendCap !== null && segmentId !== null,
  })
  const condition = segment.data?.condition
  const audience = useQuery({
    queryKey: ['broadcastAudience', workspaceId, segmentId, condition],
    queryFn: async () => {
      const { data } = await siteSegmentsPreview({
        path: { workspaceId },
        body: condition ? { condition } : {},
        throwOnError: true,
      })
      return data.count
    },
    enabled: sendCap !== null && (segmentId === null || segment.data !== undefined),
  })

  if (sendCap === null || audience.data === undefined || audience.data <= sendCap.perDay) {
    return null
  }
  const number = new Intl.NumberFormat(i18n.language)
  return (
    <Alert color="yellow" variant="light" icon={<IconClockHour4 size={18} />}>
      {t(($) => $.broadcasts.sendCapWarning, {
        audience: number.format(audience.data),
        perDay: number.format(sendCap.perDay),
        count: Math.ceil(audience.data / sendCap.perDay),
      })}
    </Alert>
  )
}
