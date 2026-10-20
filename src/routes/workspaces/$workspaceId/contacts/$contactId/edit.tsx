import { createFileRoute } from '@tanstack/react-router'

import { ContactEditPage } from '../../../../../pages/contacts/resource.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId/contacts/$contactId/edit')({
  component: ContactEditPage,
})
