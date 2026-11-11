import {
  siteTemplatesCreateMutation,
  siteTemplatesGetOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { siteTemplatesUpdate } from '../../generated/site/sdk.gen.ts'
import type { CreateTemplateInput } from '../../generated/site/types.gen.ts'
import { zCreateTemplateInput } from '../../generated/site/zod.gen.ts'
import { defineResource } from '../../resources/defineResource.tsx'
import { resourceFormSchema } from '../../resources/resourceFormSchema.ts'
import type { ResourceFormSchema } from '../../resources/resourceFormSchema.ts'
import { Route as templatesEditRoute } from '../../routes/workspaces/$workspaceId/templates/$templateId/edit.tsx'
import { TemplateForm } from './TemplateForm.tsx'
import type { TemplateValues } from './TemplateForm.tsx'

// An absent document keeps source-only content in the body editor.
const blankTemplate: TemplateValues = {
  name: '',
  subject: '',
  body: '',
  doc: null,
}

const flatSchema = resourceFormSchema(zCreateTemplateInput, blankTemplate)

// A message written in the editor is its document and the server builds the body; sending both
// is a 422.
function toBody(payload: CreateTemplateInput): CreateTemplateInput {
  const { doc, name, subject } = payload
  return doc ? { name, ...(subject !== undefined && { subject }), doc } : payload
}

const templateSchema: ResourceFormSchema<CreateTemplateInput, TemplateValues> = {
  blank: flatSchema.blank,
  create: flatSchema.create.transform(toBody),
  update: flatSchema.update.transform(toBody),
  toValues: flatSchema.toValues,
}

const templateResource = defineResource({
  schema: templateSchema,
  Form: TemplateForm,
  getOptions: siteTemplatesGetOptions,
  createMutation: siteTemplatesCreateMutation,
  update: siteTemplatesUpdate,
  operations: {},
  editRoute: templatesEditRoute,
  idParam: 'templateId',
  texts: (t) => ({
    createTitle: t(($) => $.templates.createTitle),
    editTitle: t(($) => $.templates.editTitle),
    created: t(($) => $.notifications.templateCreated),
    updated: t(($) => $.notifications.templateUpdated),
    loadErrorTitle: t(($) => $.alerts.templateLoadErrorTitle),
    saveErrorTitle: t(($) => $.alerts.templateSaveErrorTitle),
  }),
  editTarget: (created, { workspaceId }) => ({
    to: templatesEditRoute.to,
    params: { workspaceId, templateId: created.id },
  }),
})

// Function declarations (hoisted) because the route files and this module import each other.
export function TemplateCreatePage() {
  return <templateResource.CreatePage />
}

export function TemplateEditPage() {
  return <templateResource.EditPage />
}
