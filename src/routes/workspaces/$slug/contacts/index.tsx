import { createFileRoute } from '@tanstack/react-router'

import { ContactsListPage } from '../../../../pages/contacts/list.tsx'

export const Route = createFileRoute('/workspaces/$slug/contacts/')({
  component: ContactsListPage,
})
