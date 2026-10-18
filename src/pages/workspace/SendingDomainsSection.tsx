import {
  Alert,
  Badge,
  Button,
  Card,
  Code,
  CopyButton,
  Group,
  Loader,
  Stack,
  Table,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { useQuery } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import { useTranslation } from 'react-i18next'

import {
  siteSendingDomainsCreateMutation,
  siteSendingDomainsDeleteMutation,
  siteSendingDomainsListOptions,
  siteSendingDomainsListQueryKey,
  siteSendingDomainsVerifyMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteDnsRecord, SiteSendingDomainResource } from '../../generated/site/types.gen.ts'
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
}: {
  label: string
  record: SiteDnsRecord
  hint?: string
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
          </Table.Tbody>
        </Table>
      </Table.ScrollContainer>
    </Stack>
  )
}

export function SendingDomainsSection({ slug }: { slug: string }) {
  const { t } = useTranslation()
  const confirmDelete = useDeleteConfirmation()

  const queryKey = siteSendingDomainsListQueryKey({ path: { slug } })
  const domainsQuery = useQuery(siteSendingDomainsListOptions({ path: { slug } }))

  const form = useForm<DomainFormValues>({ initialValues: { domain: '' } })

  const createMutation = useResourceMutation({
    mutation: siteSendingDomainsCreateMutation(),
    invalidate: [queryKey],
    successMessage: t(($) => $.settings.sendingDomains.saved),
    errorTitle: t(($) => $.settings.sendingDomains.createError),
    onDone: () => form.reset(),
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
      onConfirm: () => deleteMutation.mutate({ path: { slug, id } }),
    })
  }

  const records = domainsQuery.data?.items ?? []

  return (
    <Card withBorder>
      <Title order={4} mb="xs">
        {t(($) => $.settings.sendingDomains.title)}
      </Title>
      <Text c="dimmed" size="sm" mb="md">
        {t(($) => $.settings.sendingDomains.description)}
      </Text>

      {domainsQuery.isError ? (
        <Alert color="red" title={t(($) => $.settings.sendingDomains.loadError)} mb="md">
          {t(($) => $.settings.sendingDomains.loadError)}
        </Alert>
      ) : null}

      <form
        onSubmit={form.onSubmit((values) =>
          createMutation.mutate({
            path: { slug },
            body: { domain: values.domain.trim() },
          }),
        )}
      >
        <Group align="flex-end" maw={520} mb="md">
          <TextInput
            flex={1}
            label={t(($) => $.settings.sendingDomains.domainLabel)}
            placeholder="mail.example.com"
            required
            {...form.getInputProps('domain')}
          />
          <Button type="submit" loading={createMutation.isPending}>
            {t(($) => $.settings.sendingDomains.create)}
          </Button>
        </Group>
      </form>

      {domainsQuery.isLoading ? <Loader /> : null}

      <DataTable
        withTableBorder
        records={records}
        idAccessor="id"
        noRecordsText={t(($) => $.settings.sendingDomains.empty)}
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
              <Group gap="xs" justify="flex-end" wrap="nowrap">
                <Button
                  size="compact-sm"
                  variant="light"
                  loading={verifyMutation.isPending}
                  onClick={() => verifyMutation.mutate({ path: { slug, id: record.id } })}
                >
                  {t(($) => $.settings.sendingDomains.verify)}
                </Button>
                <Button
                  size="compact-sm"
                  variant="light"
                  color="red"
                  onClick={() => onDelete(record.id)}
                >
                  {t(($) => $.settings.sendingDomains.delete)}
                </Button>
              </Group>
            ),
          },
        ]}
        rowExpansion={{ content: SendingDomainRowExpansion }}
      />
    </Card>
  )
}
