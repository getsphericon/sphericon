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
  SendLimitFields,
  type SendLimitValues,
  toLimit,
  useSendLimitValidators,
} from './SendLimitFields.tsx'

interface SendLimitModalProps {
  slug: string
  integration: SiteIntegrationResource | null
  onClose: () => void
}

// Edits an Integration's two manual Send rate limits. Only the limits are sent, so the
// stored credentials are untouched; a blank input clears that limit.
export function SendLimitModal({ slug, integration, onClose }: SendLimitModalProps) {
  const { t } = useTranslation()
  return (
    <Modal
      opened={integration !== null}
      onClose={onClose}
      title={t(($) => $.settings.integrations.limits.editTitle, { name: integration?.name ?? '' })}
    >
      {integration ? (
        <SendLimitForm
          key={integration.id}
          slug={slug}
          integration={integration}
          onClose={onClose}
        />
      ) : null}
    </Modal>
  )
}

interface SendLimitFormProps {
  slug: string
  integration: SiteIntegrationResource
  onClose: () => void
}

function SendLimitForm({ slug, integration, onClose }: SendLimitFormProps) {
  const { t } = useTranslation()
  const validate = useSendLimitValidators()

  const form = useForm<SendLimitValues>({
    initialValues: {
      maxPerSecond: fromLimit(integration.maxPerSecond),
      maxPerDay: fromLimit(integration.maxPerDay),
    },
    validate,
  })

  const updateMutation = useResourceMutation({
    mutation: siteIntegrationsUpdateMutation(),
    invalidate: [siteIntegrationsListQueryKey({ path: { slug } })],
    successMessage: t(($) => $.settings.integrations.limits.saved),
    errorTitle: t(($) => $.settings.integrations.limits.saveError),
    onDone: onClose,
  })

  return (
    <form
      onSubmit={form.onSubmit((values) =>
        updateMutation.mutate({
          path: { slug, id: integration.id },
          body: {
            maxPerSecond: toLimit(values.maxPerSecond),
            maxPerDay: toLimit(values.maxPerDay),
          },
        }),
      )}
    >
      <Stack>
        <Text c="dimmed" size="sm">
          {t(($) => $.settings.integrations.limits.description)}
        </Text>
        <SendLimitFields
          perSecondProps={form.getInputProps('maxPerSecond')}
          perDayProps={form.getInputProps('maxPerDay')}
        />
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            {t(($) => $.settings.integrations.limits.cancel)}
          </Button>
          <Button type="submit" loading={updateMutation.isPending}>
            {t(($) => $.settings.integrations.limits.save)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}
