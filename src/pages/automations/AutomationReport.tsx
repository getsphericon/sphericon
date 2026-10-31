import { Group, SegmentedControl, SimpleGrid, Stack, Text, Title } from '@mantine/core'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import { LinkClickTable } from '../../components/LinkClickTable.tsx'
import { StatCard } from '../../components/StatCard.tsx'
import { siteAutomationReportsGetOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import type {
  SiteAutomationReport,
  SiteAutomationReportPeriod,
  SiteAutomationResults,
} from '../../generated/site/types.gen.ts'
import { zSiteAutomationReportPeriod } from '../../generated/site/zod.gen.ts'
import { formatDate } from '../../utils/datetime.ts'
import { formatMessageRevenue } from '../../utils/messageRevenue.ts'
import { formatChange, formatDuration } from './resultsFormat.ts'

const PERIODS = zSiteAutomationReportPeriod.options

// What an Automation's Enrollments did over the period, each figure with the period before it: the
// line under a figure is its value then and the change since.
function ResultsFigures({ results }: { results: SiteAutomationResults }) {
  const { t } = useTranslation()
  const { current, previous } = results
  const label = t(($) => $.automations.results.previous, {
    period: t(($) => $.automations.results.periods[results.period]),
  })
  const count = (name: string, now: number, before: number) => (
    <StatCard
      label={name}
      value={now}
      comparison={{ label, value: before, change: formatChange(now, before) }}
    />
  )

  const duration = current.averageDurationSeconds
  const durationBefore = previous.averageDurationSeconds
  const revenue = formatMessageRevenue(current.revenue)
  const revenueBefore = formatMessageRevenue(previous.revenue)

  return (
    <>
      <Text size="sm" c="dimmed">
        {t(($) => $.automations.results.hint, {
          from: formatDate(current.from),
          to: formatDate(current.to),
        })}
      </Text>
      <SimpleGrid cols={{ base: 2, sm: 3 }}>
        {count(
          t(($) => $.automations.results.entered),
          current.entered,
          previous.entered,
        )}
        {count(
          t(($) => $.automations.results.completed),
          current.completed,
          previous.completed,
        )}
        {count(
          t(($) => $.automations.results.goalReached),
          current.goalReached,
          previous.goalReached,
        )}
        {count(
          t(($) => $.automations.results.exited),
          current.exited,
          previous.exited,
        )}
        {count(
          t(($) => $.automations.results.failed),
          current.failed,
          previous.failed,
        )}
        <StatCard
          label={t(($) => $.automations.results.averageDuration)}
          value={duration === null ? '–' : formatDuration(duration)}
          comparison={{
            label,
            value: durationBefore === null ? '–' : formatDuration(durationBefore),
            change:
              duration === null || durationBefore === null
                ? null
                : formatChange(duration, durationBefore),
          }}
        />
        {revenue !== null && revenueBefore !== null && (
          <>
            {count(
              t(($) => $.automations.results.orders),
              current.revenue.orders,
              previous.revenue.orders,
            )}
            <StatCard
              label={t(($) => $.automations.results.revenue)}
              value={revenue}
              comparison={{
                label,
                value: revenueBefore,
                change: formatChange(current.revenue.amount, previous.revenue.amount),
              }}
            />
          </>
        )}
      </SimpleGrid>
    </>
  )
}

// How the emails of an Automation were engaged with over its whole history.
function Engagement({ report }: { report: SiteAutomationReport }) {
  const { t } = useTranslation()
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

/**
 * The results of an Automation over the period picked and the engagement of its emails. The
 * period stays as picked while its figures load or fail to; until one is picked, the request names
 * none and the contract's default is the one the answer reports.
 */
export function AutomationReport({
  workspaceId,
  automationId,
}: {
  workspaceId: string
  automationId: string
}) {
  const { t } = useTranslation()
  const [picked, setPicked] = useState<SiteAutomationReportPeriod>()
  const reportQuery = useQuery({
    ...siteAutomationReportsGetOptions({
      path: { workspaceId, automationId },
      ...(picked === undefined ? {} : { query: { period: picked } }),
    }),
    placeholderData: keepPreviousData,
  })
  const report = reportQuery.data
  const period = picked ?? report?.results.period ?? null

  return (
    <Stack gap="lg">
      <Stack gap="xs">
        <Group justify="space-between" wrap="wrap">
          <Title order={5}>{t(($) => $.automations.results.title)}</Title>
          <SegmentedControl
            size="xs"
            value={period ?? ''}
            onChange={(value) => setPicked(PERIODS.find((p) => p === value))}
            data={PERIODS.map((p) => ({
              value: p,
              label: t(($) => $.automations.results.periods[p]),
            }))}
          />
        </Group>
        {reportQuery.isError && (
          <ApiErrorAlert
            error={reportQuery.error}
            title={t(($) => $.automations.results.loadError)}
            fallback={t(($) => $.automations.results.loadError)}
          />
        )}
        {report && !reportQuery.isPlaceholderData && <ResultsFigures results={report.results} />}
      </Stack>
      {report && <Engagement report={report} />}
    </Stack>
  )
}
