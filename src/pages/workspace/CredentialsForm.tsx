import { Button, Group, Stack, Text, TextInput } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import type { SiteIntegrationResource } from '../../generated/site/types.gen.ts'
import type { ResourceFormProps } from '../../resources/defineResource.tsx'
import { EmailFields, TelegramFields, TwilioFields } from './integrationFields.tsx'
import type { CredentialsBody, IntegrationFormValues } from './integrationForm.ts'

// The fields of an Integration's credentials, by the stored Integration's provider. A provider's
// refusal of them is this form's usual failure: it shows here, and the modal stays open.
export function CredentialsForm({
  form,
  isPending,
  onSubmit,
  onCancel,
  stored,
  failure,
}: ResourceFormProps<CredentialsBody, IntegrationFormValues, SiteIntegrationResource>) {
  const { t } = useTranslation()
  if (!stored) {
    return null
  }
  const { config } = stored
  return (
    // The schema judges the fields (docs/standards/ui.md), so the browser's own check is off.
    <form noValidate onSubmit={form.onSubmit(onSubmit)}>
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
        {failure ? (
          <ApiErrorAlert
            error={failure}
            title={t(($) => $.settings.integrations.credentials.saveError)}
          />
        ) : null}
        <Group justify="flex-end">
          <Button variant="default" onClick={onCancel}>
            {t(($) => $.settings.integrations.credentials.cancel)}
          </Button>
          <Button type="submit" loading={isPending}>
            {t(($) => $.settings.integrations.credentials.save)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}
