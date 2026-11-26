import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

export const Route = createFileRoute('/confirm')({
  // The GET /e/confirm/{token} endpoint redirects here (ADR 0013, GET records nothing) with the
  // link's token, an expired link's too; a token that is no link of ours arrives as none.
  validateSearch: z.object({ token: z.string().optional() }),
})
