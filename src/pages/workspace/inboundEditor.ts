import * as z from 'zod'

import { siteIntegrationsGetOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import {
  siteIntegrationsRegenerateKey,
  siteIntegrationsUpdate,
} from '../../generated/site/sdk.gen.ts'
import type { SiteIntegrationResource } from '../../generated/site/types.gen.ts'
import { defineEditForm } from '../../resources/defineResource.tsx'
import type { ResourceEditOperation } from '../../resources/defineResource.tsx'
import type { ResourceFormSchema } from '../../resources/resourceFormSchema.ts'
import { InboundForm } from './InboundForm.tsx'
import type { InboundPayload, InboundValues } from './InboundForm.tsx'
import { unmasked } from './presetParams.ts'

// A save sends the Preset, and its params when it has one; a param left as read is kept.
const inboundBody = z
  .custom<InboundValues>((value) => typeof value === 'object' && value !== null)
  .transform(({ preset, params }): InboundPayload =>
    preset === null ? { preset } : { preset, params },
  )

const inboundSchema: ResourceFormSchema<InboundPayload, InboundValues> = {
  blank: { preset: null, params: {} },
  create: inboundBody,
  update: inboundBody,
  toValues: () => ({ preset: null, params: {} }),
}

// Retiring the old secrets of a rotation and a new hook key act on the stored Integration: they are
// the session's operations, so they wait for a Save and the editor follows what each answers.
export const inboundOperations = {
  RETIRE: {
    request: async ({ subject: { path } }) => {
      const { data } = await siteIntegrationsUpdate({
        path,
        body: { retireSecrets: true },
        throwOnError: true,
      })
      return data
    },
    available: ({ resource }) => resource.secretsRetiring.length > 0,
    texts: (t) => ({
      accepted: t(($) => $.settings.integrations.inbound.retired),
      failedTitle: t(($) => $.settings.integrations.inbound.saveError),
    }),
  },
  REGENERATE: {
    request: async ({ subject: { path } }) => {
      const { data } = await siteIntegrationsRegenerateKey({ path, throwOnError: true })
      return data
    },
    available: () => true,
    texts: (t) => ({
      accepted: t(($) => $.settings.integrations.inbound.regenerated),
      failedTitle: t(($) => $.settings.integrations.inbound.regenerateError),
    }),
  },
} satisfies Record<string, ResourceEditOperation<SiteIntegrationResource, InboundValues>>

// The inbound side of an Integration in an editor session (ADR 0119): its Preset and params.
export const InboundEditForm = defineEditForm({
  editorId: 'inboundEditor',
  schema: inboundSchema,
  valuesOf: (integration) => ({ preset: integration.preset, params: unmasked(integration.params) }),
  Form: InboundForm,
  getOptions: siteIntegrationsGetOptions,
  update: siteIntegrationsUpdate,
  operations: inboundOperations,
  texts: (t) => ({
    editTitle: t(($) => $.settings.integrations.inbound.open),
    updated: t(($) => $.settings.integrations.inbound.saved),
    loadErrorTitle: t(($) => $.settings.integrations.loadError),
    saveErrorTitle: t(($) => $.settings.integrations.inbound.saveError),
  }),
})
