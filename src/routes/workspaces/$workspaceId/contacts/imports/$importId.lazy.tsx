import { createLazyFileRoute } from '@tanstack/react-router'

import { ContactImportPage } from '../../../../../pages/contacts/importPage.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/contacts/imports/$importId')({
  component: ContactImportPage,
})
