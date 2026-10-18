import {
  Alert,
  Button,
  Card,
  Checkbox,
  Group,
  Loader,
  NumberInput,
  Select,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { useQuery } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  siteIntegrationsCreateMutation,
  siteIntegrationsDeleteMutation,
  siteIntegrationsListOptions,
  siteIntegrationsListQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type {
  SiteIntegrationConfigInput,
  SiteIntegrationResource,
} from '../../generated/site/types.gen.ts'
import { canManage, useCurrentRole } from '../../hooks/useCurrentRole.ts'
import { useDeleteConfirmation } from '../../hooks/useDeleteConfirmation.tsx'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { AUDIT_TARGET_INTEGRATION } from './auditFilter.ts'
import { ChangeHistoryLink } from './AuditLogSection.tsx'
import { InboundModal } from './InboundModal.tsx'
import { NO_LIMITS, SendLimitFields, toLimit, useSendLimitValidators } from './SendLimitFields.tsx'
import type { SendLimitValues } from './SendLimitFields.tsx'
import { SendLimitModal } from './SendLimitModal.tsx'
import { SendLimitStatus } from './SendLimitStatus.tsx'

type ProviderKind = 'smtp' | 'ses'

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
  snsTopicArn: string
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
  snsTopicArn: '',
  from: '',
  fromName: '',
  ...NO_LIMITS,
}

function buildConfig(values: IntegrationFormValues): SiteIntegrationConfigInput {
  if (values.provider === 'ses') {
    return {
      kind: 'ses',
      region: values.region.trim(),
      accessKeyId: values.accessKeyId.trim(),
      secretAccessKey: values.secretAccessKey,
      endpoint: values.endpoint.trim() || null,
      snsTopicArn: values.snsTopicArn.trim() || null,
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

export function IntegrationsSection({ slug }: { slug: string }) {
  const { t } = useTranslation()
  const confirmDelete = useDeleteConfirmation()
  const role = useCurrentRole(slug)
  const canManageIntegrations = canManage(role)

  const queryKey = siteIntegrationsListQueryKey({ path: { slug } })
  const integrationsQuery = useQuery(siteIntegrationsListOptions({ path: { slug } }))

  const validateLimits = useSendLimitValidators()
  const form = useForm<IntegrationFormValues>({
    initialValues: INITIAL_VALUES,
    validate: validateLimits,
  })
  const [editing, setEditing] = useState<SiteIntegrationResource | null>(null)
  const [inbound, setInbound] = useState<SiteIntegrationResource | null>(null)

  const createMutation = useResourceMutation({
    mutation: siteIntegrationsCreateMutation(),
    invalidate: [queryKey],
    successMessage: t(($) => $.settings.integrations.saved),
    errorTitle: t(($) => $.settings.integrations.createError),
    onDone: () => form.reset(),
  })

  const deleteMutation = useResourceMutation({
    mutation: siteIntegrationsDeleteMutation(),
    invalidate: [queryKey],
    errorTitle: t(($) => $.settings.integrations.deleteError),
  })

  const onDelete = (id: string) => {
    confirmDelete({
      title: t(($) => $.settings.integrations.deleteConfirmTitle),
      description: t(($) => $.settings.integrations.deleteConfirmDescription),
      onConfirm: () => deleteMutation.mutate({ path: { slug, id } }),
    })
  }

  const isSes = form.values.provider === 'ses'
  const records = integrationsQuery.data ?? []
  const unlimited = records.filter((record) => record.sendLimit.warnings.includes('unlimited'))
  const quotaUnavailable = records.filter((record) =>
    record.sendLimit.warnings.includes('providerQuotaUnavailable'),
  )

  return (
    <Card withBorder>
      <Title order={4} mb="xs">
        {t(($) => $.settings.integrations.title)}
      </Title>
      <Text c="dimmed" size="sm" mb="md">
        {t(($) => $.settings.integrations.description)}
      </Text>

      {integrationsQuery.isError ? (
        <Alert color="red" title={t(($) => $.settings.integrations.loadError)} mb="md">
          {t(($) => $.settings.integrations.loadError)}
        </Alert>
      ) : null}

      {canManageIntegrations ? (
        <form
          onSubmit={form.onSubmit((values) =>
            createMutation.mutate({
              path: { slug },
              body: {
                name: values.name.trim(),
                isDefault: values.isDefault,
                maxPerSecond: toLimit(values.maxPerSecond),
                maxPerDay: toLimit(values.maxPerDay),
                config: buildConfig(values),
              },
            }),
          )}
        >
          <Stack maw={520} mb="md">
            <Group grow>
              <TextInput
                label={t(($) => $.settings.integrations.nameLabel)}
                required
                {...form.getInputProps('name')}
              />
              <Select
                label={t(($) => $.settings.integrations.providerLabel)}
                data={[
                  { value: 'smtp', label: 'SMTP' },
                  { value: 'ses', label: 'Amazon SES' },
                ]}
                allowDeselect={false}
                {...form.getInputProps('provider')}
              />
            </Group>

            {isSes ? (
              <>
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
                <TextInput
                  label={t(($) => $.settings.integrations.fields.endpoint)}
                  description={t(($) => $.settings.integrations.fields.endpointHint)}
                  placeholder="https://postbox.cloud.yandex.net"
                  {...form.getInputProps('endpoint')}
                />
                <TextInput
                  label={t(($) => $.settings.integrations.fields.snsTopicArn)}
                  description={t(($) => $.settings.integrations.fields.snsTopicArnHint)}
                  placeholder="arn:aws:sns:eu-west-1:123456789012:ses-events"
                  {...form.getInputProps('snsTopicArn')}
                />
              </>
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

            <Group grow>
              <TextInput
                label={t(($) => $.settings.integrations.fields.from)}
                required
                {...form.getInputProps('from')}
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

            <Checkbox
              label={t(($) => $.settings.integrations.makeDefault)}
              {...form.getInputProps('isDefault', { type: 'checkbox' })}
            />

            <Group justify="flex-end">
              <Button type="submit" loading={createMutation.isPending}>
                {t(($) => $.settings.integrations.create)}
              </Button>
            </Group>
          </Stack>
        </form>
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
              <Group gap="xs" wrap="nowrap">
                {canManageIntegrations ? (
                  <Button size="compact-sm" variant="light" onClick={() => setEditing(record)}>
                    {t(($) => $.settings.integrations.limits.edit)}
                  </Button>
                ) : null}
                {canManageIntegrations ? (
                  <Button size="compact-sm" variant="light" onClick={() => setInbound(record)}>
                    {t(($) => $.settings.integrations.inbound.open)}
                  </Button>
                ) : null}
                <ChangeHistoryLink
                  slug={slug}
                  targetType={AUDIT_TARGET_INTEGRATION}
                  targetId={record.id}
                />
                {canManageIntegrations ? (
                  <Button
                    size="compact-sm"
                    color="red"
                    variant="light"
                    onClick={() => onDelete(record.id)}
                  >
                    {t(($) => $.settings.integrations.delete)}
                  </Button>
                ) : null}
              </Group>
            ),
          },
        ]}
        noRecordsText={t(($) => $.settings.integrations.empty)}
      />
      <SendLimitModal slug={slug} integration={editing} onClose={() => setEditing(null)} />
      <InboundModal slug={slug} integration={inbound} onClose={() => setInbound(null)} />
    </Card>
  )
}
