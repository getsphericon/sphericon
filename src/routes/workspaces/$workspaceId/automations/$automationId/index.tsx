import { createFileRoute } from '@tanstack/react-router'

import { siteAutomationsGetOptions } from '../../../../../generated/site/@tanstack/react-query.gen.ts'
import { pagedSearch } from '../../../../../searchSchemas.ts'
import { preload } from '../../../../../utils/preload.ts'

export const Route = createFileRoute('/workspaces/$workspaceId/automations/$automationId/')({
  validateSearch: pagedSearch,
  // Fetched while the link is hovered; a failure is left to the page, which shows it inline.
  loader: async ({ context, params }) => {
    await preload(
      context.queryClient,
      siteAutomationsGetOptions({
        path: { workspaceId: params.workspaceId, id: params.automationId },
      }),
    )
  },
})
