import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { zSiteAnalyticsRange } from '../../../../generated/site/zod.gen.ts'

// The analytics section: a layout with the shared window. The chosen window lives here, in the
// search params, so every report under it reads the same one and the URL carries it.
export const Route = createFileRoute('/workspaces/$workspaceId/analytics')({
  validateSearch: z.object({ range: zSiteAnalyticsRange.catch('30d') }),
})
