import { siteIntegrationsGetOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import { siteIntegrationsUpdate } from '../../generated/site/sdk.gen.ts'
import type { SiteIntegrationProvider } from '../../generated/site/types.gen.ts'
import { zSiteIntegrationProvider } from '../../generated/site/zod.gen.ts'
import { defineModalEditor } from '../../resources/defineResource.tsx'
import { CredentialsForm } from './CredentialsForm.tsx'
import { credentialsSchema, INITIAL_VALUES, storedValues } from './integrationForm.ts'
import type { CredentialsVariant } from './integrationForm.ts'

// The credentials edit of an Integration of the variant: an editor session over its stored config.
function credentialsEditorOf(variant: CredentialsVariant) {
  const schema = credentialsSchema(variant)
  return defineModalEditor({
    editorId: `credentialsEditor-${variant.kind}${variant.compatible ? '-compatible' : ''}`,
    schema: {
      blank: INITIAL_VALUES,
      create: schema,
      update: schema,
      toValues: () => INITIAL_VALUES,
    },
    valuesOf: storedValues,
    Form: CredentialsForm,
    getOptions: siteIntegrationsGetOptions,
    update: siteIntegrationsUpdate,
    operations: {},
    texts: (t) => ({
      editTitle: t(($) => $.settings.integrations.credentials.open),
      updated: t(($) => $.settings.integrations.credentials.saved),
      loadErrorTitle: t(($) => $.settings.integrations.loadError),
      saveErrorTitle: t(($) => $.settings.integrations.credentials.saveError),
    }),
  })
}

type CredentialsEditor = ReturnType<typeof credentialsEditorOf>

// One editor per variant, made once: what the form requires depends on it.
const editors = new Map<string, CredentialsEditor>(
  zSiteIntegrationProvider.options.flatMap((kind: SiteIntegrationProvider) =>
    (kind === 'ses' ? [false, true] : [false]).map(
      (compatible) => [`${kind}:${compatible}`, credentialsEditorOf({ kind, compatible })] as const,
    ),
  ),
)

// credentialsEditor is the credentials edit of an Integration of the variant.
export function credentialsEditor(variant: CredentialsVariant): CredentialsEditor | undefined {
  return editors.get(`${variant.kind}:${variant.compatible}`)
}
