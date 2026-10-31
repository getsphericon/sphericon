import { Badge, Group, Stack, Title } from '@mantine/core'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'

import { PagedTable } from '../../components/PagedTable.tsx'
import { siteTransactionalEmailsListOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteTransactionalEmailStatus } from '../../generated/site/types.gen.ts'
import { useDateFormat } from '../../hooks/useDateFormat.ts'
import { Route as transactionalEmailsRoute } from '../../routes/workspaces/$workspaceId/transactional-emails/index.tsx'

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
  const { formatDateTime } = useDateFormat()
  const { t } = useTranslation()
  const { workspaceId } = transactionalEmailsRoute.useParams()

  return (
    <Stack>
      <Title order={2}>{t(($) => $.transactionalEmails.title)}</Title>

      <PagedTable
        pageSize={20}
        options={(query) => siteTransactionalEmailsListOptions({ path: { workspaceId }, query })}
        loadErrorTitle={t(($) => $.alerts.transactionalEmailsLoadErrorTitle)}
        noRecordsText={t(($) => $.transactionalEmails.noRecords)}
        columns={[
          { accessor: 'destination', title: t(($) => $.transactionalEmails.destinationLabel) },
          {
            accessor: 'status',
            title: t(($) => $.transactionalEmails.statusLabel),
            render: (record) => (
              <Group gap="xs">
                <StatusBadge t={t} status={record.status} />
                {record.captured ? (
                  <Badge color="gray" variant="outline">
                    {t(($) => $.transactionalEmails.captured)}
                  </Badge>
                ) : null}
              </Group>
            ),
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
