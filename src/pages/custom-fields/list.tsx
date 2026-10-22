import { Badge, EmptyState, Stack, Text, Title } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import { siteCustomFieldsListOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import { Route as customFieldsRoute } from '../../routes/workspaces/$workspaceId/custom-fields/index.tsx'

// The catalogue is small and the server returns it whole (the segment builder needs all of it),
// so this is a plain table, not a paged one.
export function CustomFieldsListPage() {
  const { t } = useTranslation()
  const { workspaceId } = customFieldsRoute.useParams()
  const list = useQuery(siteCustomFieldsListOptions({ path: { workspaceId } }))

  return (
    <Stack>
      <Title order={2}>{t(($) => $.nav.customFields)}</Title>
      <Text c="dimmed">{t(($) => $.customFields.hint)}</Text>

      {list.isError ? (
        <ApiErrorAlert
          error={list.error}
          title={t(($) => $.alerts.customFieldLoadErrorTitle)}
          fallback={t(($) => $.alerts.customFieldLoadErrorTitle)}
        />
      ) : null}

      <DataTable
        minHeight={list.data?.items.length ? 0 : 200}
        withTableBorder
        records={list.data?.items ?? []}
        fetching={list.isFetching}
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
    </Stack>
  )
}
