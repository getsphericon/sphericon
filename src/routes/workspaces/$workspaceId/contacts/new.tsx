import { createFileRoute } from '@tanstack/react-router'

import { ContactCreatePage } from '../../../../pages/contacts/resource.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId/contacts/new')({
  component: ContactCreatePage,
})
