import { Group, SegmentedControl, Stack } from '@mantine/core'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { BlockSkeleton } from '../../components/Skeletons.tsx'
import { siteSetupChecklistGetOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import { zSiteAnalyticsRange } from '../../generated/site/zod.gen.ts'
import { Route as overviewRoute } from '../../routes/workspaces/$workspaceId/index.tsx'
import { AttentionBlock } from './AttentionBlock.tsx'
import { FiguresBlock } from './FiguresBlock.tsx'
import { RevenueBlock } from './RevenueBlock.tsx'
import { SendsBlock } from './SendsBlock.tsx'
import { SetupChecklist } from './SetupChecklist.tsx'

const RANGES = zSiteAnalyticsRange.options

// UnfinishedChecklist is the Setup checklist until it is finished, and nothing after.
function UnfinishedChecklist({ workspaceId }: { workspaceId: string }) {
  const checklist = useSuspenseQuery(siteSetupChecklistGetOptions({ path: { workspaceId } })).data
  return checklist.complete ? null : (
    <SetupChecklist workspaceId={workspaceId} checklist={checklist} />
  )
}

// OverviewPage is the Workspace's landing page, ordered by urgency first and by how many
// Workspaces a block applies to after: what needs attention now, the Setup checklist until it is
// finished, what is going out and what went out, the period's sending and audience, and the
// revenue of the Orders once the Workspace sends any. A block with nothing to show is not there.
export function OverviewPage() {
  const { t } = useTranslation()
  const { workspaceId } = overviewRoute.useParams()
  const { range } = overviewRoute.useSearch()
  const navigate = useNavigate({ from: overviewRoute.fullPath })

  return (
    <Stack>
      <AttentionBlock workspaceId={workspaceId} />

      <QueryBoundary fallback={<BlockSkeleton />} errorTitle={t(($) => $.setupChecklist.loadError)}>
        <UnfinishedChecklist workspaceId={workspaceId} />
      </QueryBoundary>

      <SendsBlock workspaceId={workspaceId} />

      <FiguresBlock
        workspaceId={workspaceId}
        range={range}
        periodControl={
          <Group justify="flex-end">
            <SegmentedControl
              size="xs"
              value={range}
              onChange={(next) => void navigate({ search: { range: next } })}
              data={RANGES.map((r) => ({ value: r, label: t(($) => $.analytics.range[r]) }))}
            />
          </Group>
        }
      />

      <RevenueBlock workspaceId={workspaceId} range={range} />
    </Stack>
  )
}
