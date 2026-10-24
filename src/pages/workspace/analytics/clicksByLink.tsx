import { Alert, EmptyState, Group, Stack, Table, Text } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { AnchorLink } from '../../../components/RouterLink.tsx'
import { siteAnalyticsClicksByLinkOptions } from '../../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteAnalyticsLinkOwner } from '../../../generated/site/types.gen.ts'
import { useAnalyticsRange } from '../../../hooks/useAnalyticsRange.ts'
import { Route as clicksByLinkRoute } from '../../../routes/workspaces/$workspaceId/analytics/clicks-by-link.tsx'
import { Route as automationRoute } from '../../../routes/workspaces/$workspaceId/automations/$automationId/index.tsx'
import { Route as broadcastReportRoute } from '../../../routes/workspaces/$workspaceId/broadcasts/$broadcastId/report.tsx'
import { ReportState } from './ReportState.tsx'

// The window is `<days>d`; the retention note compares its days with the instance's retention.
function windowDays(range: string): number {
  return Number.parseInt(range, 10)
}

function OwnerLink({ workspaceId, owner }: { workspaceId: string; owner: SiteAnalyticsLinkOwner }) {
  return owner.kind === 'broadcast' ? (
    <AnchorLink
      to={broadcastReportRoute.to}
      params={{ workspaceId, broadcastId: owner.id }}
      size="sm"
    >
      {owner.name}
    </AnchorLink>
  ) : (
    <AnchorLink to={automationRoute.to} params={{ workspaceId, automationId: owner.id }} size="sm">
      {owner.name}
    </AnchorLink>
  )
}

// ClicksByLinkPage is the Clicks by link report: every clicked link of the Workspace in the
// section's window, split by the Broadcast or Automation that carried it.
export function ClicksByLinkPage() {
  const { t } = useTranslation()
  const { workspaceId } = clicksByLinkRoute.useParams()
  const range = useAnalyticsRange()

  const reportQuery = useQuery(
    siteAnalyticsClicksByLinkOptions({ path: { workspaceId }, query: { range } }),
  )
  const report = reportQuery.data
  const retentionDays = report?.eventsRetentionDays ?? 0

  return (
    <Stack>
      <ReportState query={reportQuery} />

      {report && retentionDays > 0 && retentionDays < windowDays(range) && (
        <Alert color="yellow">
          {t(($) => $.analytics.clicksByLink.retentionNote, { days: retentionDays })}
        </Alert>
      )}

      {report &&
        (report.links.length === 0 ? (
          <EmptyState title={t(($) => $.analytics.clicksByLink.empty)} />
        ) : (
          <Stack gap="xs">
            <Text size="sm" c="dimmed">
              {t(($) => $.analytics.clicksByLink.hint)}
            </Text>
            <Table.ScrollContainer minWidth={480}>
              <Table>
                <Table.Thead>
                  <Table.Tr>
                    <Table.Th>{t(($) => $.analytics.clicksByLink.url)}</Table.Th>
                    <Table.Th>{t(($) => $.analytics.clicksByLink.clicks)}</Table.Th>
                    <Table.Th>{t(($) => $.analytics.clicksByLink.carriedBy)}</Table.Th>
                  </Table.Tr>
                </Table.Thead>
                <Table.Tbody>
                  {report.links.map((link) => (
                    <Table.Tr key={link.url}>
                      <Table.Td>
                        <Text size="sm" lineClamp={1}>
                          {link.url}
                        </Text>
                      </Table.Td>
                      <Table.Td>{link.clicks}</Table.Td>
                      <Table.Td>
                        <Stack gap={2}>
                          {link.owners.map((owner) => (
                            <Group key={`${owner.kind}-${owner.id}`} gap="xs" wrap="nowrap">
                              <OwnerLink workspaceId={workspaceId} owner={owner} />
                              <Text size="xs" c="dimmed">
                                {t(($) => $.analytics.clicksByLink.ownerClicks, {
                                  kind: t(($) => $.analytics.clicksByLink[owner.kind]),
                                  clicks: owner.clicks,
                                })}
                              </Text>
                            </Group>
                          ))}
                        </Stack>
                      </Table.Td>
                    </Table.Tr>
                  ))}
                </Table.Tbody>
              </Table>
            </Table.ScrollContainer>
          </Stack>
        ))}
    </Stack>
  )
}
