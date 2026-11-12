import {
  Box,
  EmptyState,
  Button,
  Group,
  Menu,
  Modal,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { useDisclosure } from '@mantine/hooks'
import { useSuspenseQuery } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import { TableSkeleton } from '../../components/Skeletons.tsx'
import {
  siteSendingDomainsCreateMutation,
  siteSendingDomainsDeleteMutation,
  siteSendingDomainsListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteSendingDomainResource } from '../../generated/site/types.gen.ts'
import { useDeleteResource } from '../../hooks/useDeleteResource.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { useVerifySendingDomain } from '../../hooks/useVerifySendingDomain.ts'
import type { FieldColumn } from '../../utils/tableColumns.ts'
import { SendingDomainRecordsModal } from './SendingDomainRecordsModal.tsx'
import { SendingDomainStatus } from './SendingDomainStatus.tsx'

interface DomainFormValues {
  domain: string
}

// SendingDomainsTable lists the domains with the records of the one opened; the section's boundary
// shows the load and its failure. A deleted domain is not listed.
function SendingDomainsTable({
  workspaceId,
  canEdit,
  recordsId,
  onShowRecords,
  onCloseRecords,
  verifying,
  onVerify,
  onDelete,
}: {
  workspaceId: string
  canEdit: boolean
  recordsId: string | null
  onShowRecords: (id: string) => void
  onCloseRecords: () => void
  verifying: boolean
  onVerify: (id: string) => void
  onDelete: (record: SiteSendingDomainResource) => void
}) {
  const { t } = useTranslation()
  const records = useSuspenseQuery(siteSendingDomainsListOptions({ path: { workspaceId } })).data
    .items
  const recordsDomain = records.find((record) => record.id === recordsId)

  return (
    <>
      <DataTable
        minHeight={records.length === 0 ? 200 : 0}
        withTableBorder
        highlightOnHover
        onRowClick={({ record }) => onShowRecords(record.id)}
        records={records}
        idAccessor="id"
        emptyState={<EmptyState title={t(($) => $.settings.sendingDomains.empty)} />}
        columns={[
          { accessor: 'domain' } satisfies FieldColumn<(typeof records)[number]>,
          {
            accessor: 'verified',
            title: t(($) => $.settings.sendingDomains.statusColumn),
            render: (record) => <SendingDomainStatus record={record} />,
          },
          {
            accessor: 'actions',
            title: '',
            textAlign: 'right',
            render: (record) => {
              if (!canEdit) {
                return null
              }
              return (
                // A click in the cell must not toggle the row's expansion.
                <Group justify="flex-end" onClick={(event) => event.stopPropagation()}>
                  <RowActionsMenu>
                    <Menu.Item onClick={() => onShowRecords(record.id)}>
                      {t(($) => $.settings.sendingDomains.showRecords)}
                    </Menu.Item>
                    <Menu.Item disabled={verifying} onClick={() => onVerify(record.id)}>
                      {t(($) => $.settings.sendingDomains.verify)}
                    </Menu.Item>
                    <Menu.Item color="red" onClick={() => onDelete(record)}>
                      {t(($) => $.actions.delete)}
                    </Menu.Item>
                  </RowActionsMenu>
                </Group>
              )
            },
          },
        ]}
      />

      <SendingDomainRecordsModal
        workspaceId={workspaceId}
        record={recordsDomain}
        onClose={onCloseRecords}
        canVerify={canEdit}
      />
    </>
  )
}

export function SendingDomainsSection({
  workspaceId,
  canEdit,
}: {
  workspaceId: string
  canEdit: boolean
}) {
  const { t } = useTranslation()
  const [addOpened, { open: openAdd, close: closeAdd }] = useDisclosure(false)
  const [recordsId, setRecordsId] = useState<string | null>(null)

  const form = useForm<DomainFormValues>({ initialValues: { domain: '' } })

  const createMutation = useResourceMutation({
    mutation: siteSendingDomainsCreateMutation(),
    successMessage: t(($) => $.settings.sendingDomains.saved),
    errorTitle: t(($) => $.settings.sendingDomains.createError),
    onDone: (created) => {
      form.reset()
      closeAdd()
      setRecordsId(created.id)
    },
  })

  const verifyMutation = useVerifySendingDomain()

  // A deleted domain is never restored: the person types the domain to confirm.
  const deleteDomain = useDeleteResource({
    mutation: siteSendingDomainsDeleteMutation(),
    workspaceId,
    successMessage: t(($) => $.settings.sendingDomains.deleted),
    errorTitle: t(($) => $.settings.sendingDomains.deleteError),
    consequence: t(($) => $.settings.sendingDomains.deleteConsequence),
  })

  return (
    <Box>
      <Group justify="space-between" mb="sm">
        <Title order={4}>{t(($) => $.settings.sendingDomains.title)}</Title>
        {canEdit ? (
          <Button size="xs" onClick={openAdd}>
            {t(($) => $.settings.sendingDomains.create)}
          </Button>
        ) : null}
      </Group>
      <Text c="dimmed" size="sm" mb="md">
        {t(($) => $.settings.sendingDomains.description)}
      </Text>

      <QueryBoundary
        fallback={<TableSkeleton />}
        errorTitle={t(($) => $.settings.sendingDomains.loadError)}
      >
        <SendingDomainsTable
          workspaceId={workspaceId}
          canEdit={canEdit}
          recordsId={recordsId}
          onShowRecords={setRecordsId}
          onCloseRecords={() => setRecordsId(null)}
          verifying={verifyMutation.isPending}
          onVerify={(id) => verifyMutation.mutate({ path: { workspaceId, id } })}
          onDelete={(record) => deleteDomain(record.id, record.domain)}
        />
      </QueryBoundary>

      <Modal
        opened={addOpened}
        onClose={closeAdd}
        title={t(($) => $.settings.sendingDomains.create)}
      >
        <form
          onSubmit={form.onSubmit((values) =>
            createMutation.mutate({
              path: { workspaceId },
              body: { domain: values.domain.trim() },
            }),
          )}
        >
          <Stack>
            <TextInput
              label={t(($) => $.settings.sendingDomains.domainLabel)}
              placeholder="mail.example.com"
              required
              data-autofocus
              {...form.getInputProps('domain')}
            />
            <Button type="submit" loading={createMutation.isPending}>
              {t(($) => $.settings.sendingDomains.create)}
            </Button>
          </Stack>
        </form>
      </Modal>
    </Box>
  )
}
