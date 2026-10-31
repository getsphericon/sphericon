import type { ErrorSchema, RJSFSchema, UiSchema } from '@rjsf/utils'
import * as z from 'zod'

import type { SitePresetResource } from '../../generated/site/types.gen.ts'
import { getFieldErrors } from '../../utils/apiErrors.ts'

// The Preset params as the form holds them: each param by name.
export type PresetParams = Record<string, unknown>

const propertiesSchema = z.record(z.string(), z.record(z.string(), z.unknown()))

// The names of a Preset's params, each with whether it is a secret (`writeOnly`, ADR 0037): a
// secret is stored encrypted and never shown again.
function paramsOf(preset: SitePresetResource): { name: string; secret: boolean }[] {
  const parsed = propertiesSchema.safeParse(preset.paramsSchema.properties)
  if (!parsed.success) {
    return []
  }
  return Object.entries(parsed.data).map(([name, schema]) => ({
    name,
    secret: schema.writeOnly === true,
  }))
}

// The names of a Preset's secrets.
function secretsOf(preset: SitePresetResource): Set<string> {
  return new Set(
    paramsOf(preset)
      .filter((p) => p.secret)
      .map((p) => p.name),
  )
}

// The secrets that have a stored value: the server names one by its mask, never its value.
export function savedSecrets(preset: SitePresetResource, params: PresetParams): Set<string> {
  return new Set(
    paramsOf(preset)
      .filter(
        ({ name, secret }) => secret && typeof params[name] === 'string' && params[name] !== '',
      )
      .map(({ name }) => name),
  )
}

// The form's starting values: the stored params without the secrets, which are never filled in.
export function formValues(preset: SitePresetResource, params: PresetParams): PresetParams {
  const secrets = secretsOf(preset)
  return Object.fromEntries(Object.entries(params).filter(([name]) => !secrets.has(name)))
}

// The schema the form validates by: the Preset's own, except that a secret with a stored value is
// not required, since a blank one keeps it.
export function formSchema(preset: SitePresetResource, saved: Set<string>): RJSFSchema {
  const required = z.array(z.string()).safeParse(preset.paramsSchema.required)
  return {
    ...preset.paramsSchema,
    ...(required.success ? { required: required.data.filter((name) => !saved.has(name)) } : {}),
  }
}

// The form's look: a secret is a password field, never pre-filled; with a stored value it says a
// blank keeps it and a new one starts a rotation.
export function formUi(
  preset: SitePresetResource,
  saved: Set<string>,
  texts: { keep: string; rotates: string },
): UiSchema {
  const ui: UiSchema = { 'ui:submitButtonOptions': { norender: true } }
  for (const { name, secret } of paramsOf(preset)) {
    if (secret) {
      ui[name] = {
        'ui:widget': 'password',
        ...(saved.has(name) ? { 'ui:placeholder': texts.keep, 'ui:help': texts.rotates } : {}),
      }
    }
  }
  return ui
}

// The params to save: a blank secret is left out, so the stored one is kept.
export function paramsToSave(preset: SitePresetResource, values: PresetParams): PresetParams {
  const secrets = secretsOf(preset)
  return Object.fromEntries(
    Object.entries(values).filter(
      ([name, value]) => !(secrets.has(name) && (value === undefined || value === '')),
    ),
  )
}

// The prefix of a refused param's field in a problem's `errors`.
const PARAM_FIELD = 'params.'

// The server's refusal of the params on the form's fields: a refusal of one param is on its field
// (params.<name>), one of the params as a whole on the form. None is an empty schema.
export function serverErrors(error: unknown): ErrorSchema<PresetParams> {
  const fields = getFieldErrors(error) ?? {}
  const whole = fields.params
  const out: ErrorSchema<PresetParams> = whole ? { __errors: whole } : {}
  for (const [field, messages] of Object.entries(fields)) {
    if (field.startsWith(PARAM_FIELD)) {
      out[field.slice(PARAM_FIELD.length)] = { __errors: messages }
    }
  }
  return out
}
