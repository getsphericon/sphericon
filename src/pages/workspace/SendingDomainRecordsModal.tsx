import {
  Badge,
  Button,
  Code,
  CopyButton,
  Group,
  Modal,
  Stack,
  Tabs,
  Text,
  Textarea,
} from '@mantine/core'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import type { SiteSendingDomainResource } from '../../generated/site/types.gen.ts'
import { useVerifySendingDomain } from '../../hooks/useVerifySendingDomain.ts'

// ReadOnlyValue is a value to copy, in a read-only box that wraps its lines (a DKIM key has no
// spaces to break at, so a plain block would scroll sideways), selectable, with a copy button at
// its top right.
function ReadOnlyValue({
  value,
  ariaLabel,
  maxRows,
}: {
  value: string
  ariaLabel: string
  maxRows?: number
}) {
  return (
    <Group gap="xs" wrap="nowrap" align="flex-start">
      <Textarea
        readOnly
        autosize
        minRows={1}
        {...(maxRows === undefined ? {} : { maxRows })}
        flex={1}
        miw={0}
        value={value}
        aria-label={ariaLabel}
      />
      <CopyButton value={value}>
        {({ copied, copy }) => (
          <Button size="compact-xs" variant="light" aria-label={ariaLabel} onClick={copy}>
            {copied ? '✓' : '⧉'}
          </Button>
        )}
      </CopyButton>
    </Group>
  )
}

// DnsRecord shows one DNS record as a block, type and host above the value, so nothing scrolls
// sideways; the value wraps and has a copy button, what the user pastes into their DNS provider.
function DnsRecord({
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
  const { t } = useTranslation()
  return (
    <Stack gap={6}>
      <div>
        <Group gap="xs">
          <Text fw={600} size="sm">
            {label}
          </Text>
          {status}
        </Group>
        {hint ? (
          <Text c="dimmed" size="xs">
            {hint}
          </Text>
        ) : null}
      </div>
      <Group gap="xs" wrap="wrap">
        <Text c="dimmed" size="xs">
          {t(($) => $.settings.sendingDomains.recordType)}
        </Text>
        <Code>{record.type}</Code>
        <Text c="dimmed" size="xs">
          {t(($) => $.settings.sendingDomains.recordHost)}
        </Text>
        <Code>{record.host}</Code>
      </Group>
      <ReadOnlyValue
        value={record.value}
        ariaLabel={`${label}: ${t(($) => $.settings.sendingDomains.recordValue)}`}
      />
    </Stack>
  )
}

// BulkReady says whether the last DMARC lookup found the domain's record (ADR 0012), for a verified
// domain a lookup answered for: the rule of the overview's notice. An unverified domain does not
// send, and one no lookup answered for is not known, so it says nothing of those.
function BulkReady({ record }: { record: SiteSendingDomainResource }) {
  const { t } = useTranslation()
  if (!record.verified || !record.dmarcCheckedAt) {
    return null
  }
  return record.dmarcFoundAt ? (
    <Badge color="teal" variant="light" size="sm">
      {t(($) => $.settings.sendingDomains.bulkReady)}
    </Badge>
  ) : (
    <Badge color="yellow" variant="light" size="sm">
      {t(($) => $.settings.sendingDomains.notBulkReady)}
    </Badge>
  )
}

function SendingDomainRecords({ record }: { record: SiteSendingDomainResource }) {
  const { t } = useTranslation()
  return (
    <Stack p="md" gap="md">
      <Text size="sm">{t(($) => $.settings.sendingDomains.recordsHint)}</Text>
      <DnsRecord
        label={t(($) => $.settings.sendingDomains.dkim)}
        hint={t(($) => $.settings.sendingDomains.dkimHint)}
        record={record.dkimRecord}
      />
      <DnsRecord
        label={t(($) => $.settings.sendingDomains.spf)}
        hint={t(($) => $.settings.sendingDomains.spfHint)}
        record={record.spfRecord}
      />
      <DnsRecord
        label={t(($) => $.settings.sendingDomains.dmarc)}
        hint={t(($) => $.settings.sendingDomains.dmarcHint)}
        record={record.dmarcRecord}
        status={<BulkReady record={record} />}
      />
    </Stack>
  )
}

// agentPrompt is the text a person pastes into an agent connected to this workspace: the domain,
// the records to publish and the order of the steps. The record values are repeated so an agent
// without an API token that may write Sending domains can still publish them.
function agentPrompt(record: SiteSendingDomainResource): string {
  const records = [record.dkimRecord, record.spfRecord, record.dmarcRecord]
  const lines = records.map((r) => `- ${r.type} ${r.host} -> ${r.value}`)
  return [
    `Set up the Sending domain ${record.domain} (id ${record.id}) in sphericon.`,
    '',
    'Publish these DNS records at the DNS provider of the domain (create the ones that are missing, leave the others alone):',
    ...lines,
    '',
    'The DKIM record is required; SPF and DMARC are recommended.',
    `Then call sending_domains_verify for id ${record.id}, wait a minute, and read the domain again with sending_domains_list: it is done when "verified" is true. DNS can take a while to propagate: if it is not, say what is still missing.`,
  ].join('\n')
}

// SendingDomainRecordsModal shows a Sending domain's DNS records to every member; the check is
// offered only with `canVerify` (sending_domains:manage), as the API refuses everyone else.
export function SendingDomainRecordsModal({
  workspaceId,
  record,
  onClose,
  canVerify,
}: {
  workspaceId: string
  record: SiteSendingDomainResource | undefined
  onClose: () => void
  canVerify: boolean
}) {
  const { t } = useTranslation()

  const verifyMutation = useVerifySendingDomain()

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
          <Tabs defaultValue="records">
            <Tabs.List>
              <Tabs.Tab value="records">{t(($) => $.settings.sendingDomains.showRecords)}</Tabs.Tab>
              <Tabs.Tab value="agent">{t(($) => $.settings.sendingDomains.agentTab)}</Tabs.Tab>
            </Tabs.List>
            <Tabs.Panel value="records">
              <SendingDomainRecords record={record} />
            </Tabs.Panel>
            <Tabs.Panel value="agent">
              <Stack p="md" gap="xs">
                <Text size="sm">{t(($) => $.settings.sendingDomains.agentHint)}</Text>
                <ReadOnlyValue
                  value={agentPrompt(record)}
                  ariaLabel={t(($) => $.settings.sendingDomains.copyAgentPrompt)}
                  maxRows={16}
                />
              </Stack>
            </Tabs.Panel>
          </Tabs>
          {canVerify ? (
            <Group justify="flex-end">
              <Button
                loading={verifyMutation.isPending}
                onClick={() => verifyMutation.mutate({ path: { workspaceId, id: record.id } })}
              >
                {t(($) => $.settings.sendingDomains.verifyNow)}
              </Button>
            </Group>
          ) : null}
        </Stack>
      ) : null}
    </Modal>
  )
}
