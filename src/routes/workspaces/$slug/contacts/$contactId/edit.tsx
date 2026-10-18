import { createFileRoute } from '@tanstack/react-router'

import { ContactEditPage } from '../../../../../pages/contacts/resource.tsx'

export const Route = createFileRoute('/workspaces/$slug/contacts/$contactId/edit')({
  component: ContactEditPage,
})
