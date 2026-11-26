import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { tokenParam } from '../searchSchemas.ts'

export const Route = createFileRoute('/reset-password')({
  validateSearch: z.object({ token: tokenParam }),
})
