import { Group, NumberInput, SegmentedControl, Text, TextInput } from '@mantine/core'
import type { useForm } from '@mantine/form'
import { useTranslation } from 'react-i18next'

import type { IntegrationFormValues } from './integrationForm.ts'

// The fields of an Integration's credentials, shared by the form that connects a provider and the
// one that saves an Integration's credentials again (`stored`). The API never reads a secret back,
// so the second shows each secret blank, and a blank secret keeps the stored one.

type IntegrationForm = ReturnType<typeof useForm<IntegrationFormValues>>

// SecretInput is a secret's field: required to connect a provider when the provider needs it, and
// optional when saving one again, where a blank keeps the stored secret.
function SecretInput({
  label,
  hint,
  required,
  stored,
  inputProps,
}: {
  label: string
  hint?: string
  required: boolean
  stored: boolean
  inputProps: ReturnType<IntegrationForm['getInputProps']>
}) {
  const { t } = useTranslation()
  const keep = stored ? t(($) => $.settings.integrations.fields.secretKeep) : undefined
  const description = [hint, keep].filter(Boolean).join(' ') || undefined
  return (
    <TextInput
      label={label}
      description={description}
      type="password"
      required={required && !stored}
      {...inputProps}
    />
  )
}

// TelegramFields asks for a Telegram bot's token.
export function TelegramFields({
  form,
  stored = false,
}: {
  form: Pick<IntegrationForm, 'getInputProps'>
  stored?: boolean
}) {
  const { t } = useTranslation()
  return (
    <SecretInput
      label={t(($) => $.settings.integrations.fields.token)}
      hint={t(($) => $.settings.integrations.fields.tokenHint)}
      required
      stored={stored}
      inputProps={form.getInputProps('token')}
    />
  )
}

// EmailFields asks for an email provider's credentials, SES's or an SMTP server's, and its From.
// accessKeyIdLast4 is the end of a stored SES access key, which the person types again in full.
export function EmailFields({
  form,
  isSes,
  stored = false,
  accessKeyIdLast4,
}: {
  form: Pick<IntegrationForm, 'getInputProps'>
  isSes: boolean
  stored?: boolean
  accessKeyIdLast4?: string | null | undefined
}) {
  const { t } = useTranslation()
  return (
    <>
      {isSes ? (
        <Group grow align="flex-start">
          <TextInput
            label={t(($) => $.settings.integrations.fields.region)}
            required
            {...form.getInputProps('region')}
          />
          <TextInput
            label={t(($) => $.settings.integrations.fields.accessKeyId)}
            description={
              accessKeyIdLast4
                ? t(($) => $.settings.integrations.fields.accessKeyIdStored, {
                    last4: accessKeyIdLast4,
                  })
                : undefined
            }
            required
            {...form.getInputProps('accessKeyId')}
          />
          <SecretInput
            label={t(($) => $.settings.integrations.fields.secretAccessKey)}
            required
            stored={stored}
            inputProps={form.getInputProps('secretAccessKey')}
          />
        </Group>
      ) : null}
      {isSes ? null : (
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
              // A port is a number that names, not a quantity: it is never grouped.
              thousandSeparator={false}
              {...form.getInputProps('port')}
            />
          </Group>
          <Group grow align="flex-start">
            <TextInput
              label={t(($) => $.settings.integrations.fields.username)}
              {...form.getInputProps('username')}
            />
            <SecretInput
              label={t(($) => $.settings.integrations.fields.password)}
              required={false}
              stored={stored}
              inputProps={form.getInputProps('password')}
            />
          </Group>
        </>
      )}
      <TextInput
        label={t(($) => $.settings.integrations.fields.from)}
        required
        {...form.getInputProps('from')}
      />
    </>
  )
}

// TwilioFields asks for a Twilio account: the Account SID, the send credential (an API key and its
// secret, or the Auth Token), the Auth Token its callbacks are signed with, and the sender.
export function TwilioFields({
  form,
  stored = false,
}: {
  form: Pick<IntegrationForm, 'getInputProps' | 'values'>
  stored?: boolean
}) {
  const { t } = useTranslation()
  const withAPIKey = form.values.twilioCredential === 'apiKey'
  return (
    <>
      <TextInput
        label={t(($) => $.settings.integrations.fields.accountSid)}
        placeholder="AC…"
        required
        {...form.getInputProps('accountSid')}
      />
      <SegmentedControl
        aria-label={t(($) => $.settings.integrations.fields.twilioCredential)}
        data={[
          {
            value: 'apiKey',
            label: t(($) => $.settings.integrations.fields.twilioApiKey),
          },
          {
            value: 'authToken',
            label: t(($) => $.settings.integrations.fields.twilioAuthTokenOnly),
          },
        ]}
        {...form.getInputProps('twilioCredential')}
      />
      {withAPIKey ? (
        <Group grow align="flex-start">
          <TextInput
            label={t(($) => $.settings.integrations.fields.apiKeySid)}
            placeholder="SK…"
            required
            {...form.getInputProps('apiKeySid')}
          />
          <SecretInput
            label={t(($) => $.settings.integrations.fields.apiKeySecret)}
            required
            stored={stored}
            inputProps={form.getInputProps('apiKeySecret')}
          />
        </Group>
      ) : null}
      {/* Saving an API-key account again leaves the Auth Token that signs its callbacks: it
          rotates in the inbound settings. */}
      {stored && withAPIKey ? null : (
        <SecretInput
          label={t(($) => $.settings.integrations.fields.authToken)}
          hint={
            withAPIKey
              ? t(($) => $.settings.integrations.fields.authTokenHint)
              : t(($) => $.settings.integrations.fields.authTokenSendsHint)
          }
          required
          stored={stored}
          inputProps={form.getInputProps('authToken')}
        />
      )}
      <TextInput
        label={t(($) => $.settings.integrations.fields.smsFrom)}
        description={t(($) => $.settings.integrations.fields.smsFromHint)}
        required
        {...form.getInputProps('from')}
      />
      <Text c="dimmed" size="sm">
        {t(($) => $.settings.integrations.carrierRegistration)}
      </Text>
    </>
  )
}
