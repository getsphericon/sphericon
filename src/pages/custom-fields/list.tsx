import {
  Badge,
  Button,
  EmptyState,
  Group,
  Menu,
  Modal,
  Select,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { useQuery, useSuspenseQuery } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import { TableSkeleton } from '../../components/Skeletons.tsx'
import {
  siteCustomFieldsListOptions,
  siteCustomFieldsUpdateMutation,
  siteWorkspacesListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type {
  SiteCustomFieldResource,
  SiteCustomFieldType,
} from '../../generated/site/types.gen.ts'
import { zSiteCustomFieldType } from '../../generated/site/zod.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as customFieldsRoute } from '../../routes/workspaces/$workspaceId/custom-fields/index.tsx'
import { can } from '../../utils/permissions.ts'

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
    <Stack>
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

  return (
    <>
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
      <Modal
        opened={editing !== null}
        onClose={() => setEditing(null)}
        title={t(($) => $.customFields.editTitle)}
      >
        {editing ? (
          <EditCustomFieldForm
            key={editing.id}
            workspaceId={workspaceId}
            field={editing}
            onDone={() => setEditing(null)}
          />
        ) : null}
      </Modal>
    </>
  )
}

function EditCustomFieldForm({
  workspaceId,
  field,
  onDone,
}: {
  workspaceId: string
  field: SiteCustomFieldResource
  onDone: () => void
}) {
  const { t } = useTranslation()
  const form = useForm<{ name: string; type: SiteCustomFieldType }>({
    initialValues: { name: field.name, type: field.type },
  })
  const updateMutation = useResourceMutation({
    mutation: siteCustomFieldsUpdateMutation(),
    errorTitle: t(($) => $.customFields.updateError),
    onDone,
  })

  return (
    <form
      onSubmit={form.onSubmit((values) =>
        updateMutation.mutate({
          path: { workspaceId, id: field.id },
          body: { name: values.name.trim(), type: values.type },
        }),
      )}
    >
      <Stack>
        <TextInput
          label={t(($) => $.customFields.nameLabel)}
          required
          data-autofocus
          {...form.getInputProps('name')}
        />
        <TextInput label={t(($) => $.customFields.keyLabel)} value={field.key} readOnly />
        <Select
          label={t(($) => $.customFields.typeLabel)}
          description={t(($) => $.customFields.typeHint)}
          allowDeselect={false}
          data={zSiteCustomFieldType.options.map((type) => ({
            value: type,
            label: t(($) => $.customFields.types[type]),
          }))}
          {...form.getInputProps('type')}
        />
        <Button type="submit" loading={updateMutation.isPending}>
          {t(($) => $.actions.save)}
        </Button>
      </Stack>
    </form>
  )
}
