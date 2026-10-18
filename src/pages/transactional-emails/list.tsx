import { Badge, Stack, Text, Title } from '@mantine/core'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'

import { PagedTable } from '../../components/PagedTable.tsx'
import { siteTransactionalEmailsListOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteTransactionalEmailStatus } from '../../generated/site/types.gen.ts'
import { Route as transactionalEmailsRoute } from '../../routes/workspaces/$slug/transactional-emails/index.tsx'
import { formatDateTime } from '../../utils/datetime.ts'

const STATUS_COLORS: Record<SiteTransactionalEmailStatus, string> = {
  pending: 'yellow',
  sent: 'teal',
  suppressed: 'gray',
  failed: 'red',
}

function StatusBadge({ t, status }: { t: TFunction; status: SiteTransactionalEmailStatus }) {
  return (
    <Badge color={STATUS_COLORS[status]} variant="light">
      {t(($) => $.transactionalEmails.status[status])}
    </Badge>
  )
}

export function TransactionalEmailsListPage() {
  const { t } = useTranslation()
  const { slug } = transactionalEmailsRoute.useParams()

  return (
    <Stack>
      <Title order={2}>{t(($) => $.transactionalEmails.title)}</Title>
      <Text c="dimmed" size="sm">
        {t(($) => $.transactionalEmails.subtitle)}
      </Text>

      <PagedTable
        pageSize={20}
        options={(query) => siteTransactionalEmailsListOptions({ path: { slug }, query })}
        loadErrorTitle={t(($) => $.alerts.transactionalEmailsLoadErrorTitle)}
        noRecordsText={t(($) => $.transactionalEmails.noRecords)}
        columns={[
          { accessor: 'destination', title: t(($) => $.transactionalEmails.destinationLabel) },
          {
            accessor: 'status',
            title: t(($) => $.transactionalEmails.statusLabel),
            render: (record) => <StatusBadge t={t} status={record.status} />,
          },
          {
            accessor: 'error',
            title: t(($) => $.transactionalEmails.errorLabel),
            render: (record) => record.error ?? '',
          },
          {
            accessor: 'createdAt',
            title: t(($) => $.transactionalEmails.sentAtLabel),
            render: (record) => formatDateTime(record.createdAt),
          },
        ]}
      />
    </Stack>
  )
}
