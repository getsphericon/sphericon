import { SimpleGrid, Stack, Text, Title } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { LinkClickTable } from '../../components/LinkClickTable.tsx'
import { StatCard } from '../../components/StatCard.tsx'
import { siteAutomationReportsGetOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import { formatMessageRevenue } from '../../utils/messageRevenue.ts'

/**
 * How the emails of an Automation were engaged with: opens, clicks and the clicked links. Renders
 * nothing while the figures cannot be read: the enrollments below stay the page.
 */
export function AutomationReport({
  workspaceId,
  automationId,
}: {
  workspaceId: string
  automationId: string
}) {
  const { t } = useTranslation()
  const reportQuery = useQuery(
    siteAutomationReportsGetOptions({ path: { workspaceId, automationId } }),
  )

  const report = reportQuery.data
  if (!report) {
    return null
  }

  const revenue = formatMessageRevenue(report.revenue)

  return (
    <Stack gap="xs">
      <Title order={5}>{t(($) => $.automations.engagementTitle)}</Title>
      <Text size="sm" c="dimmed">
        {t(($) => $.automations.engagementHint)}
      </Text>
      <SimpleGrid cols={{ base: 2, sm: 3 }}>
        <StatCard label={t(($) => $.automations.opens)} value={report.opens} />
        <StatCard label={t(($) => $.automations.clicks)} value={report.clicks} />
        <StatCard label={t(($) => $.automations.delivered)} value={report.delivered} />
        <StatCard label={t(($) => $.automations.bounced)} value={report.bounced} />
        <StatCard label={t(($) => $.automations.complained)} value={report.complained} />
        <StatCard label={t(($) => $.automations.captured)} value={report.captured} />
        {revenue !== null && (
          <>
            <StatCard label={t(($) => $.messageRevenue.orders)} value={report.revenue.orders} />
            <StatCard label={t(($) => $.messageRevenue.revenue)} value={revenue} />
          </>
        )}
      </SimpleGrid>
      <LinkClickTable links={report.links} />
    </Stack>
  )
}
