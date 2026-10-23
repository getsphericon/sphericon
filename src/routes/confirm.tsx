import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

export const Route = createFileRoute('/confirm')({
  // The GET /e/confirm/{token} endpoint redirects here (ADR 0013, GET records nothing): `token`
  // records the confirmation; an expired link arrives with `expired=1` and no token.
  validateSearch: z.object({ token: z.string().optional(), expired: z.string().optional() }),
})
