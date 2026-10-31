import { createFileRoute } from '@tanstack/react-router'

import { siteBroadcastsGetOptions } from '../../../../../generated/site/@tanstack/react-query.gen.ts'
import { preload } from '../../../../../utils/preload.ts'

export const Route = createFileRoute('/workspaces/$workspaceId/broadcasts/$broadcastId/report')({
  // Fetched while the link is hovered; a failure is left to the page, which shows it inline.
  loader: async ({ context, params }) => {
    await preload(
      context.queryClient,
      siteBroadcastsGetOptions({
        path: { workspaceId: params.workspaceId, id: params.broadcastId },
      }),
    )
  },
})
