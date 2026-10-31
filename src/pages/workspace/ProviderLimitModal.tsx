import { Button, Group, Modal, Stack, Text } from '@mantine/core'
import { useForm } from '@mantine/form'
import { useTranslation } from 'react-i18next'

import {
  siteIntegrationsListQueryKey,
  siteIntegrationsUpdateMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteIntegrationResource } from '../../generated/site/types.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import {
  fromLimit,
  requiresProviderLimit,
  toLimit,
  useProviderLimitValidators,
} from './providerLimit.ts'
import type { ProviderLimitValues } from './providerLimit.ts'
import { ProviderLimitFields } from './ProviderLimitFields.tsx'

interface ProviderLimitModalProps {
  workspaceId: string
  integration: SiteIntegrationResource | null
  onClose: () => void
}

// Edits an Integration's two manual Provider limit values. Only the limits are sent, so the
// stored credentials are untouched; a blank input clears that value, except on an SES-compatible
// service, which keeps both.
export function ProviderLimitModal({ workspaceId, integration, onClose }: ProviderLimitModalProps) {
  const { t } = useTranslation()
  return (
    <Modal
      opened={integration !== null}
      onClose={onClose}
      title={t(($) => $.settings.integrations.providerLimit.editTitle, {
        name: integration?.name ?? '',
      })}
    >
      {integration ? (
        <ProviderLimitForm
          key={integration.id}
          workspaceId={workspaceId}
          integration={integration}
          onClose={onClose}
        />
      ) : null}
    </Modal>
  )
}

interface ProviderLimitFormProps {
  workspaceId: string
  integration: SiteIntegrationResource
  onClose: () => void
}

function ProviderLimitForm({ workspaceId, integration, onClose }: ProviderLimitFormProps) {
  const { t } = useTranslation()
  const { config } = integration
  const required = requiresProviderLimit(
    config.kind,
    config.kind === 'ses' ? config.endpoint : null,
  )
  const validate = useProviderLimitValidators(() => required)

  const form = useForm<ProviderLimitValues>({
    initialValues: {
      maxPerSecond: fromLimit(integration.maxPerSecond),
      maxPerDay: fromLimit(integration.maxPerDay),
    },
    validate,
  })

  const updateMutation = useResourceMutation({
    mutation: siteIntegrationsUpdateMutation(),
    invalidate: [siteIntegrationsListQueryKey({ path: { workspaceId } })],
    successMessage: t(($) => $.settings.integrations.providerLimit.saved),
    errorTitle: t(($) => $.settings.integrations.providerLimit.saveError),
    onDone: onClose,
  })

  return (
    <form
      onSubmit={form.onSubmit((values) =>
        updateMutation.mutate({
          path: { workspaceId, id: integration.id },
          body: {
            maxPerSecond: toLimit(values.maxPerSecond),
            maxPerDay: toLimit(values.maxPerDay),
          },
        }),
      )}
    >
      <Stack>
        <Text c="dimmed" size="sm">
          {required
            ? t(($) => $.settings.integrations.providerLimit.descriptionRequired)
            : t(($) => $.settings.integrations.providerLimit.description)}
        </Text>
        <ProviderLimitFields
          perSecondProps={form.getInputProps('maxPerSecond')}
          perDayProps={form.getInputProps('maxPerDay')}
          required={required}
        />
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            {t(($) => $.settings.integrations.providerLimit.cancel)}
          </Button>
          <Button type="submit" loading={updateMutation.isPending}>
            {t(($) => $.settings.integrations.providerLimit.save)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}
