import { Card, SimpleGrid, Stack, Text, Title } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import { PagedTable } from '../../components/PagedTable.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import { BlockSkeleton } from '../../components/Skeletons.tsx'
import {
  siteBroadcastsListOptions,
  siteSetupChecklistGetOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { Route as automationsRoute } from '../../routes/workspaces/$workspaceId/automations/index.tsx'
import { Route as broadcastsReportRoute } from '../../routes/workspaces/$workspaceId/broadcasts/$broadcastId/report.tsx'
import { Route as broadcastsCreateRoute } from '../../routes/workspaces/$workspaceId/broadcasts/new.tsx'
import { Route as overviewRoute } from '../../routes/workspaces/$workspaceId/index.tsx'
import { formatDateTime } from '../../utils/datetime.ts'
import { SetupChecklist } from './SetupChecklist.tsx'

// OverviewPage is the workspace's landing page: the Setup checklist until it is finished, then
// the shortcuts to create something and the broadcasts created last.
export function OverviewPage() {
  const { t } = useTranslation()
  const { workspaceId } = overviewRoute.useParams()
  const navigate = useNavigate()

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

      <Title order={3}>{t(($) => $.overview.recentBroadcastsTitle)}</Title>
      <PagedTable
        pageSize={5}
        options={(query) => siteBroadcastsListOptions({ path: { workspaceId }, query })}
        loadErrorTitle={t(($) => $.alerts.broadcastLoadErrorTitle)}
        noRecordsText={t(($) => $.broadcasts.noRecords)}
        emptyState={t(($) => $.broadcasts.emptyState)}
        onRowClick={(record) =>
          void navigate({
            to: broadcastsReportRoute.to,
            params: { workspaceId, broadcastId: record.id },
          })
        }
        columns={[
          { accessor: 'name', title: t(($) => $.broadcasts.nameLabel) },
          {
            accessor: 'status',
            title: t(($) => $.broadcasts.statusLabel),
            render: (record) => t(($) => $.broadcasts.status[record.status]),
          },
          {
            accessor: 'createdAt',
            title: t(($) => $.overview.createdLabel),
            render: (record) => formatDateTime(record.createdAt),
          },
          {
            accessor: 'openRate',
            title: t(($) => $.overview.openRateLabel),
            render: (record) =>
              record.stats.sentCount > 0
                ? `${Math.round((record.stats.openedCount / record.stats.sentCount) * 100)}%`
                : '–',
          },
        ]}
      />
    </Stack>
  )
}
