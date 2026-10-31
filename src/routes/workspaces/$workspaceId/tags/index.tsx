import { createFileRoute } from '@tanstack/react-router'

import { pagedSearch } from '../../../../searchSchemas.ts'

export const Route = createFileRoute('/workspaces/$workspaceId/tags/')({
  validateSearch: pagedSearch,
})
