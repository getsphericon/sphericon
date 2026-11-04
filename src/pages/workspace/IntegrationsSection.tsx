import {
  Box,
  EmptyState,
  Alert,
  Button,
  Checkbox,
  Collapse,
  Group,
  Menu,
  Modal,
  NumberInput,
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
  IconPlugConnected,
  IconServer,
} from '@tabler/icons-react'
import { useQuery, useSuspenseQuery } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ArchiveViewControl } from '../../components/ArchiveViewControl.tsx'
import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import { TableSkeleton } from '../../components/Skeletons.tsx'
import {
  siteIntegrationsCreateMutation,
  siteIntegrationsArchiveMutation,
  siteIntegrationsListOptions,
  siteIntegrationsUnarchiveMutation,
  siteIntegrationsUpdateMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type {
  IntegrationConfigInput,
  SiteIntegrationChannel,
  SiteIntegrationProvider,
  SiteIntegrationResource,
  SiteWorkspaceResource,
} from '../../generated/site/types.gen.ts'
import { useAuditLogOffered } from '../../hooks/useAuditLogOffered.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { AUDIT_TARGET_INTEGRATION } from '../../utils/auditFilter.ts'
import { can } from '../../utils/permissions.ts'
import { HistoryMenuItem } from './AuditLogSection.tsx'
import { InboundModal } from './InboundModal.tsx'
import {
  NO_LIMITS,
  requiresProviderLimit,
  toLimit,
  useProviderLimitValidators,
  useProviderLimitWarnings,
} from './providerLimit.ts'
import type { ProviderLimitValues } from './providerLimit.ts'
import { ProviderLimitFields } from './ProviderLimitFields.tsx'
import { ProviderLimitModal } from './ProviderLimitModal.tsx'
import { ProviderLimitStatus } from './ProviderLimitStatus.tsx'

type ProviderKind = SiteIntegrationProvider

// What the first step offers: SES-compatible services speak the SES API at an endpoint of their own.
// contract-copy: the first step's tiles, a UI choice that adds sesCompatible to the provider enum
type Tile = 'ses' | 'sesCompatible' | 'smtp' | 'inbox' | 'telegram'

// contract-copy: the same tiles in display order
const TILES: Tile[] = ['ses', 'sesCompatible', 'smtp', 'inbox', 'telegram']

// The provider a tile connects.
function providerOf(tile: Tile): ProviderKind {
  return tile === 'sesCompatible' ? 'ses' : tile
}

// The Channel a tile's provider sends on: a Telegram bot on telegram, every other on email.
function channelOf(tile: Tile | null): SiteIntegrationChannel {
  return tile === 'telegram' ? 'telegram' : 'email'
}

// contract-copy: the Channels in the order the list groups them
const CHANNELS: SiteIntegrationChannel[] = ['email', 'telegram', 'sms']

// Flat form state; the provider-specific config object is assembled on submit
// from whichever fields the selected provider uses.
interface IntegrationFormValues extends ProviderLimitValues {
  name: string
  provider: ProviderKind
  isDefault: boolean
  host: string
  port: number
  username: string
  password: string
  region: string
  accessKeyId: string
  secretAccessKey: string
  endpoint: string
  from: string
  fromName: string
  token: string
}

const INITIAL_VALUES: IntegrationFormValues = {
  name: '',
  provider: 'smtp',
  isDefault: false,
  host: '',
  port: 587,
  username: '',
  password: '',
  region: '',
  accessKeyId: '',
  secretAccessKey: '',
  endpoint: '',
  from: '',
  fromName: '',
  token: '',
  ...NO_LIMITS,
}

// A provider left unnamed is called after where it sends from.
function defaultName(values: IntegrationFormValues): string {
  if (values.provider === 'telegram') {
    return 'Telegram bot'
  }
  if (values.provider === 'smtp') {
    return values.host.trim()
  }
  const endpoint = values.endpoint.trim()
  return URL.canParse(endpoint) ? new URL(endpoint).host : `Amazon SES (${values.region.trim()})`
}

function buildConfig(values: IntegrationFormValues): IntegrationConfigInput {
  if (values.provider === 'telegram') {
    return { kind: 'telegram', token: values.token.trim() }
  }
  if (values.provider === 'ses') {
    return {
      kind: 'ses',
      region: values.region.trim(),
      accessKeyId: values.accessKeyId.trim(),
      secretAccessKey: values.secretAccessKey,
      endpoint: values.endpoint.trim() || null,
      from: values.from.trim(),
      fromName: values.fromName.trim() || null,
    }
  }
  return {
    kind: 'smtp',
    host: values.host.trim(),
    port: values.port,
    username: values.username.trim() || null,
    password: values.password || null,
    from: values.from.trim(),
    fromName: values.fromName.trim() || null,
  }
}

// The Workspace's providers, the kept ones (read by the list and by the create form) or the
// archived ones.
const integrationsOptions = (workspaceId: string, archived = false) =>
  siteIntegrationsListOptions({ path: { workspaceId }, query: { archived } })

// IntegrationsList shows the kept providers with the warnings about their send limits, or the
// archived ones to restore; the section's boundary shows the load and its failure.
function IntegrationsList({
  workspaceId,
  archived,
  canManage,
  onMakeDefault,
  onEditLimits,
  onInbound,
  onRemove,
  onRestore,
}: {
  workspaceId: string
  archived: boolean
  canManage: boolean
  onMakeDefault: (id: string) => void
  onEditLimits: (record: SiteIntegrationResource) => void
  onInbound: (record: SiteIntegrationResource) => void
  onRemove: (id: string) => void
  onRestore: (id: string) => void
}) {
  const { t } = useTranslation()
  const records = useSuspenseQuery(integrationsOptions(workspaceId, archived)).data
  const warnings = useProviderLimitWarnings()
  const historyOffered = useAuditLogOffered(workspaceId)
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
  // several providers gets at most one block per kind. An archived provider sends nothing, so its
  // limits are nothing to warn about.
  const alerts = archived
    ? []
    : warnings.flatMap((copy) => {
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
                render: (record) =>
                  record.config.kind === 'telegram'
                    ? `@${record.config.username}`
                    : record.provider,
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
                  <ProviderLimitStatus status={record.providerLimit} withWarnings={!archived} />
                ),
              },
              {
                accessor: 'actions',
                title: '',
                render: (record) => (
                  <Group gap="xs" wrap="nowrap" justify="flex-end">
                    {canManage && archived ? (
                      <RowActionsMenu>
                        <Menu.Item onClick={() => onRestore(record.id)}>
                          {t(($) => $.actions.unarchive)}
                        </Menu.Item>
                      </RowActionsMenu>
                    ) : null}
                    {!archived && (canManage || historyOffered) ? (
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
                            <Menu.Item onClick={() => onEditLimits(record)}>
                              {t(($) => $.settings.integrations.providerLimit.edit)}
                            </Menu.Item>
                            <Menu.Item onClick={() => onInbound(record)}>
                              {t(($) => $.settings.integrations.inbound.open)}
                            </Menu.Item>
                            <Menu.Item color="red" onClick={() => onRemove(record.id)}>
                              {t(($) => $.actions.remove)}
                            </Menu.Item>
                          </>
                        ) : null}
                      </RowActionsMenu>
                    ) : null}
                  </Group>
                ),
              },
            ]}
            emptyState={
              <EmptyState
                title={
                  archived
                    ? t(($) => $.settings.integrations.emptyArchived)
                    : t(($) => $.settings.integrations.empty)
                }
              />
            }
          />
        </Stack>
      ))}
    </>
  )
}

