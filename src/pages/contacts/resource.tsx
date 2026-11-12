import {
  siteContactsCreateMutation,
  siteContactsGetOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { siteContactsUpdate } from '../../generated/site/sdk.gen.ts'
import { defineResource } from '../../resources/defineResource.tsx'
import { withRevision } from '../../resources/resourceEditor.ts'
import { Route as contactsEditRoute } from '../../routes/workspaces/$workspaceId/contacts/$contactId/edit.tsx'
import { ContactForm } from './ContactForm.tsx'
import { contactSchema } from './contactSchema.ts'

const contactResource = defineResource({
  schema: contactSchema,
  Form: ContactForm,
  getOptions: siteContactsGetOptions,
  createMutation: siteContactsCreateMutation,
  update: withRevision(siteContactsUpdate),
  operations: {},
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
