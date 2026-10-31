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

import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import { TableSkeleton } from '../../components/Skeletons.tsx'
import {
  siteSendingDomainsArchiveMutation,
  siteSendingDomainsCreateMutation,
  siteSendingDomainsListOptions,
  siteSendingDomainsListQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteSendingDomainResource } from '../../generated/site/types.gen.ts'
import { useConfirmation } from '../../hooks/useConfirmation.tsx'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { useVerifySendingDomain } from '../../hooks/useVerifySendingDomain.ts'
import { SendingDomainRecordsModal } from './SendingDomainRecordsModal.tsx'

interface DomainFormValues {
  domain: string
}

// SendingDomainsTable lists the domains with the records of the one opened; the section's boundary
// shows the load and its failure.
function SendingDomainsTable({
  workspaceId,
  canEdit,
  recordsId,
  onShowRecords,
  onCloseRecords,
  verifying,
  onVerify,
  removing,
  onRemove,
}: {
  workspaceId: string
  canEdit: boolean
  recordsId: string | null
  onShowRecords: (id: string) => void
  onCloseRecords: () => void
  verifying: boolean
  onVerify: (id: string) => void
  removing: boolean
  onRemove: (record: SiteSendingDomainResource) => void
}) {
  const { t } = useTranslation()
  const records = useSuspenseQuery(
    siteSendingDomainsListOptions({ path: { workspaceId }, query: { archived: false } }),
  ).data.items
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
              canEdit ? (
                // A click in the cell must not toggle the row's expansion.
                <Group justify="flex-end" onClick={(event) => event.stopPropagation()}>
                  <RowActionsMenu>
                    <Menu.Item onClick={() => onShowRecords(record.id)}>
                      {t(($) => $.settings.sendingDomains.showRecords)}
                    </Menu.Item>
                    <Menu.Item disabled={verifying} onClick={() => onVerify(record.id)}>
                      {t(($) => $.settings.sendingDomains.verify)}
                    </Menu.Item>
                    <Menu.Item color="red" disabled={removing} onClick={() => onRemove(record)}>
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
  const confirmRemove = useConfirmation()
  const [addOpened, { open: openAdd, close: closeAdd }] = useDisclosure(false)
  const [recordsId, setRecordsId] = useState<string | null>(null)

  const queryKey = siteSendingDomainsListQueryKey({ path: { workspaceId } })

  const form = useForm<DomainFormValues>({ initialValues: { domain: '' } })

  const createMutation = useResourceMutation({
    mutation: siteSendingDomainsCreateMutation(),
    invalidate: [queryKey],
    successMessage: t(($) => $.settings.sendingDomains.saved),
    errorTitle: t(($) => $.settings.sendingDomains.createError),
    onDone: (created) => {
      form.reset()
      closeAdd()
      setRecordsId(created.id)
    },
  })

  const verifyMutation = useVerifySendingDomain(workspaceId)

  const removeMutation = useResourceMutation({
    mutation: siteSendingDomainsArchiveMutation(),
    invalidate: [queryKey],
    successMessage: t(($) => $.settings.sendingDomains.removed),
    errorTitle: t(($) => $.settings.sendingDomains.removeError),
  })

  // A removed domain stops serving its Tracking host, so a tracker snippet installed on it stops
  // collecting (ADR 0059): a host that already answers is removed only after saying so.
  const onRemove = (record: SiteSendingDomainResource) => {
    const remove = () => removeMutation.mutate({ path: { workspaceId, id: record.id } })
    const tracking = record.trackingRecord
    if (!tracking || tracking.state === 'pending') {
      remove()
      return
    }
    confirmRemove({
      title: t(($) => $.settings.sendingDomains.removeServingTitle),
      description: t(($) => $.settings.sendingDomains.removeServingDescription, {
        host: tracking.host,
      }),
      confirmLabel: t(($) => $.actions.remove),
      onConfirm: remove,
    })
  }

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
          removing={removeMutation.isPending}
          onRemove={onRemove}
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
