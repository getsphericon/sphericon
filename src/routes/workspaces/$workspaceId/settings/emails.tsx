import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

export const Route = createFileRoute('/workspaces/$workspaceId/settings/emails')({
  // The Integration whose credentials open on arrival, so a refused one is saved again from the link
  // that names it. Coerced because the router parses an all-digit id back to a number.
  validateSearch: z.object({ integration: z.coerce.string().optional().catch(undefined) }),
})
