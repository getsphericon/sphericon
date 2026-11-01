import {
  Box,
  EmptyState,
  Badge,
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

import { ArchiveViewControl } from '../../components/ArchiveViewControl.tsx'
import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import { TableSkeleton } from '../../components/Skeletons.tsx'
import {
  siteSendingDomainsArchiveMutation,
  siteSendingDomainsCreateMutation,
  siteSendingDomainsListOptions,
  siteSendingDomainsUnarchiveMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { useVerifySendingDomain } from '../../hooks/useVerifySendingDomain.ts'
import { SendingDomainRecordsModal } from './SendingDomainRecordsModal.tsx'

interface DomainFormValues {
  domain: string
}

// SendingDomainsTable lists the kept domains with the records of the one opened, or the archived
// ones to restore; the section's boundary shows the load and its failure.
function SendingDomainsTable({
  workspaceId,
  archived,
  canEdit,
  recordsId,
  onShowRecords,
  onCloseRecords,
  verifying,
  onVerify,
  removing,
  onRemove,
  restoring,
  onRestore,
}: {
  workspaceId: string
  archived: boolean
  canEdit: boolean
  recordsId: string | null
  onShowRecords: (id: string) => void
  onCloseRecords: () => void
  verifying: boolean
  onVerify: (id: string) => void
  removing: boolean
  onRemove: (id: string) => void
  restoring: boolean
  onRestore: (id: string) => void
}) {
  const { t } = useTranslation()
  const records = useSuspenseQuery(
    siteSendingDomainsListOptions({ path: { workspaceId }, query: { archived } }),
  ).data.items
  const recordsDomain = records.find((record) => record.id === recordsId)

  return (
    <>
      <DataTable
        minHeight={records.length === 0 ? 200 : 0}
        withTableBorder
        highlightOnHover={!archived}
        // An archived domain sends nothing: it has no records to set.
        onRowClick={({ record }) => {
          if (!archived) {
            onShowRecords(record.id)
          }
        }}
        records={records}
        idAccessor="id"
        emptyState={
          <EmptyState
            title={
              archived
                ? t(($) => $.settings.sendingDomains.emptyArchived)
                : t(($) => $.settings.sendingDomains.empty)
            }
          />
        }
        columns={[
          { accessor: 'domain' },
          {
            accessor: 'verified',
            title: t(($) => $.settings.sendingDomains.statusColumn),
            render: (record) =>
              record.verified ? (
                <Badge color="teal" variant="light">
                  {t(($) => $.settings.sendingDomains.verified)}
                </Badge>
              ) : record.verificationLostAt ? (
                <Badge color="red" variant="light">
                  {t(($) => $.settings.sendingDomains.lost)}
                </Badge>
              ) : (
                <Badge color="gray" variant="light">
                  {t(($) => $.settings.sendingDomains.pending)}
                </Badge>
              ),
          },
          {
            accessor: 'actions',
            title: '',
            textAlign: 'right',
            render: (record) =>
              canEdit && archived ? (
                <Group justify="flex-end">
                  <RowActionsMenu>
                    <Menu.Item disabled={restoring} onClick={() => onRestore(record.id)}>
                      {t(($) => $.actions.unarchive)}
                    </Menu.Item>
                  </RowActionsMenu>
                </Group>
              ) : canEdit ? (
                // A click in the cell must not toggle the row's expansion.
                <Group justify="flex-end" onClick={(event) => event.stopPropagation()}>
                  <RowActionsMenu>
                    <Menu.Item onClick={() => onShowRecords(record.id)}>
                      {t(($) => $.settings.sendingDomains.showRecords)}
                    </Menu.Item>
                    <Menu.Item disabled={verifying} onClick={() => onVerify(record.id)}>
                      {t(($) => $.settings.sendingDomains.verify)}
                    </Menu.Item>
                    <Menu.Item color="red" disabled={removing} onClick={() => onRemove(record.id)}>
                      {t(($) => $.actions.remove)}
                    </Menu.Item>
                  </RowActionsMenu>
                </Group>
              ) : null,
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
  const [archived, setArchived] = useState(false)

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

  const removeMutation = useResourceMutation({
    mutation: siteSendingDomainsArchiveMutation(),
    successMessage: t(($) => $.settings.sendingDomains.removed),
    errorTitle: t(($) => $.settings.sendingDomains.removeError),
  })

  const restoreMutation = useResourceMutation({
    mutation: siteSendingDomainsUnarchiveMutation(),
    successMessage: t(($) => $.settings.sendingDomains.restored),
    errorTitle: t(($) => $.settings.sendingDomains.restoreError),
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

      <ArchiveViewControl archived={archived} onChange={setArchived} mb="sm" />
      <QueryBoundary
        fallback={<TableSkeleton />}
        errorTitle={t(($) => $.settings.sendingDomains.loadError)}
      >
        <SendingDomainsTable
          workspaceId={workspaceId}
          archived={archived}
          canEdit={canEdit}
          recordsId={recordsId}
          onShowRecords={setRecordsId}
          onCloseRecords={() => setRecordsId(null)}
          verifying={verifyMutation.isPending}
          onVerify={(id) => verifyMutation.mutate({ path: { workspaceId, id } })}
          removing={removeMutation.isPending}
          onRemove={(id) => removeMutation.mutate({ path: { workspaceId, id } })}
          restoring={restoreMutation.isPending}
          onRestore={(id) => restoreMutation.mutate({ path: { workspaceId, id } })}
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
