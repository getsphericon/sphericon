import { createFileRoute } from '@tanstack/react-router'

import { archivableSearch } from '../../../../searchSchemas.ts'

export const Route = createFileRoute('/workspaces/$workspaceId/segments/')({
  // The archived view of the list, so it is linkable. Absent is the live list.
  validateSearch: archivableSearch,
})
