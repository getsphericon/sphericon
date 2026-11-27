import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { signinErrorParam } from '../../searchSchemas.ts'

export const Route = createFileRoute('/invitations/$token')({
  validateSearch: z.object({
    // Why a provider sign-in did not accept the invitation (set by the server's callback).
    error: signinErrorParam,
  }),
})
