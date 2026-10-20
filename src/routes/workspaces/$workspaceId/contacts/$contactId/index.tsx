import { createFileRoute } from '@tanstack/react-router'

import { ContactDetailPage } from '../../../../../pages/contacts/detail.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId/contacts/$contactId/')({
  component: ContactDetailPage,
})
