import { Alert } from '@mantine/core'
import { useQuery, useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import {
  siteSegmentsPreviewOptions,
  siteWorkspacesListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteSegmentResource } from '../../generated/site/types.gen.ts'
import { sendCapDays } from './sendCapDays.ts'

interface SendCapWarningProps {
  workspaceId: string
  segmentId: string | null | undefined
  segments: SiteSegmentResource[] | undefined
}

// Warns that a Broadcast to more contacts than the Send cap (ADR 0067) lets out within a day goes
// out over several days. It only informs: sending and scheduling stay open.
export function SendCapWarning(props: SendCapWarningProps) {
  const { t } = useTranslation()
  return (
    <QueryBoundary fallback={null} errorTitle={t(($) => $.broadcasts.sendCapWarningError)}>
      <Warning {...props} />
    </QueryBoundary>
  )
}

// The audience is the segment preview's count of the chosen Segment, or of all contacts for a blank
// one; nothing is asked, and nothing renders, while the Workspace has no Send cap or the audience
// goes out within 24 hours.
function Warning({ workspaceId, segmentId, segments }: SendCapWarningProps) {
  const { t } = useTranslation()
  const { data: workspaces } = useSuspenseQuery(siteWorkspacesListOptions())
  const sendCap = workspaces.find((w) => w.id === workspaceId)?.sendCap
  // A blank Segment is all contacts; a chosen one counts by its condition, once it is known.
  const segment = segmentId ? segments?.find((s) => s.id === segmentId) : undefined
  const resolved = !segmentId || segment !== undefined
  const condition = segment?.condition

  const preview = useQuery({
    ...siteSegmentsPreviewOptions({
      path: { workspaceId },
      body: condition ? { condition } : {},
    }),
    enabled: Boolean(sendCap) && resolved,
    select: (data) => data.count,
  })

  const audience = preview.data
  const days = audience === undefined ? null : sendCapDays(audience, sendCap)
  if (!sendCap || audience === undefined || days === null) {
    return null
  }
  return (
    <Alert color="yellow" variant="light">
      {t(($) => $.broadcasts.sendCapWarning, {
        audience,
        perDay: sendCap.perDay,
        count: days,
      })}
    </Alert>
  )
}
