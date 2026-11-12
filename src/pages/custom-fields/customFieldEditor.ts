import { siteCustomFieldsGetOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import { siteCustomFieldsUpdate } from '../../generated/site/sdk.gen.ts'
import { zSiteUpdateCustomFieldInput } from '../../generated/site/zod.gen.ts'
import { defineModalEditor } from '../../resources/defineResource.tsx'
import { withRevision } from '../../resources/resourceEditor.ts'
import { resourceFormSchema } from '../../resources/resourceFormSchema.ts'
import { CustomFieldForm } from './CustomFieldForm.tsx'

const customFieldSchema = resourceFormSchema(
  zSiteUpdateCustomFieldInput.pick({ name: true, type: true }).required(),
  { name: '', type: 'string' },
)

// The Custom field's edit modal over the list: an editor session like every edit page's, so a
// rename made on a revision someone moved past says so instead of overwriting it (ADR 0119).
export const customFieldEditor = defineModalEditor({
  editorId: 'customFieldEditor',
  schema: customFieldSchema,
  Form: CustomFieldForm,
  getOptions: siteCustomFieldsGetOptions,
  update: withRevision(siteCustomFieldsUpdate),
  operations: {},
  texts: (t) => ({
    editTitle: t(($) => $.customFields.editTitle),
    updated: t(($) => $.notifications.customFieldUpdated),
    loadErrorTitle: t(($) => $.alerts.customFieldLoadErrorTitle),
    saveErrorTitle: t(($) => $.customFields.updateError),
  }),
})
