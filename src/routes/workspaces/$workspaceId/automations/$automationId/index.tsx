import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { siteAutomationsGetOptions } from '../../../../../generated/site/@tanstack/react-query.gen.ts'
import { pageParam } from '../../../../../searchSchemas.ts'

export const Route = createFileRoute('/workspaces/$workspaceId/automations/$automationId/')({
  validateSearch: z.object({ page: pageParam }),
  // Fetched while the link is hovered; a failure is left to the page, which shows it inline.
  loader: async ({ context, params }) => {
    await context.queryClient.prefetchQuery(
      siteAutomationsGetOptions({
        path: { workspaceId: params.workspaceId, id: params.automationId },
      }),
    )
  },
})
