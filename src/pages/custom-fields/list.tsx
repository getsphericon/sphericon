import { Badge, Group, Menu, Stack, Text, TextInput, Title } from '@mantine/core'
import { useQuery, useSuspenseQuery } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CenteredEmptyState } from '../../components/CenteredEmptyState.tsx'
import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import { TableSkeleton } from '../../components/Skeletons.tsx'
import {
  siteCustomFieldsListOptions,
  siteWorkspacesListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteCustomFieldResource } from '../../generated/site/types.gen.ts'
import { Route as customFieldsRoute } from '../../routes/workspaces/$workspaceId/custom-fields/index.tsx'
import { can } from '../../utils/permissions.ts'
import { customFieldEditor } from './customFieldEditor.ts'

// The catalogue is small and the server returns it whole (the segment builder needs all of it),
// so this is a plain table, not a paged one. Every member reads it; renaming a field and changing
// its type are offered only with custom_fields:manage, as the API refuses everyone else.
export function CustomFieldsListPage() {
  const { t } = useTranslation()
  const { workspaceId } = customFieldsRoute.useParams()
  const workspaces = useQuery(siteWorkspacesListOptions())
  const canManage = can(
    workspaces.data?.find((w) => w.id === workspaceId),
    'custom_fields:manage',
  )

  return (
    <Stack flex={1}>
      <Title order={2}>{t(($) => $.nav.customFields)}</Title>
      <Text c="dimmed">{t(($) => $.customFields.hint)}</Text>

      <QueryBoundary
        fallback={<TableSkeleton />}
        errorTitle={t(($) => $.alerts.customFieldLoadErrorTitle)}
      >
        <CustomFieldsTable workspaceId={workspaceId} canManage={canManage} />
      </QueryBoundary>
    </Stack>
  )
}

function CustomFieldsTable({
  workspaceId,
  canManage,
}: {
  workspaceId: string
  canManage: boolean
}) {
  const { t } = useTranslation()
  const records = useSuspenseQuery(siteCustomFieldsListOptions({ path: { workspaceId } })).data
    .items
  const [editing, setEditing] = useState<SiteCustomFieldResource | null>(null)

  // No field at all shows its empty state in place of the table, centered in the page.
  if (records.length === 0) {
    return (
      <CenteredEmptyState
        title={t(($) => $.customFields.noRecords)}
        description={t(($) => $.customFields.emptyState)}
      />
    )
  }

  return (
    <>
      <DataTable
        withTableBorder
        records={records}
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
          {
            accessor: 'actions',
            title: '',
            textAlign: 'right',
            render: (record) =>
              canManage ? (
                <Group justify="flex-end">
                  <RowActionsMenu>
                    <Menu.Item onClick={() => setEditing(record)}>
                      {t(($) => $.actions.edit)}
                    </Menu.Item>
                  </RowActionsMenu>
                </Group>
              ) : null,
          },
        ]}
      />
      <customFieldEditor.EditModal id={editing?.id ?? null} onClose={() => setEditing(null)}>
        {(field) => (
          <TextInput label={t(($) => $.customFields.keyLabel)} value={field.key} readOnly />
        )}
      </customFieldEditor.EditModal>
    </>
  )
}
