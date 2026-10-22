import {
  Box,
  EmptyState,
  Alert,
  Badge,
  Button,
  CopyButton,
  Group,
  Loader,
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
import { useQuery } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import {
  siteWebhooksCreateMutation,
  siteWebhooksDeleteMutation,
  siteWebhooksListOptions,
  siteWebhooksListQueryKey,
  siteWebhooksRegenerateSecretMutation,
  siteWebhooksTestMutation,
  siteWebhooksUpdateMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type {
  SiteWebhookEndpointResource,
  SiteWebhookFailure,
  SiteWebhookTestResult,
} from '../../generated/site/types.gen.ts'
import { useDeleteConfirmation } from '../../hooks/useDeleteConfirmation.tsx'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { AUDIT_TARGET_WEBHOOK_ENDPOINT } from './auditFilter.ts'
import { ChangeHistoryLink } from './AuditLogSection.tsx'

// The domain events an endpoint can subscribe to. An empty selection means all customer
// events; audit.entry (Enterprise) is never included and must be chosen explicitly.
const EVENT_OPTIONS = [
  'contact.created',
  'contact.erased',
  'email.opened',
  'email.clicked',
  'email.unsubscribed',
  'audit.entry',
]

function FailureText({ failure }: { failure: SiteWebhookFailure }) {
  const { t } = useTranslation()
  const reason = {
    timeout: t(($) => $.settings.webhooks.errorTimeout),
    blocked_address: t(($) => $.settings.webhooks.errorBlockedAddress),
    http_status: t(($) => $.settings.webhooks.errorHttpStatus, { status: failure.status ?? 0 }),
    network: t(($) => $.settings.webhooks.errorNetwork),
  }[failure.kind]
  return (
    <Text size="xs" c="dimmed">
      {t(($) => $.settings.webhooks.failureSummary, {
        reason,
        count: failure.consecutiveFailures,
        since: new Date(failure.since).toLocaleString(),
      })}
    </Text>
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
          data={TEST_EVENT_OPTIONS}
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

export function WebhooksSection({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  const confirmDelete = useDeleteConfirmation()
  const [createOpened, { open: openCreate, close: closeCreate }] = useDisclosure(false)
  const [testingId, setTestingId] = useState<string | null>(null)

  const queryKey = siteWebhooksListQueryKey({ path: { workspaceId } })
  const webhooksQuery = useQuery(siteWebhooksListOptions({ path: { workspaceId } }))

  const form = useForm<{ url: string; eventTypes: string[] }>({
    initialValues: { url: '', eventTypes: [] },
  })

  const createMutation = useResourceMutation({
    mutation: siteWebhooksCreateMutation(),
    invalidate: [queryKey],
    errorTitle: t(($) => $.settings.webhooks.createError),
    onDone: () => {
      form.reset()
      closeCreate()
    },
  })

  const updateMutation = useResourceMutation({
    mutation: siteWebhooksUpdateMutation(),
    invalidate: [queryKey],
    errorTitle: t(($) => $.settings.webhooks.saveError),
  })

  const deleteMutation = useResourceMutation({
    mutation: siteWebhooksDeleteMutation(),
    invalidate: [queryKey],
    errorTitle: t(($) => $.settings.webhooks.deleteError),
  })

  const regenerateMutation = useResourceMutation({
    mutation: siteWebhooksRegenerateSecretMutation(),
    invalidate: [queryKey],
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

  const onDelete = (id: string) =>
    confirmDelete({
      title: t(($) => $.settings.webhooks.deleteConfirmTitle),
      onConfirm: () => deleteMutation.mutate({ path: { workspaceId, id } }),
    })

  const records = webhooksQuery.data?.items ?? []

  return (
    <Box>
      <Group justify="space-between" mb="sm">
        <Title order={4}>{t(($) => $.settings.webhooksTitle)}</Title>
        <Button onClick={openCreate}>{t(($) => $.settings.webhooks.create)}</Button>
      </Group>

      {webhooksQuery.isError ? (
        <Alert color="red" title={t(($) => $.settings.webhooks.loadError)} mb="md">
          {t(($) => $.settings.webhooks.loadError)}
        </Alert>
      ) : null}

      {webhooksQuery.isLoading ? <Loader /> : null}

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
              record.eventTypes.length === 0
                ? t(($) => $.settings.webhooks.allEvents)
                : record.eventTypes.join(', '),
          },
          {
            accessor: 'enabled',
            title: t(($) => $.settings.webhooks.statusColumn),
            render: (record) => (
              <Stack gap={2}>
                <Badge variant="light" color={record.enabled ? 'teal' : 'gray'}>
                  {record.enabled
                    ? t(($) => $.settings.webhooks.enabled)
                    : t(($) => $.settings.webhooks.disabled)}
                </Badge>
                {record.disabledReason ? (
                  <Text size="xs" c="dimmed">
                    {
                      {
                        gone: t(($) => $.settings.webhooks.disabledGone),
                        persistent_failure: t(($) => $.settings.webhooks.disabledPersistentFailure),
                      }[record.disabledReason]
                    }
                  </Text>
                ) : null}
                {record.failure ? (
                  <>
                    <Badge variant="light" color="red">
                      {t(($) => $.settings.webhooks.failing)}
                    </Badge>
                    <FailureText failure={record.failure} />
                  </>
                ) : null}
              </Stack>
            ),
          },
          {
            accessor: 'secret',
            title: t(($) => $.settings.webhooks.secret),
            render: (record) => (
              <CopyButton value={record.secret}>
                {({ copied, copy }) => (
                  <Button size="compact-sm" variant="light" onClick={copy}>
                    {copied
                      ? t(($) => $.settings.copied)
                      : t(($) => $.settings.webhooks.copySecret)}
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
                  <Menu.Item onClick={() => setTestingId(record.id)}>
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
                  <Menu.Item color="red" onClick={() => onDelete(record.id)}>
                    {t(($) => $.settings.webhooks.delete)}
                  </Menu.Item>
                </RowActionsMenu>
              </Group>
            ),
          },
        ]}
        emptyState={<EmptyState title={t(($) => $.settings.webhooks.empty)} />}
      />

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
              data={EVENT_OPTIONS}
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
