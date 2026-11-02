import {
  siteContactsCreateMutation,
  siteContactsGetOptions,
  siteContactsUpdateMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { zSiteUpdateContactInput } from '../../generated/site/zod.gen.ts'
import { defineResource } from '../../resources/defineResource.tsx'
import { resourceFormSchema } from '../../resources/resourceFormSchema.ts'
import { Route as contactsEditRoute } from '../../routes/workspaces/$workspaceId/contacts/$contactId/edit.tsx'
import { ContactForm } from './ContactForm.tsx'

// Contact custom fields are not edited here, so they never round-trip into the payload. The
// subject id is set by the tracker and the API, never typed by hand, so the form leaves it out.

const contactSchema = resourceFormSchema(
  zSiteUpdateContactInput
    .pick({
      email: true,
      phone: true,
      firstName: true,
      lastName: true,
      timeZone: true,
      country: true,
      locale: true,
      birthday: true,
      city: true,
      avatar: true,
      tags: true,
    })
    .required({ tags: true }),
  {
    email: '',
    phone: '',
    firstName: '',
    lastName: '',
    timeZone: '',
    country: '',
    locale: '',
    birthday: '',
    city: '',
    avatar: '',
    tags: [] as string[],
  },
)

const contactResource = defineResource({
  schema: contactSchema,
  Form: ContactForm,
  getOptions: siteContactsGetOptions,
  createMutation: siteContactsCreateMutation,
  updateMutation: siteContactsUpdateMutation,
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
