import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { pageParam } from '../../../../../searchSchemas.ts'

export const Route = createFileRoute(
  '/workspaces/$workspaceId/automations/$automationId/skipped-entries',
)({
  // The contact filter and the page, so a filtered list is linkable. Coerced because the router
  // parses an all-digit value back to a number.
  validateSearch: z.object({
    contactId: z.coerce.string().optional().catch(undefined),
    page: pageParam,
  }),
})
