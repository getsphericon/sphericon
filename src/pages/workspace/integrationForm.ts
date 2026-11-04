import { z } from 'zod'

import type {
  IntegrationConfigInput,
  SiteIntegrationProvider,
  SiteIntegrationResource,
} from '../../generated/site/types.gen.ts'
import { zEmailAddress } from '../../generated/site/zod.gen.ts'
import { NO_LIMITS } from './providerLimit.ts'
import type { ProviderLimitValues } from './providerLimit.ts'

// The state of a form that asks for an Integration's credentials (integrationFields.tsx), and the
// config it sends.

// How a Twilio account sends: with an API key and its secret, or with the Auth Token.
type TwilioCredential = 'apiKey' | 'authToken'

// Flat form state; the provider-specific config object is assembled on submit
// from whichever fields the selected provider uses.
export interface IntegrationFormValues extends ProviderLimitValues {
  name: string
  provider: SiteIntegrationProvider
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
  twilioCredential: TwilioCredential
  accountSid: string
  apiKeySid: string
  apiKeySecret: string
  authToken: string
}

export const INITIAL_VALUES: IntegrationFormValues = {
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
  twilioCredential: 'apiKey',
  accountSid: '',
  apiKeySid: '',
  apiKeySecret: '',
  authToken: '',
  ...NO_LIMITS,
}

// storedValues is the form of a saved Integration: what the API reads back of its config, every
// secret blank. An SES access key ID is read back only by its last four characters, so it is blank too.
export function storedValues(integration: SiteIntegrationResource): IntegrationFormValues {
  const values = { ...INITIAL_VALUES, name: integration.name, provider: integration.provider }
  const { config } = integration
  switch (config.kind) {
    case 'smtp':
      return {
        ...values,
        host: config.host,
        port: config.port,
        username: config.username ?? '',
        from: config.from,
        fromName: config.fromName ?? '',
      }
    case 'ses':
      return {
        ...values,
        region: config.region,
        endpoint: config.endpoint ?? '',
        from: config.from,
        fromName: config.fromName ?? '',
      }
    case 'twilio':
      return {
        ...values,
        twilioCredential: config.apiKeySid === null ? 'authToken' : 'apiKey',
        accountSid: config.accountSid,
        apiKeySid: config.apiKeySid ?? '',
        from: config.from,
      }
    case 'telegram':
    case 'inbox':
      // Nothing of a Telegram bot's config is read back to edit, and the Test inbox has none.
      break
  }
  return values
}

export function buildConfig(values: IntegrationFormValues): IntegrationConfigInput {
  if (values.provider === 'telegram') {
    return { kind: 'telegram', token: values.token.trim() }
  }
  if (values.provider === 'twilio') {
    const withKey = values.twilioCredential === 'apiKey'
    return {
      kind: 'twilio',
      accountSid: values.accountSid.trim(),
      apiKeySid: withKey ? values.apiKeySid.trim() : null,
      secret: withKey ? values.apiKeySecret.trim() : values.authToken.trim(),
      from: values.from.trim(),
    }
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

// paramsOf is the params a Twilio account's form sends beside its config: its callbacks are signed
// with its Auth Token, the `twilio` Preset's param, kept apart from the send credential so either
// rotates alone. A blank one (saving the credentials again) sends none and keeps the stored one.
export function paramsOf(values: IntegrationFormValues): { params?: { auth_token: string } } {
  const authToken = values.authToken.trim()
  return values.provider === 'twilio' && authToken !== ''
    ? { params: { auth_token: authToken } }
    : {}
}

// hasCredentials is whether an Integration has credentials to save again: all but the Test inbox.
export function hasCredentials(integration: SiteIntegrationResource): boolean {
  return integration.config.kind !== 'inbox'
}

// The body an Update sends to save an Integration's credentials again.
export interface CredentialsBody {
  config: IntegrationConfigInput
  params?: { auth_token: string }
}

// The fields a saved Integration's provider cannot be saved without. A secret is never one: blank,
// it keeps the stored secret. An SES-compatible service keeps its endpoint.
function requiredFields(integration: SiteIntegrationResource, values: IntegrationFormValues) {
  const { config } = integration
  switch (config.kind) {
    case 'smtp':
      return ['host', 'from'] as const
    case 'ses':
      return config.endpoint
        ? (['endpoint', 'region', 'accessKeyId', 'from'] as const)
        : (['region', 'accessKeyId', 'from'] as const)
    case 'twilio':
      return values.twilioCredential === 'apiKey'
        ? (['accountSid', 'apiKeySid', 'from'] as const)
        : (['accountSid', 'from'] as const)
    case 'telegram':
    case 'inbox':
      return [] as const
  }
  return [] as const
}

// credentialsSchema checks the form that saves an Integration's credentials again (a blank required
// field, an email provider's From that is no address) and turns it into the Update's body.
export function credentialsSchema(
  integration: SiteIntegrationResource,
): z.ZodType<CredentialsBody, IntegrationFormValues> {
  // The values come from the form's own state, so their types need no check of their own: the
  // contract's rules are the request body's, which buildConfig makes of them.
  return z
    .custom<IntegrationFormValues>()
    .superRefine((values, ctx) => {
      for (const field of requiredFields(integration, values)) {
        if (values[field].trim() === '') {
          ctx.addIssue({ code: 'custom', path: [field], input: '' })
        }
      }
      const email = integration.config.kind === 'smtp' || integration.config.kind === 'ses'
      const from = values.from.trim()
      if (email && from !== '') {
        for (const issue of zEmailAddress.safeParse(from).error?.issues ?? []) {
          ctx.addIssue({ code: 'custom', path: ['from'], message: issue.message, input: from })
        }
      }
    })
    .transform((values) => ({ config: buildConfig(values), ...paramsOf(values) }))
}
