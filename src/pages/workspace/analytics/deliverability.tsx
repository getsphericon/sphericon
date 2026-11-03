import { EmptyState, Stack, Text } from '@mantine/core'
import { useSuspenseQuery } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import { useTranslation } from 'react-i18next'

import { siteAnalyticsDeliverabilityOptions } from '../../../generated/site/@tanstack/react-query.gen.ts'
import { useAnalyticsRange } from '../../../hooks/useAnalyticsRange.ts'
import { useFormat } from '../../../hooks/useFormat.ts'
import { Route as deliverabilityRoute } from '../../../routes/workspaces/$workspaceId/analytics/deliverability.tsx'
import { ReportBoundary } from './ReportBoundary.tsx'

// DeliverabilityPage is the Deliverability report: each Sending domain's Complaint and Bounce
// rate over the section's window, with the counts they are made of. Numbers only: the core sets no
// limit on them, so no rate is coloured as good or bad.
export function DeliverabilityPage() {
  const { t } = useTranslation()
  return (
    <ReportBoundary errorTitle={t(($) => $.analytics.deliverability.loadError)}>
      <DeliverabilityReport />
    </ReportBoundary>
  )
}

function DeliverabilityReport() {
  const { t } = useTranslation()
  const { formatPercent } = useFormat()
  const { workspaceId } = deliverabilityRoute.useParams()
  const range = useAnalyticsRange()

  const domains = useSuspenseQuery(
    siteAnalyticsDeliverabilityOptions({ path: { workspaceId }, query: { range } }),
  ).data.domains
  // A rate over nothing is undefined, and is shown as such, never as 0%.
  const rate = (value: number | null) => (value === null ? '–' : formatPercent(value))

  if (domains.length === 0) {
    return <EmptyState title={t(($) => $.analytics.deliverability.empty)} />
  }

  return (
    <Stack gap="xs">
      <Text size="sm" c="dimmed">
        {t(($) => $.analytics.deliverability.note)}
      </Text>
      <DataTable
        withTableBorder
        highlightOnHover
        records={domains}
        idAccessor="sendingDomainId"
        columns={[
          { accessor: 'domain', title: t(($) => $.analytics.deliverability.domain) },
          {
            accessor: 'sent',
            title: t(($) => $.analytics.deliverability.sent),
            textAlign: 'right',
            render: (record) => record.bounceRate.denominator,
          },
          {
            accessor: 'delivered',
            title: t(($) => $.analytics.deliverability.delivered),
            textAlign: 'right',
            render: (record) => record.complaintRate.denominator,
          },
          {
            accessor: 'hardBounces',
            title: t(($) => $.analytics.deliverability.hardBounces),
            textAlign: 'right',
            render: (record) => record.bounceRate.numerator,
          },
          {
            accessor: 'complaints',
            title: t(($) => $.analytics.deliverability.complaints),
            textAlign: 'right',
            render: (record) => record.complaintRate.numerator,
          },
          {
            accessor: 'complaintRate',
            title: t(($) => $.analytics.deliverability.complaintRate),
            textAlign: 'right',
            render: (record) => rate(record.complaintRate.rate),
          },
          {
            accessor: 'bounceRate',
            title: t(($) => $.analytics.deliverability.bounceRate),
            textAlign: 'right',
            render: (record) => rate(record.bounceRate.rate),
          },
        ]}
      />
    </Stack>
  )
}
