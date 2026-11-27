import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { siteContactsGetOptions } from '../../../../../generated/site/@tanstack/react-query.gen.ts'
import { pageParam } from '../../../../../searchSchemas.ts'
import { preload } from '../../../../../utils/preload.ts'

export const Route = createFileRoute('/workspaces/$workspaceId/contacts/$contactId/')({
  // The page of the contact's events and whether machine events are shown, so the view is
  // linkable. Another contact is another path, which starts on the first page.
  validateSearch: z.object({ page: pageParam, machine: z.boolean().optional().catch(undefined) }),
  // Fetched while the link is hovered; a failure is left to the page, which shows it inline.
  loader: async ({ context, params }) => {
    await preload(
      context.queryClient,
      siteContactsGetOptions({ path: { workspaceId: params.workspaceId, id: params.contactId } }),
    )
  },
})
