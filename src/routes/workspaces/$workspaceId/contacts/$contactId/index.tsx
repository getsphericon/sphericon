import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { pageParam } from '../../../../../searchSchemas.ts'

export const Route = createFileRoute('/workspaces/$workspaceId/contacts/$contactId/')({
  // The page of the contact's events and whether machine events are shown, so the view is
  // linkable. Another contact is another path, which starts on the first page.
  validateSearch: z.object({ page: pageParam, machine: z.boolean().optional().catch(undefined) }),
})
