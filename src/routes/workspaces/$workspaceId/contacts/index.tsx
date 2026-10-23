import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

export const Route = createFileRoute('/workspaces/$workspaceId/contacts/')({
  // The email search, so a filtered list is linkable. Coerced because the router parses an
  // all-digit value back to a number.
  validateSearch: z.object({ email: z.coerce.string().optional().catch(undefined) }),
})
