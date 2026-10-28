import { Badge, Button, Code, CopyButton, Group, Modal, Stack, Table, Text } from '@mantine/core'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import {
  siteSendingDomainsListQueryKey,
  siteSendingDomainsVerifyMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type {
  SiteSendingDomainResource,
  SiteTrackingReason,
  SiteTrackingState,
} from '../../generated/site/types.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'

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

// agentPrompt is the text a person pastes into an agent connected to this workspace: the domain,
// the records to publish and the order of the steps. The record values are repeated so an agent
// without an API token that may write Sending domains can still publish them.
function agentPrompt(record: SiteSendingDomainResource): string {
  const records = [
    record.dkimRecord,
    record.spfRecord,
    record.dmarcRecord,
    ...(record.trackingRecord
      ? [
          {
            type: record.trackingRecord.type,
            host: record.trackingRecord.host,
            value: record.trackingRecord.target,
          },
        ]
      : []),
  ]
  const lines = records.map((r) => `- ${r.type} ${r.host} -> ${r.value}`)
  return [
    `Set up the Sending domain ${record.domain} (id ${record.id}) in sphericon.`,
    '',
    'Publish these DNS records at the DNS provider of the domain (create the ones that are missing, leave the others alone):',
    ...lines,
    '',
    'The DKIM record is required; SPF, DMARC and the tracking CNAME are recommended.',
    `Then call sending_domains_verify for id ${record.id}, wait a minute, and read the domain again with sending_domains_list: it is done when "verified" is true. DNS can take a while to propagate: if it is not, say what is still missing.`,
  ].join('\n')
}

export function SendingDomainRecordsModal({
  workspaceId,
  record,
  onClose,
}: {
  workspaceId: string
  record: SiteSendingDomainResource | undefined
  onClose: () => void
}) {
  const { t } = useTranslation()

  const verifyMutation = useResourceMutation({
    mutation: siteSendingDomainsVerifyMutation(),
    invalidate: [siteSendingDomainsListQueryKey({ path: { workspaceId } })],
    successMessage: t(($) => $.settings.sendingDomains.verifyQueued),
    errorTitle: t(($) => $.settings.sendingDomains.verifyError),
  })

  return (
    <Modal opened={record !== undefined} onClose={onClose} size="xl" title={record?.domain ?? ''}>
      {record ? (
        <Stack>
          <Group gap="xs">
            {record.verified ? (
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
            )}
          </Group>
          <SendingDomainRecords record={record} />
          {record.archivedAt ? null : (
            <Group justify="flex-end">
              <CopyButton value={agentPrompt(record)}>
                {({ copied, copy }) => (
                  <Button variant="light" onClick={copy}>
                    {copied
                      ? t(($) => $.settings.copied)
                      : t(($) => $.settings.sendingDomains.copyAgentPrompt)}
                  </Button>
                )}
              </CopyButton>
              <Button
                loading={verifyMutation.isPending}
                onClick={() => verifyMutation.mutate({ path: { workspaceId, id: record.id } })}
              >
                {t(($) => $.settings.sendingDomains.verifyNow)}
              </Button>
            </Group>
          )}
        </Stack>
      ) : null}
    </Modal>
  )
}
