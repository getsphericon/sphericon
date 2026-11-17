import { createFileRoute } from '@tanstack/react-router'

import { zBroadcastChannel, zBroadcastStatus } from '../../../../generated/site/zod.gen.ts'
import { archivableSearch } from '../../../../searchSchemas.ts'

export const Route = createFileRoute('/workspaces/$workspaceId/broadcasts/')({
  // The archived view of the list and its Channel and status filters, so each is linkable. Absent
  // is the live list of every Channel and status.
  validateSearch: archivableSearch.extend({
    channel: zBroadcastChannel.optional().catch(undefined),
    status: zBroadcastStatus.optional().catch(undefined),
  }),
})
