import { Card, Group, SegmentedControl, SimpleGrid, Stack, Text, Title } from '@mantine/core'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import { BlockSkeleton } from '../../components/Skeletons.tsx'
import { siteSetupChecklistGetOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import { zSiteAnalyticsRange } from '../../generated/site/zod.gen.ts'
import { Route as automationsRoute } from '../../routes/workspaces/$workspaceId/automations/index.tsx'
import { Route as broadcastsCreateRoute } from '../../routes/workspaces/$workspaceId/broadcasts/new.tsx'
import { Route as overviewRoute } from '../../routes/workspaces/$workspaceId/index.tsx'
import { AttentionBlock } from './AttentionBlock.tsx'
import { RecentlyCreated } from './RecentlyCreated.tsx'
import { RevenueBlock } from './RevenueBlock.tsx'
import { SetupChecklist } from './SetupChecklist.tsx'

const RANGES = zSiteAnalyticsRange.options

// UnfinishedChecklist is the Setup checklist until it is finished, and nothing after.
function UnfinishedChecklist({ workspaceId }: { workspaceId: string }) {
  const checklist = useSuspenseQuery(siteSetupChecklistGetOptions({ path: { workspaceId } })).data
  return checklist.complete ? null : (
    <SetupChecklist workspaceId={workspaceId} checklist={checklist} />
  )
}

// OverviewPage is the workspace's landing page: the Setup checklist until it is finished, the
// revenue block, the shortcuts to create something and the broadcasts created last.
export function OverviewPage() {
  const { t } = useTranslation()
  const { workspaceId } = overviewRoute.useParams()
  const { range } = overviewRoute.useSearch()
  const navigate = useNavigate({ from: overviewRoute.fullPath })

  return (
    <Stack>
      <Group justify="flex-end">
        <SegmentedControl
          size="xs"
          value={range}
          onChange={(next) => void navigate({ search: { range: next } })}
          data={RANGES.map((r) => ({ value: r, label: t(($) => $.analytics.range[r]) }))}
        />
      </Group>

      <AttentionBlock workspaceId={workspaceId} />

      <QueryBoundary fallback={<BlockSkeleton />} errorTitle={t(($) => $.setupChecklist.loadError)}>
        <UnfinishedChecklist workspaceId={workspaceId} />
      </QueryBoundary>

      <RevenueBlock workspaceId={workspaceId} range={range} />

      <Title order={3}>{t(($) => $.overview.createTitle)}</Title>
      <SimpleGrid cols={{ base: 1, md: 2 }}>
        <Card withBorder padding="lg">
          <Stack gap="xs" align="flex-start">
            <Title order={4}>{t(($) => $.broadcasts.addBroadcast)}</Title>
            <Text size="sm" c="dimmed">
              {t(($) => $.overview.createBroadcastDescription)}
            </Text>
            <ButtonLink to={broadcastsCreateRoute.to} params={{ workspaceId }} variant="default">
              {t(($) => $.broadcasts.addBroadcast)}
            </ButtonLink>
          </Stack>
        </Card>
        <Card withBorder padding="lg">
          <Stack gap="xs" align="flex-start">
            <Title order={4}>{t(($) => $.automations.addAutomation)}</Title>
            <Text size="sm" c="dimmed">
              {t(($) => $.overview.createAutomationDescription)}
            </Text>
            <ButtonLink to={automationsRoute.to} params={{ workspaceId }} variant="default">
              {t(($) => $.nav.automations)}
            </ButtonLink>
          </Stack>
        </Card>
      </SimpleGrid>

      <Title order={3}>{t(($) => $.overview.recentTitle)}</Title>
      <RecentlyCreated workspaceId={workspaceId} />
    </Stack>
  )
}
