import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { UnsubscribePage } from '../pages/unsubscribe.tsx'

export const Route = createFileRoute('/unsubscribe')({
  // The GET /e/u/{token} endpoint redirects here (RFC 8058 / ADR 0012, GET records nothing).
  // `all` is the optional "unsubscribe from everything" escalation URL.
  validateSearch: z.object({ token: z.string(), all: z.string().optional() }),
  component: UnsubscribePage,
})
