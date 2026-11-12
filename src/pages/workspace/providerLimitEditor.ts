import * as z from 'zod'

import { siteIntegrationsGetOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import { siteIntegrationsUpdate } from '../../generated/site/sdk.gen.ts'
import { zMaxPerDay, zMaxPerSecond } from '../../generated/site/zod.gen.ts'
import { defineModalEditor } from '../../resources/defineResource.tsx'
import type { ResourceFormSchema } from '../../resources/resourceFormSchema.ts'
import { fromLimit, NO_LIMITS, toLimit } from './providerLimit.ts'
import type { ProviderLimitValues } from './providerLimit.ts'
import { ProviderLimitForm } from './ProviderLimitForm.tsx'
import type { ProviderLimitPayload } from './ProviderLimitForm.tsx'

// A blank input is no limit (null). The form checks the inputs in its own words first; the
// contract's bounds judge what it sends.
const limits = z
  .custom<ProviderLimitValues>((value) => typeof value === 'object' && value !== null)
  .transform((values) => ({
    maxPerSecond: toLimit(values.maxPerSecond),
    maxPerDay: toLimit(values.maxPerDay),
  }))
  .pipe(z.object({ maxPerSecond: zMaxPerSecond.nullable(), maxPerDay: zMaxPerDay.nullable() }))

const providerLimitSchema: ResourceFormSchema<ProviderLimitPayload, ProviderLimitValues> = {
  blank: NO_LIMITS,
  create: limits,
  update: limits,
  toValues: (integration) => ({
    maxPerSecond: fromLimit(integration.maxPerSecond ?? null),
    maxPerDay: fromLimit(integration.maxPerDay ?? null),
  }),
}

// An Integration's Provider limit edit: an editor session over the two limits alone.
export const providerLimitEditor = defineModalEditor({
  editorId: 'providerLimitEditor',
  schema: providerLimitSchema,
  Form: ProviderLimitForm,
  getOptions: siteIntegrationsGetOptions,
  update: siteIntegrationsUpdate,
  operations: {},
  texts: (t) => ({
    editTitle: t(($) => $.settings.integrations.providerLimit.title),
    updated: t(($) => $.settings.integrations.providerLimit.saved),
    loadErrorTitle: t(($) => $.settings.integrations.loadError),
    saveErrorTitle: t(($) => $.settings.integrations.providerLimit.saveError),
  }),
})
