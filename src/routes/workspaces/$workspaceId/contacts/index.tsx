import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { pageParam } from '../../../../searchSchemas.ts'

export const Route = createFileRoute('/workspaces/$workspaceId/contacts/')({
  // The email search and the filters, so a filtered list is linkable (the Tags page links to a
  // Tag's contacts). Coerced because the router parses an all-digit value back to a number.
  validateSearch: z.object({
    email: z.coerce.string().optional().catch(undefined),
    tag: z.coerce.string().optional().catch(undefined),
    country: z.string().optional().catch(undefined),
    page: pageParam,
  }),
})
