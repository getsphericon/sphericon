import { createLazyFileRoute } from '@tanstack/react-router'

import { ContactsListPage } from '../../../../pages/contacts/list.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/contacts/')({
  component: ContactsListPage,
})
