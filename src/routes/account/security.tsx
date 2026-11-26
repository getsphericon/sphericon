import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { signinErrorParam } from '../../searchSchemas.ts'

export const Route = createFileRoute('/account/security')({
  validateSearch: z.object({
    // Why linking a provider account did not succeed (set by the server's callback).
    error: signinErrorParam,
  }),
})
