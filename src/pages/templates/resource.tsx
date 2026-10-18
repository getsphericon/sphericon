import {
  siteTemplatesCreateMutation,
  siteTemplatesGetOptions,
  siteTemplatesGetQueryKey,
  siteTemplatesListQueryKey,
  siteTemplatesUpdateMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { zSiteCreateEmailTemplateInput } from '../../generated/site/zod.gen.ts'
import { defineResource } from '../../resources/defineResource.tsx'
import { resourceFormSchema } from '../../resources/resourceFormSchema.ts'
import { Route as templatesEditRoute } from '../../routes/workspaces/$slug/templates/$templateId/edit.tsx'
import { TemplateForm } from './TemplateForm.tsx'

const templateSchema = resourceFormSchema(zSiteCreateEmailTemplateInput)

const templateResource = defineResource({
  schema: templateSchema,
  Form: TemplateForm,
  getOptions: siteTemplatesGetOptions,
  createMutation: siteTemplatesCreateMutation,
  updateMutation: siteTemplatesUpdateMutation,
  listQueryKey: siteTemplatesListQueryKey,
  getQueryKey: siteTemplatesGetQueryKey,
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
  editTarget: (created, { slug }) => ({
    to: templatesEditRoute.to,
    params: { slug, templateId: created.id },
  }),
})

// Function declarations (hoisted) because the route files and this module import each other.
export function TemplateCreatePage() {
  return <templateResource.CreatePage />
}

export function TemplateEditPage() {
  return <templateResource.EditPage />
}
