import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { tokenParam } from '../searchSchemas.ts'

export const Route = createFileRoute('/verify-email')({
  validateSearch: z.object({ token: tokenParam }),
})
