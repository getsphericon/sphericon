import { createLazyFileRoute } from '@tanstack/react-router'

import { ContactImportUploadPage } from '../../../../pages/contacts/importUpload.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/contacts/import')({
  component: ContactImportUploadPage,
})
