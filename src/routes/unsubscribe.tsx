import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { zSiteIntegrationChannel } from '../generated/site/zod.gen.ts'
import { httpUrlParam } from '../searchSchemas.ts'

export const Route = createFileRoute('/unsubscribe')({
  // The GET /e/u/{token} endpoint redirects here (RFC 8058 / ADR 0012, GET records nothing).
  // `all` is the optional "unsubscribe from everything" escalation URL; `channel` the Channel the
  // link came on, which the page's words follow (email when absent).
  validateSearch: z.object({
    token: z.string(),
    all: httpUrlParam,
    channel: zSiteIntegrationChannel.optional().catch(undefined),
  }),
})
