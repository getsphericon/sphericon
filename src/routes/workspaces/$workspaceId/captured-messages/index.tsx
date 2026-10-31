import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { pageParam } from '../../../../searchSchemas.ts'

export const Route = createFileRoute('/workspaces/$workspaceId/captured-messages/')({
  validateSearch: z.object({ page: pageParam }),
})
