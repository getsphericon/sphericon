import {
  Box,
  EmptyState,
  Alert,
  Badge,
  Button,
  Code,
  CopyButton,
  Group,
  Loader,
  Menu,
  Modal,
  Stack,
  Table,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { useDisclosure } from '@mantine/hooks'
import { useQuery } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import {
  siteSendingDomainsCreateMutation,
  siteSendingDomainsDeleteMutation,
  siteSendingDomainsListOptions,
  siteSendingDomainsListQueryKey,
  siteSendingDomainsVerifyMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type {
  SiteSendingDomainResource,
  SiteTrackingReason,
  SiteTrackingState,
} from '../../generated/site/types.gen.ts'
import { useDeleteConfirmation } from '../../hooks/useDeleteConfirmation.tsx'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'

interface DomainFormValues {
  domain: string
}

// DnsRecordRow shows one DNS record (host + value) with a copy button for the
// value — what the user pastes into their DNS provider.
function DnsRecordRow({
  label,
  record,
  hint,
  status,
}: {
  label: string
  record: { type: string; host: string; value: string }
  hint?: string
  status?: ReactNode
}) {
  return (
    <Table.Tr>
      <Table.Td>
        <Text fw={600} size="sm">
          {label}
        </Text>
        {hint ? (
          <Text c="dimmed" size="xs">
            {hint}
          </Text>
        ) : null}
        {status}
      </Table.Td>
      <Table.Td>{record.type}</Table.Td>
      <Table.Td>
        <Code>{record.host}</Code>
      </Table.Td>
      <Table.Td>
        <Group gap="xs" wrap="nowrap" align="flex-start">
          <Code block maw={420}>
            {record.value}
          </Code>
          <CopyButton value={record.value}>
            {({ copied, copy }) => (
              <Button size="compact-xs" variant="light" onClick={copy}>
                {copied ? '✓' : '⧉'}
              </Button>
            )}
          </CopyButton>
        </Group>
      </Table.Td>
    </Table.Tr>
  )
}

const trackingStateColor: Record<SiteTrackingState, string> = {
  pending: 'gray',
  dns_verified: 'yellow',
  serving: 'teal',
}

// TrackingStatus shows where the Tracking domain stands and, when something holds it
// back, why (ADR 0038).
function TrackingStatus({
  state,
  reason,
}: {
  state: SiteTrackingState
  reason?: SiteTrackingReason | undefined
}) {
  const { t } = useTranslation()
  const labels: Record<SiteTrackingState, string> = {
    pending: t(($) => $.settings.sendingDomains.trackingState.pending),
    dns_verified: t(($) => $.settings.sendingDomains.trackingState.dnsVerified),
    serving: t(($) => $.settings.sendingDomains.trackingState.serving),
  }
  const reasons: Record<SiteTrackingReason, string> = {
    host_taken: t(($) => $.settings.sendingDomains.trackingReason.hostTaken),
  }
  return (
    <>
      <Badge mt={4} color={trackingStateColor[state]} variant="light">
        {labels[state]}
      </Badge>
      {reason ? (
        <Text c="red" size="xs" mt={4}>
          {reasons[reason]}
        </Text>
      ) : null}
    </>
  )
}

// mantine-datatable calls `content` as a plain function, not as a component, so the hooks used by
// SendingDomainRecords need this wrapper to get their own component boundary.
function SendingDomainRowExpansion({ record }: { record: SiteSendingDomainResource }) {
  return <SendingDomainRecords record={record} />
}

function SendingDomainRecords({ record }: { record: SiteSendingDomainResource }) {
  const { t } = useTranslation()
  return (
    <Stack p="md" gap="xs">
      <Text size="sm">{t(($) => $.settings.sendingDomains.recordsHint)}</Text>
      <Table.ScrollContainer minWidth={480}>
        <Table>
          <Table.Thead>
            <Table.Tr>
              <Table.Th>{t(($) => $.settings.sendingDomains.recordName)}</Table.Th>
              <Table.Th>{t(($) => $.settings.sendingDomains.recordType)}</Table.Th>
              <Table.Th>{t(($) => $.settings.sendingDomains.recordHost)}</Table.Th>
              <Table.Th>{t(($) => $.settings.sendingDomains.recordValue)}</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            <DnsRecordRow
              label={t(($) => $.settings.sendingDomains.dkim)}
              hint={t(($) => $.settings.sendingDomains.dkimHint)}
              record={record.dkimRecord}
            />
            <DnsRecordRow
              label={t(($) => $.settings.sendingDomains.spf)}
              hint={t(($) => $.settings.sendingDomains.spfHint)}
              record={record.spfRecord}
            />
            <DnsRecordRow
              label={t(($) => $.settings.sendingDomains.dmarc)}
              hint={t(($) => $.settings.sendingDomains.dmarcHint)}
              record={record.dmarcRecord}
            />
            {record.trackingRecord ? (
              <DnsRecordRow
                label={t(($) => $.settings.sendingDomains.tracking)}
                hint={t(($) => $.settings.sendingDomains.trackingHint)}
                record={{
                  type: record.trackingRecord.type,
                  host: record.trackingRecord.host,
                  value: record.trackingRecord.target,
                }}
                status={
                  <TrackingStatus
                    state={record.trackingRecord.state}
                    reason={record.trackingRecord.reason}
                  />
                }
              />
            ) : null}
          </Table.Tbody>
        </Table>
      </Table.ScrollContainer>
    </Stack>
  )
}

export function SendingDomainsSection({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  const confirmDelete = useDeleteConfirmation()
  const [addOpened, { open: openAdd, close: closeAdd }] = useDisclosure(false)

  const queryKey = siteSendingDomainsListQueryKey({ path: { workspaceId } })
  const domainsQuery = useQuery(siteSendingDomainsListOptions({ path: { workspaceId } }))

  const form = useForm<DomainFormValues>({ initialValues: { domain: '' } })

  const createMutation = useResourceMutation({
    mutation: siteSendingDomainsCreateMutation(),
    invalidate: [queryKey],
    successMessage: t(($) => $.settings.sendingDomains.saved),
    errorTitle: t(($) => $.settings.sendingDomains.createError),
    onDone: () => {
      form.reset()
      closeAdd()
    },
  })

  const verifyMutation = useResourceMutation({
    mutation: siteSendingDomainsVerifyMutation(),
    invalidate: [queryKey],
    successMessage: t(($) => $.settings.sendingDomains.verifyQueued),
    errorTitle: t(($) => $.settings.sendingDomains.verifyError),
  })

  const deleteMutation = useResourceMutation({
    mutation: siteSendingDomainsDeleteMutation(),
    invalidate: [queryKey],
    errorTitle: t(($) => $.settings.sendingDomains.deleteError),
  })

  const onDelete = (id: string) => {
    confirmDelete({
      title: t(($) => $.settings.sendingDomains.deleteConfirmTitle),
      description: t(($) => $.settings.sendingDomains.deleteConfirmDescription),
      onConfirm: () => deleteMutation.mutate({ path: { workspaceId, id } }),
    })
  }

  const records = domainsQuery.data?.items ?? []

  return (
    <Box>
      <Group justify="space-between" mb="xs">
        <Title order={4}>{t(($) => $.settings.sendingDomains.title)}</Title>
        <Button onClick={openAdd}>{t(($) => $.settings.sendingDomains.create)}</Button>
      </Group>
      <Text c="dimmed" size="sm" mb="md">
        {t(($) => $.settings.sendingDomains.description)}
      </Text>

      {domainsQuery.isError ? (
        <Alert color="red" title={t(($) => $.settings.sendingDomains.loadError)} mb="md">
          {t(($) => $.settings.sendingDomains.loadError)}
        </Alert>
      ) : null}

      {domainsQuery.isLoading ? <Loader /> : null}

      <DataTable
        minHeight={200}
        withTableBorder
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
            render: (record) => (
              // A click in the cell must not toggle the row's expansion.
              <Group justify="flex-end" onClick={(event) => event.stopPropagation()}>
                <RowActionsMenu>
                  <Menu.Item
                    disabled={verifyMutation.isPending}
                    onClick={() => verifyMutation.mutate({ path: { workspaceId, id: record.id } })}
                  >
                    {t(($) => $.settings.sendingDomains.verify)}
                  </Menu.Item>
                  <Menu.Item color="red" onClick={() => onDelete(record.id)}>
                    {t(($) => $.settings.sendingDomains.delete)}
                  </Menu.Item>
                </RowActionsMenu>
              </Group>
            ),
          },
        ]}
        rowExpansion={{ content: SendingDomainRowExpansion }}
      />

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
