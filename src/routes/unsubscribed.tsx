import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { zSiteIntegrationChannel } from '../generated/site/zod.gen.ts'
import { httpUrlParam } from '../searchSchemas.ts'

export const Route = createFileRoute('/unsubscribed')({
  validateSearch: z.object({
    all: httpUrlParam,
    channel: zSiteIntegrationChannel.optional().catch(undefined),
  }),
})
