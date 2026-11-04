import { Button, Group, Modal, Stack, Text, TextInput } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import { siteIntegrationsUpdateMutation } from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteIntegrationResource } from '../../generated/site/types.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { useSchemaForm } from '../../resources/schemaForm.ts'
import { EmailFields, TelegramFields, TwilioFields } from './integrationFields.tsx'
import { credentialsSchema, storedValues } from './integrationForm.ts'
import { useIntegrationGone } from './integrationGone.ts'

interface CredentialsModalProps {
  workspaceId: string
  integration: SiteIntegrationResource | null
  onClose: () => void
}

// Saves an Integration's credentials again: new ones, or the stored ones (a blank secret), which the
// provider checks anew. A provider that refused them at a send holds the Integration until it
// accepts them here. Only the config is sent, so the name, the limits and the default stay.
export function CredentialsModal({ workspaceId, integration, onClose }: CredentialsModalProps) {
  const { t } = useTranslation()
  return (
    <Modal
      opened={integration !== null}
      onClose={onClose}
      size="lg"
      title={t(($) => $.settings.integrations.credentials.title, {
        name: integration?.name ?? '',
      })}
    >
      {integration ? (
        <CredentialsForm
          key={integration.id}
          workspaceId={workspaceId}
          integration={integration}
          onClose={onClose}
        />
      ) : null}
    </Modal>
  )
}

interface CredentialsFormProps {
  workspaceId: string
  integration: SiteIntegrationResource
  onClose: () => void
}

function CredentialsForm({ workspaceId, integration, onClose }: CredentialsFormProps) {
  const { t } = useTranslation()
  const { config } = integration
  const form = useSchemaForm(credentialsSchema(integration), storedValues(integration))

  const gone = useIntegrationGone(workspaceId, onClose)
  const errorTitle = t(($) => $.settings.integrations.credentials.saveError)
  const updateMutation = useResourceMutation({
    mutation: siteIntegrationsUpdateMutation(),
    successMessage: t(($) => $.settings.integrations.credentials.saved),
    errorTitle,
    // The provider refusing the credentials is this form's usual failure: it shows in the form,
    // which stays open to correct them. A provider deleted meanwhile closes the form instead.
    handleError: async (error) => {
      await gone(errorTitle)(error)
      return true
    },
    onDone: onClose,
  })

  return (
    // The schema judges the fields (docs/standards/ui.md), so the browser's own check is off.
    <form
      noValidate
      onSubmit={form.onSubmit((body) =>
        updateMutation.mutate({ path: { workspaceId, id: integration.id }, body }),
      )}
    >
      <Stack>
        <Text c="dimmed" size="sm">
          {t(($) => $.settings.integrations.credentials.description)}
        </Text>
        {config.kind === 'telegram' ? <TelegramFields form={form} stored /> : null}
        {config.kind === 'twilio' ? <TwilioFields form={form} stored /> : null}
        {config.kind === 'ses' && config.endpoint ? (
          <TextInput
            label={t(($) => $.settings.integrations.fields.endpoint)}
            description={t(($) => $.settings.integrations.fields.endpointHint)}
            required
            {...form.getInputProps('endpoint')}
          />
        ) : null}
        {config.kind === 'ses' || config.kind === 'smtp' ? (
          <>
            <EmailFields
              form={form}
              isSes={config.kind === 'ses'}
              stored
              accessKeyIdLast4={config.kind === 'ses' ? config.accessKeyIdLast4 : undefined}
            />
            <TextInput
              label={t(($) => $.settings.integrations.fields.fromName)}
              {...form.getInputProps('fromName')}
            />
          </>
        ) : null}
        {updateMutation.isError ? (
          <ApiErrorAlert
            error={updateMutation.error}
            title={t(($) => $.settings.integrations.credentials.saveError)}
          />
        ) : null}
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            {t(($) => $.settings.integrations.credentials.cancel)}
          </Button>
          <Button type="submit" loading={updateMutation.isPending}>
            {t(($) => $.settings.integrations.credentials.save)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}
