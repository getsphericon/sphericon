import { createLazyFileRoute } from '@tanstack/react-router'

import { ContactDetailPage } from '../../../../../pages/contacts/detail.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/contacts/$contactId/')({
  component: ContactDetailPage,
})
