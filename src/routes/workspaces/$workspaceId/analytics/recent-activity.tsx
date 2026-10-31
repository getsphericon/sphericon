import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { pageParam } from '../../../../searchSchemas.ts'

// The feed's action filter and page, so a filtered feed is linkable: the Events report links to
// an action's raw rows.
export const Route = createFileRoute('/workspaces/$workspaceId/analytics/recent-activity')({
  validateSearch: z.object({ action: z.string().optional().catch(undefined), page: pageParam }),
})