export function IntegrationsSection({ workspace }: { workspace: SiteWorkspaceResource }) {
  const workspaceId = workspace.id
  const { t } = useTranslation()
  const [createOpened, { open: openCreate, close: closeCreate }] = useDisclosure(false)
  const [advanced, { toggle: toggleAdvanced, close: closeAdvanced }] = useDisclosure(false)
  const [tile, setTile] = useState<Tile | null>(null)
  const [archived, setArchived] = useState(false)
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

  const removeMutation = useResourceMutation({
    mutation: siteIntegrationsArchiveMutation(),
    errorTitle: t(($) => $.settings.integrations.removeError),
  })

  const restoreMutation = useResourceMutation({
    mutation: siteIntegrationsUnarchiveMutation(),
    successMessage: t(($) => $.settings.integrations.restored),
    errorTitle: t(($) => $.settings.integrations.restoreError),
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
  }
  const isTelegram = form.values.provider === 'telegram'
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
      <Text c="dimmed" size="sm" mb="md">
        {t(($) => $.settings.integrations.description)}
      </Text>

      <ArchiveViewControl archived={archived} onChange={setArchived} mb="sm" />
      <QueryBoundary
        fallback={<TableSkeleton />}
        errorTitle={t(($) => $.settings.integrations.loadError)}
      >
        <IntegrationsList
          workspaceId={workspaceId}
          archived={archived}
          canManage={canManageIntegrations}
          onMakeDefault={(id) =>
            defaultMutation.mutate({ path: { workspaceId, id }, body: { isDefault: true } })
          }
          onEditLimits={setEditing}
          onInbound={setInbound}
          onRemove={(id) => removeMutation.mutate({ path: { workspaceId, id } })}
          onRestore={(id) => restoreMutation.mutate({ path: { workspaceId, id } })}
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
                    <TextInput
                      label={t(($) => $.settings.integrations.fields.token)}
                      description={t(($) => $.settings.integrations.fields.tokenHint)}
                      type="password"
                      required
                      {...form.getInputProps('token')}
                    />
                  </>
                ) : null}
                {compatible ? (
                  <TextInput
                    label={t(($) => $.settings.integrations.fields.endpoint)}
                    description={t(($) => $.settings.integrations.fields.endpointHint)}
                    placeholder="https://postbox.cloud.yandex.net"
                    required
                    {...form.getInputProps('endpoint')}
                  />
                ) : null}
                {!isTelegram && isSes ? (
                  <Group grow>
                    <TextInput
                      label={t(($) => $.settings.integrations.fields.region)}
                      required
                      {...form.getInputProps('region')}
                    />
                    <TextInput
                      label={t(($) => $.settings.integrations.fields.accessKeyId)}
                      required
                      {...form.getInputProps('accessKeyId')}
                    />
                    <TextInput
                      label={t(($) => $.settings.integrations.fields.secretAccessKey)}
                      type="password"
                      required
                      {...form.getInputProps('secretAccessKey')}
                    />
                  </Group>
                ) : null}
                {!isTelegram && !isSes ? (
                  <>
                    <Group grow>
                      <TextInput
                        label={t(($) => $.settings.integrations.fields.host)}
                        required
                        {...form.getInputProps('host')}
                      />
                      <NumberInput
                        label={t(($) => $.settings.integrations.fields.port)}
                        required
                        {...form.getInputProps('port')}
                      />
                    </Group>
                    <Group grow>
                      <TextInput
                        label={t(($) => $.settings.integrations.fields.username)}
                        {...form.getInputProps('username')}
                      />
                      <TextInput
                        label={t(($) => $.settings.integrations.fields.password)}
                        type="password"
                        {...form.getInputProps('password')}
                      />
                    </Group>
                  </>
                ) : null}

                {isTelegram ? null : (
                  <TextInput
                    label={t(($) => $.settings.integrations.fields.from)}
                    required
                    {...form.getInputProps('from')}
                  />
                )}

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
                      {isTelegram ? null : (
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
      <InboundModal
        workspaceId={workspaceId}
        integration={inbound}
        onClose={() => setInbound(null)}
      />
    </Box>
  )
}
