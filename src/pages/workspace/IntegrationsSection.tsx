import {
  Box,
  EmptyState,
  Alert,
  Button,
  Checkbox,
  Code,
  Collapse,
  CopyButton,
  Group,
  Menu,
  Modal,
  Paper,
  Stack,
  Text,
  TextInput,
  ThemeIcon,
  Title,
  UnstyledButton,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { useDisclosure } from '@mantine/hooks'
import {
  IconBrandAws,
  IconBrandTelegram,
  IconMailOpened,
  IconMessage,
  IconPlugConnected,
  IconServer,
} from '@tabler/icons-react'
import { useQuery, useSuspenseQuery } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import { TableSkeleton } from '../../components/Skeletons.tsx'
import {
  siteIntegrationsCreateMutation,
  siteIntegrationsDeleteMutation,
  siteIntegrationsListOptions,
  siteIntegrationsUpdateMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type {
  SiteIntegrationChannel,
  SiteIntegrationProvider,
  SiteIntegrationResource,
  SiteWorkspaceResource,
} from '../../generated/site/types.gen.ts'
import { useAuditLogOffered } from '../../hooks/useAuditLogOffered.ts'
import { useDeleteResource } from '../../hooks/useDeleteResource.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { AUDIT_TARGET_INTEGRATION } from '../../utils/auditFilter.ts'
import { can } from '../../utils/permissions.ts'
import { HistoryMenuItem } from './AuditLogSection.tsx'
import { CredentialsModal } from './CredentialsModal.tsx'
import { InboundModal } from './InboundModal.tsx'
import { EmailFields, TelegramFields, TwilioFields } from './integrationFields.tsx'
import { buildConfig, hasCredentials, INITIAL_VALUES, paramsOf } from './integrationForm.ts'
import type { IntegrationFormValues } from './integrationForm.ts'
import {
  requiresProviderLimit,
  toLimit,
  useProviderLimitValidators,
  useProviderLimitWarnings,
} from './providerLimit.ts'
import { ProviderLimitFields } from './ProviderLimitFields.tsx'
import { ProviderLimitModal } from './ProviderLimitModal.tsx'
import { ProviderLimitStatus } from './ProviderLimitStatus.tsx'

type ProviderKind = SiteIntegrationProvider

// What the first step offers: SES-compatible services speak the SES API at an endpoint of their own.
type Tile = SiteIntegrationProvider | 'sesCompatible'

// contract-copy: the same tiles in display order
const TILES: Tile[] = ['ses', 'sesCompatible', 'smtp', 'inbox', 'telegram', 'twilio']

// The provider a tile connects.
function providerOf(tile: Tile): ProviderKind {
  return tile === 'sesCompatible' ? 'ses' : tile
}

// The Channel each provider sends on (an Integration serves one Channel, ADR 0074). The API reads
// it back on each Integration as `channel`; a form for a provider not connected yet has none to read.
const CHANNEL_OF: Record<ProviderKind, SiteIntegrationChannel> = {
  ses: 'email',
  smtp: 'email',
  inbox: 'email',
  telegram: 'telegram',
  twilio: 'sms',
}

// The Channel a tile's provider sends on; email before a tile is picked.
function channelOf(tile: Tile | null): SiteIntegrationChannel {
  return tile === null ? 'email' : CHANNEL_OF[providerOf(tile)]
}

// contract-copy: the Channels in the order the list groups them
const CHANNELS: SiteIntegrationChannel[] = ['email', 'telegram', 'sms']

// A provider left unnamed is called after where it sends from.
function defaultName(values: IntegrationFormValues): string {
  if (values.provider === 'telegram') {
    return 'Telegram bot'
  }
  if (values.provider === 'twilio') {
    return `Twilio (${values.from.trim()})`
  }
  if (values.provider === 'smtp') {
    return values.host.trim()
  }
  const endpoint = values.endpoint.trim()
  return URL.canParse(endpoint) ? new URL(endpoint).host : `Amazon SES (${values.region.trim()})`
}

// The Workspace's providers, read by the list and by the create form. A deleted one is not listed.
const integrationsOptions = (workspaceId: string) =>
  siteIntegrationsListOptions({ path: { workspaceId } })

// IntegrationsList shows the providers with the warnings about their send limits; the section's
// boundary shows the load and its failure.
function IntegrationsList({
  workspaceId,
  canManage,
  onMakeDefault,
  onEditLimits,
  onCredentials,
  onInbound,
  onDelete,
}: {
  workspaceId: string
  canManage: boolean
  onMakeDefault: (id: string) => void
  onEditLimits: (record: SiteIntegrationResource) => void
  onCredentials: (record: SiteIntegrationResource) => void
  onInbound: (record: SiteIntegrationResource) => void
  onDelete: (record: SiteIntegrationResource) => void
}) {
  const { t } = useTranslation()
  const records = useSuspenseQuery(integrationsOptions(workspaceId)).data
  const warnings = useProviderLimitWarnings()
  const historyOffered = useAuditLogOffered(workspaceId, records.length > 0)
  const channelLabels: Record<SiteIntegrationChannel, string> = {
    email: t(($) => $.settings.integrations.channels.email),
    sms: t(($) => $.settings.integrations.channels.sms),
    telegram: t(($) => $.settings.integrations.channels.telegram),
  }
  // One list per Channel that has a provider, each with its own default; none yet is one empty list.
  const present = CHANNELS.filter((channel) => records.some((r) => r.channel === channel))
  const groups = (present.length === 0 ? (['email'] as const) : present).map((channel) => ({
    channel,
    rows: records.filter((r) => r.channel === channel),
  }))
  // One alert per kind of warning, naming every provider it applies to, so a Workspace with
  // several providers gets at most one block per kind.
  const alerts = warnings.flatMap((copy) => {
    const names = records
      .filter((record) => record.providerLimit.warnings.includes(copy.warning))
      .map((record) => record.name)
    return names.length === 0 ? [] : [{ ...copy, names }]
  })

  return (
    <>
      {alerts.map(({ warning, color, title, body, names }) => (
        <Alert key={warning} color={color} variant="light" title={title(names)} mb="sm">
          {body(names.length)}
        </Alert>
      ))}

      {records.map((record) =>
        record.config.kind === 'twilio' && !record.config.callbacksSet ? (
          <Alert
            key={`callbacks-${record.id}`}
            color="yellow"
            variant="light"
            title={t(($) => $.settings.integrations.twilioCallbacks.title, {
              name: record.name,
            })}
            mb="sm"
          >
            <Stack gap="xs">
              <Text size="sm">{t(($) => $.settings.integrations.twilioCallbacks.body)}</Text>
              <Code block>{record.hookUrl}</Code>
              <Group>
                <CopyButton value={record.hookUrl}>
                  {({ copied, copy }) => (
                    <Button size="compact-sm" variant="light" onClick={copy}>
                      {copied ? t(($) => $.settings.copied) : t(($) => $.settings.copy)}
                    </Button>
                  )}
                </CopyButton>
              </Group>
            </Stack>
          </Alert>
        ) : null,
      )}

      {groups.map(({ channel, rows }) => (
        <Stack key={channel} gap="xs" mb="md">
          {groups.length > 1 || channel !== 'email' ? (
            <Title order={5}>{channelLabels[channel]}</Title>
          ) : null}
          <DataTable
            minHeight={rows.length === 0 ? 200 : 0}
            withTableBorder
            records={rows}
            idAccessor="id"
            columns={[
              { accessor: 'name' },
              {
                accessor: 'provider',
                title: t(($) => $.settings.integrations.providerLabel),
                render: (record) => {
                  if (record.config.kind === 'telegram') {
                    return `@${record.config.username}`
                  }
                  return record.config.kind === 'twilio' ? record.config.from : record.provider
                },
              },
              {
                accessor: 'isDefault',
                title: t(($) => $.settings.integrations.default),
                render: (record) =>
                  record.isDefault ? t(($) => $.settings.integrations.default) : '',
              },
              {
                accessor: 'providerLimit',
                title: t(($) => $.settings.integrations.providerLimit.title),
                render: (record) => (
                  <ProviderLimitStatus status={record.providerLimit} withWarnings />
                ),
              },
              {
                accessor: 'actions',
                title: '',
                render: (record) => (
                  <Group gap="xs" wrap="nowrap" justify="flex-end">
                    {canManage || historyOffered ? (
                      <RowActionsMenu>
                        {historyOffered ? (
                          <HistoryMenuItem
                            workspaceId={workspaceId}
                            targetType={AUDIT_TARGET_INTEGRATION}
                            targetId={record.id}
                          />
                        ) : null}
                        {canManage && !record.isDefault ? (
                          <Menu.Item onClick={() => onMakeDefault(record.id)}>
                            {t(($) => $.settings.integrations.makeDefault)}
                          </Menu.Item>
                        ) : null}
                        {canManage ? (
                          <>
                            {hasCredentials(record) ? (
                              <Menu.Item onClick={() => onCredentials(record)}>
                                {t(($) => $.settings.integrations.credentials.open)}
                              </Menu.Item>
                            ) : null}
                            <Menu.Item onClick={() => onEditLimits(record)}>
                              {t(($) => $.settings.integrations.providerLimit.edit)}
                            </Menu.Item>
                            <Menu.Item onClick={() => onInbound(record)}>
                              {t(($) => $.settings.integrations.inbound.open)}
                            </Menu.Item>
                            <Menu.Item color="red" onClick={() => onDelete(record)}>
                              {t(($) => $.actions.delete)}
                            </Menu.Item>
                          </>
                        ) : null}
                      </RowActionsMenu>
                    ) : null}
                  </Group>
                ),
              },
            ]}
            emptyState={<EmptyState title={t(($) => $.settings.integrations.empty)} />}
          />
        </Stack>
      ))}
    </>
  )
}

// linkedIntegration is the Integration a link opened the credentials of, once the list has it: one
// with credentials, for a member who may save them.
function linkedIntegration(
  records: SiteIntegrationResource[] | undefined,
  id: string | undefined,
  canManage: boolean,
): SiteIntegrationResource | null {
  if (!canManage || id === undefined) {
    return null
  }
  return records?.find((r) => r.id === id && hasCredentials(r)) ?? null
}

interface IntegrationsSectionProps {
  workspace: SiteWorkspaceResource
  // The id of an Integration whose credentials open on arrival (a link that names it), and what
  // closing them does to that link.
  linkedId?: string | undefined
  onLinkedClosed?: () => void
}

export function IntegrationsSection({
  workspace,
  linkedId,
  onLinkedClosed,
}: IntegrationsSectionProps) {
  const workspaceId = workspace.id
  const { t } = useTranslation()
  const [createOpened, { open: openCreate, close: closeCreate }] = useDisclosure(false)
  const [advanced, { toggle: toggleAdvanced, close: closeAdvanced }] = useDisclosure(false)
  const [tile, setTile] = useState<Tile | null>(null)
  const canManageIntegrations = can(workspace, 'integrations:manage')

  // The list the boundary below loads, read here only to know whether a provider exists yet.
  const integrationsQuery = useQuery(integrationsOptions(workspaceId))

  const validateLimits = useProviderLimitValidators((values: IntegrationFormValues) =>
    requiresProviderLimit(values.provider, values.endpoint),
  )
  const form = useForm<IntegrationFormValues>({
    initialValues: INITIAL_VALUES,
    validate: validateLimits,
  })
  const [editing, setEditing] = useState<SiteIntegrationResource | null>(null)
  const [inbound, setInbound] = useState<SiteIntegrationResource | null>(null)
  // The Integration whose credentials are open, from its row's menu.
  const [credentialsOf, setCredentialsOf] = useState<SiteIntegrationResource | null>(null)
  const linked = linkedIntegration(integrationsQuery.data, linkedId, canManageIntegrations)

  function closeCredentials() {
    setCredentialsOf(null)
    if (linkedId !== undefined) {
      onLinkedClosed?.()
    }
  }

  function dismissCreate() {
    form.reset()
    setTile(null)
    closeAdvanced()
    closeCreate()
  }

  const createMutation = useResourceMutation({
    mutation: siteIntegrationsCreateMutation(),
    successMessage: t(($) => $.settings.integrations.saved),
    errorTitle: t(($) => $.settings.integrations.createError),
    onDone: dismissCreate,
  })

  // A deleted provider is never restored: the person types its name to confirm.
  const deleteIntegration = useDeleteResource({
    mutation: siteIntegrationsDeleteMutation(),
    workspaceId,
    successMessage: t(($) => $.settings.integrations.deleted),
    errorTitle: t(($) => $.settings.integrations.deleteError),
    consequence: t(($) => $.settings.integrations.deleteConsequence),
  })

  // Promoting a provider clears the channel's current default on the server.
  const defaultMutation = useResourceMutation({
    mutation: siteIntegrationsUpdateMutation(),
    successMessage: t(($) => $.settings.integrations.defaultChanged),
    errorTitle: t(($) => $.settings.integrations.defaultError),
  })

  const providerLabels = {
    ses: {
      title: 'Amazon SES',
      hint: t(($) => $.settings.integrations.providers.sesHint),
      icon: <IconBrandAws size={22} />,
    },
    sesCompatible: {
      title: t(($) => $.settings.integrations.providers.sesCompatible),
      hint: t(($) => $.settings.integrations.providers.sesCompatibleHint),
      icon: <IconPlugConnected size={22} />,
    },
    inbox: {
      title: t(($) => $.settings.integrations.providers.inbox),
      hint: t(($) => $.settings.integrations.providers.inboxHint),
      icon: <IconMailOpened size={22} />,
    },
    smtp: {
      title: 'SMTP',
      hint: t(($) => $.settings.integrations.providers.smtpHint),
      icon: <IconServer size={22} />,
    },
    telegram: {
      title: t(($) => $.settings.integrations.providers.telegram),
      hint: t(($) => $.settings.integrations.providers.telegramHint),
      icon: <IconBrandTelegram size={22} />,
    },
    twilio: {
      title: t(($) => $.settings.integrations.providers.twilio),
      hint: t(($) => $.settings.integrations.providers.twilioHint),
      icon: <IconMessage size={22} />,
    },
  }
  const isTelegram = form.values.provider === 'telegram'
  const isTwilio = form.values.provider === 'twilio'
  const isSes = form.values.provider === 'ses'
  const compatible = tile === 'sesCompatible'
  // An SES-compatible service needs its Provider limit (ADR 0067), so there the fields are required
  // and shown beside the credentials; for any other provider they are optional, in Advanced settings.
  const limitFields = (
    <ProviderLimitFields
      perSecondProps={form.getInputProps('maxPerSecond')}
      perDayProps={form.getInputProps('maxPerDay')}
      required={compatible}
    />
  )
  // The first provider of a Channel is its default; later ones ask.
  const hasActive = (integrationsQuery.data ?? []).some((r) => r.channel === channelOf(tile))

  return (
    <Box>
      <Group justify="space-between" mb="sm">
        <Title order={4}>{t(($) => $.settings.integrations.title)}</Title>
        {canManageIntegrations ? (
          <Button size="xs" onClick={openCreate}>
            {t(($) => $.settings.integrations.create)}
          </Button>
        ) : null}
      </Group>
      <Text c="dimmed" size="sm" mb="xs">
        {t(($) => $.settings.integrations.description)}
      </Text>
      <Text c="dimmed" size="sm" mb="md">
        {t(($) => $.settings.integrations.carrierRegistration)}
      </Text>

      <QueryBoundary
        fallback={<TableSkeleton />}
        errorTitle={t(($) => $.settings.integrations.loadError)}
      >
        <IntegrationsList
          workspaceId={workspaceId}
          canManage={canManageIntegrations}
          onMakeDefault={(id) =>
            defaultMutation.mutate({ path: { workspaceId, id }, body: { isDefault: true } })
          }
          onEditLimits={setEditing}
          onCredentials={setCredentialsOf}
          onInbound={setInbound}
          onDelete={(record) => deleteIntegration(record.id, record.name)}
        />
      </QueryBoundary>
      {canManageIntegrations ? (
        <Modal
          opened={createOpened}
          onClose={dismissCreate}
          size="lg"
          title={t(($) => $.settings.integrations.create)}
        >
          {tile === 'inbox' ? (
            <Stack>
              <Text size="sm">{t(($) => $.settings.integrations.inboxExplanation)}</Text>
              <Group justify="space-between">
                <Button variant="default" onClick={() => setTile(null)}>
                  {t(($) => $.settings.integrations.back)}
                </Button>
                <Button
                  loading={createMutation.isPending}
                  onClick={() =>
                    createMutation.mutate({
                      path: { workspaceId },
                      body: {
                        name: t(($) => $.settings.integrations.providers.inbox),
                        isDefault: false,
                        config: { kind: 'inbox' },
                      },
                    })
                  }
                >
                  {t(($) => $.settings.integrations.create)}
                </Button>
              </Group>
            </Stack>
          ) : null}
          {tile !== null && tile !== 'inbox' ? (
            <form
              onSubmit={form.onSubmit((values) =>
                createMutation.mutate({
                  path: { workspaceId },
                  body: {
                    name: values.name.trim() || defaultName(values),
                    isDefault: hasActive ? values.isDefault : true,
                    maxPerSecond: toLimit(values.maxPerSecond),
                    maxPerDay: toLimit(values.maxPerDay),
                    config: buildConfig(values),
                    ...paramsOf(values),
                  },
                }),
              )}
            >
              <Stack>
                {isTelegram ? (
                  <>
                    <Alert
                      color="yellow"
                      variant="light"
                      title={t(($) => $.settings.integrations.telegramWarning.title)}
                    >
                      {t(($) => $.settings.integrations.telegramWarning.body)}
                    </Alert>
                    <TelegramFields form={form} />
                  </>
                ) : null}
                {isTwilio ? <TwilioFields form={form} /> : null}
                {compatible ? (
                  <TextInput
                    label={t(($) => $.settings.integrations.fields.endpoint)}
                    description={t(($) => $.settings.integrations.fields.endpointHint)}
                    placeholder="https://postbox.cloud.yandex.net"
                    required
                    {...form.getInputProps('endpoint')}
                  />
                ) : null}
                {isTelegram || isTwilio ? null : <EmailFields form={form} isSes={isSes} />}

                {compatible ? (
                  <>
                    <Text c="dimmed" size="sm">
                      {t(($) => $.settings.integrations.providerLimit.descriptionRequired)}
                    </Text>
                    {limitFields}
                  </>
                ) : null}

                <Group>
                  <Button variant="subtle" onClick={toggleAdvanced}>
                    {t(($) => $.settings.integrations.advanced)}
                  </Button>
                </Group>
                <Collapse expanded={advanced}>
                  <Stack>
                    <Group grow align="flex-start">
                      <TextInput
                        label={t(($) => $.settings.integrations.nameLabel)}
                        placeholder={defaultName(form.values)}
                        {...form.getInputProps('name')}
                      />
                      {isTelegram || isTwilio ? null : (
                        <TextInput
                          label={t(($) => $.settings.integrations.fields.fromName)}
                          {...form.getInputProps('fromName')}
                        />
                      )}
                    </Group>
                    {compatible ? null : limitFields}
                    {hasActive ? (
                      <Checkbox
                        label={t(($) => $.settings.integrations.makeDefault)}
                        {...form.getInputProps('isDefault', { type: 'checkbox' })}
                      />
                    ) : null}
                  </Stack>
                </Collapse>

                <Group justify="space-between">
                  <Button variant="default" onClick={() => setTile(null)}>
                    {t(($) => $.settings.integrations.back)}
                  </Button>
                  <Button type="submit" loading={createMutation.isPending}>
                    {t(($) => $.settings.integrations.create)}
                  </Button>
                </Group>
              </Stack>
            </form>
          ) : null}
          {tile === null ? (
            <Stack>
              {TILES.map((kind) => (
                <UnstyledButton
                  key={kind}
                  aria-label={providerLabels[kind].title}
                  onClick={() => {
                    form.setFieldValue('provider', providerOf(kind))
                    form.setFieldValue('endpoint', '')
                    setTile(kind)
                  }}
                >
                  <Paper withBorder p="md">
                    <Group wrap="nowrap">
                      <ThemeIcon variant="light" size="xl" radius="md">
                        {providerLabels[kind].icon}
                      </ThemeIcon>
                      <Stack gap={2}>
                        <Text fw={600}>{providerLabels[kind].title}</Text>
                        <Text size="sm" c="dimmed">
                          {providerLabels[kind].hint}
                        </Text>
                      </Stack>
                    </Group>
                  </Paper>
                </UnstyledButton>
              ))}
            </Stack>
          ) : null}
        </Modal>
      ) : null}
      <ProviderLimitModal
        workspaceId={workspaceId}
        integration={editing}
        onClose={() => setEditing(null)}
      />
      <CredentialsModal
        workspaceId={workspaceId}
        integration={credentialsOf ?? linked}
        onClose={closeCredentials}
      />
      <InboundModal
        workspaceId={workspaceId}
        integration={inbound}
        onClose={() => setInbound(null)}
      />
    </Box>
  )
}
