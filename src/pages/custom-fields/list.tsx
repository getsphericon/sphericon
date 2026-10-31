import { Badge, EmptyState, Stack, Text, Title } from '@mantine/core'
import { useSuspenseQuery } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { TableSkeleton } from '../../components/Skeletons.tsx'
import { siteCustomFieldsListOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import { Route as customFieldsRoute } from '../../routes/workspaces/$workspaceId/custom-fields/index.tsx'

// The catalogue is small and the server returns it whole (the segment builder needs all of it),
// so this is a plain table, not a paged one.
export function CustomFieldsListPage() {
  const { t } = useTranslation()
  const { workspaceId } = customFieldsRoute.useParams()

  return (
    <Stack>
      <Title order={2}>{t(($) => $.nav.customFields)}</Title>
      <Text c="dimmed">{t(($) => $.customFields.hint)}</Text>

      <QueryBoundary
        fallback={<TableSkeleton />}
        errorTitle={t(($) => $.alerts.customFieldLoadErrorTitle)}
      >
        <CustomFieldsTable workspaceId={workspaceId} />
      </QueryBoundary>
    </Stack>
  )
}

function CustomFieldsTable({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  const records = useSuspenseQuery(siteCustomFieldsListOptions({ path: { workspaceId } })).data
    .items

  return (
    <DataTable
      minHeight={records.length === 0 ? 200 : 0}
      withTableBorder
      records={records}
      emptyState={
        <EmptyState
          title={t(($) => $.customFields.noRecords)}
          description={t(($) => $.customFields.emptyState)}
        />
      }
      columns={[
        { accessor: 'name', title: t(($) => $.customFields.nameLabel) },
        { accessor: 'key', title: t(($) => $.customFields.keyLabel) },
        {
          accessor: 'type',
          title: t(($) => $.customFields.typeLabel),
          render: (record) => (
            <Badge variant="light">{t(($) => $.customFields.types[record.type])}</Badge>
          ),
        },
      ]}
    />
  )
}
