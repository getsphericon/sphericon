import { createFileRoute } from '@tanstack/react-router'

import { zBroadcastChannel } from '../../../../generated/site/zod.gen.ts'
import { archivableSearch } from '../../../../searchSchemas.ts'

export const Route = createFileRoute('/workspaces/$workspaceId/broadcasts/')({
  // The archived view of the list and its Channel filter, so both are linkable. Absent is the
  // live list of every Channel.
  validateSearch: archivableSearch.extend({
    channel: zBroadcastChannel.optional().catch(undefined),
  }),
})
