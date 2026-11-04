import {
  Box,
  EmptyState,
  Alert,
  Badge,
  Button,
  CopyButton,
  Group,
  Menu,
  Modal,
  MultiSelect,
  Select,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { useDisclosure } from '@mantine/hooks'
import { modals } from '@mantine/modals'
import { useSuspenseQuery } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { DetailsBadge } from '../../components/DetailsBadge.tsx'
import { EventActionNames, EventActionOption } from '../../components/EventActionName.tsx'
import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import { TableSkeleton } from '../../components/Skeletons.tsx'
import {
  siteWebhooksCreateMutation,
  siteWebhooksDeleteMutation,
  siteWebhooksListOptions,
  siteWebhooksRegenerateSecretMutation,
  siteWebhooksTestMutation,
  siteWebhooksUpdateMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type {
  CreateWebhookInput,
  SiteWebhookEndpointResource,
  SiteWebhookFailure,
  SiteWebhookTestResult,
} from '../../generated/site/types.gen.ts'
import { useConfirmation } from '../../hooks/useConfirmation.tsx'
import { useEventActionLabel } from '../../hooks/useEventActionLabel.ts'
import { useFormat } from '../../hooks/useFormat.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { AUDIT_TARGET_WEBHOOK_ENDPOINT } from '../../utils/auditFilter.ts'
import {
  CONTACT_CHANGE_ACTIONS,
  ENROLLMENT_ACTIONS,
  TELEGRAM_ACTIONS,
} from '../../utils/systemEvents.ts'
import { ChangeHistoryLink } from './AuditLogSection.tsx'

// The domain events an endpoint can subscribe to. An empty selection means all customer
// events; audit.entry (Enterprise) is never included and must be chosen explicitly.
const EVENT_OPTIONS = [
  'contact.created',
  'contact.erased',
  ...CONTACT_CHANGE_ACTIONS,
  'email.opened',
  'email.clicked',
  'email.unsubscribed',
  ...ENROLLMENT_ACTIONS,
  ...TELEGRAM_ACTIONS,
  'audit.entry',
]

// useFailureSummary says why an endpoint's deliveries fail, how many in a row and since when.
function useFailureSummary() {
  const { t } = useTranslation()
  const { formatDateTime } = useFormat()
  return (failure: SiteWebhookFailure) =>
    t(($) => $.settings.webhooks.failureSummary, {
      reason: {
        timeout: t(($) => $.settings.webhooks.errorTimeout),
        blocked_address: t(($) => $.settings.webhooks.errorBlockedAddress),
        http_status: t(($) => $.settings.webhooks.errorHttpStatus, {
          status: failure.status ?? 0,
        }),
        network: t(($) => $.settings.webhooks.errorNetwork),
      }[failure.kind],
      count: failure.consecutiveFailures,
      since: formatDateTime(failure.since),
    })
}

// WebhookStatus is the one status of an endpoint: Disabled wins over Failing, and the reason,
// the time it was turned off and the failing deliveries open in the badge's popover.
function WebhookStatus({ record }: { record: SiteWebhookEndpointResource }) {
  const { t } = useTranslation()
  const { formatDateTime } = useFormat()
  const failureSummary = useFailureSummary()
  const failure = record.failure ? failureSummary(record.failure) : null

  if (!record.enabled) {
    const reason = record.disabledReason
      ? {
          owner: t(($) => $.settings.webhooks.disabledOwner),
          gone: t(($) => $.settings.webhooks.disabledGone),
          persistent_failure: t(($) => $.settings.webhooks.disabledPersistentFailure),
        }[record.disabledReason]
      : null
    return (
      <DetailsBadge
        variant="light"
        color="gray"
        details={[
          reason && record.disabledAt ? `${reason} (${formatDateTime(record.disabledAt)})` : reason,
          failure,
        ]}
      >
        {t(($) => $.settings.webhooks.disabled)}
      </DetailsBadge>
    )
  }
  if (failure !== null) {
    return (
      <DetailsBadge variant="light" color="red" details={[failure]}>
        {t(($) => $.settings.webhooks.failing)}
      </DetailsBadge>
    )
  }
  return (
    <Badge variant="light" color="teal">
      {t(($) => $.settings.webhooks.enabled)}
    </Badge>
  )
}

// A sample can be sent for any customer event; audit.entry is Enterprise and has no sample.
const TEST_EVENT_OPTIONS = EVENT_OPTIONS.filter((name) => name !== 'audit.entry')

function TestResultAlert({ result }: { result: SiteWebhookTestResult }) {
  const { t } = useTranslation()
  if (result.delivered) {
    return (
      <Alert color="teal" mt="md">
        {t(($) => $.settings.webhooks.testDelivered, { status: result.status ?? 0 })}
      </Alert>
    )
  }
  const reason = {
    timeout: t(($) => $.settings.webhooks.errorTimeout),
    blocked_address: t(($) => $.settings.webhooks.errorBlockedAddress),
    http_status: t(($) => $.settings.webhooks.errorHttpStatus, { status: result.status ?? 0 }),
    network: t(($) => $.settings.webhooks.errorNetwork),
  }[result.kind ?? 'network']
  return (
    <Alert color="red" mt="md">
      {t(($) => $.settings.webhooks.testFailed, { reason })}
    </Alert>
  )
}

function TestEventModal({
  workspaceId,
  id,
  onClose,
}: {
  workspaceId: string
  id: string | null
  onClose: () => void
}) {
  const { t } = useTranslation()
  const labelOf = useEventActionLabel()
  const [eventType, setEventType] = useState<string | null>(TEST_EVENT_OPTIONS[0] ?? null)
  const [result, setResult] = useState<SiteWebhookTestResult | null>(null)

  const testMutation = useResourceMutation({
    mutation: siteWebhooksTestMutation(),
    errorTitle: t(($) => $.settings.webhooks.testError),
    onDone: (data) => setResult(data),
  })

  return (
    <Modal
      opened={id !== null}
      onClose={() => {
        onClose()
        setResult(null)
      }}
      title={t(($) => $.settings.webhooks.sendTestTitle)}
    >
      <Stack gap="sm">
        <Text size="sm" c="dimmed">
          {t(($) => $.settings.webhooks.sendTestHint)}
        </Text>
        <Select
          label={t(($) => $.settings.webhooks.testEventLabel)}
          data={TEST_EVENT_OPTIONS.map((name) => ({ value: name, label: labelOf(name) }))}
          renderOption={({ option }) => <EventActionOption action={option.value} />}
          value={eventType}
          onChange={setEventType}
          allowDeselect={false}
        />
        <Button
          loading={testMutation.isPending}
          disabled={eventType === null || id === null}
          onClick={() => {
            if (eventType !== null && id !== null) {
              testMutation.mutate({ path: { workspaceId, id }, body: { eventType } })
            }
          }}
        >
          {t(($) => $.settings.webhooks.sendTestSubmit)}
        </Button>
      </Stack>
      {result ? <TestResultAlert result={result} /> : null}
    </Modal>
  )
}

// WebhooksTable lists the endpoints; the section's boundary shows the load and its failure.
function WebhooksTable({
  workspaceId,
  onTest,
  onRegenerate,
  onToggle,
  onDelete,
}: {
  workspaceId: string
  onTest: (id: string) => void
  onRegenerate: (id: string) => void
  onToggle: (record: SiteWebhookEndpointResource) => void
  onDelete: (record: { id: string; url: string }) => void
}) {
  const { t } = useTranslation()
  const records = useSuspenseQuery(siteWebhooksListOptions({ path: { workspaceId } })).data.items

  return (
    <DataTable
      minHeight={records.length === 0 ? 200 : 0}
      withTableBorder
      records={records}
      idAccessor="id"
      columns={[
        { accessor: 'url', title: t(($) => $.settings.webhooks.urlColumn) },
        {
          accessor: 'eventTypes',
          title: t(($) => $.settings.webhooks.eventsLabel),
          render: (record) =>
            record.eventTypes.length === 0 ? (
              t(($) => $.settings.webhooks.allEvents)
            ) : (
              <EventActionNames actions={record.eventTypes} />
            ),
        },
        {
          accessor: 'enabled',
          title: t(($) => $.settings.webhooks.statusColumn),
          render: (record) => <WebhookStatus record={record} />,
        },
        {
          accessor: 'secret',
          title: t(($) => $.settings.webhooks.secret),
          render: (record) => (
            <CopyButton value={record.secret}>
              {({ copied, copy }) => (
                <Button size="compact-xs" variant="light" onClick={copy}>
                  {copied ? t(($) => $.settings.copied) : t(($) => $.settings.webhooks.copySecret)}
                </Button>
              )}
            </CopyButton>
          ),
        },
        {
          accessor: 'actions',
          title: '',
          render: (record) => (
            <Group gap="xs" wrap="nowrap" justify="flex-end">
              <ChangeHistoryLink
                workspaceId={workspaceId}
                targetType={AUDIT_TARGET_WEBHOOK_ENDPOINT}
                targetId={record.id}
              />
              <RowActionsMenu>
                <Menu.Item onClick={() => onTest(record.id)}>
                  {t(($) => $.settings.webhooks.sendTest)}
                </Menu.Item>
                <Menu.Item onClick={() => onRegenerate(record.id)}>
                  {t(($) => $.settings.webhooks.regenerate)}
                </Menu.Item>
                <Menu.Item onClick={() => onToggle(record)}>
                  {record.enabled
                    ? t(($) => $.settings.webhooks.disable)
                    : t(($) => $.settings.webhooks.enable)}
                </Menu.Item>
                <Menu.Item color="red" onClick={() => onDelete(record)}>
                  {t(($) => $.settings.webhooks.delete)}
                </Menu.Item>
              </RowActionsMenu>
            </Group>
          ),
        },
      ]}
      emptyState={<EmptyState title={t(($) => $.settings.webhooks.empty)} />}
    />
  )
}

export function WebhooksSection({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  const labelOf = useEventActionLabel()
  const confirmDelete = useConfirmation()
  const [createOpened, { open: openCreate, close: closeCreate }] = useDisclosure(false)
  const [testingId, setTestingId] = useState<string | null>(null)

  const form = useForm<Required<Pick<CreateWebhookInput, 'url' | 'eventTypes'>>>({
    initialValues: { url: '', eventTypes: [] },
  })

  const createMutation = useResourceMutation({
    mutation: siteWebhooksCreateMutation(),
    errorTitle: t(($) => $.settings.webhooks.createError),
    onDone: () => {
      form.reset()
      closeCreate()
    },
  })

  const updateMutation = useResourceMutation({
    mutation: siteWebhooksUpdateMutation(),
    errorTitle: t(($) => $.settings.webhooks.saveError),
  })

  const deleteMutation = useResourceMutation({
    mutation: siteWebhooksDeleteMutation(),
    errorTitle: t(($) => $.settings.webhooks.deleteError),
  })

  const regenerateMutation = useResourceMutation({
    mutation: siteWebhooksRegenerateSecretMutation(),
    successMessage: t(($) => $.settings.webhooks.regenerated),
    errorTitle: t(($) => $.settings.webhooks.regenerateError),
  })

  const onRegenerate = (id: string) => {
    modals.openConfirmModal({
      title: t(($) => $.settings.webhooks.regenerateConfirmTitle),
      children: <Text>{t(($) => $.settings.webhooks.regenerateConfirmBody)}</Text>,
      labels: {
        cancel: t(($) => $.actions.cancel),
        confirm: t(($) => $.settings.webhooks.regenerate),
      },
      confirmProps: { color: 'red' },
      onConfirm: () => regenerateMutation.mutate({ path: { workspaceId, id } }),
    })
  }

  const onToggle = (record: SiteWebhookEndpointResource) =>
    updateMutation.mutate({
      path: { workspaceId, id: record.id },
      body: { url: record.url, eventTypes: record.eventTypes, enabled: !record.enabled },
    })

  const onDelete = ({ id, url }: { id: string; url: string }) =>
    confirmDelete({
      title: t(($) => $.settings.webhooks.deleteConfirmTitle),
      description: t(($) => $.settings.webhooks.deleteConfirmDescription),
      typedName: url,
      onConfirm: () => deleteMutation.mutate({ path: { workspaceId, id } }),
    })

  return (
    <Box>
      <Group justify="space-between" mb="sm">
        <Title order={4}>{t(($) => $.settings.webhooksTitle)}</Title>
        <Button size="xs" onClick={openCreate}>
          {t(($) => $.settings.webhooks.create)}
        </Button>
      </Group>

      <QueryBoundary
        fallback={<TableSkeleton />}
        errorTitle={t(($) => $.settings.webhooks.loadError)}
      >
        <WebhooksTable
          workspaceId={workspaceId}
          onTest={setTestingId}
          onRegenerate={onRegenerate}
          onToggle={onToggle}
          onDelete={onDelete}
        />
      </QueryBoundary>

      <Modal
        opened={createOpened}
        onClose={closeCreate}
        title={t(($) => $.settings.webhooks.create)}
      >
        <form
          onSubmit={form.onSubmit((values) =>
            createMutation.mutate({
              path: { workspaceId },
              body: { url: values.url.trim(), eventTypes: values.eventTypes },
            }),
          )}
        >
          <Stack>
            <TextInput
              label={t(($) => $.settings.webhooks.urlLabel)}
              placeholder="https://example.com/hook"
              required
              data-autofocus
              {...form.getInputProps('url')}
            />
            <MultiSelect
              label={t(($) => $.settings.webhooks.eventsLabel)}
              description={t(($) => $.settings.webhooks.eventsHint)}
              data={EVENT_OPTIONS.map((name) => ({ value: name, label: labelOf(name) }))}
              renderOption={({ option }) => <EventActionOption action={option.value} />}
              {...form.getInputProps('eventTypes')}
            />
            <Button type="submit" loading={createMutation.isPending}>
              {t(($) => $.settings.webhooks.create)}
            </Button>
          </Stack>
        </form>
      </Modal>
      <TestEventModal workspaceId={workspaceId} id={testingId} onClose={() => setTestingId(null)} />
    </Box>
  )
}
