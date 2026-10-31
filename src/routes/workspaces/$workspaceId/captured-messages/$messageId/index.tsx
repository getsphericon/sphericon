import { createFileRoute } from '@tanstack/react-router'

import { siteCapturedMessagesGetOptions } from '../../../../../generated/site/@tanstack/react-query.gen.ts'
import { preload } from '../../../../../utils/preload.ts'

export const Route = createFileRoute('/workspaces/$workspaceId/captured-messages/$messageId/')({
  // Fetched while the link is hovered; a failure is left to the page, which shows it inline.
  loader: async ({ context, params }) => {
    await preload(
      context.queryClient,
      siteCapturedMessagesGetOptions({
        path: { workspaceId: params.workspaceId, id: params.messageId },
      }),
    )
  },
})
