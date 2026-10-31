import * as z from 'zod'

import {
  siteTemplatesCreateMutation,
  siteTemplatesGetOptions,
  siteTemplatesUpdateMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { zCreateTemplateInput } from '../../generated/site/zod.gen.ts'
import { defineResource } from '../../resources/defineResource.tsx'
import { resourceFormSchema } from '../../resources/resourceFormSchema.ts'
import { Route as templatesEditRoute } from '../../routes/workspaces/$workspaceId/templates/$templateId/edit.tsx'
import { docCodec } from '../../utils/messageDoc.ts'
import { TemplateForm } from './TemplateForm.tsx'

// The document is held as JSON text; a blank one is absent, so a source-only save writes a body.
const templateSchema = resourceFormSchema(
  zCreateTemplateInput
    .pick({ name: true, subject: true, body: true })
    .extend({ doc: z.exactOptional(docCodec) }),
)

const templateResource = defineResource({
  schema: templateSchema,
  Form: TemplateForm,
  getOptions: siteTemplatesGetOptions,
  createMutation: siteTemplatesCreateMutation,
  updateMutation: siteTemplatesUpdateMutation,
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
