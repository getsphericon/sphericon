import {
  Box,
  EmptyState,
  Alert,
  Button,
  Checkbox,
  Collapse,
  Group,
  Loader,
  Menu,
  Modal,
  NumberInput,
  Paper,
  Stack,
  Switch,
  Text,
  TextInput,
  ThemeIcon,
  Title,
  UnstyledButton,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { useDisclosure } from '@mantine/hooks'
import { IconBrandAws, IconPlugConnected, IconServer } from '@tabler/icons-react'
import { useQuery } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import {
  siteIntegrationsCreateMutation,
  siteIntegrationsArchiveMutation,
  siteIntegrationsListOptions,
  siteIntegrationsListQueryKey,
  siteIntegrationsUnarchiveMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type {
  SiteIntegrationConfigInput,
  SiteIntegrationProvider,
  SiteIntegrationResource,
} from '../../generated/site/types.gen.ts'
import { canManage, useCurrentRole } from '../../hooks/useCurrentRole.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { AUDIT_TARGET_INTEGRATION } from './auditFilter.ts'
import { ChangeHistoryLink } from './AuditLogSection.tsx'
import { InboundModal } from './InboundModal.tsx'
import { NO_LIMITS, SendLimitFields, toLimit, useSendLimitValidators } from './SendLimitFields.tsx'
import type { SendLimitValues } from './SendLimitFields.tsx'
import { SendLimitModal } from './SendLimitModal.tsx'
import { SendLimitStatus } from './SendLimitStatus.tsx'

type ProviderKind = SiteIntegrationProvider

// What the first step offers: SES-compatible services speak the SES API at an endpoint of their own.
type Tile = 'ses' | 'sesCompatible' | 'smtp'

const TILES: Tile[] = ['ses', 'sesCompatible', 'smtp']

// Flat form state; the provider-specific config object is assembled on submit
// from whichever fields the selected provider uses.
interface IntegrationFormValues extends SendLimitValues {
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
  ...NO_LIMITS,
}

// A provider left unnamed is called after where it sends from.
function defaultName(values: IntegrationFormValues): string {
  if (values.provider === 'smtp') {
    return values.host.trim()
  }
  const endpoint = values.endpoint.trim()
  return URL.canParse(endpoint) ? new URL(endpoint).host : `Amazon SES (${values.region.trim()})`
}

function buildConfig(values: IntegrationFormValues): SiteIntegrationConfigInput {
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

export function IntegrationsSection({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  const [createOpened, { open: openCreate, close: closeCreate }] = useDisclosure(false)
  const [advanced, { toggle: toggleAdvanced, close: closeAdvanced }] = useDisclosure(false)
  const [tile, setTile] = useState<Tile | null>(null)
  const role = useCurrentRole(workspaceId)
  const canManageIntegrations = canManage(role)

  const [archived, setArchived] = useState(false)
  const queryKey = siteIntegrationsListQueryKey({ path: { workspaceId } })
  const integrationsQuery = useQuery(
    siteIntegrationsListOptions({ path: { workspaceId }, query: { archived } }),
  )
  const activeQuery = useQuery(
    siteIntegrationsListOptions({ path: { workspaceId }, query: { archived: false } }),
  )

  const validateLimits = useSendLimitValidators()
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
    invalidate: [queryKey],
    successMessage: t(($) => $.settings.integrations.saved),
    errorTitle: t(($) => $.settings.integrations.createError),
    onDone: dismissCreate,
  })

  const archiveMutation = useResourceMutation({
    mutation: siteIntegrationsArchiveMutation(),
    invalidate: [queryKey],
    errorTitle: t(($) => $.settings.integrations.archiveError),
  })

  const restoreMutation = useResourceMutation({
    mutation: siteIntegrationsUnarchiveMutation(),
    invalidate: [queryKey],
    errorTitle: t(($) => $.settings.integrations.restoreError),
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
    smtp: {
      title: 'SMTP',
      hint: t(($) => $.settings.integrations.providers.smtpHint),
      icon: <IconServer size={22} />,
    },
  }
  const isSes = form.values.provider === 'ses'
  const compatible = tile === 'sesCompatible'
  // The first provider of a Workspace is its default; later ones ask.
  const hasActive = (activeQuery.data ?? []).length > 0
  const records = integrationsQuery.data ?? []
  const unlimited = records.filter((record) => record.sendLimit.warnings.includes('unlimited'))
  const quotaUnavailable = records.filter((record) =>
    record.sendLimit.warnings.includes('providerQuotaUnavailable'),
  )

  return (
    <Box>
      <Group justify="space-between" mb="xs">
        <Title order={4}>{t(($) => $.settings.integrations.title)}</Title>
        {canManageIntegrations ? (
          <Button onClick={openCreate}>{t(($) => $.settings.integrations.create)}</Button>
        ) : null}
      </Group>
      <Text c="dimmed" size="sm" mb="md">
        {t(($) => $.settings.integrations.description)}
      </Text>

      <Switch
        mb="md"
        label={t(($) => $.settings.integrations.showArchived)}
        checked={archived}
        onChange={(event) => setArchived(event.currentTarget.checked)}
      />

      {integrationsQuery.isError ? (
        <Alert color="red" title={t(($) => $.settings.integrations.loadError)} mb="md">
          {t(($) => $.settings.integrations.loadError)}
        </Alert>
      ) : null}

      {integrationsQuery.isLoading ? <Loader /> : null}

      {unlimited.map((record) => (
        <Alert
          key={record.id}
          color="yellow"
          variant="light"
          title={t(($) => $.settings.integrations.limits.unlimitedTitle, { name: record.name })}
          mb="sm"
        >
          {t(($) => $.settings.integrations.limits.unlimitedWarning)}
        </Alert>
      ))}

      {quotaUnavailable.map((record) => (
        <Alert
          key={record.id}
          color="yellow"
          variant="light"
          title={t(($) => $.settings.integrations.limits.quotaUnavailableTitle, {
            name: record.name,
          })}
          mb="sm"
        >
          {t(($) => $.settings.integrations.limits.quotaUnavailableWarning)}
        </Alert>
      ))}

      <DataTable
        minHeight={records.length === 0 ? 200 : 0}
        withTableBorder
        records={records}
        idAccessor="id"
        columns={[
          { accessor: 'name' },
          { accessor: 'provider', title: t(($) => $.settings.integrations.providerLabel) },
          {
            accessor: 'isDefault',
            title: t(($) => $.settings.integrations.default),
            render: (record) => (record.isDefault ? t(($) => $.settings.integrations.default) : ''),
          },
          {
            accessor: 'sendLimit',
            title: t(($) => $.settings.integrations.limits.title),
            render: (record) => <SendLimitStatus status={record.sendLimit} />,
          },
          {
            accessor: 'actions',
            title: '',
            render: (record) => (
              <Group gap="xs" wrap="nowrap" justify="flex-end">
                <ChangeHistoryLink
                  workspaceId={workspaceId}
                  targetType={AUDIT_TARGET_INTEGRATION}
                  targetId={record.id}
                />
                {canManageIntegrations ? (
                  <RowActionsMenu>
                    <Menu.Item onClick={() => setEditing(record)}>
                      {t(($) => $.settings.integrations.limits.edit)}
                    </Menu.Item>
                    <Menu.Item onClick={() => setInbound(record)}>
                      {t(($) => $.settings.integrations.inbound.open)}
                    </Menu.Item>
                    {record.archivedAt ? (
                      <Menu.Item
                        onClick={() =>
                          restoreMutation.mutate({ path: { workspaceId, id: record.id } })
                        }
                      >
                        {t(($) => $.actions.unarchive)}
                      </Menu.Item>
                    ) : (
                      <Menu.Item
                        color="red"
                        onClick={() =>
                          archiveMutation.mutate({ path: { workspaceId, id: record.id } })
                        }
                      >
                        {t(($) => $.actions.archive)}
                      </Menu.Item>
                    )}
                  </RowActionsMenu>
                ) : null}
              </Group>
            ),
          },
        ]}
        emptyState={<EmptyState title={t(($) => $.settings.integrations.empty)} />}
      />
      {canManageIntegrations ? (
        <Modal
          opened={createOpened}
          onClose={dismissCreate}
          size="lg"
          title={t(($) => $.settings.integrations.create)}
        >
          {tile !== null ? (
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
                {compatible ? (
                  <TextInput
                    label={t(($) => $.settings.integrations.fields.endpoint)}
                    description={t(($) => $.settings.integrations.fields.endpointHint)}
                    placeholder="https://postbox.cloud.yandex.net"
                    required
                    {...form.getInputProps('endpoint')}
                  />
                ) : null}
                {isSes ? (
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
                ) : (
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
                )}

                <TextInput
                  label={t(($) => $.settings.integrations.fields.from)}
                  required
                  {...form.getInputProps('from')}
                />

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
                      <TextInput
                        label={t(($) => $.settings.integrations.fields.fromName)}
                        {...form.getInputProps('fromName')}
                      />
                    </Group>
                    <SendLimitFields
                      perSecondProps={form.getInputProps('maxPerSecond')}
                      perDayProps={form.getInputProps('maxPerDay')}
                    />
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
          ) : (
            <Stack>
              {TILES.map((kind) => (
                <UnstyledButton
                  key={kind}
                  aria-label={providerLabels[kind].title}
                  onClick={() => {
                    form.setFieldValue('provider', kind === 'smtp' ? 'smtp' : 'ses')
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
          )}
        </Modal>
      ) : null}
      <SendLimitModal
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
