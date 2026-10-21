import {
  siteContactsCreateMutation,
  siteContactsGetOptions,
  siteContactsGetQueryKey,
  siteContactsListQueryKey,
  siteContactsUpdateMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { zSiteUpdateContactInput } from '../../generated/site/zod.gen.ts'
import { defineResource } from '../../resources/defineResource.tsx'
import { resourceFormSchema } from '../../resources/resourceFormSchema.ts'
import { Route as contactsEditRoute } from '../../routes/workspaces/$workspaceId/contacts/$contactId/edit.tsx'
import { ContactForm } from './ContactForm.tsx'

// Contact custom fields are not edited here, so they never round-trip into the payload.
const contactSchema = resourceFormSchema(
  zSiteUpdateContactInput.pick({
    subjectId: true,
    email: true,
    phone: true,
    firstName: true,
    lastName: true,
    timeZone: true,
  }),
)

const contactResource = defineResource({
  schema: contactSchema,
  Form: ContactForm,
  getOptions: siteContactsGetOptions,
  createMutation: siteContactsCreateMutation,
  updateMutation: siteContactsUpdateMutation,
  listQueryKey: siteContactsListQueryKey,
  getQueryKey: siteContactsGetQueryKey,
  editRoute: contactsEditRoute,
  idParam: 'contactId',
  texts: (t) => ({
    createTitle: t(($) => $.form.createTitle),
    editTitle: t(($) => $.form.editTitle),
    created: t(($) => $.notifications.contactCreated),
    updated: t(($) => $.notifications.contactUpdated),
    loadErrorTitle: t(($) => $.alerts.loadErrorTitle),
    saveErrorTitle: t(($) => $.alerts.saveErrorTitle),
  }),
  editTarget: (created, { workspaceId }) => ({
    to: contactsEditRoute.to,
    params: { workspaceId, contactId: created.id },
  }),
})

// Function declarations (hoisted) because the route files and this module import each other.
export function ContactCreateModal(props: { opened: boolean; onClose: () => void }) {
  return <contactResource.CreateModal {...props} />
}

export function ContactEditPage() {
  return <contactResource.EditPage />
}
