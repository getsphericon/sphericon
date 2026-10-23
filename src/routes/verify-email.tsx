import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

export const Route = createFileRoute('/verify-email')({
  validateSearch: z.object({ token: z.string().catch('') }),
})
