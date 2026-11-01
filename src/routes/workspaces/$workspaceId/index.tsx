import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { zSiteAnalyticsRange } from '../../../generated/site/zod.gen.ts'

// The overview: its period is the `range` search param, 30 Report days until another is picked, so
// the URL carries it.
export const Route = createFileRoute('/workspaces/$workspaceId/')({
  validateSearch: z.object({ range: zSiteAnalyticsRange.catch('30d') }),
})
