import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

// `action` seeds the feed's action filter, so the Events report can link to its raw rows.
export const Route = createFileRoute('/workspaces/$workspaceId/analytics/recent-activity')({
  validateSearch: z.object({ action: z.string().optional().catch(undefined) }),
})
