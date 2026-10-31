import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { pageParam } from '../../../../searchSchemas.ts'

export const Route = createFileRoute('/workspaces/$workspaceId/broadcasts/')({
  // The archived view of the list, so it is linkable. Absent is the live list.
  validateSearch: z.object({ archived: z.boolean().optional().catch(undefined), page: pageParam }),
})
