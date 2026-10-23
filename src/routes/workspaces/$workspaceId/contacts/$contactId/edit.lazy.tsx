import { createLazyFileRoute } from '@tanstack/react-router'

import { ContactEditPage } from '../../../../../pages/contacts/resource.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/contacts/$contactId/edit')({
  component: ContactEditPage,
})
