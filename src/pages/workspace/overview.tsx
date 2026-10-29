import { Card, SimpleGrid, Stack, Text, Title } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import { BlockSkeleton } from '../../components/Skeletons.tsx'
import { siteSetupChecklistGetOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import { Route as automationsRoute } from '../../routes/workspaces/$workspaceId/automations/index.tsx'
import { Route as broadcastsCreateRoute } from '../../routes/workspaces/$workspaceId/broadcasts/new.tsx'
import { Route as overviewRoute } from '../../routes/workspaces/$workspaceId/index.tsx'
import { RecentlyCreated } from './RecentlyCreated.tsx'
import { RevenueBlock } from './RevenueBlock.tsx'
import { SetupChecklist } from './SetupChecklist.tsx'

// OverviewPage is the workspace's landing page: the Setup checklist until it is finished, the
// revenue block, the shortcuts to create something and the broadcasts created last.
export function OverviewPage() {
  const { t } = useTranslation()
  const { workspaceId } = overviewRoute.useParams()

  const checklistQuery = useQuery(siteSetupChecklistGetOptions({ path: { workspaceId } }))
  const checklist = checklistQuery.data
  const showChecklist = checklist !== undefined && !checklist.complete

  return (
    <Stack>
      {checklistQuery.isLoading && <BlockSkeleton />}

      {checklistQuery.isError && (
        <ApiErrorAlert
          error={checklistQuery.error}
          title={t(($) => $.setupChecklist.loadError)}
          fallback={t(($) => $.setupChecklist.loadError)}
        />
      )}

      {showChecklist && <SetupChecklist workspaceId={workspaceId} checklist={checklist} />}

      <RevenueBlock workspaceId={workspaceId} />

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
